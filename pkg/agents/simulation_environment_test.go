package agents_test

// Validation for SimulationEnvironment against a known-answer planning problem:
// battery arbitrage over a cyclic price. The environment is deterministic (its
// noise is pinned by the state-action seed), so the optimal return can be found
// exactly by brute force over every action sequence — which is what these tests
// compare the search against, rather than a hand-reasoned figure.
//
// The problem is deliberately one where a myopic policy loses: selling requires
// having bought earlier, at a price that looked bad at the time.

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/agents"
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// priceCycle is the repeating price path: two cheap steps, then two dear ones.
var priceCycle = []float64{10, 10, 90, 90}

// cyclicPriceIteration walks priceCycle. The phase is carried in the row rather
// than read from the coordinator's step counter, because SimulationEnvironment
// restarts the coordinator for every transition — the sub-simulation has to be
// Markov in its own rows.
//
// Row: [price, phase].
type cyclicPriceIteration struct{}

func (p *cyclicPriceIteration) Configure(int, *simulator.Settings) {}

func (p *cyclicPriceIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	row := stateHistories[partitionIndex].Values.RawRowView(0)
	phase := math.Mod(row[1]+1, float64(len(priceCycle)))
	return []float64{priceCycle[int(phase)], phase}
}

// batteryIteration applies the dispatch action to the state of charge, clamped
// to [0, capacity], and reports the flow that actually happened.
//
// Row: [soc, flow]; params: "dispatch" of +1 charge / 0 hold / -1 discharge.
type batteryIteration struct{ capacity float64 }

func (b *batteryIteration) Configure(int, *simulator.Settings) {}

func (b *batteryIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	row := stateHistories[partitionIndex].Values.RawRowView(0)
	soc := row[0]
	next := math.Min(b.capacity, math.Max(0, soc+params.GetIndex("dispatch", 0)))
	return []float64{next, next - soc}
}

const testHorizon = 8

