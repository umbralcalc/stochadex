package analysis

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// SMCInnerSimConfig describes the inner simulation that evaluates
// N particles through data.
type SMCInnerSimConfig struct {
	// Partitions for the inner simulation (data, model, loglike, etc.).
	// They are registered in the order given.
	Partitions []*simulator.PartitionConfig
	// Simulation config for the inner simulation.
	Simulation *simulator.SimulationConfig
	// LoglikePartitions lists, for each particle p (length N), the name
	// of the inner partition whose state[0] is the cumulative
	// log-likelihood for that particle.
	LoglikePartitions []string
	// ParamForwarding maps "innerPartitionName/paramName" to indices
	// into the N*d flat proposal state. These are forwarded from the
	// proposal partition to inner partitions via the embedded sim.
	// Partition and param names must be alphanumeric+underscore only.
	ParamForwarding map[string][]int
}

// SMCParticleModel describes a user-defined model for particle
// evaluation inside the SMC inner simulation.
type SMCParticleModel struct {
	// Build creates the inner simulation configuration for N particles
	// with nParams parameters each.
	Build func(N int, nParams int) *SMCInnerSimConfig
}

// AppliedSMCInference configures batch SMC (Sequential Monte Carlo)
// inference using iterated importance sampling.
type AppliedSMCInference struct {
	ProposalName  string
	SimName       string
	PosteriorName string
	NumParticles  int
	NumRounds     int
	Priors        []inference.Prior
	ParamNames    []string
	Model         SMCParticleModel
	Seed          uint64
	Verbose       bool
}

