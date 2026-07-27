package agents_test

// Expected-behaviour tests for planning: each subtest is a named response claim
// about how the planner reacts to a lever a user actually sets, asserted in the
// direction a user would expect. A wrong sign here is a wrong recommendation.
//
// The example problem is a sooner-versus-later choice, which is the smallest
// setup that makes horizon and discount both matter:
//
//	take   — pays 5 now, and can be taken again next step
//	invest — pays nothing now, then 20 two steps later
//
// Over four steps, taking every step earns 20 while investing first earns 25, so
// the better plan is only visible to a planner that looks far enough ahead and
// does not discount the future away.

import (
	"math"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

const (
	takeNow    = 0
	investLate = 1
)

// delayedPayoffIteration is the sooner-versus-later payoff.
//
// Row: [payout, countdown]. A countdown in progress must finish before another
// choice matters, which is what makes investing cost the steps it does.
type delayedPayoffIteration struct{}

func (d *delayedPayoffIteration) Configure(int, *simulator.Settings) {}

func (d *delayedPayoffIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	row := stateHistories[partitionIndex].Values.RawRowView(0)
	countdown := row[1]
	choice := params.GetIndex("choice", 0)

	if countdown > 0 {
		remaining := countdown - 1
		if remaining == 0 {
			return []float64{20, 0} // the investment matures
		}
		return []float64{0, remaining}
	}
	if choice == investLate {
		return []float64{0, 2}
	}
	return []float64{5, 0}
}

// newDelayedPayoffEnvironment builds the choice over the given horizon and
// discount.
func newDelayedPayoffEnvironment(
	t *testing.T,
	horizon int,
	discount float64,
	returnRange [2]float64,
) *agents.SimulationEnvironment {
	t.Helper()
	gen := simulator.NewConfigGenerator()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "choice",
		Iteration:         &general.ParamValuesIteration{},
		Params:            simulator.NewParams(map[string][]float64{"param_values": {0}}),
		InitStateValues:   []float64{0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "payout",
		Iteration: &delayedPayoffIteration{},
		Params:    simulator.NewParams(map[string][]float64{"choice": {0}}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"choice": {Upstream: "choice"},
		},
		InitStateValues:   []float64{0, 0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	impl.ExecutionStrategy = &simulator.InlineExecution{}

	return agents.NewSimulationEnvironment(settings, impl, agents.SimulationEnvironmentSpec{
		Actions:         [][]float64{{takeNow}, {investLate}},
		ActionPartition: "choice",
		ActionParam:     "param_values",
		Horizon:         horizon,
		Reward:          func(rows map[string][]float64) float64 { return rows["payout"][0] },
		Discount:        discount,
		MinReturn:       returnRange[0],
		MaxReturn:       returnRange[1],
		ScenarioSeed:    2,
	})
}

// openingChoice plans from the start and reports the first action taken.
func openingChoice(t *testing.T, env *agents.SimulationEnvironment, sims int) int {
	t.Helper()
	cfg := agents.MCTSConfig[[]float64, int]{
		Simulations:     sims,
		MaxTreeDepth:    8,
		RolloutMaxSteps: 8,
		Rollout: agents.FromProgress(
			agents.UniformRandomRollout[[]float64, int](), env.Progress),
	}
	best, _, err := agents.RunMCTSSearch(env, env.InitialState(), cfg, 41, sims)
	if err != nil {
		t.Fatalf("RunMCTSSearch: %v", err)
	}
	return best
}

// planReturn plans the whole episode MPC-style and reports what it earned.
func planReturn(t *testing.T, env *agents.SimulationEnvironment, sims int) float64 {
	t.Helper()
	cfg := agents.MCTSConfig[[]float64, int]{
		Simulations:     sims,
		MaxTreeDepth:    8,
		RolloutMaxSteps: 8,
		Rollout: agents.FromProgress(
			agents.UniformRandomRollout[[]float64, int](), env.Progress),
	}
	state := env.InitialState()
	for step := 0; ; step++ {
		if _, done := env.Terminal(state); done {
			return env.Return(state)
		}
		best, _, err := agents.RunMCTSSearch(env, state, cfg, uint64(step)+41, sims)
		if err != nil {
			t.Fatalf("RunMCTSSearch: %v", err)
		}
		state, err = env.Apply(state, best)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}
}

func TestPlanningBehaviour(t *testing.T) {
	const sims = 400
	wide := [2]float64{-100, 100}

	t.Run("a_longer_horizon_captures_a_later_payoff", func(t *testing.T) {
		// One step ahead, investing looks like pure loss: its payout is outside
		// what the planner can see.
		short := openingChoice(t, newDelayedPayoffEnvironment(t, 1, 1, wide), sims)
		if short != takeNow {
			t.Errorf("with a one-step horizon the planner should take the sure 5, it invested")
		}
		long := openingChoice(t, newDelayedPayoffEnvironment(t, 4, 1, wide), sims)
		if long != investLate {
			t.Errorf("with a four-step horizon investing is worth 25 against 20 for taking, " +
				"but the planner took")
		}
	})

	t.Run("a_heavier_discount_prefers_the_sooner_payoff", func(t *testing.T) {
		// Undiscounted the delayed 20 wins; discounted hard it is worth 20*0.5^2 = 5
		// at best, so the immediate 5 is no worse and arrives sooner.
		patient := openingChoice(t, newDelayedPayoffEnvironment(t, 4, 1, wide), sims)
		impatient := openingChoice(t, newDelayedPayoffEnvironment(t, 4, 0.5, wide), sims)
		if patient != investLate {
			t.Errorf("an undiscounted planner should invest, it took")
		}
		if impatient != takeNow {
			t.Errorf("a heavily discounted planner should take the sooner payoff, it invested")
		}
	})

	t.Run("widening_the_return_range_does_not_change_the_choice", func(t *testing.T) {
		// return_range only sets the normalisation UCB1 needs. Widening it makes
		// every score smaller but must not reorder the actions — if it does, the
		// search is reading the scale rather than the problem.
		narrow := planReturn(t, newDelayedPayoffEnvironment(t, 4, 1, [2]float64{-40, 40}), sims)
		broad := planReturn(t, newDelayedPayoffEnvironment(t, 4, 1, [2]float64{-4000, 4000}), sims)
		if narrow != broad {
			t.Errorf("return range changed the plan: %v against %v", narrow, broad)
		}
	})

	t.Run("more_simulations_does_not_lose_value", func(t *testing.T) {
		// The problem is deterministic and small enough that a bigger budget
		// should never plan worse.
		previous := math.Inf(-1)
		for _, budget := range []int{50, 200, 800} {
			earned := planReturn(t, newDelayedPayoffEnvironment(t, 4, 1, wide), budget)
			t.Logf("  %d simulations earned %v", budget, earned)
			if earned < previous {
				t.Errorf("raising the budget to %d lost value: %v after %v",
					budget, earned, previous)
			}
			previous = earned
		}
	})

	t.Run("the_planner_beats_always_taking", func(t *testing.T) {
		env := newDelayedPayoffEnvironment(t, 4, 1, wide)
		planned := planReturn(t, env, sims)
		greedy := fixedPolicyReturn(t, newDelayedPayoffEnvironment(t, 4, 1, wide), takeNow)
		t.Logf("planned %v against always-taking %v", planned, greedy)
		if planned <= greedy {
			t.Errorf("planning earned %v, no better than always taking (%v)", planned, greedy)
		}
	})
}