func newBatteryEnvironment(t *testing.T, scenarioSeed uint64) *agents.SimulationEnvironment {
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
		Name:      "price",
		Iteration: &cyclicPriceIteration{},
		// Start at phase 3 so the first transition lands on phase 0 (cheap).
		InitStateValues:   []float64{priceCycle[3], 3},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "battery",
		Iteration:         &batteryIteration{capacity: 1},
		Params:            simulator.NewParams(map[string][]float64{"dispatch": {0}}),
		InitStateValues:   []float64{0, 0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	impl.ExecutionStrategy = &simulator.InlineExecution{}

	return agents.NewSimulationEnvironment(settings, impl, agents.SimulationEnvironmentSpec{
		Actions:         [][]float64{{1}, {0}, {-1}}, // charge / hold / discharge
		ActionPartition: "battery",
		ActionParam:     "dispatch",
		Horizon:         testHorizon,
		// Revenue: discharging (flow < 0) sells at the prevailing price.
		Reward: func(rows map[string][]float64) float64 {
			return -rows["battery"][1] * rows["price"][0]
		},
		MinReturn:    -400,
		MaxReturn:    400,
		ScenarioSeed: scenarioSeed,
	})
}

// bestReturnByBruteForce enumerates every action sequence and returns the best
// achievable accumulated reward. Exact, because the environment is deterministic.
func bestReturnByBruteForce(t *testing.T, env *agents.SimulationEnvironment) float64 {
	t.Helper()
	best := math.Inf(-1)
	var walk func(state []float64, depth int)
	walk = func(state []float64, depth int) {
		if _, done := env.Terminal(state); done {
			best = math.Max(best, env.Return(state))
			return
		}
		for _, action := range env.Legal(state) {
			next, err := env.Apply(state, action)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			walk(next, depth+1)
		}
	}
	walk(env.InitialState(), 0)
	return best
}

func TestSimulationEnvironmentApplyIsPure(t *testing.T) {
	env := newBatteryEnvironment(t, 1)
	state := env.InitialState()
	first, err := env.Apply(state, 0)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Interleave other transitions, then repeat the original.
	if _, err := env.Apply(state, 2); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := env.Apply(first, 1); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	again, err := env.Apply(state, 0)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	for i := range first {
		if first[i] != again[i] {
			t.Fatalf("Apply is not pure: %v vs %v", first, again)
		}
	}
	// And it must not have mutated the state it was given.
	for i, value := range env.InitialState() {
		if state[i] != value {
			t.Fatalf("Apply mutated its input state at %d: %v", i, state)
		}
	}
}

// TestSimulationEnvironmentMCTSFindsOptimalPlan is the headline check: planning
// with MCTS over a sub-simulation recovers the brute-force optimum. The battery
// must be charged while the price is low to have anything to sell when it is
// high, so a myopic policy cannot reach this return — the comparison against
// hold-forever and greedy-discharge below is what makes that concrete.
func TestSimulationEnvironmentMCTSFindsOptimalPlan(t *testing.T) {
	env := newBatteryEnvironment(t, 1)
	optimal := bestReturnByBruteForce(t, env)
	if optimal <= 0 {
		t.Fatalf("problem is degenerate: brute-force optimum is %v", optimal)
	}

	cfg := agents.MCTSConfig[[]float64, int]{
		Simulations:     400,
		MaxTreeDepth:    testHorizon + 1,
		RolloutMaxSteps: testHorizon + 1,
		Rollout:         agents.UniformRandomRollout[[]float64, int](),
	}

	// Replan from each state reached, MPC-style: the search chooses one action,
	// the environment advances, and the search runs again.
	state := env.InitialState()
	for step := 0; ; step++ {
		if _, done := env.Terminal(state); done {
			break
		}
		best, _, err := agents.RunMCTSSearch(env, state, cfg, uint64(step)+99, cfg.Simulations)
		if err != nil {
			t.Fatalf("RunMCTSSearch: %v", err)
		}
		state, err = env.Apply(state, best)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}
	planned := env.Return(state)

	holdReturn := fixedPolicyReturn(t, env, 1)
	t.Logf("brute-force optimum %.0f, MCTS %.0f, hold-forever %.0f", optimal, planned, holdReturn)
	if planned < optimal {
		t.Logf("MCTS left %.0f on the table (%.1f%% of optimum)",
			optimal-planned, 100*planned/optimal)
	}
	if planned < 0.9*optimal {
		t.Errorf("MCTS plan returned %v, want at least 90%% of the optimum %v", planned, optimal)
	}
	if planned <= holdReturn {
		t.Errorf("MCTS (%v) did no better than doing nothing (%v)", planned, holdReturn)
	}
}

// fixedPolicyReturn runs one action for the whole episode, as a floor to beat.
func fixedPolicyReturn(t *testing.T, env *agents.SimulationEnvironment, action int) float64 {
	t.Helper()
	state := env.InitialState()
	for {
		if _, done := env.Terminal(state); done {
			return env.Return(state)
		}
		next, err := env.Apply(state, action)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		state = next
	}
}

// countingIteration increments a counter each step, so the contents of its
// history window are predictable: [n, n-1, n-2, ...].
type countingIteration struct{}

func (c *countingIteration) Configure(int, *simulator.Settings) {}

func (c *countingIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	return []float64{stateHistories[partitionIndex].Values.RawRowView(0)[0] + 1}
}

// lookbackIteration reports the OLDEST row of the counter's window, which is only
// correct if the whole window survived the transition. This is the shape a real
// model uses for a rolling count — trywizard's match state counts yellow cards
// active in the last ten minutes exactly this way.
type lookbackIteration struct{ depth int }

func (l *lookbackIteration) Configure(int, *simulator.Settings) {}

func (l *lookbackIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	counter := stateHistories[0]
	return []float64{counter.Values.RawRowView(counter.StateHistoryDepth - 1)[0]}
}

// TestSimulationEnvironmentCarriesDeepHistory checks that a partition reading
// further back than one step plans correctly. Each transition restarts the
// sub-simulation, so unless the whole window is carried in the encoded state and
// handed back, an iteration looking N steps into the past sees the initial value
// forever. Real models need this: a rolling ten-minute count is a deep window.
func TestSimulationEnvironmentCarriesDeepHistory(t *testing.T) {
	const depth = 3
	gen := simulator.NewConfigGenerator()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "counter",
		Iteration:         &countingIteration{},
		Params:            simulator.NewParams(map[string][]float64{"unused": {0}}),
		InitStateValues:   []float64{0},
		StateHistoryDepth: depth,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "lookback",
		Iteration:         &lookbackIteration{depth: depth},
		InitStateValues:   []float64{0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	impl.ExecutionStrategy = &simulator.InlineExecution{}

	env := agents.NewSimulationEnvironment(settings, impl, agents.SimulationEnvironmentSpec{
		Actions:         [][]float64{{0}},
		ActionPartition: "counter",
		ActionParam:     "unused",
		Horizon:         6,
		Reward:          func(map[string][]float64) float64 { return 0 },
		MinReturn:       -1,
		MaxReturn:       1,
	})

	// Encoded layout: [step, return, counter window (depth), lookback (1)].
	if got, want := env.StateWidth(), 2+depth+1; got != want {
		t.Fatalf("state width %d, want %d (the window has to be in the state)", got, want)
	}

	state := env.InitialState()
	for step := 1; step <= 5; step++ {
		next, err := env.Apply(state, 0)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		state = next

		window := state[2 : 2+depth]
		for row := 0; row < depth; row++ {
			want := float64(step - row)
			if want < 0 {
				want = 0
			}
			if window[row] != want {
				t.Fatalf("step %d: window %v, want row %d to be %v — the history is "+
					"not being carried across transitions", step, window, row, want)
			}
		}
		// An iteration reading the oldest row must see the real history, not the
		// initial value. It reads the window as it stands at the START of the
		// step — before the counter advances — so at step N the oldest row it
		// sees is N-depth, floored at the initial 0.
		wantOldest := math.Max(0, float64(step-depth))
		if got := state[2+depth]; got != wantOldest {
			t.Fatalf("step %d: an iteration looking %d steps back read %v, want %v — "+
				"it is seeing the initial value instead of the carried history",
				step, depth, got, wantOldest)
		}
	}
}

// noisyPriceIteration is cyclicPriceIteration with Gaussian noise on the price.
// The RNG is rebuilt in Configure from the partition seed, which is exactly the
// contract SimulationEnvironment relies on: because the environment derives that
// seed from (scenario, state, action), the noise is pinned per transition.
type noisyPriceIteration struct {
	amplitude float64
	rng       *rand.Rand
}

func (p *noisyPriceIteration) Configure(partitionIndex int, settings *simulator.Settings) {
	p.rng = rand.New(rand.NewPCG(settings.Iterations[partitionIndex].Seed, 0x5eed))
}

func (p *noisyPriceIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	row := stateHistories[partitionIndex].Values.RawRowView(0)
	phase := math.Mod(row[1]+1, float64(len(priceCycle)))
	return []float64{priceCycle[int(phase)] + p.amplitude*p.rng.NormFloat64(), phase}
}

func newNoisyBatteryEnvironment(t *testing.T, scenarioSeed uint64) *agents.SimulationEnvironment {
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
		Name:              "price",
		Iteration:         &noisyPriceIteration{amplitude: 25},
		InitStateValues:   []float64{priceCycle[3], 3},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "battery",
		Iteration:         &batteryIteration{capacity: 1},
		Params:            simulator.NewParams(map[string][]float64{"dispatch": {0}}),
		InitStateValues:   []float64{0, 0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	impl.ExecutionStrategy = &simulator.InlineExecution{}

	return agents.NewSimulationEnvironment(settings, impl, agents.SimulationEnvironmentSpec{
		Actions:         [][]float64{{1}, {0}, {-1}},
		ActionPartition: "battery",
		ActionParam:     "dispatch",
		Horizon:         testHorizon,
		Reward: func(rows map[string][]float64) float64 {
			return -rows["battery"][1] * rows["price"][0]
		},
		MinReturn:    -400,
		MaxReturn:    400,
		ScenarioSeed: scenarioSeed,
	})
}

// planActions searches from each state in turn and returns the chosen sequence
// together with the return the planner believed it would get.
func planActions(t *testing.T, env *agents.SimulationEnvironment, sims int) ([]int, float64) {
	t.Helper()
	cfg := agents.MCTSConfig[[]float64, int]{
		Simulations:     sims,
		MaxTreeDepth:    testHorizon + 1,
		RolloutMaxSteps: testHorizon + 1,
		Rollout:         agents.UniformRandomRollout[[]float64, int](),
	}
	state := env.InitialState()
	actions := make([]int, 0, testHorizon)
	for step := 0; ; step++ {
		if _, done := env.Terminal(state); done {
			return actions, env.Return(state)
		}
		best, _, err := agents.RunMCTSSearch(env, state, cfg, uint64(step)+99, sims)
		if err != nil {
			t.Fatalf("RunMCTSSearch: %v", err)
		}
		actions = append(actions, best)
		state, err = env.Apply(state, best)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}
}

// replayActions runs a fixed action sequence in an environment, returning the
// accumulated reward it actually earns there.
func replayActions(t *testing.T, env *agents.SimulationEnvironment, actions []int) float64 {
	t.Helper()
	state := env.InitialState()
	for _, action := range actions {
		if _, done := env.Terminal(state); done {
			break
		}
		next, err := env.Apply(state, action)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		state = next
	}
	return env.Return(state)
}

// TestSimulationEnvironmentOptimismGap measures the cost of pinning the noise.
//
// Planning against a fixed scenario seed lets the search exploit the particular
// noise draw it is going to receive, so the return it predicts is optimistic
// relative to what the same plan earns under other draws. This test quantifies
// that gap rather than leaving it as a caveat in a doc comment: it plans under
// one scenario, then replays the identical action sequence under others.
//
// The assertion is deliberately about the direction and the mechanism, not a
// magnitude — the magnitude is what gets reported, and is the number that should
// inform whether a chance-node treatment is worth building.
func TestSimulationEnvironmentOptimismGap(t *testing.T) {
	planningEnv := newNoisyBatteryEnvironment(t, 1)
	actions, believed := planActions(t, planningEnv, 400)

	realised := make([]float64, 0, 12)
	total := 0.0
	for seed := uint64(2); seed < 14; seed++ {
		value := replayActions(t, newNoisyBatteryEnvironment(t, seed), actions)
		realised = append(realised, value)
		total += value
	}
	mean := total / float64(len(realised))

	spread := 0.0
	for _, value := range realised {
		spread += (value - mean) * (value - mean)
	}
	spread = math.Sqrt(spread / float64(len(realised)))

	t.Logf("planned return under its own scenario: %.1f", believed)
	t.Logf("same plan under 12 other scenarios:    mean %.1f, sd %.1f", mean, spread)
	t.Logf("optimism gap: %.1f (%.0f%% of the planned figure)",
		believed-mean, 100*(believed-mean)/math.Abs(believed))

	if spread == 0 {
		t.Fatal("replays produced identical returns; the noise is not actually varying " +
			"per scenario, so this test cannot measure anything")
	}
	if believed <= mean {
		t.Logf("NOTE: no optimism observed on this seed — the gap is a bias in " +
			"expectation, not a guarantee for any single plan")
	}
}

// BenchmarkSimulationEnvironmentApply records the per-transition cost, which is
// what bounds how much search a plan can afford. One MCTS iteration costs
// roughly one Apply per rollout step, so a 400-simulation search over an
// 8-step horizon is on the order of 3,200 of these.
func BenchmarkSimulationEnvironmentApply(b *testing.B) {
	env := newBatteryEnvironment(&testing.T{}, 1)
	state := env.InitialState()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := env.Apply(state, i%3); err != nil {
			b.Fatal(err)
		}
	}
}

// planActionsWithChanceNodes is planActions using the chance-node search, so each
// action's value is an average over sampled successors.
func planActionsWithChanceNodes(
	t *testing.T,
	env *agents.SimulationEnvironment,
	sims int,
) ([]int, float64) {
	t.Helper()
	cfg := agents.MCTSConfig[[]float64, int]{
		Simulations:     sims,
		MaxTreeDepth:    testHorizon + 1,
		RolloutMaxSteps: testHorizon + 1,
		Rollout:         agents.UniformRandomRollout[[]float64, int](),
	}
	state := env.InitialState()
	actions := make([]int, 0, testHorizon)
	for step := 0; ; step++ {
		if _, done := env.Terminal(state); done {
			return actions, env.Return(state)
		}
		best, _, err := agents.RunChanceMCTSSearch(env, state, cfg, uint64(step)+99, sims)
		if err != nil {
			t.Fatalf("RunChanceMCTSSearch: %v", err)
		}
		actions = append(actions, best)
		state, err = env.Apply(state, best)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}
}

// meanOptimism measures how much a planner over-promises, averaged over the
// scenario it plans in.
//
// For each planning scenario it records what the plan earned THERE, then replays
// the same actions in other scenarios and takes the mean. The difference is the
// planner's advantage from having known its own noise. Averaging over planning
// scenarios is what makes the number meaningful: a single planning scenario is
// one draw, so its own-scenario return is mostly noise for any planner that is
// not exploiting it — which is precisely the planner being tested.
func meanOptimism(
	t *testing.T,
	plan func(*testing.T, *agents.SimulationEnvironment, int) ([]int, float64),
	sims int,
) (ownScenario, crossScenario, optimism float64) {
	t.Helper()
	const planningScenarios = 6
	const replayScenarios = 8
	for planSeed := uint64(1); planSeed <= planningScenarios; planSeed++ {
		actions, own := plan(t, newNoisyBatteryEnvironment(t, planSeed), sims)
		ownScenario += own
		total := 0.0
		for offset := uint64(0); offset < replayScenarios; offset++ {
			replaySeed := 100 + planSeed*replayScenarios + offset
			total += replayActions(t, newNoisyBatteryEnvironment(t, replaySeed), actions)
		}
		crossScenario += total / replayScenarios
	}
	ownScenario /= planningScenarios
	crossScenario /= planningScenarios
	return ownScenario, crossScenario, ownScenario - crossScenario
}

// TestChanceNodesReduceOptimism is the reason MCTSChanceTree exists. Planning
// against a pinned noise realisation lets the search exploit the draw it is going
// to receive, so what it achieves in its own scenario overstates what the same
// plan earns anywhere else. Averaging over sampled outcomes at chance nodes
// should shrink that advantage towards zero.
//
// The assertion is about CALIBRATION, not return: a planner that earns the same
// but does not over-promise is the improvement being bought here.
func TestChanceNodesReduceOptimism(t *testing.T) {
	const sims = 400

	pinnedOwn, pinnedCross, pinnedOptimism := meanOptimism(t, planActions, sims)
	chanceOwn, chanceCross, chanceOptimism := meanOptimism(
		t, planActionsWithChanceNodes, sims)

	t.Logf("pinned noise:  own scenario %6.1f, other scenarios %6.1f, optimism %6.1f",
		pinnedOwn, pinnedCross, pinnedOptimism)
	t.Logf("chance nodes:  own scenario %6.1f, other scenarios %6.1f, optimism %6.1f",
		chanceOwn, chanceCross, chanceOptimism)

	if math.Abs(chanceOptimism) >= math.Abs(pinnedOptimism) {
		t.Errorf("chance nodes did not improve calibration: optimism %.1f vs %.1f",
			chanceOptimism, pinnedOptimism)
	}
}

// TestSimulationEnvironmentGuards covers the misconfigurations and bad calls
// that would otherwise produce a plausible-looking but meaningless plan.
func TestSimulationEnvironmentGuards(t *testing.T) {
	env := newBatteryEnvironment(t, 1)
	state := env.InitialState()

	t.Run("an out-of-range action is an error", func(t *testing.T) {
		if _, err := env.Apply(state, 99); err == nil {
			t.Error("expected an error for an action outside the set")
		}
		if _, err := env.Apply(state, -1); err == nil {
			t.Error("expected an error for a negative action")
		}
	})

	t.Run("a mis-sized state is an error", func(t *testing.T) {
		if _, err := env.Apply([]float64{1, 2}, 0); err == nil {
			t.Error("expected an error for a state of the wrong width")
		}
	})

	t.Run("a terminal state offers no actions", func(t *testing.T) {
		terminal := append([]float64(nil), state...)
		terminal[0] = testHorizon
		if legal := env.Legal(terminal); len(legal) != 0 {
			t.Errorf("expected no legal actions at the horizon, got %v", legal)
		}
	})

	t.Run("single decision maker", func(t *testing.T) {
		if got := env.Players(state); got != 1 {
			t.Errorf("Players = %d, want 1", got)
		}
		if got := env.Actor(state); got != 0 {
			t.Errorf("Actor = %d, want 0", got)
		}
	})

	for _, testCase := range []struct {
		name   string
		mutate func(*agents.SimulationEnvironmentSpec)
	}{
		{"no actions", func(s *agents.SimulationEnvironmentSpec) { s.Actions = nil }},
		{"no horizon", func(s *agents.SimulationEnvironmentSpec) { s.Horizon = 0 }},
		{"no reward", func(s *agents.SimulationEnvironmentSpec) { s.Reward = nil }},
		{"empty return range", func(s *agents.SimulationEnvironmentSpec) {
			s.MinReturn, s.MaxReturn = 1, 1
		}},
		{"unknown action partition", func(s *agents.SimulationEnvironmentSpec) {
			s.ActionPartition = "absent"
		}},
	} {
		t.Run(testCase.name+" is rejected", func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("expected a panic for %s", testCase.name)
				}
			}()
			gen := simulator.NewConfigGenerator()
			gen.SetSimulation(&simulator.SimulationConfig{
				OutputCondition:      &simulator.NilOutputCondition{},
				OutputFunction:       &simulator.NilOutputFunction{},
				TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
				TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
				InitTimeValue:        0,
			})
			gen.SetPartition(&simulator.PartitionConfig{
				Name:              "battery",
				Iteration:         &batteryIteration{capacity: 1},
				Params:            simulator.NewParams(map[string][]float64{"dispatch": {0}}),
				InitStateValues:   []float64{0, 0},
				StateHistoryDepth: 1,
				Seed:              0,
			})
			settings, impl := gen.GenerateConfigs()
			spec := agents.SimulationEnvironmentSpec{
				Actions:         [][]float64{{0}},
				ActionPartition: "battery",
				ActionParam:     "dispatch",
				Horizon:         2,
				Reward:          func(map[string][]float64) float64 { return 0 },
				MinReturn:       -1,
				MaxReturn:       1,
			}
			testCase.mutate(&spec)
			agents.NewSimulationEnvironment(settings, impl, spec)
		})
	}
}

