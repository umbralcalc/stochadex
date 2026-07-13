package rugby

import (
	"sync"

	"github.com/umbralcalc/stochadex/models/cardgen"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// This file holds the model's runnable expected-behaviour definitions, shared by
// two consumers: the behaviour test (which asserts each claim's direction) and
// cmd/model-graphs (which renders the observed numbers into card.md). Keeping the
// computation here — not in a _test.go file — is what lets the card show exactly
// the numbers the test asserts on, so the two can never disagree.

// runStrategy runs the stub for an arbitrary substitution strategy with an
// optional override hook applied to the generated config, so a behaviour can vary
// the substitution plan and/or any partition's params (coefficients, conversion
// probabilities) without bloating BuildStub's one-driver signature.
func runStrategy(
	strategy *SubstitutionStrategy,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	gen := buildStubWithStrategy(strategy, numSteps, seed)
	if override != nil {
		override(gen)
	}
	settings, implementations := gen.GenerateConfigs()
	store := simulator.NewStateTimeStorage()
	implementations.OutputFunction = &simulator.StateTimeStorageOutputFunction{Store: store}
	coordinator := simulator.NewPartitionCoordinator(settings, implementations)
	var wg sync.WaitGroup
	for !coordinator.ReadyToTerminate() {
		coordinator.Step(&wg)
	}
	return store
}

// finalRow returns the last recorded state row of the named partition.
func finalRow(store *simulator.StateTimeStorage, partition string) []float64 {
	rows := store.GetValues(partition)
	return rows[len(rows)-1]
}

// meanFinal ensemble-averages the final value of component idx of the named
// partition over nMembers seeds, for a given strategy and override.
func meanFinal(
	strategy *SubstitutionStrategy,
	numSteps, nMembers int,
	partition string,
	idx int,
	override func(*simulator.ConfigGenerator),
) float64 {
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStrategy(strategy, numSteps, uint64(5000+m), override)
		sum += finalRow(store, partition)[idx]
	}
	return sum / float64(nMembers)
}

// homeWinProbability is the fraction of an nMembers ensemble in which the home
// side finishes ahead (final score difference > 0).
func homeWinProbability(
	strategy *SubstitutionStrategy,
	numSteps, nMembers int,
) float64 {
	wins := 0
	for m := 0; m < nMembers; m++ {
		store := runStrategy(strategy, numSteps, uint64(6000+m), nil)
		if finalRow(store, "match_state")[StateIdxScoreDiff] > 0 {
			wins++
		}
	}
	return float64(wins) / float64(nMembers)
}

// homeSubsAll builds a strategy with all four home groups substituted at
// homeMinute and all four away groups at awayMinute.
func homeSubsAll(homeMinute, awayMinute int) *SubstitutionStrategy {
	s := &SubstitutionStrategy{}
	for g := 0; g < NumPositionGroups; g++ {
		s.HomeSubs[g] = homeMinute
		s.AwaySubs[g] = awayMinute
	}
	return s
}

// scoreCoeffsWith returns the default score coefficients after applying edit.
func scoreCoeffsWith(edit func(c []float64)) []float64 {
	c := DefaultScoreCoefficients()
	edit(c)
	return c
}

