package continuous

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat"
)

func TestCompoundPoissonProcess(t *testing.T) {
	t.Run(
		"test that the Compound Poisson process runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"compound_poisson_process_settings.yaml",
			)
			iterations := make([]simulator.Iteration, 0)
			for partitionIndex := range settings.Iterations {
				iteration := &CompoundPoissonProcessIteration{
					JumpDist: &GammaJumpDistribution{},
				}
				iteration.Configure(partitionIndex, settings)
				iterations = append(iterations, iteration)
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordinator := simulator.NewPartitionCoordinator(
				settings,
				implementations,
			)
			coordinator.Run()
		},
	)
	t.Run(
		"test that the Compound Poisson process runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"compound_poisson_process_settings.yaml",
			)
			iterations := make([]simulator.Iteration, 0)
			for range settings.Iterations {
				iteration := &CompoundPoissonProcessIteration{
					JumpDist: &GammaJumpDistribution{},
				}
				iterations = append(iterations, iteration)
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 100,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
	t.Run(
		"test that jump sizes are independent of the event draws",
		func(t *testing.T) {
			// The rate makes a jump certain, so the first step draws one event
			// uniform and one jump size. Over many seeds, the event uniform is
			// reproduced from a sampler seeded as the partition is. A gamma
			// shape below 1 starts its draw from a uniform, so were the jump
			// size drawn from a stream seeded the same way, it would grow with
			// that event uniform.
			const seeds = 4000
			uniforms := make([]float64, 0, seeds)
			jumps := make([]float64, 0, seeds)
			var alpha, beta float64
			for seed := range uint64(seeds) {
				settings := simulator.LoadSettingsFromYaml(
					"compound_poisson_process_independence_settings.yaml",
				)
				settings.Iterations[0].Seed = seed
				alpha = settings.Iterations[0].Params.GetIndex("gamma_alphas", 0)
				beta = settings.Iterations[0].Params.GetIndex("gamma_betas", 0)
				iteration := &CompoundPoissonProcessIteration{
					JumpDist: &GammaJumpDistribution{},
				}
				iteration.Configure(0, settings)
				store := simulator.NewStateTimeStorage()
				implementations := &simulator.Implementations{
					Iterations:      []simulator.Iteration{iteration},
					OutputCondition: &simulator.EveryStepOutputCondition{},
					OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
					TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
						MaxNumberOfSteps: 1,
					},
					TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
				}
				simulator.NewPartitionCoordinator(settings, implementations).Run()
				// The first stored row is the initial state of 0.
				jump := store.GetValues("always_jumping")[1][0]
				if jump <= 0 {
					t.Fatalf("seed %d: no jump at a certain event", seed)
				}
				uniforms = append(uniforms, rng.New(seed).Float64())
				jumps = append(jumps, jump)
			}
			// Four standard errors either side of the true value.
			if mean := stat.Mean(jumps, nil); math.Abs(mean-alpha/beta) >
				4*math.Sqrt(alpha/(beta*beta)/seeds) {
				t.Errorf("jump mean %v, want %v", mean, alpha/beta)
			}
			if corr := stat.Correlation(uniforms, jumps, nil); math.Abs(corr) > 4/math.Sqrt(seeds) {
				t.Errorf("correlation of jump size with event draw %v, want |corr| <= %v",
					corr, 4/math.Sqrt(seeds))
			}
		},
	)
}