// TestSimulationEnvironmentLegalFilter checks a spec-supplied Legal narrows the
// action set, which is how a model states that an action is unavailable in some
// states — a battery that cannot discharge when empty, say.
func TestSimulationEnvironmentLegalFilter(t *testing.T) {
	gen := simulator.NewConfigGenerator()
	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition:      &simulator.NilOutputCondition{},
		OutputFunction:       &simulator.NilOutputFunction{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{MaxNumberOfSteps: 1},
		TimestepFunction:     &simulator.ConstantTimestepFunction{Stepsize: 1.0},
		InitTimeValue:        0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "battery",
		Iteration:         &batteryIteration{capacity: 1},
		Params:            simulator.NewParams(map[string][]float64{"dispatch": {0}}),
		InitStateValues:   []float64{0, 0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	env := agents.NewSimulationEnvironment(settings, impl, agents.SimulationEnvironmentSpec{
		Actions:         [][]float64{{1}, {0}, {-1}},
		ActionPartition: "battery",
		ActionParam:     "dispatch",
		Horizon:         4,
		Reward:          func(map[string][]float64) float64 { return 0 },
		MinReturn:       -1,
		MaxReturn:       1,
		// Discharging is unavailable while the battery is empty.
		Legal: func(rows map[string][]float64) []int {
			if rows["battery"][0] <= 0 {
				return []int{0, 1}
			}
			return []int{0, 1, 2}
		},
	})

	if legal := env.Legal(env.InitialState()); len(legal) != 2 {
		t.Errorf("empty battery should offer 2 actions, got %v", legal)
	}
	charged, err := env.Apply(env.InitialState(), 0)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if legal := env.Legal(charged); len(legal) != 3 {
		t.Errorf("charged battery should offer 3 actions, got %v", legal)
	}
}

// newLongBatteryEnvironment is the battery problem over a longer horizon, so a
// rollout capped well below it truncates on almost every simulation.
func newLongBatteryEnvironment(t *testing.T, horizon int) *agents.SimulationEnvironment {
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
		Name:              "price",
		Iteration:         &cyclicPriceIteration{},
		InitStateValues:   []float64{priceCycle[3], 3},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:              "battery",
		Iteration:         &batteryIteration{capacity: 1},
		Params:            simulator.NewParams(map[string][]float64{"dispatch": {0}}),
		InitStateValues:   []float64{0, 0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	impl.ExecutionStrategy = &simulator.InlineExecution{}
	return agents.NewSimulationEnvironment(settings, impl, agents.SimulationEnvironmentSpec{
		Actions:         [][]float64{{1}, {0}, {-1}},
		ActionPartition: "battery",
		ActionParam:     "dispatch",
		Horizon:         horizon,
		Reward: func(rows map[string][]float64) float64 {
			return -rows["battery"][1] * rows["price"][0]
		},
		MinReturn:    -100 * float64(horizon),
		MaxReturn:    100 * float64(horizon),
		ScenarioSeed: 1,
	})
}

// planWithRollout plans MPC-style under a supplied rollout and returns the
// achieved return.
func planWithRollout(
	t *testing.T,
	env *agents.SimulationEnvironment,
	rollout agents.MCTSRolloutFn[[]float64, int],
	rolloutMaxSteps, sims int,
) float64 {
	t.Helper()
	cfg := agents.MCTSConfig[[]float64, int]{
		Simulations:     sims,
		MaxTreeDepth:    6,
		RolloutMaxSteps: rolloutMaxSteps,
		Rollout:         rollout,
	}
	state := env.InitialState()
	for step := 0; ; step++ {
		if _, done := env.Terminal(state); done {
			return env.Return(state)
		}
		best, _, err := agents.RunChanceMCTSSearch(env, state, cfg, uint64(step)+5, sims)
		if err != nil {
			t.Fatalf("RunChanceMCTSSearch: %v", err)
		}
		state, err = env.Apply(state, best)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}
}

// TestProgressProxyScoresTruncatedRollouts is the reason SimulationEnvironment
// has a Progress proxy. When the horizon is much longer than a rollout is
// allowed to run, a plain rollout truncates without a score on nearly every
// simulation, leaving the search to explore on visit counts alone. Scoring the
// truncated state instead should plan better.
func TestProgressProxyScoresTruncatedRollouts(t *testing.T) {
	const horizon = 24
	const rolloutMaxSteps = 3 // far below the horizon: nearly every rollout truncates
	const sims = 300

	plain := planWithRollout(t, newLongBatteryEnvironment(t, horizon),
		agents.UniformRandomRollout[[]float64, int](), rolloutMaxSteps, sims)

	scored := newLongBatteryEnvironment(t, horizon)
	withProgress := planWithRollout(t, scored,
		agents.FromProgress(
			agents.UniformRandomRollout[[]float64, int](), scored.Progress),
		rolloutMaxSteps, sims)

	t.Logf("horizon %d, rollout capped at %d: plain %.0f, progress-scored %.0f",
		horizon, rolloutMaxSteps, plain, withProgress)

	if withProgress <= plain {
		t.Errorf("progress scoring did not help: %.0f vs %.0f", withProgress, plain)
	}
}

// newsvendorIteration is the classic order-quantity payoff: you order q before
// demand is known, sell what you can, and eat the cost of the rest.
//
//	profit = price * min(q, demand) - cost * q
//
// Row: [profit]; params: "quantity" (the action) and "demand" (the uncertain
// parameter a posterior sample supplies).
type newsvendorIteration struct{ price, cost float64 }

func (n *newsvendorIteration) Configure(int, *simulator.Settings) {}

func (n *newsvendorIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	quantity := params.GetIndex("quantity", 0)
	demand := params.GetIndex("demand", 0)
	return []float64{n.price*math.Min(quantity, demand) - n.cost*quantity}
}

// orderQuantities are the actions; demandPosterior is a deliberately skewed
// posterior — usually 10, occasionally 100.
var (
	orderQuantities = [][]float64{{10}, {25}, {100}}
	demandPosterior = [][]float64{{10}, {10}, {10}, {10}, {10}, {100}}
)

// newNewsvendorEnvironment builds the decision. samples nil plans at the
// posterior MEAN (25) instead of over the posterior.
func newNewsvendorEnvironment(
	t *testing.T,
	samples [][]float64,
	meanDemand float64,
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
		Name:      "order",
		Iteration: &general.ParamValuesIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"param_values": {0},
		}),
		InitStateValues:   []float64{0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "profit",
		Iteration: &newsvendorIteration{price: 10, cost: 6},
		Params: simulator.NewParams(map[string][]float64{
			"quantity": {0}, "demand": {meanDemand},
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"quantity": {Upstream: "order"},
		},
		InitStateValues:   []float64{0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	impl.ExecutionStrategy = &simulator.InlineExecution{}

	spec := agents.SimulationEnvironmentSpec{
		Actions:         orderQuantities,
		ActionPartition: "order",
		ActionParam:     "param_values",
		Horizon:         1,
		Reward:          func(rows map[string][]float64) float64 { return rows["profit"][0] },
		MinReturn:       -700,
		MaxReturn:       300,
		ScenarioSeed:    3,
	}
	if samples != nil {
		spec.ParameterSamples = samples
		spec.ParameterTargets = []agents.SimulationParamTarget{
			{Partition: "profit", Param: "demand", Indices: []int{0}},
		}
	}
	return agents.NewSimulationEnvironment(settings, impl, spec)
}

// TestPosteriorPredictivePlanningBeatsPointEstimate is the payoff of planning
// against a posterior rather than a fitted point.
//
// The newsvendor is the textbook case where the two provably disagree. Ordering
// 25 is optimal if demand really is its posterior mean of 25 (profit 100), but
// demand is 10 five times in six, so 25 loses money on average (−25) while
// ordering 10 earns a certain 40. A planner told only the mean cannot see this;
// one that averages over the posterior can.
func TestPosteriorPredictivePlanningBeatsPointEstimate(t *testing.T) {
	cfg := agents.MCTSConfig[[]float64, int]{
		Simulations:     600,
		MaxTreeDepth:    2,
		RolloutMaxSteps: 2,
		Rollout:         agents.UniformRandomRollout[[]float64, int](),
	}

	// Planning at the posterior mean.
	pointEnv := newNewsvendorEnvironment(t, nil, 25)
	atMean, _, err := agents.RunMCTSSearch(
		pointEnv, pointEnv.InitialState(), cfg, 11, cfg.Simulations)
	if err != nil {
		t.Fatalf("RunMCTSSearch: %v", err)
	}

	// Planning over the posterior.
	posteriorEnv := newNewsvendorEnvironment(t, demandPosterior, 25)
	underPosterior, _, err := agents.RunChanceMCTSSearch(
		posteriorEnv, posteriorEnv.InitialState(), cfg, 11, cfg.Simulations)
	if err != nil {
		t.Fatalf("RunChanceMCTSSearch: %v", err)
	}

	t.Logf("at the posterior mean: order %v | over the posterior: order %v",
		orderQuantities[atMean][0], orderQuantities[underPosterior][0])

	if orderQuantities[atMean][0] != 25 {
		t.Errorf("planning at the mean should order 25 (its best response to demand=25), got %v",
			orderQuantities[atMean][0])
	}
	if orderQuantities[underPosterior][0] != 10 {
		t.Errorf("planning over the posterior should order 10 (expected profit 40, against "+
			"-25 for ordering 25), got %v", orderQuantities[underPosterior][0])
	}
}

// probeIteration is a probe-then-commit decision. An unknown parameter theta
// says which of two commitments pays; probing reveals it but earns nothing.
//
// Row: [signal, payoff]. The signal is what a decision-maker observes, and it
// only carries information when the probe action was taken — otherwise both
// hypotheses predict the same thing, so an observation of it teaches nothing.
//
// Params: "choice" (the action) and "theta" (the uncertain parameter).
type probeIteration struct{}

func (p *probeIteration) Configure(int, *simulator.Settings) {}

func (p *probeIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	choice := params.GetIndex("choice", 0)
	theta := params.GetIndex("theta", 0)
	switch {
	case choice == 0: // probe: reveals theta, pays nothing
		return []float64{theta, 0}
	case choice == 1: // commit A: right when theta is 0
		return []float64{0, 10 - 20*theta}
	default: // commit B: right when theta is 1
		return []float64{0, -10 + 20*theta}
	}
}

// newProbeEnvironment builds the decision over a two-point posterior on theta.
// belief nil plans on a fixed draw, which cannot value information.
func newProbeEnvironment(t *testing.T, belief *agents.BeliefSpec) *agents.SimulationEnvironment {
	return newProbeEnvironmentWithSamples(t, belief, [][]float64{{0}, {1}})
}

func newProbeEnvironmentWithSamples(
	t *testing.T,
	belief *agents.BeliefSpec,
	samples [][]float64,
) *agents.SimulationEnvironment {
	return newProbeEnvironmentWeighted(t, belief, samples, nil)
}

func newProbeEnvironmentWeighted(
	t *testing.T,
	belief *agents.BeliefSpec,
	samples [][]float64,
	weights []float64,
) *agents.SimulationEnvironment {
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
		Name:      "world",
		Iteration: &probeIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"choice": {0}, "theta": {0},
		}),
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
		Actions:          [][]float64{{0}, {1}, {2}}, // probe / commit A / commit B
		ActionPartition:  "choice",
		ActionParam:      "param_values",
		Horizon:          2,
		Reward:           func(rows map[string][]float64) float64 { return rows["world"][1] },
		MinReturn:        -30,
		MaxReturn:        30,
		ScenarioSeed:     5,
		ParameterSamples: samples,
		ParameterWeights: weights,
		ParameterTargets: []agents.SimulationParamTarget{
			{Partition: "world", Param: "theta", Indices: []int{0}},
		},
		Belief: belief,
	})
}