// ObservedBehaviour returns the model's named response claims, each with the
// ensemble numbers it produces. It is the single source of the card's "Observed
// behaviour" numbers AND the values the behaviour test asserts on, so the two can
// never disagree. Each claim's Monotone direction is what its binding test checks.
//
// The claim IDs match the subtest names under TestRugbyExpectedBehaviour.
func ObservedBehaviour() []cardgen.Claim {
	const steps = DefaultNumSteps
	const (
		homeTries = "ensemble-mean home tries"
		awayTries = "ensemble-mean away tries"
	)

	// --- Headline: earlier home substitution raises the home try count ---
	lateHomeTries := meanFinal(homeSubsAll(70, DefaultAwaySubMinute), steps, 24, "score_events", 0, nil)
	earlyHomeTries := meanFinal(homeSubsAll(20, DefaultAwaySubMinute), steps, 24, "score_events", 0, nil)

	// --- Decision-path responses (actionable levers) ---
	lateWinProb := homeWinProbability(homeSubsAll(65, DefaultAwaySubMinute), steps, 60)
	earlyWinProb := homeWinProbability(homeSubsAll(15, DefaultAwaySubMinute), steps, 60)

	partial := &SubstitutionStrategy{}
	partial.HomeSubs[0] = 20 // only one home group
	for g := 0; g < NumPositionGroups; g++ {
		partial.AwaySubs[g] = DefaultAwaySubMinute
	}
	partialTries := meanFinal(partial, steps, 30, "score_events", 0, nil)
	fullTries := meanFinal(homeSubsAll(20, DefaultAwaySubMinute), steps, 30, "score_events", 0, nil)

	lateAwayTries := meanFinal(homeSubsAll(DefaultHomeSubMinute, 70), steps, 30, "score_events", 1, nil)
	earlyAwayTries := meanFinal(homeSubsAll(DefaultHomeSubMinute, 20), steps, 30, "score_events", 1, nil)

	// --- Structural-driver responses (non-actionable) ---
	defaultStrategy := DefaultSubstitutionStrategy(DefaultHomeSubMinute)

	interceptBase := meanFinal(defaultStrategy, steps, 24, "score_events", 0, nil)
	interceptStrong := meanFinal(defaultStrategy, steps, 24, "score_events", 0, func(g *simulator.ConfigGenerator) {
		g.GetPartition("score_rates").Params.Map["coefficients"] = scoreCoeffsWith(func(c []float64) {
			c[0] = -2.5 // raise home try intercept from -3.0
		})
	})

	convBase := meanFinal(defaultStrategy, steps, 24, "match_state", StateIdxHomeScore, nil)
	convHigh := meanFinal(defaultStrategy, steps, 24, "match_state", StateIdxHomeScore, func(g *simulator.ConfigGenerator) {
		g.GetPartition("conversion_events").Params.Map["conversion_probs"] = []float64{0.98, DefaultAwayConversionProb}
	})

	subEffectStrategy := homeSubsAll(20, DefaultAwaySubMinute) // early sub so the boost window is large
	subEffectBase := meanFinal(subEffectStrategy, steps, 30, "score_events", 0, nil)
	subEffectStrong := meanFinal(subEffectStrategy, steps, 30, "score_events", 0, func(g *simulator.ConfigGenerator) {
		g.GetPartition("score_rates").Params.Map["coefficients"] = scoreCoeffsWith(func(c []float64) {
			for j := 1; j <= NumPositionGroups; j++ {
				c[j] = 0.4 // stronger home try covariate effect (from 0.15)
			}
		})
	})

	symmetric := homeSubsAll(DefaultHomeSubMinute, DefaultHomeSubMinute)
	symHomeTries := meanFinal(symmetric, steps, 40, "score_events", 0, nil)
	symAwayTries := meanFinal(symmetric, steps, 40, "score_events", 1, nil)

	cardBase := meanFinal(defaultStrategy, steps, 30, "card_events", 0, nil)
	cardStrict := meanFinal(defaultStrategy, steps, 30, "card_events", 0, func(g *simulator.ConfigGenerator) {
		c := DefaultCardCoefficients()
		c[0] = -3.3 // raise home yellow intercept from -4.5
		g.GetPartition("card_rates").Params.Map["coefficients"] = c
	})

	return []cardgen.Claim{
		{
			ID:        "earlier_home_substitution_raises_home_tries",
			Statement: "Earlier home substitution raises home tries (headline driver)",
			Unit:      homeTries,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "sub min 70 (late)", Value: lateHomeTries},
				{Label: "sub min 20 (early)", Value: earlyHomeTries},
			},
		},
		{
			ID:        "earlier_home_substitution_raises_home_win_probability",
			Statement: "Earlier home substitution raises home win probability",
			Unit:      "home win probability",
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "sub min 65 (late)", Value: lateWinProb},
				{Label: "sub min 15 (early)", Value: earlyWinProb},
			},
		},
		{
			ID:        "substituting_more_position_groups_raises_home_tries",
			Statement: "Substituting more position groups raises home tries",
			Unit:      homeTries,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "1 group", Value: partialTries},
				{Label: "4 groups", Value: fullTries},
			},
		},
		{
			ID:        "earlier_away_substitution_raises_away_tries",
			Statement: "Earlier away substitution raises away tries",
			Unit:      awayTries,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "sub min 70 (late)", Value: lateAwayTries},
				{Label: "sub min 20 (early)", Value: earlyAwayTries},
			},
		},
		{
			ID:        "higher_try_intercept_raises_home_tries",
			Statement: "Higher home try intercept raises home tries",
			Unit:      homeTries,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "intercept -3.0", Value: interceptBase},
				{Label: "intercept -2.5", Value: interceptStrong},
			},
		},
		{
			ID:        "higher_conversion_probability_raises_home_score",
			Statement: "Higher conversion probability raises home score",
			Unit:      "ensemble-mean home score",
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "p=0.75", Value: convBase},
				{Label: "p=0.98", Value: convHigh},
			},
		},
		{
			ID:        "stronger_substitution_effect_raises_home_tries",
			Statement: "Stronger per-group substitution effect raises home tries",
			Unit:      homeTries,
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "β=0.15", Value: subEffectBase},
				{Label: "β=0.40", Value: subEffectStrong},
			},
		},
		{
			ID:        "home_advantage_intercept_makes_home_outscore_away",
			Statement: "Home-advantage intercept makes home outscore away under symmetric subs",
			Unit:      "ensemble-mean tries",
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "away tries", Value: symAwayTries},
				{Label: "home tries", Value: symHomeTries},
			},
		},
		{
			ID:        "higher_yellow_card_intercept_raises_cards",
			Statement: "Higher yellow-card intercept raises cards",
			Unit:      "ensemble-mean home yellow cards",
			Monotone:  1,
			Observations: []cardgen.Observation{
				{Label: "intercept -4.5", Value: cardBase},
				{Label: "intercept -3.3", Value: cardStrict},
			},
		},
	}
}
