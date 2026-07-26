package api

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/macros"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// smc_inference is a live macro. Its model is stated ONCE, as an ordinary set of
// partitions, and instantiated per particle by macros.SMCParticleEvaluationIteration.
//
// It used to be a template instead: the same partition set written with a
// "{particle}" placeholder, expanded here into N copies inside one simulation.
// That made the particle count a property of the configuration, and left
// per-particle random seeding to whoever wrote the template — which is exactly
// what silently was not happening, putting every particle on one noise
// realisation. Seeds are now derived per particle by the evaluation iteration,
// so a config cannot get them wrong.

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
	Simulation   simulator.SimulationConfigStrings `yaml:"simulation,omitempty"`
	ObservedData *smcObservedData                  `yaml:"observed_data,omitempty"`
	// Partitions is one particle's model, written once with no templating. Every
	// particle gets its own instance.
	Partitions []simulator.PartitionConfig `yaml:"partitions"`
	// LoglikePartition names the partition whose state[0] is the particle's
	// cumulative log-likelihood.
	LoglikePartition string `yaml:"loglike_partition"`
	// ParamForwarding maps "partition_name/param_name" to indices into the
	// particle's own parameter vector (length = len(priors)).
	ParamForwarding map[string][]int `yaml:"param_forwarding,omitempty"`
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
	partitions := macros.NewSMCInferencePartitions(macros.AppliedSMCInference{
		ProposalName:  s.ProposalName,
		SimName:       s.SimName,
		PosteriorName: s.PosteriorName,
		NumParticles:  s.NumParticles,
		NumRounds:     s.NumRounds,
		Priors:        priors,
		ParamNames:    s.ParamNames,
		Model:         macros.SMCParticleModel{Build: build},
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

// builder returns the SMCParticleModel.Build closure that instantiates one
// particle's model, with nParams parameters.
func (m *smcModelSpec) builder(
	storage *simulator.StateTimeStorage,
) (func(nParams int) *macros.SMCInnerSimConfig, error) {
	// The inner simulation is either auto-built from the observed data's timeline,
	// or given explicitly as data specs.
	var explicitSim *simulator.SimulationConfig
	if m.ObservedData == nil {
		resolved, err := m.Simulation.ResolveDataComponents()
		if err != nil {
			return nil, err
		}
		if !m.Simulation.OutputCondition.IsData() ||
			!m.Simulation.OutputFunction.IsData() ||
			!m.Simulation.TerminationCondition.IsData() ||
			!m.Simulation.TimestepFunction.IsData() {
			return nil, fmt.Errorf(
				"smc_inference model.simulation must be fully data specs (or set observed_data)")
		}
		explicitSim = resolved
	}

	return func(nParams int) *macros.SMCInnerSimConfig {
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
		for i := range m.Partitions {
			partition, err := instantiateModelPartition(m.Partitions[i])
			if err != nil {
				panic(fmt.Sprintf("smc_inference model partition: %v", err))
			}
			partitions = append(partitions, partition)
		}

		simCopy := *simulation
		return &macros.SMCInnerSimConfig{
			Partitions:       partitions,
			Simulation:       &simCopy,
			LoglikePartition: m.LoglikePartition,
			ParamForwarding:  m.ParamForwarding,
		}
	}, nil
}

// instantiateModelPartition builds one particle's copy of a model partition, with
// a fresh iteration instance and deep-copied params and wiring maps — so that
// nothing is shared between particles, which would couple their evaluations.
//
// Seeds are deliberately not touched here: the evaluation iteration reseeds every
// particle's whole model per round (see simulator.ReentrantSimulation), so a
// config cannot leave two particles sharing a random stream.
func instantiateModelPartition(
	template simulator.PartitionConfig,
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

	params := make(map[string][]float64, len(partition.Params.Map))
	for key, values := range partition.Params.Map {
		copied := make([]float64, len(values))
		copy(copied, values)
		params[key] = copied
	}
	partition.Params = simulator.NewParams(params)

	fromUpstream := make(map[string]simulator.NamedUpstreamConfig, len(partition.ParamsFromUpstream))
	for key, upstream := range partition.ParamsFromUpstream {
		fromUpstream[key] = upstream
	}
	partition.ParamsFromUpstream = fromUpstream

	asPartitions := make(map[string][]string, len(partition.ParamsAsPartitions))
	for key, refs := range partition.ParamsAsPartitions {
		copied := make([]string, len(refs))
		copy(copied, refs)
		asPartitions[key] = copied
	}
	partition.ParamsAsPartitions = asPartitions

	return &partition, nil
}
