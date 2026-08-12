package api

import (
	"reflect"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// yamlRows renders a [][]float64 the way YAML decoding hands it to the registry:
// a []interface{} of []interface{} of numbers. Using it (rather than passing the
// [][]float64 straight through) exercises the same floatMatrix path a real config
// takes.
func yamlRows(rows [][]float64) []interface{} {
	out := make([]interface{}, len(rows))
	for i, row := range rows {
		lane := make([]interface{}, len(row))
		for j, v := range row {
			lane[j] = v
		}
		out[i] = lane
	}
	return out
}

// TestFromStorageReplayBehaviour pins the replay contract of the from_storage
// data-spec iteration and its timestep function *as run through the config path* —
// that step n emits row n, that the offset shifts the replay, that a downstream
// partition sees the driver within-step, and that the timestep function replays
// the clock. The resolution tests in registry_test.go prove it builds; these prove
// it does the right thing when run. This is the behaviour
// cfg/example_from_storage_config.yaml depends on and the semantics the closed gap
// (solar-fleet/STOCHADEX_GAPS.md) needs.
func TestFromStorageReplayBehaviour(t *testing.T) {
	// A two-lane series, so a width/lane mis-wiring would surface rather than hide
	// behind a scalar.
	rows := [][]float64{{1, -1}, {2, -2}, {3, -3}, {4, -4}, {5, -5}, {6, -6}}
	fromStorage := func(fields map[string]interface{}) simulator.Iteration {
		it, err := ResolveIteration(simulator.ComponentSpec{Type: "from_storage", Fields: fields})
		if err != nil {
			t.Fatalf("resolve from_storage: %v", err)
		}
		return it
	}

	t.Run("emits row n at step n, seeded by row 0", func(t *testing.T) {
		it := fromStorage(map[string]interface{}{"data": yamlRows(rows)})
		// init row + 4 steps = rows[0..4], each identical to the input series.
		got := runOnePartition(it, map[string][]float64{}, rows[0], 0, 4)
		want := rows[:5]
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("replayed series = %v, want %v", got, want)
		}
	})

	t.Run("init_steps_taken shifts where the replay begins", func(t *testing.T) {
		it := fromStorage(map[string]interface{}{
			"data": yamlRows(rows), "init_steps_taken": 2,
		})
		// step s reads rows[s+2]; row 0 is the init, which we seed with rows[2].
		got := runOnePartition(it, map[string][]float64{}, rows[2], 0, 3)
		want := [][]float64{rows[2], rows[3], rows[4], rows[5]}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("offset replay = %v, want %v", got, want)
		}
	})

	t.Run("drives a downstream partition within-step", func(t *testing.T) {
		driver := fromStorage(map[string]interface{}{"data": yamlRows(rows)})
		site := &general.ExpressionIteration{
			Fields:  []general.ExpressionField{{Name: "y", Width: 2}},
			Outputs: []string{"2 * irradiance"},
		}
		storage := analysis.NewStateTimeStorageFromPartitions(
			[]*simulator.PartitionConfig{
				{
					Name:            "driver",
					Iteration:       driver,
					Params:          simulator.NewParams(map[string][]float64{}),
					InitStateValues: rows[0], StateHistoryDepth: 1, Seed: 0,
				},
				{
					Name:      "site",
					Iteration: site,
					Params:    simulator.NewParams(map[string][]float64{"irradiance": {0, 0}}),
					ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
						"irradiance": {Upstream: "driver"},
					},
					InitStateValues: []float64{0, 0}, StateHistoryDepth: 1, Seed: 0,
				},
			},
			&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 4},
			&simulator.ConstantTimestepFunction{Stepsize: 1.0}, 0.0,
		)
		driverOut := storage.GetValues("driver")
		siteOut := storage.GetValues("site")
		// params_from_upstream is a within-step read: at step s the site sees the
		// driver's this-step row, so site[s] == 2 * driver[s] for every stepped row.
		for s := 1; s < len(siteOut); s++ {
			for lane := range siteOut[s] {
				if want := 2 * driverOut[s][lane]; siteOut[s][lane] != want {
					t.Fatalf("step %d lane %d: site %v != 2*driver %v",
						s, lane, siteOut[s][lane], want)
				}
			}
		}
		// Guard against a vacuous pass: the driver must actually move.
		if reflect.DeepEqual(driverOut[1], driverOut[2]) {
			t.Fatal("driver never varied — the coupling check would be vacuous")
		}
	})

	t.Run("timestep function replays the clock", func(t *testing.T) {
		times := []float64{0, 2, 5, 9, 14, 20}
		tf, err := simulator.ResolveTimestepFunction(simulator.ComponentSpec{
			Type:   "from_storage",
			Fields: map[string]interface{}{"data": []interface{}{0.0, 2.0, 5.0, 9.0, 14.0, 20.0}},
		})
		if err != nil {
			t.Fatalf("resolve timestep from_storage: %v", err)
		}
		storage := analysis.NewStateTimeStorageFromPartitions(
			[]*simulator.PartitionConfig{{
				Name:            "p",
				Iteration:       &general.ConstantValuesIteration{},
				Params:          simulator.NewParams(map[string][]float64{}),
				InitStateValues: []float64{0}, StateHistoryDepth: 1, Seed: 0,
			}},
			&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 3},
			tf, times[0],
		)
		got := storage.GetTimes()
		want := times[:4] // init time + 3 stepped times, read straight from the series.
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("replayed clock = %v, want %v", got, want)
		}
	})
}
