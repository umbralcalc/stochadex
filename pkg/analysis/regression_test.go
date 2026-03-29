package analysis

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
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

func TestScalarRegressionStatsIteration(t *testing.T) {
	t.Run("cumulative through-origin matches batch OLS", func(t *testing.T) {
		const nStep = 50
		xs := make([]float64, nStep)
		ys := make([]float64, nStep)
		for i := range xs {
			tv := float64(i + 1)
			xs[i] = tv
			ys[i] = 2.5 * tv
		}
		bWant, s2Want := batchThroughOrigin(xs, ys)

		settings := &simulator.Settings{
			InitTimeValue:         0,
			TimestepsHistoryDepth: 1,
			Iterations: []simulator.IterationSettings{
				{
					Name: "x", Params: simulator.NewParams(map[string][]float64{}),
					InitStateValues: []float64{0}, StateWidth: 1,
					StateHistoryDepth: 1, Seed: 0,
				},
				{
					Name: "y", Params: simulator.NewParams(map[string][]float64{}),
					InitStateValues: []float64{0}, StateWidth: 1,
					StateHistoryDepth: 1, Seed: 0,
				},
				{
					Name: "reg", Params: simulator.NewParams(map[string][]float64{
						scalarRegressionInterceptKey:      {0},
						scalarRegressionModeKey:           {0},
						scalarRegressionWindowLengthKey:   {0},
						scalarRegressionMinDenominatorKey: {1e-12},
					}),
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"x": {Upstream: 0, Indices: []int{0}},
						"y": {Upstream: 1, Indices: []int{0}},
					},
					InitStateValues:   make([]float64, 6),
					StateWidth:        6,
					StateHistoryDepth: 1,
					Seed:              0,
				},
			},
		}
		settings.Init()

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
				return []float64{2.5 * timeFromTimesteps(th)}
			},
		}
		reg := &ScalarRegressionStatsIteration{}
		implementations := &simulator.Implementations{
			Iterations:           []simulator.Iteration{iterX, iterY, reg},
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: nStep},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		}
		iterX.Configure(0, settings)
		iterY.Configure(1, settings)
		reg.Configure(2, settings)

		coordinator := simulator.NewPartitionCoordinator(settings, implementations)
		coordinator.Run()
		last := coordinator.Shared.StateHistories[2].Values.RawRowView(0)
		if len(last) != 6 {
			t.Fatalf("state width %d", len(last))
		}
		if !closeEnough(last[4], bWant, 1e-9) {
			t.Errorf("beta got %v want %v", last[4], bWant)
		}
		if !closeEnough(last[5], s2Want, 1e-9) {
			t.Errorf("sigma^2 got %v want %v", last[5], s2Want)
		}
	})

	t.Run("RunWithHarnesses cumulative through-origin", func(t *testing.T) {
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
	})

	t.Run("zero denominator yields zero beta without NaN", func(t *testing.T) {
		settings := &simulator.Settings{
			InitTimeValue:         0,
			TimestepsHistoryDepth: 1,
			Iterations: []simulator.IterationSettings{
				{Name: "x", Params: simulator.NewParams(map[string][]float64{}),
					InitStateValues: []float64{0}, StateWidth: 1, StateHistoryDepth: 1, Seed: 0},
				{Name: "y", Params: simulator.NewParams(map[string][]float64{}),
					InitStateValues: []float64{1}, StateWidth: 1, StateHistoryDepth: 1, Seed: 0},
				{Name: "reg", Params: simulator.NewParams(map[string][]float64{
					scalarRegressionInterceptKey:      {0},
					scalarRegressionModeKey:           {0},
					scalarRegressionWindowLengthKey:   {0},
					scalarRegressionMinDenominatorKey: {1e-12},
				}),
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"x": {Upstream: 0}, "y": {Upstream: 1},
					},
					InitStateValues: make([]float64, 6), StateWidth: 6, StateHistoryDepth: 1, Seed: 0},
			},
		}
		settings.Init()
		constX := &general.ConstantValuesIteration{}
		constY := &general.ConstantValuesIteration{}
		reg := &ScalarRegressionStatsIteration{}
		implementations := &simulator.Implementations{
			Iterations:           []simulator.Iteration{constX, constY, reg},
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 5},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		}
		constX.Configure(0, settings)
		constY.Configure(1, settings)
		reg.Configure(2, settings)
		if err := simulator.RunWithHarnesses(settings, implementations); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("sliding window matches batch on last W points", func(t *testing.T) {
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

		settings := &simulator.Settings{
			InitTimeValue:         0,
			TimestepsHistoryDepth: 1,
			Iterations: []simulator.IterationSettings{
				{Name: "x", Params: simulator.NewParams(map[string][]float64{}),
					InitStateValues: []float64{0}, StateWidth: 1, StateHistoryDepth: 1, Seed: 0},
				{Name: "y", Params: simulator.NewParams(map[string][]float64{}),
					InitStateValues: []float64{0}, StateWidth: 1, StateHistoryDepth: 1, Seed: 0},
				{Name: "reg", Params: simulator.NewParams(map[string][]float64{
					scalarRegressionInterceptKey:      {0},
					scalarRegressionModeKey:           {1},
					scalarRegressionWindowLengthKey:   {W},
					scalarRegressionMinDenominatorKey: {1e-12},
				}),
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"x": {Upstream: 0, Indices: []int{0}},
						"y": {Upstream: 1, Indices: []int{0}},
					},
					InitStateValues:   make([]float64, ScalarRegressionStateWidth(false, RegressionStatsWindow, W)),
					StateWidth:        ScalarRegressionStateWidth(false, RegressionStatsWindow, W),
					StateHistoryDepth: 1, Seed: 0},
			},
		}
		settings.Init()
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
				tv := timeFromTimesteps(th)
				if tv <= 10 {
					return []float64{2 * tv}
				}
				return []float64{-0.5 * tv}
			},
		}
		reg := &ScalarRegressionStatsIteration{}
		implementations := &simulator.Implementations{
			Iterations:           []simulator.Iteration{iterX, iterY, reg},
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: total},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		}
		iterX.Configure(0, settings)
		iterY.Configure(1, settings)
		reg.Configure(2, settings)
		coordinator := simulator.NewPartitionCoordinator(settings, implementations)
		coordinator.Run()
		last := coordinator.Shared.StateHistories[2].Values.RawRowView(0)
		base := 2*W + 1
		if !closeEnough(last[base+4], bWant, 1e-9) {
			t.Errorf("window beta got %v want %v", last[base+4], bWant)
		}
		if !closeEnough(last[base+5], s2Want, 1e-9) {
			t.Errorf("window sigma^2 got %v want %v", last[base+5], s2Want)
		}
	})

	t.Run("cumulative intercept matches batch", func(t *testing.T) {
		const nStep = 30
		xs := make([]float64, nStep)
		ys := make([]float64, nStep)
		for i := range xs {
			tv := float64(i + 1)
			xs[i] = tv
			ys[i] = 1.0 + 0.25*tv
		}
		aWant, bWant, s2Want := batchIntercept(xs, ys)

		settings := &simulator.Settings{
			InitTimeValue:         0,
			TimestepsHistoryDepth: 1,
			Iterations: []simulator.IterationSettings{
				{Name: "x", Params: simulator.NewParams(map[string][]float64{}),
					InitStateValues: []float64{0}, StateWidth: 1, StateHistoryDepth: 1, Seed: 0},
				{Name: "y", Params: simulator.NewParams(map[string][]float64{}),
					InitStateValues: []float64{0}, StateWidth: 1, StateHistoryDepth: 1, Seed: 0},
				{Name: "reg", Params: simulator.NewParams(map[string][]float64{
					scalarRegressionInterceptKey:      {1},
					scalarRegressionModeKey:           {0},
					scalarRegressionWindowLengthKey:   {0},
					scalarRegressionMinDenominatorKey: {1e-12},
				}),
					ParamsFromUpstream: map[string]simulator.UpstreamConfig{
						"x": {Upstream: 0, Indices: []int{0}},
						"y": {Upstream: 1, Indices: []int{0}},
					},
					InitStateValues:   make([]float64, 9),
					StateWidth:        9,
					StateHistoryDepth: 1, Seed: 0},
			},
		}
		settings.Init()
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
				tv := timeFromTimesteps(th)
				return []float64{1.0 + 0.25*tv}
			},
		}
		reg := &ScalarRegressionStatsIteration{}
		implementations := &simulator.Implementations{
			Iterations:           []simulator.Iteration{iterX, iterY, reg},
			OutputCondition:      &simulator.EveryStepOutputCondition{},
			OutputFunction:       &simulator.NilOutputFunction{},
			TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: nStep},
			TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		}
		iterX.Configure(0, settings)
		iterY.Configure(1, settings)
		reg.Configure(2, settings)
		coordinator := simulator.NewPartitionCoordinator(settings, implementations)
		coordinator.Run()
		last := coordinator.Shared.StateHistories[2].Values.RawRowView(0)
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
}

