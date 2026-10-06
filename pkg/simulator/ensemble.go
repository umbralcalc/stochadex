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

// RunSeededEnsembleMembers is RunSeededEnsemble for members with their own
// outputs. build receives each member's index and seed, so it can give the
// member its own sinks (a log file per member, say); each member's storage is
// recorded alongside that output rather than replacing it, using the member's
// own output condition. The isolation contract is RunSeededEnsemble's: every
// build call must return fresh Iteration and output instances.
func RunSeededEnsembleMembers(
	build func(member int, seed uint64) *ConfigGenerator,
	seeds []uint64,
	maxConcurrency int,
) []EnsembleRun {
	if maxConcurrency <= 0 {
		maxConcurrency = runtime.GOMAXPROCS(0)
	}
	results := make([]EnsembleRun, len(seeds))
	semaphore := make(chan struct{}, maxConcurrency)
	var waitGroup sync.WaitGroup
	for member, seed := range seeds {
		waitGroup.Add(1)
		semaphore <- struct{}{}
		go func(member int, seed uint64) {
			defer waitGroup.Done()
			defer func() { <-semaphore }()
			results[member] = EnsembleRun{
				Seed:    seed,
				Storage: runMemberWithOutputs(build(member, seed), seed),
			}
		}(member, seed)
	}
	waitGroup.Wait()
	return results
}

// runMemberWithOutputs runs one member under its global seed, teeing its own
// output with an in-memory storage gated by the member's output condition.
func runMemberWithOutputs(generator *ConfigGenerator, seed uint64) *StateTimeStorage {
	generator.SetGlobalSeed(seed)
	settings, implementations := generator.GenerateConfigs()
	storage := NewStateTimeStorage()
	views := []OutputView{{
		Name:      "member",
		Condition: implementations.OutputCondition,
		Function:  &StateTimeStorageOutputFunction{Store: storage},
	}}
	switch own := implementations.OutputFunction.(type) {
	case nil, *NilOutputFunction:
	case *OutputViews:
		views = append(views, own.Views...)
	default:
		views = append(views, OutputView{
			Name: "output", Condition: implementations.OutputCondition, Function: own})
	}
	implementations.OutputFunction = &OutputViews{Views: views}
	implementations.OutputCondition = &EveryStepOutputCondition{}
	NewPartitionCoordinator(settings, implementations).Run()
	return storage
}
