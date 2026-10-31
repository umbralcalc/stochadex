package macros

import (
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/mat"
)

// mat1 is a one-row history of zeros, width wide.
func mat1(width int) *mat.Dense { return mat.NewDense(1, width, nil) }

func TestVectorCumulativeMeanPartition(t *testing.T) {
	// A ramp whose initial row stands apart from its later ones.
	storage := xySeriesToStorage(20, -40, 9,
		&general.ValuesFunctionIteration{Function: func(
			_ *simulator.Params, _ int, _ []*simulator.StateHistory,
			th *simulator.CumulativeTimestepsHistory,
		) []float64 {
			return []float64{timeFromTimesteps(th)}
		}},
		&general.ValuesFunctionIteration{Function: func(
			_ *simulator.Params, _ int, _ []*simulator.StateHistory,
			th *simulator.CumulativeTimestepsHistory,
		) []float64 {
			return []float64{2 * timeFromTimesteps(th)}
		}},
	)

	t.Run("its state is the mean of every row so far, the initial one included", func(t *testing.T) {
		partition := NewVectorCumulativeMeanPartition(AppliedAggregation{
			Name: "mean",
			Data: analysis.DataRef{PartitionName: "x"},
		}, storage)
		means := analysis.AddPartitionsToStateTimeStorage(
			storage, []*simulator.PartitionConfig{partition}, nil).GetValues("mean")
		xs := storage.GetValues("x")
		sum := 0.0
		for row := range xs {
			sum += xs[row][0]
			if want := sum / float64(row+1); !closeEnough(means[row][0], want, 1e-12) {
				t.Fatalf("row %d: mean %v, want %v", row, means[row][0], want)
			}
		}
	})

	t.Run("it reads only the value indices it is given", func(t *testing.T) {
		wide := analysis.NewStateTimeStorageFromPartitions(
			[]*simulator.PartitionConfig{{
				Name: "xy",
				Iteration: &general.ValuesFunctionIteration{Function: func(
					_ *simulator.Params, _ int, _ []*simulator.StateHistory,
					th *simulator.CumulativeTimestepsHistory,
				) []float64 {
					return []float64{timeFromTimesteps(th), -3 * timeFromTimesteps(th)}
				}},
				Params:            simulator.NewParams(nil),
				InitStateValues:   []float64{-40, 9},
				StateHistoryDepth: 1,
			}},
			&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 20},
			&simulator.ConstantTimestepFunction{Stepsize: 1.0},
			0.0,
		)
		partition := NewVectorCumulativeMeanPartition(AppliedAggregation{
			Name: "mean",
			Data: analysis.DataRef{PartitionName: "xy", ValueIndices: []int{1}},
		}, wide)
		if len(partition.InitStateValues) != 1 {
			t.Fatalf("a mean over one value index has width %d", len(partition.InitStateValues))
		}
		means := analysis.AddPartitionsToStateTimeStorage(
			wide, []*simulator.PartitionConfig{partition}, nil).GetValues("mean")
		var column []float64
		for _, row := range wide.GetValues("xy") {
			column = append(column, row[1])
		}
		if got, want := means[len(means)-1][0], floats.Sum(column)/float64(len(column)); !closeEnough(got, want, 1e-12) {
			t.Fatalf("final mean of the second value %v, want %v", got, want)
		}
	})
}

func TestScalarRegressionInitState(t *testing.T) {
	// The initial state is one step of the iteration from no pairs, in each
	// layout the options give.
	for _, c := range []struct {
		name      string
		intercept bool
		mode      RegressionStatsMode
		window    int
	}{
		{"cumulative through the origin", false, RegressionStatsCumulative, 0},
		{"cumulative with an intercept", true, RegressionStatsCumulative, 0},
		{"window through the origin", false, RegressionStatsWindow, 3},
		{"window with an intercept", true, RegressionStatsWindow, 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := ScalarRegressionInitState(c.intercept, c.mode, c.window, 1e-12, 4, -6)
			width := ScalarRegressionStateWidth(c.intercept, c.mode, c.window)
			iteration := &ScalarRegressionStatsIteration{}
			iteration.Configure(0, &simulator.Settings{Iterations: []simulator.IterationSettings{{
				Params: simulator.NewParams(map[string][]float64{
					scalarRegressionInterceptKey:    {map[bool]float64{false: 0, true: 1}[c.intercept]},
					scalarRegressionModeKey:         {float64(c.mode)},
					scalarRegressionWindowLengthKey: {float64(c.window)},
				}),
				StateWidth: width,
			}}})
			history := &simulator.StateHistory{
				Values:            mat1(width),
				StateWidth:        width,
				StateHistoryDepth: 1,
			}
			params := simulator.NewParams(map[string][]float64{
				ScalarRegressionParamX: {4}, ScalarRegressionParamY: {-6}})
			want := iteration.Iterate(&params, 0, []*simulator.StateHistory{history}, nil)
			if !floats.Equal(got, want) {
				t.Fatalf("initial state %v, want one step from no pairs %v", got, want)
			}
		})
	}
	t.Run("a zero min_denominator takes the iteration's default", func(t *testing.T) {
		// Sxx = 1e-14 is under the default 1e-12, so the slope is left at 0;
		// a min_denominator of 0 taken as given would divide by it.
		state := ScalarRegressionInitState(false, RegressionStatsCumulative, 0, 0, 1e-7, 1)
		if beta := state[4]; beta != 0 {
			t.Fatalf("beta = %v, want 0 under the default min_denominator", beta)
		}
	})
}
