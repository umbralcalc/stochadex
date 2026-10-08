package general

import (
	"fmt"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// forwardingRun is an outer run of one host partition embedding a short inner
// run of one Wiener process, whose variances the host forwards from its own
// params ("inner/variances"), alongside params it does not forward. It returns
// the coordinator and the inner settings.
func forwardingRun(outerSteps int, hostParams map[string][]float64) (*simulator.PartitionCoordinator, *simulator.Settings) {
	inner := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{
			Name:              "inner",
			Params:            simulator.NewParams(map[string][]float64{"variances": {1}}),
			InitStateValues:   []float64{0},
			Seed:              5,
			StateWidth:        1,
			StateHistoryDepth: 1,
		}},
		InitTimeValue:         0,
		TimestepsHistoryDepth: 1,
	}
	inner.Init()
	embedded := NewEmbeddedSimulationRunIteration(inner, &simulator.Implementations{
		Iterations:           []simulator.Iteration{&continuous.WienerProcessIteration{}},
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 5},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
	})
	outer := &simulator.Settings{
		Iterations: []simulator.IterationSettings{{
			Name:              "host",
			Params:            simulator.NewParams(hostParams),
			InitStateValues:   []float64{0},
			Seed:              1,
			StateWidth:        1,
			StateHistoryDepth: 1,
		}},
		InitTimeValue:         0,
		TimestepsHistoryDepth: 1,
	}
	outer.Init()
	embedded.Configure(0, outer)
	return simulator.NewPartitionCoordinator(outer, &simulator.Implementations{
		Iterations:           []simulator.Iteration{embedded},
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: outerSteps},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		ExecutionStrategy:    &simulator.InlineExecution{},
	}), inner
}

// BenchmarkEmbeddedRunForwarding is 500 outer steps of a host forwarding one
// param into a 5-step inner run, with a handful of params it does not forward.
func BenchmarkEmbeddedRunForwarding(b *testing.B) {
	params := map[string][]float64{"inner/variances": {2}, "burn_in_steps": {0},
		"a": {1}, "b": {1, 2}, "c": {3}, "d": {4, 5, 6}}
	b.ReportAllocs()
	for b.Loop() {
		coordinator, _ := forwardingRun(500, params)
		coordinator.Run()
	}
}

// hostRows runs the forwarding topology and returns the host's rows.
func hostRows(t *testing.T, outerSteps int, hostParams map[string][]float64,
	setInner func(*simulator.Settings)) [][]float64 {
	t.Helper()
	coordinator, inner := forwardingRun(outerSteps, hostParams)
	if setInner != nil {
		setInner(inner)
	}
	rows := [][]float64{}
	stepper := coordinator.NewStepper()
	for !coordinator.ReadyToTerminate() {
		stepper.Step()
		rows = append(rows, append([]float64(nil), coordinator.Shared.StateHistories[0].Values.RawRowView(0)...))
	}
	stepper.Close()
	return rows
}

