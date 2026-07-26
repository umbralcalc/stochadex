package macros

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/inference"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestRunSMCInference(t *testing.T) {
	t.Run(
		"test SMC parameter recovery on synthetic normal data",
		func(t *testing.T) {
			// True parameters: mean=2.0, variance=0.5
			trueMean := 2.0
			trueVar := 0.5
			T := 20
			rng := rand.New(rand.NewPCG(123, 124))

			// Generate synthetic data
			data := make([][]float64, T)
			for i := range T {
				data[i] = []float64{trueMean + rng.NormFloat64()*math.Sqrt(trueVar)}
			}
			times := make([]float64, T)
			for i := range T {
				times[i] = float64(i)
			}

			// Build SMC particle model: each particle proposes a mean value,
			// and we compare against data using a normal likelihood with
			// known variance.
			// One particle's model, instantiated once per particle by the macro.
			model := SMCParticleModel{
				Build: func(nParams int) *SMCInnerSimConfig {
					return &SMCInnerSimConfig{
						Partitions: []*simulator.PartitionConfig{
							{
								Name:              "observed_data",
								Iteration:         &general.FromStorageIteration{Data: data},
								Params:            simulator.NewParams(make(map[string][]float64)),
								InitStateValues:   data[0],
								StateHistoryDepth: 2,
								Seed:              0,
							},
							{
								// Outputs this particle's proposed mean.
								Name:      "pred",
								Iteration: &general.ParamValuesIteration{},
								Params: simulator.NewParams(map[string][]float64{
									"param_values": {0.0},
								}),
								InitStateValues:   []float64{0.0},
								StateHistoryDepth: 2,
								Seed:              0,
							},
							{
								Name: "loglike",
								Iteration: &inference.DataComparisonIteration{
									Likelihood: &inference.NormalLikelihoodDistribution{},
								},
								Params: simulator.NewParams(map[string][]float64{
									"mean":               {0.0},
									"variance":           {trueVar},
									"latest_data_values": data[0],
									"cumulative":         {1},
									"burn_in_steps":      {0},
								}),
								ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
									"mean":               {Upstream: "pred"},
									"latest_data_values": {Upstream: "observed_data"},
								},
								InitStateValues:   []float64{0.0},
								StateHistoryDepth: 2,
								Seed:              0,
							},
						},
						Simulation: &simulator.SimulationConfig{
							OutputCondition: &simulator.NilOutputCondition{},
							OutputFunction:  &simulator.NilOutputFunction{},
							TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
								MaxNumberOfSteps: T - 1,
							},
							TimestepFunction: &general.FromStorageTimestepFunction{
								Data: times,
							},
							InitTimeValue: times[0],
						},
						LoglikePartition: "loglike",
						// Index into this particle's own parameter vector.
						ParamForwarding: map[string][]int{"pred/param_values": {0}},
					}
				},
			}

			result := RunSMCInference(AppliedSMCInference{
				ProposalName:  "smc_proposals",
				SimName:       "smc_sim",
				PosteriorName: "smc_posterior",
				NumParticles:  100,
				NumRounds:     3,
				Priors: []inference.Prior{
					&inference.UniformPrior{Lo: -5.0, Hi: 10.0},
				},
				ParamNames: []string{"mean"},
				Model:      model,
				Seed:       42,
				Verbose:    false,
			})

			if result == nil {
				t.Fatal("RunSMCInference returned nil")
			}
			// Posterior mean should be close to true mean
			if math.Abs(result.PosteriorMean[0]-trueMean) > 0.5 {
				t.Errorf("posterior mean=%.4f, expected ~%.1f",
					result.PosteriorMean[0], trueMean)
			}
			// Posterior std should be positive and reasonable
			if result.PosteriorStd[0] <= 0 || result.PosteriorStd[0] > 2.0 {
				t.Errorf("posterior std=%.4f, expected positive and <2",
					result.PosteriorStd[0])
			}
			// Log marginal likelihood should be finite
			if math.IsNaN(result.LogMarginalLik) || math.IsInf(result.LogMarginalLik, 0) {
				t.Errorf("log marginal likelihood=%f, expected finite",
					result.LogMarginalLik)
			}
			// Weights should sum to 1
			wSum := 0.0
			for _, w := range result.Weights {
				wSum += w
			}
			if math.Abs(wSum-1.0) > 1e-6 {
				t.Errorf("weights sum=%.6f, expected 1.0", wSum)
			}
		},
	)
}
