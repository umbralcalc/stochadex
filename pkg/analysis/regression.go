package analysis

import (
	"fmt"
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Param keys for ParamsFromUpstream wiring into ScalarRegressionStatsIteration.
// Each upstream slice should have length 1 (or use Indices on NamedUpstreamConfig).
const (
	ScalarRegressionParamY = "y"
	ScalarRegressionParamX = "x"
)

// Params keys set by NewScalarRegressionStatsPartition (also readable in YAML).
const (
	scalarRegressionInterceptKey      = "scalar_regression_intercept"
	scalarRegressionModeKey           = "scalar_regression_mode"
	scalarRegressionWindowLengthKey   = "scalar_regression_window_length"
	scalarRegressionMinDenominatorKey = "scalar_regression_min_denominator"
)

// RegressionStatsMode selects cumulative vs sliding-window sufficient statistics.
type RegressionStatsMode int

const (
	// RegressionStatsCumulative accumulates (x, y) sums over all prior steps.
	RegressionStatsCumulative RegressionStatsMode = iota
	// RegressionStatsWindow keeps the last WindowLength pairs and recomputes sums.
	RegressionStatsWindow
)

// ScalarRegressionStatsIteration maintains OLS-relevant sufficient statistics for
// scalar y on scalar x, optionally with intercept, and writes closed-form
// estimates each step. Row 0 of state history is the latest values, matching
// LaggedValues / FromHistory conventions elsewhere.
//
// Through-origin state (cumulative): [Sxx, Sxy, Syy, n, beta, sigma2] (width 6).
// With intercept (cumulative): [n, Sx, Sy, Sxx, Sxy, Syy, alpha, beta, sigma2] (width 9).
// Window modes prefix a packed ring of the last W (x, y) pairs plus a count slot,
// then the same trailing statistic/estimate block.
type ScalarRegressionStatsIteration struct {
	intercept    bool
	mode         RegressionStatsMode
	windowLength int
	minDenom     float64
	expectWidth  int
}

// ScalarRegressionStateWidth returns the required InitStateValues length for the
// given options (same layout ScalarRegressionStatsIteration uses).
func ScalarRegressionStateWidth(
	intercept bool,
	mode RegressionStatsMode,
	windowLength int,
) int {
	if mode == RegressionStatsWindow {
		if windowLength < 1 {
			panic("scalar regression window length must be >= 1")
		}
		if intercept {
			return 2*windowLength + 10
		}
		return 2*windowLength + 7
	}
	if intercept {
		return 9
	}
	return 6
}

func firstFloat(p *simulator.Params, key string, defaultVal float64) float64 {
	v, ok := p.GetOk(key)
	if !ok || len(v) == 0 {
		return defaultVal
	}
	return v[0]
}

// Configure reads scalar_regression_* params from the partition settings.
func (s *ScalarRegressionStatsIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	p := &settings.Iterations[partitionIndex].Params
	s.intercept = firstFloat(p, scalarRegressionInterceptKey, 0) != 0
	modeVal := int(firstFloat(p, scalarRegressionModeKey, 0))
	switch modeVal {
	case 0:
		s.mode = RegressionStatsCumulative
	case 1:
		s.mode = RegressionStatsWindow
	default:
		panic(fmt.Sprintf("scalar regression: unknown mode %d", modeVal))
	}
	s.windowLength = int(firstFloat(p, scalarRegressionWindowLengthKey, 0))
	if s.mode == RegressionStatsWindow && s.windowLength < 1 {
		panic("scalar regression window mode requires window_length >= 1")
	}
	s.minDenom = firstFloat(p, scalarRegressionMinDenominatorKey, 0)
	if s.minDenom <= 0 {
		s.minDenom = 1e-12
	}
	s.expectWidth = ScalarRegressionStateWidth(s.intercept, s.mode, s.windowLength)
	got := settings.Iterations[partitionIndex].StateWidth
	if got != s.expectWidth {
		panic(fmt.Sprintf(
			"scalar regression: state width %d != expected %d (intercept=%v mode=%v W=%d)",
			got, s.expectWidth, s.intercept, s.mode, s.windowLength,
		))
	}
}

func (s *ScalarRegressionStatsIteration) readXY(params *simulator.Params) (x, y float64) {
	yv := params.Get(ScalarRegressionParamY)
	xv := params.Get(ScalarRegressionParamX)
	if len(yv) != 1 || len(xv) != 1 {
		panic("scalar regression: params y and x must each have length 1")
	}
	return xv[0], yv[0]
}

// Iterate updates sufficient statistics and returns the next state vector.
func (s *ScalarRegressionStatsIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	x, y := s.readXY(params)
	prev := stateHistories[partitionIndex].Values.RawRowView(0)
	out := make([]float64, len(prev))
	copy(out, prev)

	if s.mode == RegressionStatsCumulative {
		if s.intercept {
			s.iterateCumulativeIntercept(out, x, y)
		} else {
			s.iterateCumulativeOrigin(out, x, y)
		}
		return out
	}
	if s.intercept {
		s.iterateWindowIntercept(out, x, y)
	} else {
		s.iterateWindowOrigin(out, x, y)
	}
	return out
}

func (s *ScalarRegressionStatsIteration) iterateCumulativeOrigin(
	state []float64, x, y float64,
) {
	Sxx := state[0] + x*x
	Sxy := state[1] + x*y
	Syy := state[2] + y*y
	n := state[3] + 1
	state[0], state[1], state[2], state[3] = Sxx, Sxy, Syy, n
	beta, sigma2 := solveThroughOrigin(Sxx, Sxy, Syy, n, s.minDenom)
	state[4], state[5] = beta, sigma2
}

func solveThroughOrigin(Sxx, Sxy, Syy, n, minDenom float64) (beta, sigma2 float64) {
	if Sxx <= minDenom || n < 1 {
		return 0, 0
	}
	beta = Sxy / Sxx
	if n < 2 {
		return beta, 0
	}
	sse := Syy - Sxy*Sxy/Sxx
	if sse < 0 && sse > -1e-9*math.Max(1, Syy) {
		sse = 0
	}
	return beta, sse / (n - 1)
}

func (s *ScalarRegressionStatsIteration) iterateCumulativeIntercept(
	state []float64, x, y float64,
) {
	n := state[0] + 1
	Sx := state[1] + x
	Sy := state[2] + y
	Sxx := state[3] + x*x
	Sxy := state[4] + x*y
	Syy := state[5] + y*y
	state[0], state[1], state[2], state[3], state[4], state[5] = n, Sx, Sy, Sxx, Sxy, Syy
	alpha, beta, sigma2 := solveIntercept(n, Sx, Sy, Sxx, Sxy, Syy, s.minDenom)
	state[6], state[7], state[8] = alpha, beta, sigma2
}

func solveIntercept(n, Sx, Sy, Sxx, Sxy, Syy, minDenom float64) (alpha, beta, sigma2 float64) {
	if n < 1 {
		return 0, 0, 0
	}
	D := n*Sxx - Sx*Sx
	if math.Abs(D) <= minDenom || n < 2 {
		return Sy / n, 0, 0
	}
	beta = (n*Sxy - Sx*Sy) / D
	alpha = (Sy - beta*Sx) / n
	if n < 3 {
		return alpha, beta, 0
	}
	sse := Syy - alpha*Sy - beta*Sxy
	if sse < 0 && sse > -1e-9*math.Max(1, Syy) {
		sse = 0
	}
	return alpha, beta, sse / (n - 2)
}

// Window layout (no intercept): pairs [0:2W], count at [2W], then Sxx,Sxy,Syy,n,beta,sigma2.
func (s *ScalarRegressionStatsIteration) iterateWindowOrigin(
	state []float64, x, y float64,
) {
	W := s.windowLength
	kf := state[2*W]
	k := int(kf)
	if kf != float64(k) {
		k = int(math.Round(kf))
	}
	if k < 0 {
		k = 0
	}

	if k < W {
		state[2*k] = x
		state[2*k+1] = y
		k++
	} else {
		copy(state[0:2*W-2], state[2:2*W])
		state[2*W-2], state[2*W-1] = x, y
		k = W
	}
	state[2*W] = float64(k)

	var Sxx, Sxy, Syy, nPts float64
	for i := 0; i < k; i++ {
		xi := state[2*i]
		yi := state[2*i+1]
		Sxx += xi * xi
		Sxy += xi * yi
		Syy += yi * yi
		nPts += 1
	}
	base := 2*W + 1
	state[base+0], state[base+1], state[base+2], state[base+3] = Sxx, Sxy, Syy, nPts
	beta, sigma2 := solveThroughOrigin(Sxx, Sxy, Syy, nPts, s.minDenom)
	state[base+4], state[base+5] = beta, sigma2
}

// Window layout (intercept): pairs [0:2W], count at [2W], then 9-vector tail matching cumulative intercept.
func (s *ScalarRegressionStatsIteration) iterateWindowIntercept(
	state []float64, x, y float64,
) {
	W := s.windowLength
	kf := state[2*W]
	k := int(kf)
	if kf != float64(k) {
		k = int(math.Round(kf))
	}
	if k < 0 {
		k = 0
	}

	if k < W {
		state[2*k] = x
		state[2*k+1] = y
		k++
	} else {
		copy(state[0:2*W-2], state[2:2*W])
		state[2*W-2], state[2*W-1] = x, y
		k = W
	}
	state[2*W] = float64(k)

	var nPts, Sx, Sy, Sxx, Sxy, Syy float64
	for i := 0; i < k; i++ {
		xi := state[2*i]
		yi := state[2*i+1]
		nPts += 1
		Sx += xi
		Sy += yi
		Sxx += xi * xi
		Sxy += xi * yi
		Syy += yi * yi
	}
	base := 2*W + 1
	state[base+0] = nPts
	state[base+1] = Sx
	state[base+2] = Sy
	state[base+3] = Sxx
	state[base+4] = Sxy
	state[base+5] = Syy
	alpha, beta, sigma2 := solveIntercept(nPts, Sx, Sy, Sxx, Sxy, Syy, s.minDenom)
	state[base+6], state[base+7], state[base+8] = alpha, beta, sigma2
}

// AppliedScalarRegressionStats configures a partition that streams scalar OLS
// sufficient statistics and closed-form estimates. Wire Y and X from upstream
// partitions via ParamsFromUpstream (keys ScalarRegressionParamY and
// ScalarRegressionParamX); each side must be a single scalar state element per step.
type AppliedScalarRegressionStats struct {
	Name              string
	Y                 DataRef
	X                 DataRef
	Intercept         bool
	Mode              RegressionStatsMode
	WindowLength      int
	MinDenominator    float64
	StateHistoryDepth int
}

func oneScalarValueIndex(ref DataRef, storage *simulator.StateTimeStorage) int {
	indices := ref.GetValueIndices(storage)
	if len(indices) != 1 {
		panic("scalar regression: Y and X DataRef must reference exactly one value index each")
	}
	return indices[0]
}

// NewScalarRegressionStatsPartition builds a PartitionConfig for
// ScalarRegressionStatsIteration. storage is used to resolve DataRef indices;
// it must already contain the series partitions referenced by Y and X.
func NewScalarRegressionStatsPartition(
	applied AppliedScalarRegressionStats,
	storage *simulator.StateTimeStorage,
) *simulator.PartitionConfig {
	yIdx := oneScalarValueIndex(applied.Y, storage)
	xIdx := oneScalarValueIndex(applied.X, storage)
	w := applied.WindowLength
	if applied.Mode == RegressionStatsWindow && w < 1 {
		panic("scalar regression: WindowLength must be >= 1 when Mode is RegressionStatsWindow")
	}
	width := ScalarRegressionStateWidth(applied.Intercept, applied.Mode, w)
	interceptFlag := 0.0
	if applied.Intercept {
		interceptFlag = 1.0
	}
	modeVal := 0.0
	if applied.Mode == RegressionStatsWindow {
		modeVal = 1.0
	}
	minD := applied.MinDenominator
	if minD <= 0 {
		minD = 1e-12
	}
	depth := applied.StateHistoryDepth
	if depth <= 0 {
		depth = 1
	}
	return &simulator.PartitionConfig{
		Name:      applied.Name,
		Iteration: &ScalarRegressionStatsIteration{},
		Params: simulator.NewParams(map[string][]float64{
			scalarRegressionInterceptKey:      {interceptFlag},
			scalarRegressionModeKey:           {modeVal},
			scalarRegressionWindowLengthKey:   {float64(w)},
			scalarRegressionMinDenominatorKey: {minD},
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			ScalarRegressionParamY: {
				Upstream: applied.Y.PartitionName,
				Indices:  []int{yIdx},
			},
			ScalarRegressionParamX: {
				Upstream: applied.X.PartitionName,
				Indices:  []int{xIdx},
			},
		},
		InitStateValues:   make([]float64, width),
		StateHistoryDepth: depth,
		Seed:              0,
	}
}
