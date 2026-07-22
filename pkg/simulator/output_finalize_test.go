package simulator

import "testing"

// finalizeCountingOutput records how many rows it saw and how many times Finalize
// was called, so a test can assert finalization happens exactly once and only after
// every row has been output.
type finalizeCountingOutput struct {
	rows          int
	finalizeCalls int
	rowsAtFinal   int
}

func (f *finalizeCountingOutput) Configure(*Settings) {}

func (f *finalizeCountingOutput) Output(string, []float64, float64) { f.rows++ }

func (f *finalizeCountingOutput) Finalize() {
	f.finalizeCalls++
	f.rowsAtFinal = f.rows
}

// plainOutput implements only OutputFunction — the existing shape — to prove a sink
// without Finalize is untouched by the hook.
type plainOutput struct{ rows int }

func (p *plainOutput) Configure(*Settings)               {}
func (p *plainOutput) Output(string, []float64, float64) { p.rows++ }

func TestOutputFunctionFinalize(t *testing.T) {
	// Build a minimal one-partition run so Run() drives real output.
	newRun := func(out OutputFunction) *PartitionCoordinator {
		generator := NewConfigGenerator()
		generator.SetPartition(&PartitionConfig{
			Name:              "p",
			Iteration:         &doublingProcessIteration{},
			Params:            NewParams(make(map[string][]float64)),
			InitStateValues:   []float64{1.0},
			StateHistoryDepth: 1,
			Seed:              0,
		})
		generator.SetSimulation(&SimulationConfig{
			OutputCondition:      &EveryStepOutputCondition{},
			OutputFunction:       out,
			TerminationCondition: &NumberOfStepsTerminationCondition{MaxNumberOfSteps: 5},
			TimestepFunction:     &ConstantTimestepFunction{Stepsize: 1.0},
			InitTimeValue:        0.0,
		})
		return NewPartitionCoordinator(generator.GenerateConfigs())
	}

	t.Run("Finalize runs once, after every row", func(t *testing.T) {
		sink := &finalizeCountingOutput{}
		newRun(sink).Run()

		if sink.finalizeCalls != 1 {
			t.Errorf("Finalize called %d times, want exactly 1", sink.finalizeCalls)
		}
		if sink.rows == 0 {
			t.Fatal("no rows were output — the test run did nothing")
		}
		// The point of the hook: nothing may still be pending when Finalize runs, or a
		// columnar sink would seal a buffer that is missing its last rows.
		if sink.rowsAtFinal != sink.rows {
			t.Errorf("Finalize saw %d rows but %d were output — it ran too early",
				sink.rowsAtFinal, sink.rows)
		}
	})

	t.Run("a sink without Finalize is unaffected", func(t *testing.T) {
		sink := &plainOutput{}
		newRun(sink).Run() // must not panic on the type assertion
		if sink.rows == 0 {
			t.Error("plain output function recorded no rows")
		}
	})
}
