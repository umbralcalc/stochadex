package macros

import (
	"fmt"
	"math"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// SMCInnerSimConfig describes the model ONE particle runs.
//
// The model is stated once and instantiated per particle (see
// general.ReplicatedSimulationRunIteration), rather than replicated into a
// single simulation with names templated by particle index.
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

	// One model per particle, evaluated in parallel each round. The particle
	// count is a property of this run rather than of the model's configuration.
	evaluation, err := general.NewReplicatedSimulationRunIteration(
		func() *general.ReplicatedModel {
			model := applied.Model.Build(d)
			return &general.ReplicatedModel{
				Partitions:      model.Partitions,
				Simulation:      model.Simulation,
				OutputPartition: model.LoglikePartition,
				ParamForwarding: model.ParamForwarding,
			}
		}, N, d,
	)
	if err != nil {
		panic(fmt.Sprintf("macros: SMC particle model: %v", err))
	}

	layout := NewSMCLayout(applied.ProposalName, applied.SimName, applied.PosteriorName,
		N, applied.Priors, applied.Verbose, applied.Seed)
	partitions := []*simulator.PartitionConfig{
		layout.Proposal.partition(applied.ProposalName,
			&inference.SMCProposalIteration{Priors: applied.Priors}),
		layout.Evaluation.partition(applied.SimName, evaluation),
		layout.Posterior.partition(applied.PosteriorName,
			&inference.SMCPosteriorIteration{ParamNames: applied.ParamNames}),
	}
	for _, partition := range partitions {
		partition.StateHistoryDepth = layout.HistoryDepth
	}
	partitions[0].Seed, partitions[1].Seed = layout.ProposalSeed, layout.EvaluationSeed
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
