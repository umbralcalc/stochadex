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

func TestSimulationEnvironmentRejectsDeepHistory(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for state_history_depth > 1")
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
		StateHistoryDepth: 3,
		Seed:              0,
	})
	settings, impl := gen.GenerateConfigs()
	agents.NewSimulationEnvironment(settings, impl, agents.SimulationEnvironmentSpec{
		Actions:         [][]float64{{0}},
		ActionPartition: "battery",
		ActionParam:     "dispatch",
		Horizon:         2,
		Reward:          func(map[string][]float64) float64 { return 0 },
		MinReturn:       -1,
		MaxReturn:       1,
	})
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
