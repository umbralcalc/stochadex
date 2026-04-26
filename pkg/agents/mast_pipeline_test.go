package agents_test

// Integration test: the full three-partition MAST inner pipeline
// (MCTSTreePartition + MASTRolloutPartition + MASTAggregationPartition)
// running against tic-tac-toe. Verifies (a) the agg row accumulates
// observations as rollouts complete and (b) the search still finds the
// winning move from a forcing position.

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/agents/agentstest"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// buildMASTPipeline wires the three partitions into a ConfigGenerator,
// returning the generator ready to GenerateConfigs.
func buildMASTPipeline(rootInit agentstest.TTTState, sims int, seed uint64) *simulator.ConfigGenerator {
	const W = agentstest.TTTWidth
	const K = 9
	const P = 2
	const MaxPath = 9

	tree := &agents.MCTSTreePartition[agentstest.TTTState, agentstest.TTTAction]{
		Env:             &agentstest.TTTGame{},
		Decoder:         agentstest.TTTDecode,
		Encoder:         agentstest.TTTEncode,
		MaxLegalActions: K,
		StateWidth:      W,
		Players:         P,
	}
	rollout := &agents.MASTRolloutPartition[agentstest.TTTState, agentstest.TTTAction]{
		Env:      &agentstest.TTTGame{},
		Cfg:      agents.MCTSConfig[agentstest.TTTState, agentstest.TTTAction]{RolloutMaxSteps: 30},
		Decoder:  agentstest.TTTDecode,
		KeyToIdx: tttKeyToIdx,
		MaxKeys:  K,
		MaxPath:  MaxPath,
		Players:  P,
	}
	agg := &agents.MASTAggregationPartition[agentstest.TTTAction]{MaxKeys: K}

	gen := simulator.NewConfigGenerator()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: sims},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})

	treeInit := make([]float64, agents.MCTSTreeRowWidth(W, K))
	copy(treeInit[agents.MCTSTreeRowLeafStateOffset:], agentstest.TTTEncode(rootInit))

	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "tree",
		Iteration: tree,
		ParamsAsPartitions: map[string][]string{
			agents.MCTSTreeParamRolloutScoresPartition: {"rollout"},
		},
		InitStateValues:   treeInit,
		StateHistoryDepth: 1,
		Seed:              seed,
	})

	leafIndices := make([]int, 0, W+1)
	for i := 0; i < W; i++ {
		leafIndices = append(leafIndices, agents.MCTSTreeRowLeafStateOffset+i)
	}
	leafIndices = append(leafIndices, agents.MCTSTreeRowHasLeafOffset(W))
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "rollout",
		Iteration: rollout,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			agents.MCTSRolloutParamLeaf: {Upstream: "tree", Indices: leafIndices},
		},
		ParamsAsPartitions: map[string][]string{
			agents.MASTAggregationParamPartition: {"agg"},
		},
		InitStateValues:   make([]float64, agents.MASTRolloutRowWidth(P, MaxPath)),
		StateHistoryDepth: 1,
		Seed:              seed ^ 1,
	})

	pathStart := agents.MASTRolloutNumPathOffset(P)
	pathLen := 1 + 2*MaxPath
	updateIndices := make([]int, pathLen)
	for i := 0; i < pathLen; i++ {
		updateIndices[i] = pathStart + i
	}
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "agg",
		Iteration: agg,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			agents.MASTAggregationParamUpdates: {Upstream: "rollout", Indices: updateIndices},
		},
		InitStateValues:   make([]float64, agents.MASTAggregationRowWidth(K)),
		StateHistoryDepth: 1,
		Seed:              seed ^ 2,
	})
	return gen
}

func TestMASTPipelineAccumulatesAggregates(t *testing.T) {
	gen := buildMASTPipeline(agentstest.TTTState{}, 100, 1234)
	store := simulator.NewStateTimeStorage()
	gen.GetSimulation().OutputCondition = &simulator.EveryStepOutputCondition{}
	gen.GetSimulation().OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}

	settings, impl := gen.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, impl).Run()

	aggRows := store.GetValues("agg")
	last := aggRows[len(aggRows)-1]
	totalCount := 0.0
	for k := 0; k < 9; k++ {
		totalCount += last[agents.MASTAggregationCountSlot(k)]
	}
	if totalCount == 0 {
		t.Fatal("expected non-zero total observations after 100 sims")
	}
	observedKeys := 0
	for k := 0; k < 9; k++ {
		if last[agents.MASTAggregationCountSlot(k)] > 0 {
			observedKeys++
		}
	}
	if observedKeys < 5 {
		t.Fatalf("expected MAST to observe at least 5 distinct keys; got %d", observedKeys)
	}
}

func TestMASTPipelineFindsWinningMove(t *testing.T) {
	root := agentstest.TTTFromGrid([9]int8{1, 1, 0, 0, 2, 0, 0, 2, 0}, 0)
	gen := buildMASTPipeline(root, 200, 9999)
	settings, impl := gen.GenerateConfigs()
	coord := simulator.NewPartitionCoordinator(settings, impl)
	coord.Run()

	treeIter := impl.Iterations[0].(*agents.MCTSTreePartition[agentstest.TTTState, agentstest.TTTAction])
	bestI, ok := treeIter.MCTSTree().RootBestLegalIdx()
	if !ok {
		t.Fatal("RootBestLegalIdx returned ok=false after sims")
	}
	leg := (&agentstest.TTTGame{}).Legal(root)
	if leg[bestI] != agentstest.TTTAction(2) {
		t.Fatalf("MAST-driven MCTS missed winning move; got legal[%d]=%d", bestI, leg[bestI])
	}
}

func TestMASTPipelineRunsWithHarnesses(t *testing.T) {
	gen := buildMASTPipeline(agentstest.TTTState{}, 20, 11)
	settings, impl := gen.GenerateConfigs()
	if err := simulator.RunWithHarnesses(settings, impl); err != nil {
		t.Fatalf("RunWithHarnesses: %v", err)
	}
}