// TestBeliefUpdatingValuesInformation is what belief updating buys that
// posterior-predictive planning alone does not: taking an action for what it
// reveals rather than what it pays.
//
// Committing blind is worth zero — theta decides which commitment is right and
// it is equally likely to be either. Probing pays nothing and burns a step, so a
// planner that cannot learn from the probe's result sees no reason to do it. One
// that updates its belief probes, then commits correctly, for +10.
func TestBeliefUpdatingValuesInformation(t *testing.T) {
	cfg := agents.MCTSConfig[[]float64, int]{
		Simulations:     800,
		MaxTreeDepth:    3,
		RolloutMaxSteps: 3,
		Rollout:         agents.UniformRandomRollout[[]float64, int](),
	}

	blind := newProbeEnvironment(t, nil)
	blindFirst, _, err := agents.RunChanceMCTSSearch(
		blind, blind.InitialState(), cfg, 21, cfg.Simulations)
	if err != nil {
		t.Fatalf("RunChanceMCTSSearch: %v", err)
	}

	learner := newProbeEnvironment(t, &agents.BeliefSpec{
		ObservationPartition: "world", Variance: 0.01,
	})
	learnerFirst, _, err := agents.RunChanceMCTSSearch(
		learner, learner.InitialState(), cfg, 21, cfg.Simulations)
	if err != nil {
		t.Fatalf("RunChanceMCTSSearch: %v", err)
	}

	names := []string{"probe", "commit A", "commit B"}
	t.Logf("fixed draw opens with %s | belief updating opens with %s",
		names[blindFirst], names[learnerFirst])

	if learnerFirst != 0 {
		t.Errorf("a planner that can learn should probe first, it opened with %s",
			names[learnerFirst])
	}
}