// NewSMCInferencePartitions creates three PartitionConfigs for SMC
// inference: a proposal partition, an embedded simulation partition,
// and a posterior partition.
func NewSMCInferencePartitions(
	applied AppliedSMCInference,
) []*simulator.PartitionConfig {
	N := applied.NumParticles
	d := len(applied.Priors)
	posteriorWidth := inference.PosteriorStateWidth(d)

	verboseFlag := 0.0
	if applied.Verbose {
		verboseFlag = 1.0
	}
	priorTypes, priorParams := inference.EncodePriors(applied.Priors)

	// Build inner simulation
	innerConfig := applied.Model.Build(N, d)

	// Use ConfigGenerator to produce inner Settings/Implementations
	generator := simulator.NewConfigGenerator()
	generator.SetSimulation(innerConfig.Simulation)
	for _, partition := range innerConfig.Partitions {
		generator.SetPartition(partition)
	}
	innerSettings, innerImpl := generator.GenerateConfigs()

	// Compute embedded state width as sum of inner partition init state widths
	embeddedWidth := 0
	for _, partition := range innerConfig.Partitions {
		embeddedWidth += len(partition.InitStateValues)
	}

	// Compute loglike extraction indices from inner partition layout.
	// The concatenated output is ordered by partition registration order.
	partitionOffsets := make(map[string]int)
	offset := 0
	for _, partition := range innerConfig.Partitions {
		partitionOffsets[partition.Name] = offset
		offset += len(partition.InitStateValues)
	}
	loglikeIndices := make([]int, N)
	for p, name := range innerConfig.LoglikePartitions {
		loglikeIndices[p] = partitionOffsets[name] // state[0] of that partition
	}

	// Convert ParamForwarding to NamedUpstreamConfig pointing at proposal
	embeddedParamsFromUpstream := make(map[string]simulator.NamedUpstreamConfig)
	for key, indices := range innerConfig.ParamForwarding {
		embeddedParamsFromUpstream[key] = simulator.NamedUpstreamConfig{
			Upstream: applied.ProposalName,
			Indices:  indices,
		}
	}

	// Init states
	proposalInit := make([]float64, N*d)
	embeddedInit := make([]float64, embeddedWidth)
	posteriorInit := make([]float64, posteriorWidth)
	for j := range d {
		posteriorInit[d+j*d+j] = 1.0 // identity covariance
	}

	partitions := make([]*simulator.PartitionConfig, 0, 3)

	// [0] Proposal
	partitions = append(partitions, &simulator.PartitionConfig{
		Name: applied.ProposalName,
		Iteration: &inference.SMCProposalIteration{
			Priors: applied.Priors,
		},
		Params: simulator.NewParams(map[string][]float64{
			"verbose":       {verboseFlag},
			"num_particles": {float64(N)},
			"prior_types":   priorTypes,
			"prior_params":  priorParams,
		}),
		ParamsAsPartitions: map[string][]string{
			"posterior_partition": {applied.PosteriorName},
		},
		InitStateValues:   proposalInit,
		StateHistoryDepth: 2,
		Seed:              applied.Seed,
	})

	// [1] Embedded simulation
	embeddedParams := simulator.NewParams(map[string][]float64{
		"init_time_value": {innerSettings.InitTimeValue},
		"burn_in_steps":   {0},
	})
	partitions = append(partitions, &simulator.PartitionConfig{
		Name: applied.SimName,
		Iteration: general.NewEmbeddedSimulationRunIteration(
			innerSettings, innerImpl,
		),
		Params:             embeddedParams,
		ParamsFromUpstream: embeddedParamsFromUpstream,
		InitStateValues:    embeddedInit,
		StateHistoryDepth:  2,
		Seed:               applied.Seed + 100,
	})

	// [2] Posterior
	partitions = append(partitions, &simulator.PartitionConfig{
		Name: applied.PosteriorName,
		Iteration: &inference.SMCPosteriorIteration{
			ParamNames: applied.ParamNames,
		},
		Params: simulator.NewParams(map[string][]float64{
			"verbose":           {verboseFlag},
			"num_particles":     {float64(N)},
			"num_params":        {float64(d)},
			"particle_loglikes": make([]float64, N),
			"particle_params":   make([]float64, N*d),
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"particle_loglikes": {
				Upstream: applied.SimName,
				Indices:  loglikeIndices,
			},
			"particle_params": {
				Upstream: applied.ProposalName,
			},
		},
		InitStateValues:   posteriorInit,
		StateHistoryDepth: 2,
		Seed:              0,
	})

	return partitions
}

// RunSMCInference builds and runs SMC inference, returning the
// posterior result from the final round.
func RunSMCInference(
	applied AppliedSMCInference,
) *inference.SMCResult {
	partitions := NewSMCInferencePartitions(applied)

	storage := NewStateTimeStorageFromPartitions(
		partitions,
		&simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: applied.NumRounds,
		},
		&simulator.ConstantTimestepFunction{Stepsize: 1.0},
		0.0,
	)

	N := applied.NumParticles
	d := len(applied.Priors)

	// Extract final round results
	proposalVals := storage.GetValues(applied.ProposalName)
	simVals := storage.GetValues(applied.SimName)
	if len(proposalVals) == 0 || len(simVals) == 0 {
		return nil
	}

	// Final round particle params
	finalProposal := proposalVals[len(proposalVals)-1]
	particleParams := make([][]float64, N)
	for p := range N {
		particleParams[p] = make([]float64, d)
		copy(particleParams[p], finalProposal[p*d:(p+1)*d])
	}

	// Final round log-likelihoods — compute loglike indices from inner sim
	innerConfig := applied.Model.Build(N, d)
	partitionOffsets := make(map[string]int)
	offset := 0
	for _, partition := range innerConfig.Partitions {
		partitionOffsets[partition.Name] = offset
		offset += len(partition.InitStateValues)
	}

	finalSim := simVals[len(simVals)-1]
	logLiks := make([]float64, N)
	for p, name := range innerConfig.LoglikePartitions {
		idx := partitionOffsets[name]
		ll := finalSim[idx]
		if math.IsNaN(ll) {
			ll = math.Inf(-1)
		}
		logLiks[p] = ll
	}

	result := inference.ComputePosterior(
		applied.ParamNames, particleParams, logLiks, nil,
	)
	result.ParticleParams = particleParams
	result.ParticleLogLik = logLiks
	return result
}
