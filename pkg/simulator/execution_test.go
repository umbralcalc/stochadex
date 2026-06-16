package simulator

import (
	"testing"

	"gonum.org/v1/gonum/floats"
)

// namedStrategies returns every non-default execution strategy keyed by a
// label, so equivalence and harness tests can iterate over all of them. The
// default (nil) strategy is tested separately as the reference behaviour.
func namedStrategies() map[string]ExecutionStrategy {
	return map[string]ExecutionStrategy{
		"spawn_per_step":    &SpawnPerStepExecution{},
		"persistent_worker": &PersistentWorkerExecution{},
	}
}

// runStrategyConfig runs the given topology to termination under the supplied
// strategy (nil for the default) and returns the recorded output. Fresh
// iteration instances are built per run so no mutable state is shared between
// runs being compared.
func runStrategyConfig(
	settings *Settings,
	makeIterations func() []Iteration,
	maxSteps int,
	strategy ExecutionStrategy,
) *StateTimeStorage {
	iterations := makeIterations()
	for index, iteration := range iterations {
		iteration.Configure(index, settings)
	}
	store := NewStateTimeStorage()
	implementations := &Implementations{
		Iterations:      iterations,
		OutputCondition: &EveryStepOutputCondition{},
		OutputFunction:  &StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: maxSteps,
		},
		TimestepFunction:  &ConstantTimestepFunction{Stepsize: 1.0},
		ExecutionStrategy: strategy,
	}
	coordinator := NewPartitionCoordinator(settings, implementations)
	coordinator.Run()
	return store
}

// assertStoresEqual fails the test unless got matches want for every partition,
// timestep and value index exactly.
func assertStoresEqual(t *testing.T, want, got *StateTimeStorage, label string) {
	t.Helper()
	for _, name := range want.GetNames() {
		wantValues := want.GetValues(name)
		gotValues := got.GetValues(name)
		if len(wantValues) != len(gotValues) {
			t.Fatalf("%s: partition %s has %d rows, want %d",
				label, name, len(gotValues), len(wantValues))
		}
		for tIndex := range wantValues {
			if !floats.Equal(wantValues[tIndex], gotValues[tIndex]) {
				t.Fatalf("%s: partition %s row %d = %v, want %v",
					label, name, tIndex, gotValues[tIndex], wantValues[tIndex])
			}
		}
	}
}

// chainSettings builds a three-partition topology with a within-step
// params_from_upstream edge (partition_1 -> partition_2), which exercises the
// cross-goroutine broadcast/receive path under each strategy.
func chainSettings() (*Settings, func() []Iteration) {
	settings := LoadSettingsFromYaml("execution_settings.yaml")
	settings.Init()
	makeIterations := func() []Iteration {
		return []Iteration{
			&doublingProcessIteration{},
			&paramMultProcessIteration{},
			&paramMultProcessIteration{},
		}
	}
	return settings, makeIterations
}

// independentSettings builds a topology of independent doubling partitions
// with no edges between them — the ensemble-shaped case.
func independentSettings(numPartitions int) (*Settings, func() []Iteration) {
	iterationSettings := make([]IterationSettings, 0, numPartitions)
	for i := 0; i < numPartitions; i++ {
		iterationSettings = append(iterationSettings, IterationSettings{
			Params:            NewParams(make(map[string][]float64)),
			InitStateValues:   []float64{float64(i) + 1.0, float64(i) + 2.0},
			Seed:              uint64(i * 13),
			StateWidth:        2,
			StateHistoryDepth: 3,
		})
	}
	settings := &Settings{
		Iterations:            iterationSettings,
		InitTimeValue:         0.0,
		TimestepsHistoryDepth: 3,
	}
	settings.Init()
	makeIterations := func() []Iteration {
		iterations := make([]Iteration, 0, numPartitions)
		for i := 0; i < numPartitions; i++ {
			iterations = append(iterations, &doublingProcessIteration{})
		}
		return iterations
	}
	return settings, makeIterations
}

// misorderedSettings builds a topology whose within-step edge runs "backwards"
// in partition order: partition_0 reads upstream partition_1. The channel
// strategies handle this (the consumer blocks until the producer broadcasts),
// but inline execution cannot, since it would read the producer's output
// before the producer has run this step.
func misorderedSettings() (*Settings, func() []Iteration) {
	settings := &Settings{
		Iterations: []IterationSettings{
			{
				Name:   "partition_0",
				Params: NewParams(make(map[string][]float64)),
				ParamsFromUpstream: map[string]UpstreamConfig{
					"multipliers": {Upstream: 1},
				},
				InitStateValues:   []float64{1.0, 2.0, 3.0},
				StateWidth:        3,
				StateHistoryDepth: 2,
			},
			{
				Name:              "partition_1",
				Params:            NewParams(make(map[string][]float64)),
				InitStateValues:   []float64{1.0, 1.0, 1.0},
				StateWidth:        3,
				StateHistoryDepth: 2,
			},
		},
		InitTimeValue:         0.0,
		TimestepsHistoryDepth: 2,
	}
	settings.Init()
	makeIterations := func() []Iteration {
		return []Iteration{
			&paramMultProcessIteration{},
			&doublingProcessIteration{},
		}
	}
	return settings, makeIterations
}