func TestEmbeddedRunForwarding(t *testing.T) {
	t.Run("a forwarded param sets the inner run's param", func(t *testing.T) {
		got := hostRows(t, 20, map[string][]float64{"inner/variances": {4}, "burn_in_steps": {0},
			"unrelated": {1}}, nil)
		// Reference: the inner run given the value directly, with nothing forwarded.
		want := hostRows(t, 20, map[string][]float64{"burn_in_steps": {0}},
			func(inner *simulator.Settings) { inner.Iterations[0].Params.Set("variances", []float64{4}) })
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("forwarding variances gave %v, setting them directly %v", got[:3], want[:3])
		}
		unforwarded := hostRows(t, 20, map[string][]float64{"burn_in_steps": {0}}, nil)
		if fmt.Sprint(got) == fmt.Sprint(unforwarded) {
			t.Error("positive control: forwarding variances 4 should change the run")
		}
	})

	t.Run("init_state_values forwards into the inner run's initial state", func(t *testing.T) {
		rows := hostRows(t, 3, map[string][]float64{"inner/init_state_values": {7},
			"inner/variances": {0}, "burn_in_steps": {0}}, nil)
		for k, row := range rows {
			if row[0] != 7 {
				t.Fatalf("outer step %d: the inner run ended at %v, want 7 (no noise, started at 7)", k+1, row)
			}
		}
	})

	t.Run("a key set only by params_from_upstream is forwarded too", func(t *testing.T) {
		// The host's params do not declare inner/variances: an upstream sets it
		// each step, so Configure must find it among params_from_upstream.
		inner := &simulator.Settings{
			Iterations: []simulator.IterationSettings{{Name: "inner",
				Params:          simulator.NewParams(map[string][]float64{"variances": {1}}),
				InitStateValues: []float64{0}, Seed: 5, StateWidth: 1, StateHistoryDepth: 1}},
			TimestepsHistoryDepth: 1,
		}
		inner.Init()
		run := func(forward bool) [][]float64 {
			innerCopy := *inner
			innerCopy.Iterations = []simulator.IterationSettings{inner.Iterations[0]}
			innerCopy.Iterations[0].Params = simulator.NewParams(map[string][]float64{"variances": {1}})
			if !forward {
				innerCopy.Iterations[0].Params.Set("variances", []float64{9})
			}
			embedded := NewEmbeddedSimulationRunIteration(&innerCopy, &simulator.Implementations{
				Iterations:           []simulator.Iteration{&continuous.WienerProcessIteration{}},
				OutputCondition:      &simulator.NilOutputCondition{},
				OutputFunction:       &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 5},
				TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			})
			host := simulator.IterationSettings{Name: "host",
				Params:          simulator.NewParams(map[string][]float64{"burn_in_steps": {0}}),
				InitStateValues: []float64{0}, Seed: 1, StateWidth: 1, StateHistoryDepth: 1}
			if forward {
				host.ParamsFromUpstream = map[string]simulator.UpstreamConfig{"inner/variances": {Upstream: 0}}
			}
			outer := &simulator.Settings{Iterations: []simulator.IterationSettings{
				{Name: "driver", Params: simulator.NewParams(map[string][]float64{"param_values": {9}}),
					InitStateValues: []float64{9}, StateWidth: 1, StateHistoryDepth: 1},
				host}, TimestepsHistoryDepth: 1}
			outer.Init()
			iterations := []simulator.Iteration{&ParamValuesIteration{}, embedded}
			for index, iteration := range iterations {
				iteration.Configure(index, outer)
			}
			store := simulator.NewStateTimeStorage()
			simulator.NewPartitionCoordinator(outer, &simulator.Implementations{
				Iterations: iterations, OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 10},
				TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}).Run()
			return store.GetValues("host")
		}
		if got, want := run(true), run(false); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("forwarding from upstream gave %v, setting variances 9 directly %v", got[:3], want[:3])
		}
	})

	t.Run("a forwarded key missing from a step's params leaves the inner param alone", func(t *testing.T) {
		coordinator, _ := forwardingRun(20, map[string][]float64{"inner/variances": {4}, "burn_in_steps": {0}})
		delete(coordinator.Iterators[0].Params.Map, "inner/variances")
		store := [][]float64{}
		stepper := coordinator.NewStepper()
		for !coordinator.ReadyToTerminate() {
			stepper.Step()
			store = append(store, append([]float64(nil), coordinator.Shared.StateHistories[0].Values.RawRowView(0)...))
		}
		stepper.Close()
		want := hostRows(t, 20, map[string][]float64{"burn_in_steps": {0}}, nil)
		if fmt.Sprint(store) != fmt.Sprint(want) {
			t.Errorf("with the key gone the run should use the inner run's own variances")
		}
	})

	t.Run("a key naming an inner partition that does not exist panics at Configure", func(t *testing.T) {
		defer func() {
			recovered := recover()
			if recovered == nil || !strings.Contains(fmt.Sprint(recovered),
				`params key "inenr/variances" forwards into partition "inenr"`) {
				t.Errorf("expected a panic naming the key, got %v", recovered)
			}
		}()
		forwardingRun(5, map[string][]float64{"inenr/variances": {2}, "burn_in_steps": {0}})
	})

	t.Run("the topology passes the harnesses", func(t *testing.T) {
		inner := &simulator.Settings{Iterations: []simulator.IterationSettings{{Name: "inner",
			Params:          simulator.NewParams(map[string][]float64{"variances": {1}}),
			InitStateValues: []float64{0}, Seed: 5, StateWidth: 1, StateHistoryDepth: 1}},
			TimestepsHistoryDepth: 1}
		inner.Init()
		outer := &simulator.Settings{Iterations: []simulator.IterationSettings{{Name: "host",
			Params:          simulator.NewParams(map[string][]float64{"inner/variances": {2}, "burn_in_steps": {0}}),
			InitStateValues: []float64{0}, Seed: 1, StateWidth: 1, StateHistoryDepth: 1}},
			TimestepsHistoryDepth: 1}
		outer.Init()
		if err := simulator.RunWithHarnesses(outer, &simulator.Implementations{
			Iterations: []simulator.Iteration{NewEmbeddedSimulationRunIteration(inner, &simulator.Implementations{
				Iterations:           []simulator.Iteration{&continuous.WienerProcessIteration{}},
				OutputCondition:      &simulator.NilOutputCondition{},
				OutputFunction:       &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 5},
				TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			})},
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 10},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		}); err != nil {
			t.Error(err)
		}
	})
}

func TestForwardedParamTarget(t *testing.T) {
	for key, want := range map[string]string{
		"inner/variances": "inner variances true",
		"burn_in_steps":   "  false",
		"a/b":             "a b true",
	} {
		inner, param, ok := ForwardedParamTarget(key)
		if got := fmt.Sprint(inner, " ", param, " ", ok); got != want {
			t.Errorf("%q: got %q, want %q", key, got, want)
		}
	}
}
