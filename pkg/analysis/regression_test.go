package analysis

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
)

func timeFromTimesteps(
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) float64 {
	return timestepsHistory.Values.AtVec(0) + timestepsHistory.NextIncrement
}

func closeEnough(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func batchThroughOrigin(xs, ys []float64) (beta, sigma2 float64) {
	n := float64(len(xs))
	if n < 1 {
		return 0, 0
	}
	var sxx, sxy, syy float64
	for i := range xs {
		sxx += xs[i] * xs[i]
		sxy += xs[i] * ys[i]
		syy += ys[i] * ys[i]
	}
	if sxx <= 1e-12 {
		return 0, 0
	}
	beta = sxy / sxx
	if n < 2 {
		return beta, 0
	}
	sse := syy - sxy*sxy/sxx
	if sse < 0 && sse > -1e-9*math.Max(1, syy) {
		sse = 0
	}
	return beta, sse / (n - 1)
}

func batchIntercept(xs, ys []float64) (alpha, beta, sigma2 float64) {
	n := float64(len(xs))
	if n < 1 {
		return 0, 0, 0
	}
	var sx, sy, sxx, sxy, syy float64
	for i := range xs {
		sx += xs[i]
		sy += ys[i]
		sxx += xs[i] * xs[i]
		sxy += xs[i] * ys[i]
		syy += ys[i] * ys[i]
	}
	D := n*sxx - sx*sx
	if math.Abs(D) <= 1e-12 || n < 2 {
		return sy / n, 0, 0
	}
	beta = (n*sxy - sx*sy) / D
	alpha = (sy - beta*sx) / n
	if n < 3 {
		return alpha, beta, 0
	}
	sse := syy - alpha*sy - beta*sxy
	if sse < 0 && sse > -1e-9*math.Max(1, syy) {
		sse = 0
	}
	return alpha, beta, sse / (n - 2)
}

// xySeriesToStorage runs two scalar partitions for nSteps and records them in storage.
func xySeriesToStorage(
	nSteps int,
	xInit, yInit float64,
	iterX, iterY simulator.Iteration,
) *simulator.StateTimeStorage {
	return NewStateTimeStorageFromPartitions(
		[]*simulator.PartitionConfig{
			{
				Name:              "x",
				Iteration:         iterX,
				Params:            simulator.NewParams(nil),
				InitStateValues:   []float64{xInit},
				StateHistoryDepth: 1,
				Seed:              0,
			},
			{
				Name:              "y",
				Iteration:         iterY,
				Params:            simulator.NewParams(nil),
				InitStateValues:   []float64{yInit},
				StateHistoryDepth: 1,
				Seed:              0,
			},
		},
		&simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: nSteps},
		&simulator.ConstantTimestepFunction{Stepsize: 1.0},
		0.0,
	)
}

func TestScalarRegressionStatsIterationHarness(t *testing.T) {
	settings := simulator.LoadSettingsFromYaml("./regression_stats_settings.yaml")
	iterX := &general.ValuesFunctionIteration{
		Function: func(
			_ *simulator.Params,
			_ int,
			_ []*simulator.StateHistory,
			th *simulator.CumulativeTimestepsHistory,
		) []float64 {
			return []float64{timeFromTimesteps(th)}
		},
	}
	iterY := &general.ValuesFunctionIteration{
		Function: func(
			_ *simulator.Params,
			_ int,
			_ []*simulator.StateHistory,
			th *simulator.CumulativeTimestepsHistory,
		) []float64 {
			return []float64{2.0 * timeFromTimesteps(th)}
		},
	}
	reg := &ScalarRegressionStatsIteration{}
	iterX.Configure(0, settings)
	iterY.Configure(1, settings)
	reg.Configure(2, settings)
	implementations := &simulator.Implementations{
		Iterations:           []simulator.Iteration{iterX, iterY, reg},
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 100},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
	}
	if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
		t.Fatal(err)
	}
}