func TestExecutionStrategies(t *testing.T) {
	const maxSteps = 20

	topologies := map[string]func() (*Settings, func() []Iteration){
		"chain":            func() (*Settings, func() []Iteration) { return chainSettings() },
		"single_partition": func() (*Settings, func() []Iteration) { return independentSettings(1) },
		"independent_8":    func() (*Settings, func() []Iteration) { return independentSettings(8) },
	}

	t.Run("output is byte-identical across strategies", func(t *testing.T) {
		for topologyName, build := range topologies {
			settings, makeIterations := build()
			reference := runStrategyConfig(settings, makeIterations, maxSteps, nil)
			for strategyName, strategy := range namedStrategies() {
				got := runStrategyConfig(settings, makeIterations, maxSteps, strategy)
				assertStoresEqual(t, reference, got,
					topologyName+"/"+strategyName)
			}
		}
	})

	t.Run("default Run matches explicit SpawnPerStepExecution", func(t *testing.T) {
		settings, makeIterations := chainSettings()
		reference := runStrategyConfig(settings, makeIterations, maxSteps, nil)
		explicit := runStrategyConfig(
			settings, makeIterations, maxSteps, &SpawnPerStepExecution{})
		assertStoresEqual(t, reference, explicit, "default-vs-explicit")
	})

	t.Run("RunWithHarnesses passes under every strategy", func(t *testing.T) {
		// nil strategy (default) plus each explicit strategy.
		strategies := namedStrategies()
		strategies["default"] = nil
		for strategyName, strategy := range strategies {
			settings, makeIterations := chainSettings()
			implementations := &Implementations{
				Iterations:      makeIterations(),
				OutputCondition: &EveryStepOutputCondition{},
				OutputFunction:  &NilOutputFunction{},
				TerminationCondition: &NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: maxSteps,
				},
				TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := RunWithHarnessesUsing(
				settings, implementations, strategy); err != nil {
				t.Errorf("strategy %s: %v", strategyName, err)
			}
		}
	})

	t.Run("InlineExecution matches default when ordering is valid",
		func(t *testing.T) {
			// Inline execution is valid for edge-free topologies and for
			// within-step edges where upstreams precede consumers (the chain's
			// partition_2 reads partition_1, and 1 < 2).
			inlineValid := map[string]func() (*Settings, func() []Iteration){
				"single_partition": func() (*Settings, func() []Iteration) {
					return independentSettings(1)
				},
				"independent_8": func() (*Settings, func() []Iteration) {
					return independentSettings(8)
				},
				"chain": func() (*Settings, func() []Iteration) {
					return chainSettings()
				},
			}
			for topologyName, build := range inlineValid {
				settings, makeIterations := build()
				reference := runStrategyConfig(settings, makeIterations, maxSteps, nil)
				got := runStrategyConfig(
					settings, makeIterations, maxSteps, &InlineExecution{})
				assertStoresEqual(t, reference, got, "inline/"+topologyName)
			}
		})

	t.Run("InlineExecution passes RunWithHarnesses on a single partition",
		func(t *testing.T) {
			settings, makeIterations := independentSettings(1)
			implementations := &Implementations{
				Iterations:      makeIterations(),
				OutputCondition: &EveryStepOutputCondition{},
				OutputFunction:  &NilOutputFunction{},
				TerminationCondition: &NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: maxSteps,
				},
				TimestepFunction: &ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := RunWithHarnessesUsing(
				settings, implementations, &InlineExecution{}); err != nil {
				t.Errorf("inline harness: %v", err)
			}
		})

	t.Run("InlineExecution panics on mis-ordered edges", func(t *testing.T) {
		// A consumer ordered before its upstream is fine for the channel
		// strategies (which block) but cannot be serviced inline; it must
		// panic rather than read stale values.
		settings, makeIterations := misorderedSettings()

		// The default strategy handles the reverse-ordered edge fine.
		runStrategyConfig(settings, makeIterations, maxSteps, nil)

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic for mis-ordered upstream under inline")
			}
		}()
		runStrategyConfig(settings, makeIterations, maxSteps, &InlineExecution{})
	})
}

// benchmarkStrategy runs a many-partition, many-step simulation under the
// given strategy. Comparing the spawn-per-step and persistent-worker results
// shows the per-step goroutine-spawn cost the latter removes.
func benchmarkStrategy(b *testing.B, strategy ExecutionStrategy) {
	settings, makeIterations := independentSettings(16)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runStrategyConfig(settings, makeIterations, 200, strategy)
	}
}

func BenchmarkSpawnPerStepExecution(b *testing.B) {
	benchmarkStrategy(b, &SpawnPerStepExecution{})
}

func BenchmarkPersistentWorkerExecution(b *testing.B) {
	benchmarkStrategy(b, &PersistentWorkerExecution{})
}

func BenchmarkInlineExecution(b *testing.B) {
	benchmarkStrategy(b, &InlineExecution{})
}

// benchmarkSinglePartition runs a single-partition simulation, where the
// per-step goroutine spawn and channel round-trip are pure overhead. This is
// the case InlineExecution targets: comparing it against the default strategy
// shows the handover cost removed.
func benchmarkSinglePartition(b *testing.B, strategy ExecutionStrategy) {
	settings, makeIterations := independentSettings(1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runStrategyConfig(settings, makeIterations, 1000, strategy)
	}
}

func BenchmarkSinglePartitionSpawnPerStep(b *testing.B) {
	benchmarkSinglePartition(b, &SpawnPerStepExecution{})
}

func BenchmarkSinglePartitionInline(b *testing.B) {
	benchmarkSinglePartition(b, &InlineExecution{})
}

// TestPersistentWorkerExecutionLongRun exercises worker setup/teardown over
// many steps. Combined with `go test -race`, this guards against worker
// goroutine leaks and races in the two-phase barrier.
func TestPersistentWorkerExecutionLongRun(t *testing.T) {
	settings, makeIterations := independentSettings(4)
	reference := runStrategyConfig(settings, makeIterations, 200, nil)
	got := runStrategyConfig(
		settings, makeIterations, 200, &PersistentWorkerExecution{})
	assertStoresEqual(t, reference, got, "persistent_worker_long_run")
}
