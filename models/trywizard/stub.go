package rugby

import (
	"github.com/umbralcalc/stochadex/pkg/discrete"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Default generative parameters for the flagship scenario: one 80-minute rugby
// union match between a home and an away side, generated as coupled stochastic
// counting processes. These are illustrative values chosen so the generative
// core runs with zero external inputs — NOT calibrated posteriors. In the
// downstream repo the intercepts and covariate coefficients are a Poisson-GLM
// fitted by warm-start SGD to real SportDevs match-event data, and the baseline
// rates are adaptive-bandwidth kernel-smoothed per minute; see card.md.
//
// The intercepts are chosen to give realistic per-match event counts
// (exp(intercept) is the per-minute event rate against the zero baseline): tries
// ~exp(-3.0)≈0.05/min → ~4 per side, penalties ~exp(-3.5)≈0.03/min, yellow cards
// ~exp(-4.5)≈0.011/min. The substitution covariate coefficients are positive, so
// a "fresh legs" substitution lifts the substituting side's try and penalty
// rates — the effect the swept driver, homeSubMinute, exercises.
const (
	// Intercepts (baseline log-rates against the zero baseline). Home carries a
	// small advantage on tries.
	DefaultHomeTryIntercept     = -3.0
	DefaultAwayTryIntercept     = -3.1
	DefaultHomePenaltyIntercept = -3.5
	DefaultAwayPenaltyIntercept = -3.6
	DefaultHomeYellowIntercept  = -4.5
	DefaultAwayYellowIntercept  = -4.5

	// Per-position-group substitution effect on the substituting side's log-rate.
	// With all four groups on, a side's try log-rate rises by 4·0.15 = 0.6
	// (×exp(0.6) ≈ ×1.82).
	DefaultSubEffectTry     = 0.15
	DefaultSubEffectPenalty = 0.05

	// Conversion success probabilities [home, away].
	DefaultHomeConversionProb = 0.75
	DefaultAwayConversionProb = 0.72

	// DefaultHomeSubMinute is the baseline value of the swept driver (the home
	// side substitutes its bench around the hour mark). DefaultAwaySubMinute
	// holds the away side's timing fixed so home timing can be varied in isolation.
	DefaultHomeSubMinute = 55
	DefaultAwaySubMinute = 55

	// DefaultNumSteps is the flagship horizon: 80 one-minute steps (a full match).
	DefaultNumSteps = 80

	// stepSizeMinutes is one minute of match time per step.
	stepSizeMinutes = 1.0
)

// DefaultConversionProbs is the [home, away] conversion-probability vector.
var DefaultConversionProbs = []float64{DefaultHomeConversionProb, DefaultAwayConversionProb}

// homeCovariateVector returns an 8-wide covariate coefficient row that applies
// effect to each of the four home position groups and zero to the away groups.
func homeCovariateVector(effect float64) []float64 {
	return []float64{effect, effect, effect, effect, 0, 0, 0, 0}
}

// awayCovariateVector is the mirror of homeCovariateVector for the away groups.
func awayCovariateVector(effect float64) []float64 {
	return []float64{0, 0, 0, 0, effect, effect, effect, effect}
}

// setChannel writes one rate channel's coefficients (intercept + 8 covariates)
// into the flat coefficient vector at the CoeffsPerRate-strided offset.
func setChannel(coeffs []float64, channel int, intercept float64, covariates []float64) {
	offset := channel * CoeffsPerRate
	coeffs[offset] = intercept
	copy(coeffs[offset+1:offset+CoeffsPerRate], covariates)
}

// DefaultScoreCoefficients builds the 36-wide score-rate coefficient vector:
// home_try / away_try / home_penalty / away_penalty, each an intercept plus the
// eight substitution covariates. Each side's substitutions lift only that side's
// rates.
func DefaultScoreCoefficients() []float64 {
	c := make([]float64, ScoreCoeffWidth)
	setChannel(c, 0, DefaultHomeTryIntercept, homeCovariateVector(DefaultSubEffectTry))
	setChannel(c, 1, DefaultAwayTryIntercept, awayCovariateVector(DefaultSubEffectTry))
	setChannel(c, 2, DefaultHomePenaltyIntercept, homeCovariateVector(DefaultSubEffectPenalty))
	setChannel(c, 3, DefaultAwayPenaltyIntercept, awayCovariateVector(DefaultSubEffectPenalty))
	return c
}

// DefaultCardCoefficients builds the 18-wide card-rate coefficient vector:
// home_yellow / away_yellow intercepts with no substitution covariate effect.
func DefaultCardCoefficients() []float64 {
	c := make([]float64, CardCoeffWidth)
	setChannel(c, 0, DefaultHomeYellowIntercept, make([]float64, SubCovWidth))
	setChannel(c, 1, DefaultAwayYellowIntercept, make([]float64, SubCovWidth))
	return c
}

// DefaultSubstitutionStrategy substitutes all four of each side's position groups
// at the given minutes (home at homeSubMinute, away at DefaultAwaySubMinute).
func DefaultSubstitutionStrategy(homeSubMinute int) *SubstitutionStrategy {
	s := &SubstitutionStrategy{}
	for g := 0; g < NumPositionGroups; g++ {
		s.HomeSubs[g] = homeSubMinute
		s.AwaySubs[g] = DefaultAwaySubMinute
	}
	return s
}

// BuildStub constructs the data-free generative core of the rugby-match model:
// eight coupled partitions that turn log-linear event rates into a stochastic
// scoreline over 80 minutes. A constant baseline and a substitution-covariate
// series feed two log-linear rate channels (scores, cards); those drive two Cox
// counting processes; a Bernoulli conversion process fires on each new try; and
// a match-state partition derives the running scores, active yellow cards, and
// half from the event streams.
//
// This is the generative core only — no data ingestion, no kernel-smoothed
// baseline, no Poisson-GLM training. The one scientifically-interesting driver,
// homeSubMinute, sets when the home side empties its bench: earlier
// substitutions leave the "fresh legs" covariate switched on for more of the
// match, which (with the illustrative positive coefficients) lifts the home
// side's scoring. It is exactly the parameter the CI test sweeps to check the
// model's headline claim: earlier home substitution raises home scoring.
//
// Partitions (declaration order):
//
//	baseline_rates     constant zero baseline (fall back to exp(intercept+cov))
//	sub_covariates     per-minute binary substitution covariates from the strategy
//	score_rates        log-linear try/penalty rates (width 4)
//	card_rates         log-linear yellow-card rates (width 2)
//	score_events       Cox process: cumulative tries & penalties (width 4)
//	card_events        Cox process: cumulative yellow cards (width 2)
//	conversion_events  Bernoulli conversions per new try (width 2)
//	match_state        derived [home_score, away_score, diff, active yellows, half]
func BuildStub(homeSubMinute int, numSteps int, seed uint64) *simulator.ConfigGenerator {
	return buildStubWithStrategy(DefaultSubstitutionStrategy(homeSubMinute), numSteps, seed)
}

// buildStubWithStrategy is the internal constructor that wires the eight
// partitions for an arbitrary substitution strategy. BuildStub exposes only the
// one headline driver; behaviour tests in this package reach for this directly
// (or override partition params) to sweep the wider decision and structural
// surface without bloating BuildStub's signature.
func buildStubWithStrategy(
	strategy *SubstitutionStrategy,
	numSteps int,
	seed uint64,
) *simulator.ConfigGenerator {
	baseline := NewBaselineRatesConstantPartition()
	subCov := NewSubCovariatesFromStrategyPartition(strategy, numSteps)

	scoreRates := NewScoreRatesPartition(DefaultScoreCoefficients())
	scoreRates.ParamsFromUpstream = map[string]simulator.NamedUpstreamConfig{
		"covariates": {Upstream: "sub_covariates"},
		"baseline":   {Upstream: "baseline_rates"},
	}

	cardRates := NewCardRatesPartition(DefaultCardCoefficients())
	cardRates.ParamsFromUpstream = map[string]simulator.NamedUpstreamConfig{
		"covariates": {Upstream: "sub_covariates"},
		"baseline":   {Upstream: "baseline_rates"},
	}

	scoreEvents := &simulator.PartitionConfig{
		Name:      "score_events",
		Iteration: &discrete.CoxProcessIteration{},
		Params:    simulator.NewParams(make(map[string][]float64)),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"rates": {Upstream: "score_rates"},
		},
		InitStateValues:   make([]float64, ScoreRateWidth),
		StateHistoryDepth: 2,
		Seed:              seed,
	}

	cardEvents := &simulator.PartitionConfig{
		Name:      "card_events",
		Iteration: &discrete.CoxProcessIteration{},
		Params:    simulator.NewParams(make(map[string][]float64)),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"rates": {Upstream: "card_rates"},
		},
		InitStateValues:   make([]float64, CardRateWidth),
		StateHistoryDepth: YellowCardMinutes + 1,
		Seed:              seed + 1,
	}

	conversionEvents := &simulator.PartitionConfig{
		Name:      "conversion_events",
		Iteration: &ConversionIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"conversion_probs": append([]float64(nil), DefaultConversionProbs...),
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"try_values": {Upstream: "score_events"},
		},
		ParamsAsPartitions: map[string][]string{
			"score_events_partition": {"score_events"},
		},
		InitStateValues:   make([]float64, 2),
		StateHistoryDepth: 1,
		Seed:              seed + 2,
	}

	matchState := NewMatchStatePartition()

	gen := simulator.NewConfigGenerator()
	for _, p := range []*simulator.PartitionConfig{
		baseline, subCov, scoreRates, cardRates,
		scoreEvents, cardEvents, conversionEvents, matchState,
	} {
		gen.SetPartition(p)
	}
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: stepSizeMinutes},
		InitTimeValue:    0.0,
	})
	return gen
}
