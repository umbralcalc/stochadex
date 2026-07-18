package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// smc_inference is a live macro whose inner model is a per-particle template: the
// same partition set is instantiated once per particle, with the "{particle}"
// placeholder in partition names and upstream references replaced by the particle
// index. This is how a config expresses the N-way particle structure of an SMC
// run (analysis.SMCParticleModel.Build) without a general loop construct — the
// loop lives here, in the macro's Go.

type smcInferenceSpec struct {
	macroTypeField `yaml:",inline"`
	ProposalName   string                    `yaml:"proposal_name"`
	SimName        string                    `yaml:"sim_name"`
	PosteriorName  string                    `yaml:"posterior_name"`
	NumParticles   int                       `yaml:"num_particles"`
	NumRounds      int                       `yaml:"num_rounds"`
	Seed           uint64                    `yaml:"seed"`
	Verbose        bool                      `yaml:"verbose,omitempty"`
	Priors         []simulator.ComponentSpec `yaml:"priors"`
	ParamNames     []string                  `yaml:"param_names"`
	Model          smcModelSpec              `yaml:"model"`
	Timestep       float64                   `yaml:"timestep,omitempty"`
}

// smcModelSpec is the data form of the per-particle inner model. When ObservedData
// is set (the standard SMC-over-recorded-data pattern), it becomes a shared
// from_storage partition seeded from the data: storage, and the inner simulation
// is built automatically to replay that data's timeline (a nil-output run whose
// timestep function walks the storage times). Otherwise a Simulation spec is
// required.
type smcModelSpec struct {
	Simulation            simulator.SimulationConfigStrings `yaml:"simulation,omitempty"`
	ObservedData          *smcObservedData                  `yaml:"observed_data,omitempty"`
	SharedPartitions      []simulator.PartitionConfig       `yaml:"shared_partitions,omitempty"`
	PerParticlePartitions []simulator.PartitionConfig       `yaml:"per_particle_partitions"`
	LoglikePartition      string                            `yaml:"loglike_partition"`
	ParamForwarding       map[string][]int                  `yaml:"param_forwarding,omitempty"`
}

type smcObservedData struct {
	Name string      `yaml:"name"`
	Ref  dataRefSpec `yaml:"ref"`
}

// resolve reports that smc_inference is a live macro.
func (s *smcInferenceSpec) resolve(
	*simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, map[string]int, error) {
	return nil, nil, fmt.Errorf("smc_inference is a live macro (it runs its own rounds)")
}

func (s *smcInferenceSpec) resolveLive(
	storage *simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, int, float64, error) {
	priors := make([]inference.Prior, len(s.Priors))
	for i, spec := range s.Priors {
		prior, err := resolvePrior(spec)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("smc_inference prior %d: %w", i, err)
		}
		priors[i] = prior
	}
	if s.Model.ObservedData != nil && storage == nil {
		return nil, 0, 0, fmt.Errorf(
			"smc_inference model.observed_data needs a data: block to read %q from",
			s.Model.ObservedData.Ref.PartitionName)
	}
	build, err := s.Model.builder(storage)
	if err != nil {
		return nil, 0, 0, err
	}
	partitions := analysis.NewSMCInferencePartitions(analysis.AppliedSMCInference{
		ProposalName:  s.ProposalName,
		SimName:       s.SimName,
		PosteriorName: s.PosteriorName,
		NumParticles:  s.NumParticles,
		NumRounds:     s.NumRounds,
		Priors:        priors,
		ParamNames:    s.ParamNames,
		Model:         analysis.SMCParticleModel{Build: build},
		Seed:          s.Seed,
		Verbose:       s.Verbose,
	})
	timestep := s.Timestep
	if timestep == 0 {
		timestep = 1.0
	}
	// SMC's own inner run length is NumRounds; the outer run steps once per round.
	return partitions, s.NumRounds, timestep, nil
}