// TestBeliefUpdatingSharpensOnAnInformativeAction checks the mechanism directly:
// probing must move the belief onto the true parameter, and committing (which
// reveals nothing) must leave it alone.
func TestBeliefUpdatingSharpensOnAnInformativeAction(t *testing.T) {
	env := newProbeEnvironment(t, &agents.BeliefSpec{
		ObservationPartition: "world", Variance: 0.01,
	})
	start := env.InitialState()

	probed, err := env.Apply(start, 0)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	committed, err := env.Apply(start, 1)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// The belief weights are the last two slots of the encoded state.
	width := env.StateWidth()
	probedBelief := probed[width-2:]
	committedBelief := committed[width-2:]
	t.Logf("after probing: %v | after committing: %v", probedBelief, committedBelief)

	sharpest := math.Max(probedBelief[0], probedBelief[1])
	if sharpest < 0.9 {
		t.Errorf("probing should nearly resolve theta, belief is %v", probedBelief)
	}
	for i, weight := range committedBelief {
		if math.Abs(weight-0.5) > 1e-6 {
			t.Errorf("committing reveals nothing, so weight %d should stay 0.5, got %v",
				i, weight)
		}
	}
}

// TestParameterWeightsSteerTheDraw checks the planner honours the credence an
// inference tier attached to each sample, not just the samples themselves.
//
// Without weights a planner treats every sample as equally likely, which is only
// right when the samples were drawn from the posterior. SMC hands over particles
// AND weights, and the weights are the posterior — ignoring them plans against
// the proposal instead.
//
// Demand is 10 or 100; the weights say which is credible. Ordering 100 is right
// only if the large demand is, so the plan should follow the weights.
func TestParameterWeightsSteerTheDraw(t *testing.T) {
	cfg := agents.MCTSConfig[[]float64, int]{
		Simulations:     600,
		MaxTreeDepth:    2,
		RolloutMaxSteps: 2,
		Rollout:         agents.UniformRandomRollout[[]float64, int](),
	}
	samples := [][]float64{{10}, {100}}

	for _, testCase := range []struct {
		name    string
		weights []float64
		want    float64
	}{
		{name: "credence on low demand", weights: []float64{0.95, 0.05}, want: 10},
		{name: "credence on high demand", weights: []float64{0.05, 0.95}, want: 100},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			env := newWeightedNewsvendorEnvironment(t, samples, testCase.weights)
			best, _, err := agents.RunChanceMCTSSearch(
				env, env.InitialState(), cfg, 13, cfg.Simulations)
			if err != nil {
				t.Fatalf("RunChanceMCTSSearch: %v", err)
			}
			if got := orderQuantities[best][0]; got != testCase.want {
				t.Errorf("ordered %v, want %v — the plan is not following the weights",
					got, testCase.want)
			}
		})
	}
}

