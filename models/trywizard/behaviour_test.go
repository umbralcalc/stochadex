package rugby

import (
	"sync"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// runStrategy runs the stub for an arbitrary substitution strategy with an
// optional override hook applied to the generated config, so a behaviour test can
// vary the substitution plan and/or any partition's params (coefficients,
// conversion probabilities) without bloating BuildStub's one-driver signature.
func runStrategy(
	t *testing.T,
	strategy *SubstitutionStrategy,
	numSteps int,
	seed uint64,
	override func(*simulator.ConfigGenerator),
) *simulator.StateTimeStorage {
	t.Helper()
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

// meanFinal ensemble-averages the final value of component idx of the named
// partition over nMembers seeds, for a given strategy and override.
func meanFinal(
	t *testing.T,
	strategy *SubstitutionStrategy,
	numSteps, nMembers int,
	partition string,
	idx int,
	override func(*simulator.ConfigGenerator),
) float64 {
	t.Helper()
	var sum float64
	for m := 0; m < nMembers; m++ {
		store := runStrategy(t, strategy, numSteps, uint64(5000+m), override)
		sum += finalRow(store, partition)[idx]
	}
	return sum / float64(nMembers)
}

// homeWinProbability is the fraction of an nMembers ensemble in which the home
// side finishes ahead (final score difference > 0).
func homeWinProbability(
	t *testing.T,
	strategy *SubstitutionStrategy,
	numSteps, nMembers int,
) float64 {
	t.Helper()
	wins := 0
	for m := 0; m < nMembers; m++ {
		store := runStrategy(t, strategy, numSteps, uint64(6000+m), nil)
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

// TestRugbyExpectedBehaviour is the expected-behaviour suite: each subtest name
// states, in plain language, a response the model is claimed to produce, and the
// body checks it. Together they specify how the model behaves for a downstream
// decision-maker (actionable substitution levers) and why it should be trusted
// off-sample (structural rate / conversion / card drivers).
func TestRugbyExpectedBehaviour(t *testing.T) {
	const steps = DefaultNumSteps

	// ----- Decision-path responses (actionable levers a coach / analyst controls) -----

	// The headline decision, on the metric that matters most: bringing the home
	// bench on earlier keeps the "fresh legs" advantage switched on for more of the
	// match, raising the home side's win probability. A wrong sign here is a wrong
	// substitution recommendation.
	t.Run("earlier_home_substitution_raises_home_win_probability", func(t *testing.T) {
		const nMembers = 60
		early := homeWinProbability(t, homeSubsAll(15, DefaultAwaySubMinute), steps, nMembers)
		late := homeWinProbability(t, homeSubsAll(65, DefaultAwaySubMinute), steps, nMembers)
		if !(early > late) {
			t.Fatalf("expected earlier home substitution to raise home win probability: "+
				"early(min=15)=%.2f late(min=65)=%.2f", early, late)
		}
	})

	// Substituting more position groups switches on more of the covariate boost, so
	// the same-timing but fuller substitution scores more tries. (state = which
	// groups are fresh, action = how many to bring on.)
	t.Run("substituting_more_position_groups_raises_home_tries", func(t *testing.T) {
		const nMembers = 30
		full := homeSubsAll(20, DefaultAwaySubMinute) // all four home groups at min 20
		partial := &SubstitutionStrategy{}
		partial.HomeSubs[0] = 20 // only one home group
		for g := 0; g < NumPositionGroups; g++ {
			partial.AwaySubs[g] = DefaultAwaySubMinute
		}
		fullTries := meanFinal(t, full, steps, nMembers, "score_events", 0, nil)
		partialTries := meanFinal(t, partial, steps, nMembers, "score_events", 0, nil)
		if !(fullTries > partialTries) {
			t.Fatalf("expected substituting more groups to raise home tries: "+
				"full(4 groups)=%.3f partial(1 group)=%.3f", fullTries, partialTries)
		}
	})

	// The mirror lever for the away side: an earlier away substitution raises the
	// away try count. Confirms the covariate channel is wired symmetrically and is
	// an actionable lever for either coach. Away tries are score_events index 1.
	t.Run("earlier_away_substitution_raises_away_tries", func(t *testing.T) {
		const nMembers = 30
		early := meanFinal(t, homeSubsAll(DefaultHomeSubMinute, 20), steps, nMembers, "score_events", 1, nil)
		late := meanFinal(t, homeSubsAll(DefaultHomeSubMinute, 70), steps, nMembers, "score_events", 1, nil)
		if !(early > late) {
			t.Fatalf("expected earlier away substitution to raise away tries: "+
				"early(min=20)=%.3f late(min=70)=%.3f", early, late)
		}
	})

	// ----- Structural-driver responses (non-actionable; out-of-sample credibility) -----

	// The baseline scoring-propensity driver: a higher home try intercept (a
	// stronger attacking side, set by the world/opposition, not a decision) raises
	// the home try count. Home try intercept is coefficient index 0.
	t.Run("higher_try_intercept_raises_home_tries", func(t *testing.T) {
		const nMembers = 24
		strategy := DefaultSubstitutionStrategy(DefaultHomeSubMinute)
		base := meanFinal(t, strategy, steps, nMembers, "score_events", 0, nil)
		strong := meanFinal(t, strategy, steps, nMembers, "score_events", 0, func(g *simulator.ConfigGenerator) {
			g.GetPartition("score_rates").Params.Map["coefficients"] = scoreCoeffsWith(func(c []float64) {
				c[0] = -2.5 // raise home try intercept from -3.0
			})
		})
		if !(strong > base) {
			t.Fatalf("expected a higher try intercept to raise home tries: "+
				"base(-3.0)=%.3f strong(-2.5)=%.3f", base, strong)
		}
	})

	// The tries → points channel: a higher home conversion probability turns the
	// same tries into more points, raising the home score. Home score is
	// match_state index 0.
	t.Run("higher_conversion_probability_raises_home_score", func(t *testing.T) {
		const nMembers = 24
		strategy := DefaultSubstitutionStrategy(DefaultHomeSubMinute)
		base := meanFinal(t, strategy, steps, nMembers, "match_state", StateIdxHomeScore, nil)
		high := meanFinal(t, strategy, steps, nMembers, "match_state", StateIdxHomeScore, func(g *simulator.ConfigGenerator) {
			g.GetPartition("conversion_events").Params.Map["conversion_probs"] = []float64{0.98, DefaultAwayConversionProb}
		})
		if !(high > base) {
			t.Fatalf("expected a higher conversion probability to raise home score: "+
				"base(0.75)=%.3f high(0.98)=%.3f", base, high)
		}
	})

	// Structural sensitivity of the substitution mechanism: with the same
	// substitution timing, a larger per-group "fresh legs" coefficient produces a
	// bigger scoring gain. The model was not tuned on this effect size — getting the
	// sign right is a credibility check. Home try covariates are indices 1..4.
	t.Run("stronger_substitution_effect_raises_home_tries", func(t *testing.T) {
		const nMembers = 30
		strategy := homeSubsAll(20, DefaultAwaySubMinute) // early sub so the boost window is large
		base := meanFinal(t, strategy, steps, nMembers, "score_events", 0, nil)
		strong := meanFinal(t, strategy, steps, nMembers, "score_events", 0, func(g *simulator.ConfigGenerator) {
			g.GetPartition("score_rates").Params.Map["coefficients"] = scoreCoeffsWith(func(c []float64) {
				for j := 1; j <= NumPositionGroups; j++ {
					c[j] = 0.4 // stronger home try covariate effect (from 0.15)
				}
			})
		})
		if !(strong > base) {
			t.Fatalf("expected a stronger substitution effect to raise home tries: "+
				"base(0.15)=%.3f strong(0.40)=%.3f", base, strong)
		}
	})

	// Home advantage: with substitutions held symmetric (both sides sub at the same
	// minute), the default home-favouring try intercept makes the home side outscore
	// the away side on tries, on average. Isolates the structural home edge from any
	// substitution asymmetry.
	t.Run("home_advantage_intercept_makes_home_outscore_away", func(t *testing.T) {
		const nMembers = 40
		symmetric := homeSubsAll(DefaultHomeSubMinute, DefaultHomeSubMinute)
		homeTries := meanFinal(t, symmetric, steps, nMembers, "score_events", 0, nil)
		awayTries := meanFinal(t, symmetric, steps, nMembers, "score_events", 1, nil)
		if !(homeTries > awayTries) {
			t.Fatalf("expected home advantage to make home outscore away on tries under symmetric subs: "+
				"home=%.3f away=%.3f", homeTries, awayTries)
		}
	})

	// The independent card counting-process channel: a higher yellow-card intercept
	// (a stricter referee / dirtier match, set by the world) produces more yellow
	// cards. Home yellow intercept is card coefficient index 0; home yellows are
	// card_events index 0.
	t.Run("higher_yellow_card_intercept_raises_cards", func(t *testing.T) {
		const nMembers = 30
		strategy := DefaultSubstitutionStrategy(DefaultHomeSubMinute)
		base := meanFinal(t, strategy, steps, nMembers, "card_events", 0, nil)
		strict := meanFinal(t, strategy, steps, nMembers, "card_events", 0, func(g *simulator.ConfigGenerator) {
			c := DefaultCardCoefficients()
			c[0] = -3.3 // raise home yellow intercept from -4.5
			g.GetPartition("card_rates").Params.Map["coefficients"] = c
		})
		if !(strict > base) {
			t.Fatalf("expected a higher yellow-card intercept to raise cards: "+
				"base(-4.5)=%.3f strict(-3.3)=%.3f", base, strict)
		}
	})
}
