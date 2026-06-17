package continuous

import (
	"math"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// allocatingWienerProcessIteration is the pre-refactor Wiener step that
// allocates a fresh state row on every Iterate. It exists only as the
// benchmark baseline against the reusable-buffer WienerProcessIteration, so
// the allocation difference can be measured directly (run with -benchmem).
type allocatingWienerProcessIteration struct {
	unitNormalDist *distuv.Normal
}

func (w *allocatingWienerProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	w.unitNormalDist = &distuv.Normal{
		Mu:    0.0,
		Sigma: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

func (w *allocatingWienerProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) +
			math.Sqrt(params.GetIndex("variances", i)*
				timestepsHistory.NextIncrement)*w.unitNormalDist.Rand()
	}
	return values
}

// runWienerBenchmark builds a topology of independent (edge-free) Wiener
// partitions and runs it to termination under InlineExecution, which adds no
// per-step goroutine/channel allocation so the only per-step allocation left to
// measure is whatever the iteration itself does. NilOutputFunction is used so
// the comparison isolates the iteration's allocation rather than the copy made
// at the storage retain point.
func runWienerBenchmark(b *testing.B, makeIteration func() simulator.Iteration) {
	const (
		partitions = 8
		width      = 32
		steps      = 1000
	)
	variances := make([]float64, width)
	for i := range variances {
		variances[i] = 1.0
	}
	initStateValues := make([]float64, width)

	iterationSettings := make([]simulator.IterationSettings, partitions)
	for p := range iterationSettings {
		iterationSettings[p] = simulator.IterationSettings{
			Name: strconv.Itoa(p),
			Params: simulator.NewParams(map[string][]float64{
				"variances": variances,
			}),
			InitStateValues:   initStateValues,
			Seed:              uint64(p),
			StateWidth:        width,
			StateHistoryDepth: 1,
		}
	}
	settings := &simulator.Settings{
		Iterations:            iterationSettings,
		InitTimeValue:         0.0,
		TimestepsHistoryDepth: 1,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		iterations := make([]simulator.Iteration, partitions)
		for p := range iterations {
			iterations[p] = makeIteration()
		}
		for index, iteration := range iterations {
			iteration.Configure(index, settings)
		}
		coordinator := simulator.NewPartitionCoordinator(
			settings,
			&simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: steps,
				},
				TimestepFunction:  &simulator.ConstantTimestepFunction{Stepsize: 1.0},
				ExecutionStrategy: &simulator.InlineExecution{},
			},
		)
		coordinator.Run()
	}
}

// BenchmarkWienerProcessAllocating is the baseline: a fresh state row is
// allocated every step.
func BenchmarkWienerProcessAllocating(b *testing.B) {
	runWienerBenchmark(b, func() simulator.Iteration {
		return &allocatingWienerProcessIteration{}
	})
}

// BenchmarkWienerProcessReusedBuffer is the current implementation, which
// writes the next state into the partition's reusable NextValues buffer.
func BenchmarkWienerProcessReusedBuffer(b *testing.B) {
	runWienerBenchmark(b, func() simulator.Iteration {
		return &WienerProcessIteration{}
	})
}
