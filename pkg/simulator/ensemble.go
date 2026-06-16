package simulator

import (
	"runtime"
	"sync"
)

// EnsembleRun pairs the seed used for a single ensemble member with the data
// recorded from running it.
type EnsembleRun struct {
	Seed    uint64
	Storage *StateTimeStorage
}

// RunSeededEnsemble launches one independent PartitionCoordinator per seed and
// runs them concurrently, varying the global seed applied to each via the
// ConfigGenerator. It returns one EnsembleRun per seed, index-aligned to the
// seeds slice.
//
// This is the data-parallel ensemble expressed through the existing
// abstractions: every member is a whole, independent coordinator with its own
// histories, timesteps and termination, advanced by today's unmodified
// execution. There is therefore no shared-state coupling between members and
// no simulator invariant is weakened — in contrast to trying to parallelise
// partitions within a single coordinator.
//
// The build closure MUST construct a fresh ConfigGenerator (and therefore
// fresh Iteration instances) on every call. This is load-bearing:
// ConfigGenerator.GenerateConfigs hands back the same Iteration pointers it was
// given and reconfigures them in place, so two members sharing one generator
// would share mutable iteration state (RNGs, buffers) and race. Building anew
// per member guarantees isolation.
//
// Each member's OutputFunction is replaced with a fresh StateTimeStorage sink
// so its trajectory is captured into the returned EnsembleRun; the member's
// OutputCondition (and every other part of its SimulationConfig, including any
// ExecutionStrategy such as PersistentWorkerExecution) is respected.
//
// maxConcurrency bounds how many members run at once; values <= 0 default to
// runtime.GOMAXPROCS(0). Results are deterministic: re-running with the same
// seeds yields identical per-member output regardless of maxConcurrency.
func RunSeededEnsemble(
	build func() *ConfigGenerator,
	seeds []uint64,
	maxConcurrency int,
) []EnsembleRun {
	if maxConcurrency <= 0 {
		maxConcurrency = runtime.GOMAXPROCS(0)
	}

	results := make([]EnsembleRun, len(seeds))
	semaphore := make(chan struct{}, maxConcurrency)
	var waitGroup sync.WaitGroup

	for runIndex, seed := range seeds {
		waitGroup.Add(1)
		semaphore <- struct{}{}
		go func(runIndex int, seed uint64) {
			defer waitGroup.Done()
			defer func() { <-semaphore }()
			results[runIndex] = EnsembleRun{
				Seed:    seed,
				Storage: runSeededMember(build, seed),
			}
		}(runIndex, seed)
	}

	waitGroup.Wait()
	return results
}

// runSeededMember builds one ensemble member, applies the seed, runs it to
// termination and returns its recorded output.
func runSeededMember(
	build func() *ConfigGenerator,
	seed uint64,
) *StateTimeStorage {
	generator := build()
	generator.SetGlobalSeed(seed)
	settings, implementations := generator.GenerateConfigs()
	storage := NewStateTimeStorage()
	implementations.OutputFunction = &StateTimeStorageOutputFunction{
		Store: storage,
	}
	coordinator := NewPartitionCoordinator(settings, implementations)
	coordinator.Run()
	return storage
}
