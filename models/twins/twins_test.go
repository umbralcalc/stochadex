package twins

import (
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

// walk is a build of a Wiener process, and a constant beside it, for steps steps.
func walk(steps int, seed uint64) *simulator.ConfigGenerator {
	gen := simulator.NewConfigGenerator()
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "walk",
		Iteration:         &continuous.WienerProcessIteration{},
		Params:            simulator.NewParams(map[string][]float64{"variances": {1}}),
		InitStateValues:   []float64{0},
		StateHistoryDepth: 1,
		Seed:              seed,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "level",
		Iteration:         &general.ConstantValuesIteration{},
		Params:            simulator.NewParams(map[string][]float64{}),
		InitStateValues:   []float64{4},
		StateHistoryDepth: 1,
	})
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1},
	})
	return gen
}

func TestRunAndCompare(t *testing.T) {
	t.Run("Run records every step of every partition", func(t *testing.T) {
		storage := Run(walk(5, 1))
		if len(storage.GetValues("walk")) != 6 || len(storage.GetValues("level")) != 6 {
			t.Fatalf("got %d and %d rows, want 6 each",
				len(storage.GetValues("walk")), len(storage.GetValues("level")))
		}
	})

	t.Run("the same build agrees with itself", func(t *testing.T) {
		if err := Compare(Run(walk(20, 1)), Run(walk(20, 1)), 1e-12); err != nil {
			t.Error(err)
		}
	})

	for _, c := range []struct {
		name string
		got  *simulator.StateTimeStorage
		want string
	}{
		{"another seed", Run(walk(20, 2)), "partition walk, step 1"},
		{"fewer steps", Run(walk(10, 1)), "partition level: 11 rows, want 21"},
	} {
		t.Run(c.name+" differs, saying where", func(t *testing.T) {
			if err := Compare(c.got, Run(walk(20, 1)), 1e-12); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want an error containing %q", err, c.want)
			}
		})
	}

	t.Run("a different width differs", func(t *testing.T) {
		got := simulator.NewStateTimeStorage()
		want := simulator.NewStateTimeStorage()
		want.SetValues("x", [][]float64{{0}, {1}})
		got.SetValues("x", [][]float64{{0}, {1, 2}})
		if err := Compare(got, want, 1e-12); err == nil || !strings.Contains(err.Error(), "width 2, want 1") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("agreement where nothing moves shows nothing", func(t *testing.T) {
		still := simulator.NewStateTimeStorage()
		still.SetValues("x", [][]float64{{3}, {3}})
		if err := Compare(still, still, 1e-12); err == nil || !strings.Contains(err.Error(), "moved") {
			t.Errorf("got %v", err)
		}
	})

	t.Run("a value within tolerance of a large one agrees", func(t *testing.T) {
		want := simulator.NewStateTimeStorage()
		want.SetValues("x", [][]float64{{0}, {1e6}})
		got := simulator.NewStateTimeStorage()
		got.SetValues("x", [][]float64{{0}, {1e6 + 1e-7}})
		if err := Compare(got, want, 1e-12); err != nil {
			t.Errorf("1e-7 on 1e6 is within 1e-12 relative tolerance: %v", err)
		}
	})

	t.Run("WithStepsize sets the clock's stepsize", func(t *testing.T) {
		times := Run(WithStepsize(walk(4, 1), 0.25)).GetTimes()
		if !floats.Equal(times, []float64{0, 0.25, 0.5, 0.75, 1}) {
			t.Errorf("got times %v", times)
		}
	})

	t.Run("RunQuietly runs to termination, recording nothing", func(t *testing.T) {
		gen := walk(5, 1)
		RunQuietly(gen)
		if _, ok := gen.GetSimulation().OutputFunction.(*simulator.NilOutputFunction); !ok {
			t.Error("RunQuietly should record nothing")
		}
	})
}
