package simulator

import (
	"math/rand/v2"
	"strconv"
	"testing"

	"gonum.org/v1/gonum/floats"
)

// seededRandomWalkIteration is a seed-sensitive test iteration: each step adds
// a standard-normal increment to every state value, drawn from an RNG seeded
// from the partition's Seed. This makes its output depend on the global seed
// applied per ensemble member, while remaining fully re-initialisable from
// Configure (so it passes the statefulness-residue harness).
type seededRandomWalkIteration struct {
	rng *rand.Rand
}

func (s *seededRandomWalkIteration) Configure(
	partitionIndex int,
	settings *Settings,
) {
	seed := settings.Iterations[partitionIndex].Seed
	s.rng = rand.New(rand.NewPCG(seed, seed))
}

func (s *seededRandomWalkIteration) Iterate(
	params *Params,
	partitionIndex int,
	stateHistories []*StateHistory,
	timestepsHistory *CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) + s.rng.NormFloat64()
	}
	return values
}

// ensembleBuilder returns a closure that constructs a fresh ConfigGenerator
// (and fresh Iteration instances) on every call — the isolation contract that
// RunSeededEnsemble depends on. Each member is a small bank of independent
// seeded random walks, so output is sensitive to the global seed applied per
// member.
func ensembleBuilder(numPartitions, steps int) func() *ConfigGenerator {
	return func() *ConfigGenerator {
		generator := NewConfigGenerator()
		generator.SetSimulation(&SimulationConfig{
			OutputCondition: &EveryStepOutputCondition{},
			OutputFunction:  &NilOutputFunction{},
			TerminationCondition: &NumberOfStepsTerminationCondition{
				MaxNumberOfSteps: steps,
			},
			TimestepFunction: &ConstantTimestepFunction{Stepsize: 0.1},
			InitTimeValue:    0.0,
		})
		for i := 0; i < numPartitions; i++ {
			generator.SetPartition(&PartitionConfig{
				Name:              "walk_" + strconv.Itoa(i),
				Iteration:         &seededRandomWalkIteration{},
				Params:            NewParams(make(map[string][]float64)),
				InitStateValues:   []float64{0.0, 0.0},
				StateHistoryDepth: 2,
			})
		}
		return generator
	}
}

func storageDiffers(a, b *StateTimeStorage) bool {
	for _, name := range a.GetNames() {
		aValues := a.GetValues(name)
		bValues := b.GetValues(name)
		for tIndex := range aValues {
			if !floats.Equal(aValues[tIndex], bValues[tIndex]) {
				return true
			}
		}
	}
	return false
}

func TestRunSeededEnsemble(t *testing.T) {
	build := ensembleBuilder(3, 25)
	seeds := []uint64{11, 22, 33, 44}

	t.Run("results are index-aligned to seeds", func(t *testing.T) {
		runs := RunSeededEnsemble(build, seeds, 4)
		if len(runs) != len(seeds) {
			t.Fatalf("got %d runs, want %d", len(runs), len(seeds))
		}
		for i, run := range runs {
			if run.Seed != seeds[i] {
				t.Errorf("run %d seed = %d, want %d", i, run.Seed, seeds[i])
			}
			if run.Storage == nil {
				t.Errorf("run %d has nil storage", i)
			}
		}
	})

	t.Run("output is deterministic across repeat runs", func(t *testing.T) {
		first := RunSeededEnsemble(build, seeds, 4)
		second := RunSeededEnsemble(build, seeds, 4)
		for i := range seeds {
			assertStoresEqual(t, first[i].Storage, second[i].Storage,
				"repeat-run-"+strconv.Itoa(i))
		}
	})

	t.Run("output is independent of maxConcurrency", func(t *testing.T) {
		serial := RunSeededEnsemble(build, seeds, 1)
		parallel := RunSeededEnsemble(build, seeds, 8)
		for i := range seeds {
			assertStoresEqual(t, serial[i].Storage, parallel[i].Storage,
				"concurrency-"+strconv.Itoa(i))
		}
	})

	t.Run("each member equals a standalone seeded run", func(t *testing.T) {
		// This proves the build-closure isolation works: running every member
		// concurrently yields exactly what building and running each one
		// independently would. Combined with -race, it shows no mutable
		// iteration state is shared between members.
		runs := RunSeededEnsemble(build, seeds, len(seeds))
		for i, seed := range seeds {
			generator := build()
			generator.SetGlobalSeed(seed)
			settings, implementations := generator.GenerateConfigs()
			store := NewStateTimeStorage()
			implementations.OutputFunction =
				&StateTimeStorageOutputFunction{Store: store}
			NewPartitionCoordinator(settings, implementations).Run()
			assertStoresEqual(t, store, runs[i].Storage,
				"standalone-"+strconv.Itoa(i))
		}
	})

	t.Run("different seeds give different trajectories", func(t *testing.T) {
		runs := RunSeededEnsemble(build, []uint64{1, 2}, 2)
		if !storageDiffers(runs[0].Storage, runs[1].Storage) {
			t.Error("distinct seeds produced identical output")
		}
	})

	t.Run("composes with PersistentWorkerExecution", func(t *testing.T) {
		// Members built with a persistent-worker strategy must produce the same
		// output as the default execution: the ensemble runner respects the
		// member's SimulationConfig, including its ExecutionStrategy.
		defaultRuns := RunSeededEnsemble(build, seeds, 4)
		workerBuild := func() *ConfigGenerator {
			generator := build()
			sim := generator.GetSimulation()
			sim.ExecutionStrategy = &PersistentWorkerExecution{}
			generator.SetSimulation(sim)
			return generator
		}
		workerRuns := RunSeededEnsemble(workerBuild, seeds, 4)
		for i := range seeds {
			assertStoresEqual(t, defaultRuns[i].Storage, workerRuns[i].Storage,
				"worker-compose-"+strconv.Itoa(i))
		}
	})
}

// TestRunSeededEnsembleMemberHarness validates the ensemble member fixture
// against the standard correctness/statefulness harness, per the testing
// convention.
func TestRunSeededEnsembleMemberHarness(t *testing.T) {
	generator := ensembleBuilder(3, 25)()
	generator.SetGlobalSeed(99)
	settings, implementations := generator.GenerateConfigs()
	if err := RunWithHarnesses(settings, implementations); err != nil {
		t.Fatalf("RunWithHarnesses: %v", err)
	}
}
