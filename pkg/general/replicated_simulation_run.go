package general

import (
	"fmt"
	"sort"
	"sync"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// ReplicatedModel describes the model ONE replica runs. It is stated once and
// instantiated per replica by NewReplicatedSimulationRunIteration.
type ReplicatedModel struct {
	// Partitions are one replica's model, registered in the order given.
	Partitions []*simulator.PartitionConfig
	// Simulation is one replica's run: its clock and termination.
	Simulation *simulator.SimulationConfig
	// OutputPartition names the partition whose final row is the replica's
	// output.
	OutputPartition string
	// ParamForwarding maps "partition_name/param_name" to indices into the
	// replica's own slice of replica_params, in order.
	ParamForwarding map[string][]int
}

// ReplicaParamForwarding routes part of a replica's parameter slice into one
// of its model's params.
type ReplicaParamForwarding struct {
	// Partition and Param name the destination.
	Partition int
	Param     string
	// Indices selects which of the replica's parameters to send, in order.
	Indices []int
}

// ReplicatedSimulationRunIteration states a model once, runs it once per
// replica at every step, and outputs each replica's final row of one partition.
//
// A slice of a flat parameter vector is forwarded into each replica before it
// runs, so replicas differ by their parameters (an SMC particle's proposal, an
// evolution-strategy candidate, a sensitivity-sweep point) and by their seeds.
// Seeds are derived here, per step and per replica, so no config can leave the
// replicas of a step sharing a random stream, or one replica repeating its
// stream every step.
// Replicas run concurrently and synchronise once per step.
//
// # Row layout
//
//	row[r*w : (r+1)*w] = replica r's final row of the output partition,
//
// where w is the output partition's state width.
//
// Params:
//   - replica_params: the flat replicas*ParamsPerReplica vector, replica r's
//     parameters at [r*ParamsPerReplica, (r+1)*ParamsPerReplica). Normally wired
//     from an upstream partition via params_from_upstream. Not read when
//     ParamsPerReplica is 0.
type ReplicatedSimulationRunIteration struct {
	// Simulations holds one independent model per replica. They are separate
	// values because they run concurrently and a ReentrantSimulation is not
	// safe to share.
	Simulations []*simulator.ReentrantSimulation
	// Forwarding says how a replica's parameters reach its model.
	Forwarding []ReplicaParamForwarding
	// OutputPartition is the index of the partition whose final row each
	// replica outputs.
	OutputPartition int
	// ParamsPerReplica is each replica's parameter count.
	ParamsPerReplica int

	seed  uint64
	width int
	out   []float64
}

// Configure implements simulator.Iteration.
func (r *ReplicatedSimulationRunIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	if len(r.Simulations) == 0 {
		panic("general.ReplicatedSimulationRunIteration: no replica simulations")
	}
	if r.ParamsPerReplica < 0 {
		panic("general.ReplicatedSimulationRunIteration: ParamsPerReplica must be >= 0")
	}
	r.seed = settings.Iterations[partitionIndex].Seed
	r.width = r.Simulations[0].StateWidths()[r.OutputPartition]
	r.out = make([]float64, len(r.Simulations)*r.width)
}

// Iterate implements simulator.Iteration: one run of every replica.
func (r *ReplicatedSimulationRunIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	var replicaParams []float64
	if r.ParamsPerReplica > 0 {
		replicaParams = params.Get("replica_params")
	}
	step := timestepsHistory.CurrentStepNumber

	var group sync.WaitGroup
	for replica := range r.Simulations {
		group.Add(1)
		go func(replica int) {
			defer group.Done()
			simulation := r.Simulations[replica]
			offset := replica * r.ParamsPerReplica
			for _, forward := range r.Forwarding {
				values := make([]float64, len(forward.Indices))
				for i, index := range forward.Indices {
					if slot := offset + index; slot < len(replicaParams) {
						values[i] = replicaParams[slot]
					}
				}
				simulation.SetParam(forward.Partition, forward.Param, values)
			}
			// Vary the seed by both step and replica: sharing either would put
			// replicas on one noise realisation, or repeat the same one every
			// step.
			seed := simulator.DeriveSeed(simulator.DeriveSeed(r.seed, step), replica)
			rows := simulation.Run(simulator.ReentrantRun{Seed: &seed})
			copy(r.out[replica*r.width:(replica+1)*r.width], rows[r.OutputPartition])
		}(replica)
	}
	group.Wait()
	return r.out
}

// NewReplicatedSimulationRunIteration builds the replicated run from a
// one-replica model, calling build once per replica. build must return a fresh
// model each time: replicas that shared an iteration would be coupled.
func NewReplicatedSimulationRunIteration(
	build func() *ReplicatedModel,
	replicas int,
	paramsPerReplica int,
) (*ReplicatedSimulationRunIteration, error) {
	if replicas <= 0 {
		return nil, fmt.Errorf("replicas must be > 0, got %d", replicas)
	}
	if paramsPerReplica < 0 {
		return nil, fmt.Errorf("params_per_replica must be >= 0, got %d", paramsPerReplica)
	}
	iteration := &ReplicatedSimulationRunIteration{
		Simulations:      make([]*simulator.ReentrantSimulation, replicas),
		ParamsPerReplica: paramsPerReplica,
	}
	for replica := range iteration.Simulations {
		model := build()
		generator := simulator.NewConfigGenerator()
		generator.SetSimulation(model.Simulation)
		for _, partition := range model.Partitions {
			generator.SetPartition(partition)
		}
		settings, implementations := generator.GenerateConfigs()
		iteration.Simulations[replica] = simulator.NewReentrantSimulation(settings, implementations)
		if replica > 0 {
			continue
		}
		// Every replica has the same shape, so resolve names to indices once,
		// against the first.
		if err := iteration.resolveNames(model); err != nil {
			return nil, err
		}
	}
	return iteration, nil
}

// resolveNames sets the output partition's index and the forwarding from the
// first replica's model.
func (r *ReplicatedSimulationRunIteration) resolveNames(model *ReplicatedModel) error {
	first := r.Simulations[0]
	index, ok := first.PartitionIndex(model.OutputPartition)
	if !ok {
		return fmt.Errorf("output partition %q is not in the replicated model",
			model.OutputPartition)
	}
	r.OutputPartition = index
	targets := make([]string, 0, len(model.ParamForwarding))
	for target := range model.ParamForwarding {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		partitionName, paramName, ok := splitForwardingTarget(target)
		if !ok {
			return fmt.Errorf("param forwarding key %q must be \"partition_name/param_name\"", target)
		}
		partition, ok := first.PartitionIndex(partitionName)
		if !ok {
			return fmt.Errorf("param forwarding names partition %q, which is not in "+
				"the replicated model", partitionName)
		}
		indices := model.ParamForwarding[target]
		for _, index := range indices {
			if index < 0 || index >= r.ParamsPerReplica {
				return fmt.Errorf("param forwarding %q sends parameter %d, but each "+
					"replica has %d (params_per_replica)", target, index, r.ParamsPerReplica)
			}
		}
		r.Forwarding = append(r.Forwarding, ReplicaParamForwarding{
			Partition: partition, Param: paramName, Indices: indices})
	}
	return nil
}

// splitForwardingTarget splits "partition_name/param_name" at the last slash.
func splitForwardingTarget(target string) (partition, param string, ok bool) {
	for i := len(target) - 1; i >= 0; i-- {
		if target[i] == '/' {
			return target[:i], target[i+1:], i > 0 && i < len(target)-1
		}
	}
	return "", "", false
}
