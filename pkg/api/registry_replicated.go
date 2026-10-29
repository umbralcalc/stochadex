package api

import (
	"fmt"
	"sort"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gopkg.in/yaml.v2"
)

// replicated_simulation_run states a model once and runs it once per replica
// at every step (general.ReplicatedSimulationRunIteration):
//
//	iteration:
//	  type: replicated_simulation_run
//	  replicas: 100
//	  params_per_replica: 1
//	  run: {partitions: [...], simulation: {...}}
//	  output_partition: loglike
//	  param_forwarding: {"pred/param_values": [0]}
//
// The partition's replica_params param carries every replica's parameters, and
// its row is each replica's final output_partition row, concatenated.

// replicatedRunSpec is the data form of a replicated_simulation_run.
type replicatedRunSpec struct {
	Replicas         int              `yaml:"replicas"`
	ParamsPerReplica int              `yaml:"params_per_replica,omitempty"`
	Run              RunConfig        `yaml:"run"`
	OutputPartition  string           `yaml:"output_partition"`
	ParamForwarding  map[string][]int `yaml:"param_forwarding,omitempty"`
}

func init() {
	scopedIterationBuilders["replicated_simulation_run"] = buildReplicatedSimulationRun
}

func buildReplicatedSimulationRun(
	fields map[string]interface{},
	scope *runScope,
) (simulator.Iteration, error) {
	// Fields decoded from YAML always marshal back (yaml.v2 panics, rather
	// than erring, on a value that cannot).
	raw, _ := yaml.Marshal(fields)
	// Each replica decodes the run afresh, so no two share an iteration.
	decode := func() (*replicatedRunSpec, error) {
		var spec replicatedRunSpec
		if err := yaml.UnmarshalStrict(raw, &spec); err != nil {
			return nil, err
		}
		// A replica runs concurrently with the others, so it shares no
		// environment with them: each resolves in a scope of its own, whose
		// input readers are bound with the run's.
		replicaScope := scope.forReplica()
		if err := spec.Run.resolveIn(true, replicaScope); err != nil {
			return nil, err
		}
		spec.Run.recordInputReaders(replicaScope)
		return &spec, nil
	}
	spec, err := decode()
	if err == nil {
		err = spec.validate()
	}
	if err == nil {
		err = checkRunWiring(spec.Run.GetConfigGenerator())
	}
	if err != nil {
		return nil, fmt.Errorf("replicated_simulation_run: %w", err)
	}
	// The first replica takes the run already decoded and checked; the rest
	// decode their own from the same bytes, which cannot fail now.
	first := spec
	iteration, err := general.NewReplicatedSimulationRunIteration(func() *general.ReplicatedModel {
		replica := first
		if replica == nil {
			replica, _ = decode()
		}
		first = nil
		generator := replica.Run.GetConfigGenerator()
		partitions := make([]*simulator.PartitionConfig, 0, len(replica.Run.Partitions))
		for _, name := range generator.PartitionNames() {
			partitions = append(partitions, generator.GetPartition(name))
		}
		return &general.ReplicatedModel{
			Partitions:      partitions,
			Simulation:      generator.GetSimulation(),
			OutputPartition: replica.OutputPartition,
			ParamForwarding: replica.ParamForwarding,
		}
	}, spec.Replicas, spec.ParamsPerReplica)
	if err != nil {
		return nil, fmt.Errorf("replicated_simulation_run: %w", err)
	}
	return iteration, nil
}

// validate checks what a replica's run may not do: write its own output,
// which every replica would write at once, or read a history its host feeds,
// which no replica has; and that each forwarded param is one its partition
// declares.
func (s *replicatedRunSpec) validate() error {
	if s.OutputPartition == "" {
		return fmt.Errorf("needs output_partition: the partition whose final row each replica outputs")
	}
	if function := s.Run.SimulationStrings.OutputFunction.Type; function != "" && function != "nil" {
		return fmt.Errorf("a replica's run writes no output of its own (every replica would "+
			"write it at once), so remove its output_function %q; the partition's row "+
			"is each replica's final %s row", function, s.OutputPartition)
	}
	declared := map[string]map[string][]float64{}
	for _, partition := range s.Run.Partitions {
		if _, ok := partition.Iteration.(general.StateMemoryIteration); ok {
			return fmt.Errorf("partition %q reads a history an embedded run's host feeds "+
				"it, which no replica has", partition.Name)
		}
		if len(partition.ParamsFromInput) > 0 {
			return fmt.Errorf("params_from_input is not yet supported inside a replicated "+
				"run (partition %q)", partition.Name)
		}
		declared[partition.Name] = partition.Params.Map
	}
	targets := make([]string, 0, len(s.ParamForwarding))
	for target := range s.ParamForwarding {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		partition, param, ok := general.ForwardedParamTarget(target)
		if !ok {
			continue // the constructor names the malformed key
		}
		if params, exists := declared[partition]; exists {
			if _, ok := params[param]; !ok {
				return fmt.Errorf("param_forwarding %q sets %q, which partition %q does not "+
					"declare in its params", target, param, partition)
			}
		}
	}
	return nil
}

// checkRunWiring builds a run held inside a partition (one replica's, or a
// nested run's), classifying the wiring panics GenerateConfigs raises, and a
// within-step cycle that would deadlock it at every outer step, as errors.
func checkRunWiring(generator *simulator.ConfigGenerator) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("invalid run wiring: %v", recovered)
		}
	}()
	if err := CheckForDeadlock(generator); err != nil {
		return err
	}
	generator.GenerateConfigs()
	return nil
}