// builder returns the SMCParticleModel.Build closure that instantiates the inner
// model for N particles with nParams parameters each.
func (m *smcModelSpec) builder(
	storage *simulator.StateTimeStorage,
) (func(N, nParams int) *analysis.SMCInnerSimConfig, error) {
	// The inner simulation is either auto-built from the observed data's timeline,
	// or given explicitly as data specs.
	var explicitSim *simulator.SimulationConfig
	if m.ObservedData == nil {
		resolved, err := m.Simulation.ResolveDataComponents()
		if err != nil {
			return nil, err
		}
		if !m.Simulation.FullyData() {
			return nil, fmt.Errorf(
				"smc_inference model.simulation must be fully data specs (or set observed_data)")
		}
		explicitSim = resolved
	}

	return func(N, nParams int) *analysis.SMCInnerSimConfig {
		partitions := make([]*simulator.PartitionConfig, 0)

		simulation := explicitSim
		if m.ObservedData != nil {
			data := storage.GetValues(m.ObservedData.Ref.PartitionName)
			partitions = append(partitions, &simulator.PartitionConfig{
				Name:              m.ObservedData.Name,
				Iteration:         &general.FromStorageIteration{Data: data},
				Params:            simulator.NewParams(map[string][]float64{}),
				InitStateValues:   data[0],
				StateHistoryDepth: 2,
				Seed:              0,
			})
			times := storage.GetTimes()
			simulation = &simulator.SimulationConfig{
				OutputCondition:      &simulator.NilOutputCondition{},
				OutputFunction:       &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: len(times) - 1},
				TimestepFunction:     &general.FromStorageTimestepFunction{Data: times},
				InitTimeValue:        times[0],
			}
		}
		for i := range m.SharedPartitions {
			shared, err := instantiateParticle(m.SharedPartitions[i], -1)
			if err != nil {
				panic(fmt.Sprintf("smc_inference shared partition: %v", err))
			}
			partitions = append(partitions, shared)
		}

		loglikePartitions := make([]string, N)
		paramForwarding := make(map[string][]int)
		for p := 0; p < N; p++ {
			for i := range m.PerParticlePartitions {
				partition, err := instantiateParticle(m.PerParticlePartitions[i], p)
				if err != nil {
					panic(fmt.Sprintf("smc_inference per-particle partition: %v", err))
				}
				partitions = append(partitions, partition)
			}
			loglikePartitions[p] = substituteParticle(m.LoglikePartition, p)
			for key, offsets := range m.ParamForwarding {
				indices := make([]int, len(offsets))
				for j, offset := range offsets {
					indices[j] = p*nParams + offset
				}
				paramForwarding[substituteParticle(key, p)] = indices
			}
		}

		simCopy := *simulation
		return &analysis.SMCInnerSimConfig{
			Partitions:        partitions,
			Simulation:        &simCopy,
			LoglikePartitions: loglikePartitions,
			ParamForwarding:   paramForwarding,
		}
	}, nil
}

// substituteParticle replaces the {particle} placeholder with the particle index
// (a bare placeholder, for the shared pass with index -1, is left as-is).
func substituteParticle(s string, particle int) string {
	if particle < 0 {
		return s
	}
	return strings.ReplaceAll(s, "{particle}", strconv.Itoa(particle))
}

// instantiateParticle builds one partition from a template for the given particle,
// with a fresh iteration instance, a deep-copied params map (so per-particle
// upstream injection cannot cross-contaminate), and {particle} substituted through
// the name and every upstream / partition reference.
func instantiateParticle(
	template simulator.PartitionConfig,
	particle int,
) (*simulator.PartitionConfig, error) {
	partition := template
	partition.Init()
	if partition.IterationSpec.IsData() {
		iteration, err := ResolveIteration(partition.IterationSpec)
		if err != nil {
			return nil, err
		}
		partition.Iteration = iteration
	}
	partition.Name = substituteParticle(partition.Name, particle)

	params := make(map[string][]float64, len(partition.Params.Map))
	for key, values := range partition.Params.Map {
		copied := make([]float64, len(values))
		copy(copied, values)
		params[key] = copied
	}
	partition.Params = simulator.NewParams(params)

	fromUpstream := make(map[string]simulator.NamedUpstreamConfig, len(partition.ParamsFromUpstream))
	for key, upstream := range partition.ParamsFromUpstream {
		upstream.Upstream = substituteParticle(upstream.Upstream, particle)
		fromUpstream[substituteParticle(key, particle)] = upstream
	}
	partition.ParamsFromUpstream = fromUpstream

	asPartitions := make(map[string][]string, len(partition.ParamsAsPartitions))
	for key, refs := range partition.ParamsAsPartitions {
		substituted := make([]string, len(refs))
		for i, ref := range refs {
			substituted[i] = substituteParticle(ref, particle)
		}
		asPartitions[substituteParticle(key, particle)] = substituted
	}
	partition.ParamsAsPartitions = asPartitions

	return &partition, nil
}