func TestScalarRegressionStatsStorage(t *testing.T) {
	t.Run("cumulative through-origin vs batch OLS", func(t *testing.T) {
		const nStep = 50
		xs := make([]float64, nStep)
		ys := make([]float64, nStep)
		for i := range xs {
			tv := float64(i + 1)
			xs[i] = tv
			ys[i] = 2.5 * tv
		}
		bWant, s2Want := batchThroughOrigin(xs, ys)

		storage := xySeriesToStorage(nStep, 0, 0,
			&general.ValuesFunctionIteration{
				Function: func(
					_ *simulator.Params,
					_ int,
					_ []*simulator.StateHistory,
					th *simulator.CumulativeTimestepsHistory,
				) []float64 {
					return []float64{timeFromTimesteps(th)}
				},
			},
			&general.ValuesFunctionIteration{
				Function: func(
					_ *simulator.Params,
					_ int,
					_ []*simulator.StateHistory,
					th *simulator.CumulativeTimestepsHistory,
				) []float64 {
					return []float64{2.5 * timeFromTimesteps(th)}
				},
			},
		)
		reg := NewScalarRegressionStatsPartition(AppliedScalarRegressionStats{
			Name:      "ols",
			Y:         DataRef{PartitionName: "y"},
			X:         DataRef{PartitionName: "x"},
			Intercept: false,
			Mode:      RegressionStatsCumulative,
		}, storage)
		storage = AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{reg}, nil)
		vs := storage.GetValues("ols")
		last := vs[len(vs)-1]
		if floats.HasNaN(last) {
			t.Fatal("unexpected NaN in regression state")
		}
		if !closeEnough(last[4], bWant, 1e-9) {
			t.Errorf("beta got %v want %v", last[4], bWant)
		}
		if !closeEnough(last[5], s2Want, 1e-9) {
			t.Errorf("sigma^2 got %v want %v", last[5], s2Want)
		}
	})

	t.Run("zero Sxx yields finite outputs in storage replay", func(t *testing.T) {
		storage := xySeriesToStorage(5, 0, 1,
			&general.ConstantValuesIteration{},
			&general.ConstantValuesIteration{},
		)
		reg := NewScalarRegressionStatsPartition(AppliedScalarRegressionStats{
			Name:      "ols",
			Y:         DataRef{PartitionName: "y"},
			X:         DataRef{PartitionName: "x"},
			Intercept: false,
			Mode:      RegressionStatsCumulative,
		}, storage)
		storage = AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{reg}, nil)
		for _, row := range storage.GetValues("ols") {
			if floats.HasNaN(row) {
				t.Fatal("unexpected NaN when Sxx=0")
			}
		}
		last := storage.GetValues("ols")[len(storage.GetValues("ols"))-1]
		if !closeEnough(last[4], 0, 1e-15) || !closeEnough(last[5], 0, 1e-15) {
			t.Errorf("expected beta=sigma2=0, got beta=%v sigma2=%v", last[4], last[5])
		}
	})

	t.Run("sliding window through-origin vs batch on last W", func(t *testing.T) {
		const W = 4
		const total = 20
		xs := make([]float64, total)
		ys := make([]float64, total)
		for i := range xs {
			tv := float64(i + 1)
			xs[i] = tv
			if i < 10 {
				ys[i] = 2 * tv
			} else {
				ys[i] = -0.5 * tv
			}
		}
		bWant, s2Want := batchThroughOrigin(xs[total-W:], ys[total-W:])
		base := 2*W + 1

		storage := xySeriesToStorage(total, 0, 0,
			&general.ValuesFunctionIteration{
				Function: func(
					_ *simulator.Params,
					_ int,
					_ []*simulator.StateHistory,
					th *simulator.CumulativeTimestepsHistory,
				) []float64 {
					return []float64{timeFromTimesteps(th)}
				},
			},
			&general.ValuesFunctionIteration{
				Function: func(
					_ *simulator.Params,
					_ int,
					_ []*simulator.StateHistory,
					th *simulator.CumulativeTimestepsHistory,
				) []float64 {
					tv := timeFromTimesteps(th)
					if tv <= 10 {
						return []float64{2 * tv}
					}
					return []float64{-0.5 * tv}
				},
			},
		)
		reg := NewScalarRegressionStatsPartition(AppliedScalarRegressionStats{
			Name:         "ols",
			Y:            DataRef{PartitionName: "y"},
			X:            DataRef{PartitionName: "x"},
			Intercept:    false,
			Mode:         RegressionStatsWindow,
			WindowLength: W,
		}, storage)
		storage = AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{reg}, nil)
		last := storage.GetValues("ols")[len(storage.GetValues("ols"))-1]
		if !closeEnough(last[base+4], bWant, 1e-9) {
			t.Errorf("window beta got %v want %v", last[base+4], bWant)
		}
		if !closeEnough(last[base+5], s2Want, 1e-9) {
			t.Errorf("window sigma^2 got %v want %v", last[base+5], s2Want)
		}
	})

	t.Run("cumulative intercept vs batch", func(t *testing.T) {
		const nStep = 30
		xs := make([]float64, nStep)
		ys := make([]float64, nStep)
		for i := range xs {
			tv := float64(i + 1)
			xs[i] = tv
			ys[i] = 1.0 + 0.25*tv
		}
		aWant, bWant, s2Want := batchIntercept(xs, ys)

		storage := xySeriesToStorage(nStep, 0, 0,
			&general.ValuesFunctionIteration{
				Function: func(
					_ *simulator.Params,
					_ int,
					_ []*simulator.StateHistory,
					th *simulator.CumulativeTimestepsHistory,
				) []float64 {
					return []float64{timeFromTimesteps(th)}
				},
			},
			&general.ValuesFunctionIteration{
				Function: func(
					_ *simulator.Params,
					_ int,
					_ []*simulator.StateHistory,
					th *simulator.CumulativeTimestepsHistory,
				) []float64 {
					tv := timeFromTimesteps(th)
					return []float64{1.0 + 0.25*tv}
				},
			},
		)
		reg := NewScalarRegressionStatsPartition(AppliedScalarRegressionStats{
			Name:      "ols",
			Y:         DataRef{PartitionName: "y"},
			X:         DataRef{PartitionName: "x"},
			Intercept: true,
			Mode:      RegressionStatsCumulative,
		}, storage)
		storage = AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{reg}, nil)
		last := storage.GetValues("ols")[len(storage.GetValues("ols"))-1]
		if !closeEnough(last[6], aWant, 1e-9) {
			t.Errorf("alpha got %v want %v", last[6], aWant)
		}
		if !closeEnough(last[7], bWant, 1e-9) {
			t.Errorf("beta got %v want %v", last[7], bWant)
		}
		if !closeEnough(last[8], s2Want, 1e-9) {
			t.Errorf("sigma^2 got %v want %v", last[8], s2Want)
		}
	})

	t.Run("sliding window intercept vs batch on last W", func(t *testing.T) {
		const W = 3
		const nStep = 25
		xs := make([]float64, nStep)
		ys := make([]float64, nStep)
		for i := range xs {
			tv := float64(i + 1)
			xs[i] = tv
			ys[i] = 0.5 + 1.5*tv
		}
		aWant, bWant, s2Want := batchIntercept(xs[nStep-W:], ys[nStep-W:])
		base := 2*W + 1

		storage := xySeriesToStorage(nStep, 0, 0,
			&general.ValuesFunctionIteration{
				Function: func(
					_ *simulator.Params,
					_ int,
					_ []*simulator.StateHistory,
					th *simulator.CumulativeTimestepsHistory,
				) []float64 {
					return []float64{timeFromTimesteps(th)}
				},
			},
			&general.ValuesFunctionIteration{
				Function: func(
					_ *simulator.Params,
					_ int,
					_ []*simulator.StateHistory,
					th *simulator.CumulativeTimestepsHistory,
				) []float64 {
					tv := timeFromTimesteps(th)
					return []float64{0.5 + 1.5*tv}
				},
			},
		)
		reg := NewScalarRegressionStatsPartition(AppliedScalarRegressionStats{
			Name:         "ols",
			Y:            DataRef{PartitionName: "y"},
			X:            DataRef{PartitionName: "x"},
			Intercept:    true,
			Mode:         RegressionStatsWindow,
			WindowLength: W,
		}, storage)
		storage = AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{reg}, nil)
		last := storage.GetValues("ols")[len(storage.GetValues("ols"))-1]
		if !closeEnough(last[base+6], aWant, 1e-9) {
			t.Errorf("window alpha got %v want %v", last[base+6], aWant)
		}
		if !closeEnough(last[base+7], bWant, 1e-9) {
			t.Errorf("window beta got %v want %v", last[base+7], bWant)
		}
		if !closeEnough(last[base+8], s2Want, 1e-9) {
			t.Errorf("window sigma^2 got %v want %v", last[base+8], s2Want)
		}
	})
}
