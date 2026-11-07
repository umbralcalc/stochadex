package macros

import "github.com/umbralcalc/stochadex/pkg/simulator"

// ScalarRegressionLayout is the data a scalar regression partition is wired with:
// everything about it but its name and iteration. The constructor here and a
// config's expansion both read it.
type ScalarRegressionLayout struct {
	Partition    InnerLayout
	HistoryDepth int
}

// NewScalarRegressionLayout is the layout of the regression applied describes, given
// the value indices Y and X read and the pair in their first rows, from which the
// statistics start. A minimum denominator that is not positive is 1e-12, and a
// history depth that is not positive is 1.
func NewScalarRegressionLayout(
	applied AppliedScalarRegressionStats,
	yIndex, xIndex int,
	xFirst, yFirst float64,
) ScalarRegressionLayout {
	flag := func(on bool) []float64 {
		if on {
			return []float64{1}
		}
		return []float64{0}
	}
	minDenominator := applied.MinDenominator
	if minDenominator <= 0 {
		minDenominator = 1e-12
	}
	return ScalarRegressionLayout{
		Partition: InnerLayout{
			Params: map[string][]float64{
				scalarRegressionInterceptKey:      flag(applied.Intercept),
				scalarRegressionModeKey:           flag(applied.Mode == RegressionStatsWindow),
				scalarRegressionWindowLengthKey:   {float64(applied.WindowLength)},
				scalarRegressionMinDenominatorKey: {minDenominator},
			},
			ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
				ScalarRegressionParamY: {Upstream: applied.Y.PartitionName, Indices: []int{yIndex}},
				ScalarRegressionParamX: {Upstream: applied.X.PartitionName, Indices: []int{xIndex}},
			},
			InitStateValues: ScalarRegressionInitState(applied.Intercept, applied.Mode,
				applied.WindowLength, minDenominator, xFirst, yFirst),
		},
		HistoryDepth: max(applied.StateHistoryDepth, 1),
	}
}
