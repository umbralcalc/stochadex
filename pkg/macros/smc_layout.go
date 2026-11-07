package macros

import (
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// SMCLayout is the data an SMC inference's three partitions are wired with:
// everything about them but their iterations. Every partition keeps HistoryDepth
// steps of history; the proposal draws from ProposalSeed and the particle
// evaluation derives each particle's per-round seed from EvaluationSeed. The
// constructors here and a config's expansion both read it.
type SMCLayout struct {
	Proposal, Evaluation, Posterior InnerLayout
	HistoryDepth                    int
	ProposalSeed, EvaluationSeed    uint64
}

// NewSMCLayout is the layout of an SMC inference over the priors with this many
// particles. The evaluation row is one log-likelihood per particle, so the posterior
// reads it whole; the posterior starts from a zero mean and identity covariance.
func NewSMCLayout(
	proposalName, simName, posteriorName string,
	particles int,
	priors []inference.Prior,
	verbose bool,
	seed uint64,
) SMCLayout {
	dimension := len(priors)
	verboseFlag := 0.0
	if verbose {
		verboseFlag = 1.0
	}
	priorTypes, priorParams := inference.EncodePriors(priors)
	posteriorInit := make([]float64, inference.PosteriorStateWidth(dimension))
	for j := range dimension {
		posteriorInit[dimension+j*dimension+j] = 1.0
	}
	return SMCLayout{
		Proposal: InnerLayout{
			Params: map[string][]float64{
				"verbose":       {verboseFlag},
				"num_particles": {float64(particles)},
				"prior_types":   priorTypes,
				"prior_params":  priorParams,
			},
			ParamsAsPartitions: map[string][]string{"posterior_partition": {posteriorName}},
			InitStateValues:    make([]float64, particles*dimension),
		},
		Evaluation: InnerLayout{
			Params: map[string][]float64{"replica_params": make([]float64, particles*dimension)},
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"replica_params": {Upstream: proposalName}},
			InitStateValues: make([]float64, particles),
		},
		Posterior: InnerLayout{
			Params: map[string][]float64{
				"verbose":           {verboseFlag},
				"num_particles":     {float64(particles)},
				"num_params":        {float64(dimension)},
				"particle_loglikes": make([]float64, particles),
				"particle_params":   make([]float64, particles*dimension),
			},
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				"particle_loglikes": {Upstream: simName},
				"particle_params":   {Upstream: proposalName},
			},
			InitStateValues: posteriorInit,
		},
		HistoryDepth:   2,
		ProposalSeed:   seed,
		EvaluationSeed: seed + 100,
	}
}