func TestNewScalarRegressionStatsPartitionAddToStorage(t *testing.T) {
	const nStep = 40
	settings := &simulator.Settings{
		InitTimeValue:         0,
		TimestepsHistoryDepth: 1,
		Iterations: []simulator.IterationSettings{
			{Name: "x", Params: simulator.NewParams(map[string][]float64{}),
				InitStateValues: []float64{0}, StateWidth: 1, StateHistoryDepth: 1, Seed: 0},
			{Name: "y", Params: simulator.NewParams(map[string][]float64{}),
				InitStateValues: []float64{0}, StateWidth: 1, StateHistoryDepth: 1, Seed: 0},
		},
	}
	settings.Init()
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
			return []float64{3.0 * timeFromTimesteps(th)}
		},
	}
	implementations := &simulator.Implementations{
		Iterations:           []simulator.Iteration{iterX, iterY},
		OutputCondition:      &simulator.EveryStepOutputCondition{},
		OutputFunction:       &simulator.StateTimeStorageOutputFunction{Store: simulator.NewStateTimeStorage()},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: nStep},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
	}
	iterX.Configure(0, settings)
	iterY.Configure(1, settings)
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	coordinator.Run()
	storage := implementations.OutputFunction.(*simulator.StateTimeStorageOutputFunction).Store

	xs := make([]float64, nStep)
	ys := make([]float64, nStep)
	for i := range xs {
		tv := float64(i + 1)
		xs[i] = tv
		ys[i] = 3 * tv
	}
	bWant, s2Want := batchThroughOrigin(xs, ys)

	regPart := NewScalarRegressionStatsPartition(AppliedScalarRegressionStats{
		Name:      "ols",
		Y:         DataRef{PartitionName: "y"},
		X:         DataRef{PartitionName: "x"},
		Intercept: false,
		Mode:      RegressionStatsCumulative,
	}, storage)
	storage = AddPartitionsToStateTimeStorage(storage, []*simulator.PartitionConfig{regPart}, nil)
	vs := storage.GetValues("ols")
	last := vs[len(vs)-1]
	if !closeEnough(last[4], bWant, 1e-9) {
		t.Errorf("beta got %v want %v", last[4], bWant)
	}
	if !closeEnough(last[5], s2Want, 1e-9) {
		t.Errorf("sigma^2 got %v want %v", last[5], s2Want)
	}
}
