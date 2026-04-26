package agents_test

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/agents/agentstest"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// newMASTAggregationImpls builds the [update_provider, agg] iteration
// pair that the YAML wires together for the standalone aggregation
// tests. The provider emits a constant 9-float batch (MaxPath=4,
// num_updates=3: key 1 +0.5, key 2 +1.0, key 1 +0.25; trailing pair
// unused).
func newMASTAggregationImpls() []simulator.Iteration {
	return []simulator.Iteration{
		&general.ConstantValuesIteration{},
		&agents.MASTAggregationIteration[agentstest.TTTAction]{MaxKeys: 4},
	}
}

func TestMASTAggregationIteration(t *testing.T) {
	t.Run(
		"test that the MAST aggregation partition runs",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./mast_aggregation_iteration_settings.yaml")
			iterations := newMASTAggregationImpls()
			for partitionIndex, iter := range iterations {
				iter.Configure(partitionIndex, settings)
			}
			store := simulator.NewStateTimeStorage()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.EveryStepOutputCondition{},
				OutputFunction:  &simulator.StateTimeStorageOutputFunction{Store: store},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 5,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			coordinator := simulator.NewPartitionCoordinator(settings, implementations)
			coordinator.Run()

			rows := store.GetValues("agg")
			if len(rows) < 2 {
				t.Fatalf("expected multiple rows, got %d", len(rows))
			}
			// Step 1: 3 updates absorbed (key 1 twice, key 2 once).
			//   counts = [0, 2, 1, 0]; sums = [0, 0.75, 1.0, 0]
			step1 := rows[1]
			if step1[agents.MASTAggregationCountSlot(1)] != 2 {
				t.Errorf("step 1 count[1]: got %v want 2", step1[agents.MASTAggregationCountSlot(1)])
			}
			if step1[agents.MASTAggregationSumSlot(1)] != 0.75 {
				t.Errorf("step 1 sum[1]: got %v want 0.75", step1[agents.MASTAggregationSumSlot(1)])
			}
			if step1[agents.MASTAggregationCountSlot(2)] != 1 {
				t.Errorf("step 1 count[2]: got %v want 1", step1[agents.MASTAggregationCountSlot(2)])
			}
			// Step 5: 5 steps absorbed identical batches; counts scale 5×.
			//   counts = [0, 10, 5, 0]; sums = [0, 3.75, 5.0, 0]
			lastStep := rows[len(rows)-1]
			if lastStep[agents.MASTAggregationCountSlot(1)] != 10 {
				t.Errorf("final count[1]: got %v want 10", lastStep[agents.MASTAggregationCountSlot(1)])
			}
			if lastStep[agents.MASTAggregationSumSlot(1)] != 3.75 {
				t.Errorf("final sum[1]: got %v want 3.75", lastStep[agents.MASTAggregationSumSlot(1)])
			}
			if lastStep[agents.MASTAggregationCountSlot(2)] != 5 {
				t.Errorf("final count[2]: got %v want 5", lastStep[agents.MASTAggregationCountSlot(2)])
			}
		},
	)
	t.Run(
		"test that the MAST aggregation partition runs with harnesses",
		func(t *testing.T) {
			settings := simulator.LoadSettingsFromYaml("./mast_aggregation_iteration_settings.yaml")
			iterations := newMASTAggregationImpls()
			implementations := &simulator.Implementations{
				Iterations:      iterations,
				OutputCondition: &simulator.NilOutputCondition{},
				OutputFunction:  &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
					MaxNumberOfSteps: 5,
				},
				TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: 1.0},
			}
			if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
				t.Errorf("test harness failed: %v", err)
			}
		},
	)
}

func TestMASTAggregationIterationRowLayout(t *testing.T) {
	if got, want := agents.MASTAggregationRowWidth(5), 10; got != want {
		t.Errorf("MASTAggregationRowWidth(5): got %d want %d", got, want)
	}
	if agents.MASTAggregationCountSlot(0) != 0 || agents.MASTAggregationSumSlot(0) != 1 {
		t.Errorf("slot 0 layout wrong")
	}
	if agents.MASTAggregationCountSlot(3) != 6 || agents.MASTAggregationSumSlot(3) != 7 {
		t.Errorf("slot 3 layout wrong: got count=%d sum=%d", agents.MASTAggregationCountSlot(3), agents.MASTAggregationSumSlot(3))
	}
}

func TestMASTAggregationIterationDropsOutOfRangeKeys(t *testing.T) {
	provider := &general.ConstantValuesIteration{}
	agg := &agents.MASTAggregationIteration[agentstest.TTTAction]{MaxKeys: 4}
	gen := simulator.NewConfigGenerator()
	store := simulator.NewStateTimeStorage()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: store},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 2},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "provider",
		Iteration:         provider,
		InitStateValues:   []float64{2, 99, 0.7, 1, 0.3}, // num=2, then (99, 0.7), (1, 0.3)
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "agg",
		Iteration: agg,
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			agents.MASTAggregationParamUpdates: {Upstream: "provider", Indices: []int{0, 1, 2, 3, 4}},
		},
		InitStateValues:   make([]float64, agents.MASTAggregationRowWidth(4)),
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	simulator.NewPartitionCoordinator(settings, impl).Run()

	last := store.GetValues("agg")[1]
	if last[agents.MASTAggregationCountSlot(1)] != 1 {
		t.Errorf("expected key 1 absorbed (count=1), got %v", last[agents.MASTAggregationCountSlot(1)])
	}
	if last[agents.MASTAggregationSumSlot(1)] != 0.3 {
		t.Errorf("expected key 1 sum=0.3, got %v", last[agents.MASTAggregationSumSlot(1)])
	}
	for k := 0; k < 4; k++ {
		if k == 1 {
			continue
		}
		if last[agents.MASTAggregationCountSlot(k)] != 0 {
			t.Errorf("expected key %d untouched, got count=%v", k, last[agents.MASTAggregationCountSlot(k)])
		}
	}
}

func TestMASTMeanForKey(t *testing.T) {
	row := make([]float64, agents.MASTAggregationRowWidth(4))
	row[agents.MASTAggregationCountSlot(2)] = 4
	row[agents.MASTAggregationSumSlot(2)] = 2.0
	mean, n := agents.MASTMeanForKey(row, 2)
	if n != 4 {
		t.Errorf("expected count 4, got %d", n)
	}
	if mean != 0.5 {
		t.Errorf("expected mean 0.5, got %v", mean)
	}
	mean, n = agents.MASTMeanForKey(row, 0)
	if mean != 0 || n != 0 {
		t.Errorf("expected (0,0) for unobserved key, got (%v, %d)", mean, n)
	}
	mean, n = agents.MASTMeanForKey(row, 99)
	if mean != 0 || n != 0 {
		t.Errorf("expected (0,0) for out-of-range key, got (%v, %d)", mean, n)
	}
}
