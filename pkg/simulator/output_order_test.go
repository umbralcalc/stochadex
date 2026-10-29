package simulator

import (
	"runtime"
	"sync"
	"testing"
)

// recordingOutputFunction records the partition name of each row it is sent,
// in the order sent, and panics on the row of partition panicOn at time
// panicAt when panicOn is set.
type recordingOutputFunction struct {
	mutex   sync.Mutex
	names   []string
	panicOn string
	panicAt float64
}

func (r *recordingOutputFunction) Configure(*Settings) {}

func (r *recordingOutputFunction) Output(
	partitionName string,
	state []float64,
	cumulativeTimesteps float64,
) {
	if r.panicOn == partitionName && cumulativeTimesteps == r.panicAt {
		panic("sink failed")
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.names = append(r.names, partitionName)
}

func TestOutputOrder(t *testing.T) {
	t.Run("each step's rows come out in partition order under every strategy", func(t *testing.T) {
		// Many independent partitions finish their work in no fixed order on
		// the concurrent strategies; the rows must not follow it.
		settings, makeIterations := independentSettings(16)
		for strategyName, strategy := range allStrategies() {
			iterations := makeIterations()
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			output := &recordingOutputFunction{}
			NewPartitionCoordinator(settings, &Implementations{
				Iterations:           iterations,
				OutputCondition:      &EveryStepOutputCondition{},
				OutputFunction:       output,
				TerminationCondition: &NumberOfStepsTerminationCondition{MaxNumberOfSteps: 50},
				TimestepFunction:     &ConstantTimestepFunction{Stepsize: 1.0},
				ExecutionStrategy:    strategy,
			}).Run()
			width := len(settings.Iterations)
			if len(output.names) != 51*width {
				t.Fatalf("%s: got %d rows, want %d", strategyName, len(output.names), 51*width)
			}
			for i, name := range output.names {
				if want := settings.Iterations[i%width].Name; name != want {
					t.Fatalf("%s: row %d (step %d) is partition %s, want %s",
						strategyName, i, i/width, name, want)
				}
			}
		}
	})

	t.Run("a panicking output names its partition and commits nothing", func(t *testing.T) {
		for strategyName, strategy := range allStrategies() {
			before := runtime.NumGoroutine()
			settings, makeIterations := chainSettings()
			iterations := makeIterations()
			for index, iteration := range iterations {
				iteration.Configure(index, settings)
			}
			coordinator := NewPartitionCoordinator(settings, &Implementations{
				Iterations:           iterations,
				OutputCondition:      &EveryStepOutputCondition{},
				OutputFunction:       &recordingOutputFunction{panicOn: "partition_1", panicAt: 5},
				TerminationCondition: &NumberOfStepsTerminationCondition{MaxNumberOfSteps: 20},
				TimestepFunction:     &ConstantTimestepFunction{Stepsize: 1.0},
				ExecutionStrategy:    strategy,
			})
			recovered := panicFrom(coordinator.Run)
			failure, ok := recovered.(*PartitionPanic)
			if !ok {
				t.Fatalf("%s: want a *PartitionPanic, got %T: %v", strategyName, recovered, recovered)
			}
			if failure.Partition != "partition_1" || failure.Value != "sink failed" {
				t.Errorf("%s: got %v", strategyName, failure)
			}
			if got := coordinator.Shared.TimestepsHistory.Values.AtVec(0); got != 4.0 {
				t.Errorf("%s: want the time left at 4, got %v", strategyName, got)
			}
			waitForGoroutines(t, before)
		}
	})
}
