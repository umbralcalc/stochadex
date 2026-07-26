package macros

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// SMCInnerSimConfig describes the model ONE particle runs.
//
// The model is stated once and instantiated per particle (see
// SMCParticleEvaluationIteration), rather than replicated into a single
// simulation with names templated by particle index.
type SMCInnerSimConfig struct {
	// Partitions for one particle's model (data, model, loglike, etc.).
	// They are registered in the order given.
	Partitions []*simulator.PartitionConfig
	// Simulation config for one particle's run.
	Simulation *simulator.SimulationConfig
	// LoglikePartition names the partition whose state[0] is that particle's
	// cumulative log-likelihood.
	LoglikePartition string
	// ParamForwarding maps "partitionName/paramName" to indices into the
	// particle's own d-length parameter vector.
	ParamForwarding map[string][]int
}

// SMCParticleModel describes a user-defined model for particle evaluation.
type SMCParticleModel struct {
	// Build creates one particle's model, with nParams parameters. It is called
	// once per particle, so it must return a fresh, independent configuration
	// each time — sharing iteration objects between particles would couple them.
	Build func(nParams int) *SMCInnerSimConfig
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

	// One model per particle, evaluated in parallel each round. The particle
	// count is a property of this run rather than of the model's configuration.
	evaluation := NewSMCParticleEvaluationIteration(
		func() *SMCInnerSimConfig { return applied.Model.Build(d) }, N, d,
	)

	// Init states. The evaluation partition's row is one log-likelihood per
	// particle, so the posterior reads it whole.
	proposalInit := make([]float64, N*d)
	evaluationInit := make([]float64, N)
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

	// [1] Particle evaluation
	partitions = append(partitions, &simulator.PartitionConfig{
		Name:      applied.SimName,
		Iteration: evaluation,
		Params: simulator.NewParams(map[string][]float64{
			"particle_params": make([]float64, N*d),
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"particle_params": {Upstream: applied.ProposalName},
		},
		InitStateValues:   evaluationInit,
		StateHistoryDepth: 2,
		// The base every particle's per-round seed is derived from.
		Seed: applied.Seed + 100,
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
			// The evaluation partition's row is exactly one log-likelihood per
			// particle, so it is read whole.
			"particle_loglikes": {Upstream: applied.SimName},
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

	storage := analysis.NewStateTimeStorageFromPartitions(
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

	// Final round log-likelihoods. The evaluation partition's row is one per
	// particle, so no layout arithmetic over the inner model is needed.
	finalSim := simVals[len(simVals)-1]
	logLiks := make([]float64, N)
	for p := range N {
		ll := finalSim[p]
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
