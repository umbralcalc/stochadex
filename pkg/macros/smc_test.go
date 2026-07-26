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

// TestSMCParticleEvaluationRejectsBadModel covers the two ways a particle model
// can name something that is not there. Both would otherwise surface as a
// silently unscored or unparameterised particle, which looks like a converging
// run producing meaningless numbers.
func TestSMCParticleEvaluationRejectsBadModel(t *testing.T) {
	build := func(loglike string, forwarding map[string][]int) func() *SMCInnerSimConfig {
		return func() *SMCInnerSimConfig {
			return &SMCInnerSimConfig{
				Partitions: []*simulator.PartitionConfig{{
					Name:      "pred",
					Iteration: &general.ParamValuesIteration{},
					Params: simulator.NewParams(map[string][]float64{
						"param_values": {0.0},
					}),
					InitStateValues:   []float64{0.0},
					StateHistoryDepth: 1,
					Seed:              0,
				}},
				Simulation: &simulator.SimulationConfig{
					OutputCondition:      &simulator.NilOutputCondition{},
					OutputFunction:       &simulator.NilOutputFunction{},
					TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
					TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
					InitTimeValue:        0,
				},
				LoglikePartition: loglike,
				ParamForwarding:  forwarding,
			}
		}
	}

	for _, testCase := range []struct {
		name       string
		loglike    string
		forwarding map[string][]int
	}{
		{
			name:       "unknown loglike partition",
			loglike:    "absent",
			forwarding: map[string][]int{"pred/param_values": {0}},
		},
		{
			name:       "forwarding names an unknown partition",
			loglike:    "pred",
			forwarding: map[string][]int{"absent/param_values": {0}},
		},
		{
			name:       "forwarding key is not partition/param",
			loglike:    "pred",
			forwarding: map[string][]int{"param_values": {0}},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("expected a panic for %s", testCase.name)
				}
			}()
			NewSMCParticleEvaluationIteration(
				build(testCase.loglike, testCase.forwarding), 2, 1)
		})
	}
}

// TestSMCParticleEvaluationIndependence checks each particle gets its own model
// instance. Sharing one would couple the particles, since parameter forwarding
// writes into params at evaluation time.
func TestSMCParticleEvaluationIndependence(t *testing.T) {
	build := func() *SMCInnerSimConfig {
		return &SMCInnerSimConfig{
			Partitions: []*simulator.PartitionConfig{{
				Name:      "pred",
				Iteration: &general.ParamValuesIteration{},
				Params: simulator.NewParams(map[string][]float64{
					"param_values": {0.0},
				}),
				InitStateValues:   []float64{0.0},
				StateHistoryDepth: 1,
				Seed:              0,
			}},
			Simulation: &simulator.SimulationConfig{
				OutputCondition:      &simulator.NilOutputCondition{},
				OutputFunction:       &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
				TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
				InitTimeValue:        0,
			},
			LoglikePartition: "pred",
			ParamForwarding:  map[string][]int{"pred/param_values": {0}},
		}
	}
	const particles = 3
	evaluation := NewSMCParticleEvaluationIteration(build, particles, 1)
	if len(evaluation.Simulations) != particles {
		t.Fatalf("expected one simulation per particle, got %d", len(evaluation.Simulations))
	}
	for i := 0; i < particles; i++ {
		for j := i + 1; j < particles; j++ {
			if evaluation.Simulations[i] == evaluation.Simulations[j] {
				t.Fatalf("particles %d and %d share a model instance", i, j)
			}
		}
	}

	// Each particle's own parameter reaches its own model: the partition echoes
	// param_values, so the row it reports is the parameter it was given.
	evaluation.Configure(0, &simulator.Settings{
		Iterations: []simulator.IterationSettings{{Name: "eval", Seed: 3}},
	})
	params := simulator.NewParams(map[string][]float64{
		"particle_params": {10, 20, 30},
	})
	out := evaluation.Iterate(&params, 0, nil,
		&simulator.CumulativeTimestepsHistory{CurrentStepNumber: 1})
	for particle, want := range []float64{10, 20, 30} {
		if out[particle] != want {
			t.Errorf("particle %d scored %v, want its own parameter %v",
				particle, out[particle], want)
		}
	}
}
