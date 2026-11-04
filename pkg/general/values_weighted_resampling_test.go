package general

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

func TestValuesWeightedResamplingIteration(t *testing.T) {
	t.Run(
		"test that the values weighted resampling iteration runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"values_weighted_resampling_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				&ValuesWeightedResamplingIteration{},
			}
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
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
		"test that the values weighted resampling iteration runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml(
				"values_weighted_resampling_settings.yaml",
			)
			iterations := []simulator.Iteration{
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				&ConstantValuesIteration{},
				&ValuesWeightedResamplingIteration{},
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
		"test that each historical value is resampled with its own weight",
		func(t *testing.T) {
			// weights_a alternates between log 9 and log 1 from step to step
			// and weights_b holds log 2; the data values identify the
			// partition by sign and the step by size, so each output names
			// the (depth, partition) pair it was drawn from.
			const steps = 4000
			store := runHistoryResampling(steps, func(step int) float64 {
				return float64(step%2) * math.Log(9)
			}, func(int) float64 { return math.Log(2) })
			logWeights := [][][]float64{
				store.GetValues("weights_a"), store.GetValues("weights_b")}
			data := [][][]float64{store.GetValues("data_a"), store.GetValues("data_b")}
			resampled := store.GetValues("resampled")
			const depth, discount = 3, 0.5
			// At stored row r the history rows k < min(depth, r) are stored
			// rows r - 1 - k (deeper rows are not yet filled), and pair (k, j)
			// has probability proportional to exp(log weight of j at that
			// row) * discount^k. Sum each pair's probability, and its
			// variance as a Bernoulli count, over every step.
			var expected, variance, observed [depth][2]float64
			for r := 1; r < len(resampled); r++ {
				var p [depth][2]float64
				total := 0.0
				for k := range min(depth, r) {
					for j := range 2 {
						p[k][j] = math.Exp(logWeights[j][r-1-k][0]) * math.Pow(discount, float64(k))
						total += p[k][j]
					}
				}
				found := false
				for k := range min(depth, r) {
					for j := range 2 {
						p[k][j] /= total
						expected[k][j] += p[k][j]
						variance[k][j] += p[k][j] * (1 - p[k][j])
						if resampled[r][0] == data[j][r-1-k][0] {
							observed[k][j]++
							found = true
						}
					}
				}
				if !found {
					t.Fatalf("row %d: resampled %v is none of the history's values", r, resampled[r])
				}
			}
			for k := range depth {
				for j := range 2 {
					if diff := observed[k][j] - expected[k][j]; math.Abs(diff) > 4*math.Sqrt(variance[k][j]) {
						t.Errorf("depth %d partition %d: drawn %v times, want %.1f ± %.1f",
							k, j, observed[k][j], expected[k][j], 4*math.Sqrt(variance[k][j]))
					}
				}
			}
		},
	)
	t.Run(
		"test that a weight negligible beside the others is never drawn",
		func(t *testing.T) {
			// Relative to data_b's weight, data_a's underflows to 0, so every
			// draw is from data_b; and both are far below the log-weight of 0
			// in the history's unfilled rows, which must not be drawn.
			store := runHistoryResampling(20,
				func(int) float64 { return -2000 },
				func(int) float64 { return -1000 })
			for r, row := range store.GetValues("resampled")[1:] {
				if row[0] >= 0 {
					t.Errorf("step %d: resampled %v, want a value of data_b", r+1, row)
				}
			}
		},
	)
}

// runHistoryResampling resamples, for the given number of steps, between two
// data partitions whose values identify the partition by sign and the step by
// size, weighted by the log-weights of weights_a and weights_b from logWeightA
// and logWeightB (see values_weighted_resampling_history_settings.yaml).
func runHistoryResampling(
	steps int,
	logWeightA, logWeightB func(step int) float64,
) *simulator.StateTimeStorage {
	series := func(f func(step int) float64) [][]float64 {
		rows := make([][]float64, steps+1)
		for step := range rows {
			rows[step] = []float64{f(step)}
		}
		return rows
	}
	settings := simulator.LoadSettingsFromYaml(
		"values_weighted_resampling_history_settings.yaml",
	)
	iterations := []simulator.Iteration{
		&FromStorageIteration{Data: series(logWeightA)},
		&FromStorageIteration{Data: series(logWeightB)},
		&FromStorageIteration{Data: series(func(step int) float64 { return float64(step + 1) })},
		&FromStorageIteration{Data: series(func(step int) float64 { return -float64(step + 1) })},
		&ValuesWeightedResamplingIteration{},
	}
	// Each series partition starts from its series' first value, so the
	// initial row in the history follows the series too.
	for index, iteration := range iterations[:4] {
		settings.Iterations[index].InitStateValues =
			iteration.(*FromStorageIteration).Data[0]
	}
	for index, iteration := range iterations {
		iteration.Configure(index, settings)
	}
	store := simulator.NewStateTimeStorage()
	simulator.NewPartitionCoordinator(settings, &simulator.Implementations{
		Iterations:      iterations,
		OutputCondition: &simulator.EveryStepOutputCondition{},
		OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: steps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
	}).Run()
	return store
}