// newWeightedNewsvendorEnvironment is the newsvendor over a weighted sample set.
func newWeightedNewsvendorEnvironment(
	t *testing.T,
	samples [][]float64,
	weights []float64,
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
		Name:              "order",
		Iteration:         &general.ParamValuesIteration{},
		Params:            simulator.NewParams(map[string][]float64{"param_values": {0}}),
		InitStateValues:   []float64{0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      "profit",
		Iteration: &newsvendorIteration{price: 10, cost: 6},
		Params: simulator.NewParams(map[string][]float64{
			"quantity": {0}, "demand": {10},
		}),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"quantity": {Upstream: "order"},
		},
		InitStateValues:   []float64{0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	impl.ExecutionStrategy = &simulator.InlineExecution{}

	return agents.NewSimulationEnvironment(settings, impl, agents.SimulationEnvironmentSpec{
		Actions:          orderQuantities,
		ActionPartition:  "order",
		ActionParam:      "param_values",
		Horizon:          1,
		Reward:           func(rows map[string][]float64) float64 { return rows["profit"][0] },
		MinReturn:        -700,
		MaxReturn:        500,
		ScenarioSeed:     3,
		ParameterSamples: samples,
		ParameterWeights: weights,
		ParameterTargets: []agents.SimulationParamTarget{
			{Partition: "profit", Param: "demand", Indices: []int{0}},
		},
	})
}

// TestBeliefStartsFromThePrior checks the belief begins where the inference tier
// left off rather than flat, so a planner does not discard what was already
// concluded the moment an episode starts.
func TestBeliefStartsFromThePrior(t *testing.T) {
	env := newProbeEnvironmentWithSamples(t,
		&agents.BeliefSpec{ObservationPartition: "world", Variance: 0.01},
		[][]float64{{0}, {1}})
	flat := env.InitialState()
	width := env.StateWidth()
	if flat[width-2] != 0.5 || flat[width-1] != 0.5 {
		t.Fatalf("with no weights the belief should start flat, got %v", flat[width-2:])
	}

	weighted := newProbeEnvironmentWeighted(t,
		&agents.BeliefSpec{ObservationPartition: "world", Variance: 0.01},
		[][]float64{{0}, {1}}, []float64{3, 1})
	start := weighted.InitialState()
	if math.Abs(start[width-2]-0.75) > 1e-9 || math.Abs(start[width-1]-0.25) > 1e-9 {
		t.Errorf("the belief should start at the normalised prior [0.75 0.25], got %v",
			start[width-2:])
	}
}
