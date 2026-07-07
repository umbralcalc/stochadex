package bathingwater

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// BathingConcentrationIteration maps a single bathing site's latent
// log-concentration to its exceedance probability against the statutory
// threshold, each step, given the shared regional anomaly delivered from an
// upstream partition.
//
// State (and output): [mu, p_exceed]
//   - mu       — latent log-concentration = baseline + season(t) + loading·anomaly
//   - p_exceed — P(log c > log threshold) = Φ((mu − log_threshold) / sample_scale)
//
// The step is deterministic given the shared anomaly and the time: all stochastic,
// cross-site-correlated variation enters through the upstream Ornstein–Uhlenbeck
// anomaly z(t), while sample_scale is the within-site log-normal scale of an
// individual sample and is integrated out analytically inside Φ (as in the
// downstream censored-log-normal likelihood). This mirrors the downstream regional
// composition, which computes exactly μ = base + λ·z and P = Φ((μ − logThr)/σ);
// here the seasonal term is added and computed from the current time so the stub
// self-drives over a season rather than replaying fitted per-day covariates.
//
// Params:
//   - baseline:           [b]      — log-concentration baseline (log of the clean-water count)
//   - seasonal_amplitude: [A]      — amplitude of the day-of-year seasonal term (log units)
//   - seasonal_phase:     [phi]    — phase offset of the seasonal term (radians)
//   - period:             [P]      — seasonal period in steps (e.g. 365 days)
//   - anomaly_loading:    [lambda] — how strongly the shared anomaly loads onto this site
//   - sample_scale:       [sigma]  — within-site log-normal sample scale (>0)
//   - log_threshold:      [logThr] — log of the statutory exceedance threshold
//   - anomaly:            [z]      — shared regional anomaly, from the upstream OU partition
type BathingConcentrationIteration struct{}

func (b *BathingConcentrationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
}

func (b *BathingConcentrationIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	baseline := params.Map["baseline"][0]
	amplitude := params.Map["seasonal_amplitude"][0]
	phase := params.Map["seasonal_phase"][0]
	period := params.Map["period"][0]
	loading := params.Map["anomaly_loading"][0]
	sigma := params.Map["sample_scale"][0]
	logThreshold := params.Map["log_threshold"][0]
	z := params.Map["anomaly"][0]

	// Current cumulative time drives the day-of-year seasonal term.
	t := timestepsHistory.Values.AtVec(0)
	season := amplitude * math.Sin(2*math.Pi*t/period+phase)

	mu := baseline + season + loading*z
	pExceed := normalCDF((mu - logThreshold) / sigma)
	return []float64{mu, pExceed}
}

const sqrt2 = 1.4142135623730951

// normalCDF is the standard-normal cumulative distribution function.
func normalCDF(x float64) float64 { return 0.5 * math.Erfc(-x/sqrt2) }
