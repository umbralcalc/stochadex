package simulator

import (
	"fmt"
	"strings"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// paramEchoIteration's next state is a copy of its "level" param, so its
// output shows exactly which params value each step read.
type paramEchoIteration struct{}

func (p *paramEchoIteration) Configure(int, *Settings) {}

func (p *paramEchoIteration) Iterate(
	params *Params,
	_ int,
	_ []*StateHistory,
	_ *CumulativeTimestepsHistory,
) []float64 {
	return append([]float64(nil), params.Get("level")...)
}

// injectSettings is two echo partitions: dial reads its own "level" param, and
// follower's "level" is dial's output this step (params_from_upstream).
func injectSettings() *Settings {
	settings := &Settings{
		Iterations: []IterationSettings{
			{
				Name:              "dial",
				Params:            NewParams(map[string][]float64{"level": {1, 2}}),
				InitStateValues:   []float64{0, 0},
				StateWidth:        2,
				StateHistoryDepth: 1,
			},
			{
				Name:               "follower",
				Params:             NewParams(map[string][]float64{"level": {0, 0}}),
				ParamsFromUpstream: map[string]UpstreamConfig{"level": {Upstream: 0}},
				InitStateValues:    []float64{0, 0},
				StateWidth:         2,
				StateHistoryDepth:  1,
			},
		},
		InitTimeValue:         0.0,
		TimestepsHistoryDepth: 1,
	}
	settings.Init()
	return settings
}

func injectImplementations(
	steps int,
	strategy ExecutionStrategy,
	store *StateTimeStorage,
) *Implementations {
	return &Implementations{
		Iterations:           []Iteration{&paramEchoIteration{}, &paramEchoIteration{}},
		OutputCondition:      &EveryStepOutputCondition{},
		OutputFunction:       &StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &NumberOfStepsTerminationCondition{MaxNumberOfSteps: steps},
		TimestepFunction:     &ConstantTimestepFunction{Stepsize: 1.0},
		ExecutionStrategy:    strategy,
	}
}

// stepWithInjections runs the injection topology for steps steps, calling
// inject(coordinator, k) before each step k (1-based), and returns its output.
func stepWithInjections(
	t *testing.T,
	settings *Settings,
	steps int,
	strategy ExecutionStrategy,
	inject func(coordinator *PartitionCoordinator, step int),
) *StateTimeStorage {
	t.Helper()
	store := NewStateTimeStorage()
	coordinator := NewPartitionCoordinator(settings, injectImplementations(steps, strategy, store))
	stepper := coordinator.NewStepper()
	defer stepper.Close()
	for step := 1; !coordinator.ReadyToTerminate(); step++ {
		inject(coordinator, step)
		stepper.Step()
	}
	return store
}

// assertRows fails unless partition's output row for each step (row 0 is the
// initial state) equals want(step).
func assertRows(t *testing.T, store *StateTimeStorage, partition string, steps int,
	want func(step int) []float64) {
	t.Helper()
	rows := store.GetValues(partition)
	if len(rows) != steps+1 {
		t.Fatalf("%s: %d rows, want %d", partition, len(rows), steps+1)
	}
	for step := 1; step <= steps; step++ {
		if !floats.Equal(rows[step], want(step)) {
			t.Fatalf("%s at step %d = %v, want %v", partition, step, rows[step], want(step))
		}
	}
}

func TestInjectParams(t *testing.T) {
	strategies := map[string]ExecutionStrategy{
		"default":           nil,
		"spawn_per_step":    &SpawnPerStepExecution{},
		"persistent_worker": &PersistentWorkerExecution{},
		"inline":            &InlineExecution{},
	}
	configured, injected, again := []float64{1, 2}, []float64{7, 9}, []float64{-3, 4}

	for name, strategy := range strategies {
		t.Run(name+": an injection before step k is seen from step k, and not before", func(t *testing.T) {
			const steps, k, k2 = 8, 4, 7
			store := stepWithInjections(t, injectSettings(), steps, strategy,
				func(coordinator *PartitionCoordinator, step int) {
					switch step {
					case k:
						if err := coordinator.InjectParams("dial", "level", injected); err != nil {
							t.Fatal(err)
						}
					case k2:
						if err := coordinator.InjectParams("dial", "level", again); err != nil {
							t.Fatal(err)
						}
					}
				})
			want := func(step int) []float64 {
				switch {
				case step < k:
					return configured
				case step < k2:
					return injected
				default:
					return again
				}
			}
			assertRows(t, store, "dial", steps, want)
			// follower reads dial's output within the same step, so it sees the
			// injection in that step too.
			assertRows(t, store, "follower", steps, want)
		})
	}

	t.Run("it writes in place: a coordinator rebuilt from the same settings starts from it", func(t *testing.T) {
		settings := injectSettings()
		stepWithInjections(t, settings, 3, nil, func(coordinator *PartitionCoordinator, step int) {
			if step == 1 {
				if err := coordinator.InjectParams("dial", "level", injected); err != nil {
					t.Fatal(err)
				}
			}
		})
		if got := settings.Iterations[0].Params.Map["level"]; !floats.Equal(got, injected) {
			t.Fatalf("settings' level = %v after an injection, want %v", got, injected)
		}
		rerun := stepWithInjections(t, settings, 3, nil, func(*PartitionCoordinator, int) {})
		assertRows(t, rerun, "dial", 3, func(int) []float64 { return injected })
		// Building from the config again starts from the configured values.
		fresh := stepWithInjections(t, injectSettings(), 3, nil, func(*PartitionCoordinator, int) {})
		assertRows(t, fresh, "dial", 3, func(int) []float64 { return configured })
	})

	t.Run("an injection allocates only the copy of its values", func(t *testing.T) {
		coordinator := NewPartitionCoordinator(injectSettings(),
			injectImplementations(3, nil, NewStateTimeStorage()))
		for i := range 6 { // a partition with several params keys
			coordinator.Iterators[0].Params.Map[fmt.Sprint("key", i)] = []float64{1, 2, 3}
		}
		allocs := testing.AllocsPerRun(1000, func() {
			if err := coordinator.InjectParams("dial", "level", injected); err != nil {
				t.Fatal(err)
			}
		})
		if allocs > 1 {
			t.Errorf("InjectParams made %v allocations, want at most 1 (the values' copy)", allocs)
		}
	})

	t.Run("the caller's slice is copied", func(t *testing.T) {
		values := []float64{7, 9}
		store := stepWithInjections(t, injectSettings(), 3, nil, func(coordinator *PartitionCoordinator, step int) {
			if step == 1 {
				if err := coordinator.InjectParams("dial", "level", values); err != nil {
					t.Fatal(err)
				}
				values[0] = 100
			}
		})
		assertRows(t, store, "dial", 3, func(int) []float64 { return injected })
	})

	errorCases := []struct {
		name, partition, key string
		values               []float64
		want                 string
	}{
		{"an unknown partition", "dail", "level", injected, `no partition named "dail"`},
		{"a key set from upstream", "follower", "level", injected,
			`partition "follower" sets params key "level" from params_from_upstream`},
		{"an undeclared key", "dial", "levle", injected,
			`partition "dial" has no params key "levle"`},
		{"a different width", "dial", "level", []float64{1, 2, 3},
			`params key "level" has width 2, got 3 values`},
	}
	for _, c := range errorCases {
		t.Run(c.name+" is an error, and changes nothing", func(t *testing.T) {
			store := stepWithInjections(t, injectSettings(), 3, nil, func(coordinator *PartitionCoordinator, step int) {
				if step == 2 {
					err := coordinator.InjectParams(c.partition, c.key, c.values)
					if err == nil || !strings.Contains(err.Error(), c.want) {
						t.Errorf("expected an error containing %q, got %v", c.want, err)
					}
				}
			})
			assertRows(t, store, "dial", 3, func(int) []float64 { return configured })
		})
	}

	t.Run("the topology passes the harnesses", func(t *testing.T) {
		if err := RunWithHarnesses(injectSettings(),
			injectImplementations(5, nil, NewStateTimeStorage())); err != nil {
			t.Error(err)
		}
	})
}

func BenchmarkInjectParams(b *testing.B) {
	coordinator := NewPartitionCoordinator(injectSettings(),
		injectImplementations(3, nil, NewStateTimeStorage()))
	values := []float64{7, 9}
	b.ReportAllocs()
	for b.Loop() {
		if err := coordinator.InjectParams("dial", "level", values); err != nil {
			b.Fatal(err)
		}
	}
}
