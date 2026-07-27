package agents

import (
	"fmt"
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// SimulationEnvironment adapts a stochadex sub-simulation into an
// Environment[[]float64, int], so MCTS can plan over the same forward model the
// rest of the engine simulates and calibrates — rather than over game rules
// written by hand.
//
// This is the counterpart to the environment registry in pkg/api. That hook lets
// a config NAME decision rules a downstream module wrote in Go; this type needs
// no rules at all, because the dynamics are already stated as partitions. An
// action is a params injection, a transition is one step of the sub-simulation,
// and the reward is read off the resulting rows.
//
// # State encoding
//
//	s[0]                 step index within the episode
//	s[1]                 accumulated (discounted) reward so far
//	s[2 .. 2+totalWidth] every partition's state-history window, flattened
//	                     row-major with the latest row first, concatenated in
//	                     partition order
//
// Carrying the accumulated reward in the state is what lets a finite-horizon,
// per-step-reward problem satisfy the existing Environment contract without
// changing it: Terminal fires at the horizon and normalises the accumulated
// reward into the [0,1] score the UCT backups expect. ReturnRange declares the
// normalisation bounds, since UCB1 is only meaningful on a bounded value scale.
//
// # Determinism, and the approximation it encodes
//
// Apply must be pure — MCTS materialises a child state once per edge and reuses
// it on every later visit — but a stochastic sub-simulation is not. That is not
// a problem this type solves for itself: it delegates to
// simulator.ReentrantSimulation, the engine's re-entrant evaluation tier, and
// supplies a seed derived from (ScenarioSeed, state, action). Two calls with the
// same arguments therefore return the same successor.
//
// That is common random numbers: the noise is pinned as a function of where you
// are and what you do, turning the stochastic model into a deterministic
// surrogate that MCTSTree searches exactly. It is a real modelling choice, not a
// free win — a planner solving a pinned scenario can exploit the particular noise
// draw it is going to receive, which biases its values optimistic. On the battery
// test problem that over-promise reaches tens of percent and *grows* with the
// simulation budget, since more search means more exploitation of the one
// realisation it can see.
//
// This type therefore also implements StochasticEnvironment, via ApplySample. A
// search that uses it (MCTSChanceTree) builds chance nodes and averages over
// sampled successors, which removes most of the bias:
// TestChanceNodesReduceOptimism measures both on the same problem.
//
// # Requirements and limits
//
//   - A partition's whole state-history window is carried in the encoded state,
//     so models whose iterations read further back than one step work — a
//     10-minute rolling count, say. Deep windows cost encoded width: the state
//     grows as sum(depth * width) over partitions.
//   - The iterations must honour the framework rule that all mutable state is
//     re-initialisable in Configure (which RunWithHarnesses already enforces).
//     That rule is exactly what makes a pure Apply possible.
//   - Not safe for concurrent use: the sub-simulation's iteration objects are
//     shared and re-Configured per transition. Build one SimulationEnvironment
//     per goroutine.
type SimulationEnvironment struct {
	simulation      *simulator.ReentrantSimulation
	spec            SimulationEnvironmentSpec
	partitionNames  []string
	partitionWidths []int
	partitionDepths []int
	// windowWidths[i] = depth*width, the encoded size of partition i's window.
	windowWidths    []int
	initRows        [][]float64
	totalWidth      int
	actionPartition int
}

// SimulationEnvironmentSpec configures a SimulationEnvironment.
type SimulationEnvironmentSpec struct {
	// Actions is the discrete action set. Each entry is the params value
	// injected under ActionParam on ActionPartition, so actions are whatever
	// the model already reads as parameters — a dispatch rate, a policy
	// threshold, an intervention size.
	Actions [][]float64
	// ActionPartition names the partition receiving the action, and ActionParam
	// the params key it is written to.
	ActionPartition string
	ActionParam     string
	// Horizon is the episode length in steps.
	Horizon int
	// Reward scores one transition from the post-step rows, addressed by
	// partition name. Returning the per-step reward (not the cumulative one)
	// keeps the discounting in one place.
	Reward func(rows map[string][]float64) float64
	// Discount is the per-step discount factor applied to rewards. Zero means
	// undiscounted (treated as 1).
	Discount float64
	// MinReturn and MaxReturn bound the achievable accumulated reward and are
	// used to normalise it into the [0,1] score UCB1 needs. A return outside
	// the range is clamped — widen the range rather than leaving it clamped,
	// since a saturated score carries no gradient for the search.
	MinReturn, MaxReturn float64
	// ScenarioSeed pins the noise realisation this environment plans against.
	ScenarioSeed uint64
	// Legal optionally restricts the action set given the decoded rows. Nil
	// means every action is always legal.
	Legal func(rows map[string][]float64) []int
	// Progress optionally scores an unfinished position in [0,1], for rollouts
	// that hit their step limit before the horizon. Nil falls back to the reward
	// banked so far — see Progress.
	Progress func(rows map[string][]float64) (float64, bool)
}

// NewSimulationEnvironment validates the spec against the sub-simulation and
// returns the environment. It panics on a misconfiguration, matching the rest of
// the config-assembly tier: these are programming errors caught before any
// search starts, and a silently misshapen environment would produce plausible
// but meaningless recommendations.
func NewSimulationEnvironment(
	settings *simulator.Settings,
	implementations *simulator.Implementations,
	spec SimulationEnvironmentSpec,
) *SimulationEnvironment {
	if len(spec.Actions) == 0 {
		panic("agents.NewSimulationEnvironment: at least one action is required")
	}
	if spec.Horizon <= 0 {
		panic("agents.NewSimulationEnvironment: Horizon must be > 0")
	}
	if spec.Reward == nil {
		panic("agents.NewSimulationEnvironment: Reward is required")
	}
	if spec.MaxReturn <= spec.MinReturn {
		panic("agents.NewSimulationEnvironment: MaxReturn must exceed MinReturn")
	}
	actionPartition := -1
	count := len(settings.Iterations)
	names := make([]string, count)
	widths := make([]int, count)
	depths := make([]int, count)
	windowWidths := make([]int, count)
	initRows := make([][]float64, count)
	total := 0
	for index, iteration := range settings.Iterations {
		depth := iteration.StateHistoryDepth
		if depth < 1 {
			depth = 1
		}
		names[index] = iteration.Name
		widths[index] = iteration.StateWidth
		depths[index] = depth
		windowWidths[index] = depth * iteration.StateWidth
		// The initial window repeats the configured row: before the run starts
		// there is no history, and this is what a coordinator would begin from.
		window := make([]float64, 0, windowWidths[index])
		for row := 0; row < depth; row++ {
			window = append(window, iteration.InitStateValues...)
		}
		initRows[index] = window
		total += windowWidths[index]
		if iteration.Name == spec.ActionPartition {
			actionPartition = index
		}
	}
	if actionPartition < 0 {
		panic(fmt.Sprintf(
			"agents.NewSimulationEnvironment: no partition named %q to receive the action",
			spec.ActionPartition,
		))
	}
	if spec.Discount == 0 {
		spec.Discount = 1.0
	}
	return &SimulationEnvironment{
		simulation:      simulator.NewReentrantSimulation(settings, implementations),
		spec:            spec,
		partitionNames:  names,
		partitionWidths: widths,
		partitionDepths: depths,
		windowWidths:    windowWidths,
		initRows:        initRows,
		totalWidth:      total,
		actionPartition: actionPartition,
	}
}

// StateWidth is the encoded state width: the step index, the accumulated
// reward, and every partition's row.
func (e *SimulationEnvironment) StateWidth() int { return 2 + e.totalWidth }

// InitialState encodes the sub-simulation's configured initial rows as the
// episode's starting state.
func (e *SimulationEnvironment) InitialState() []float64 {
	state := make([]float64, e.StateWidth())
	offset := 2
	for index, window := range e.initRows {
		copy(state[offset:], window)
		offset += e.windowWidths[index]
	}
	return state
}

// rowsByName decodes the per-partition rows out of an encoded state.
func (e *SimulationEnvironment) rowsByName(s []float64) map[string][]float64 {
	rows := make(map[string][]float64, len(e.partitionWidths))
	offset := 2
	for index, width := range e.partitionWidths {
		// The latest row, which is what a reward is written against.
		rows[e.partitionNames[index]] = s[offset : offset+width]
		offset += e.windowWidths[index]
	}
	return rows
}

// Legal implements Environment.
func (e *SimulationEnvironment) Legal(s []float64) []int {
	if _, done := e.Terminal(s); done {
		return nil
	}
	if e.spec.Legal != nil {
		return e.spec.Legal(e.rowsByName(s))
	}
	actions := make([]int, len(e.spec.Actions))
	for index := range actions {
		actions[index] = index
	}
	return actions
}

// Apply implements Environment: one step of the sub-simulation under action a.
//
// The transition is pure. Its seed is derived from the scenario seed, the
// encoded state and the action, so the same arguments always give the same
// successor — see the type docs for what that pins and what it costs.
func (e *SimulationEnvironment) Apply(s []float64, a int) ([]float64, error) {
	return e.apply(s, a, 0)
}

// apply is the shared transition. sampleSeed selects which draw from the
// transition distribution is taken: zero is Apply's pinned scenario draw, and any
// other value is one of the alternatives a chance-node search averages over.
func (e *SimulationEnvironment) apply(
	s []float64,
	a int,
	sampleSeed uint64,
) ([]float64, error) {
	if a < 0 || a >= len(e.spec.Actions) {
		return nil, fmt.Errorf("agents: action %d out of range", a)
	}
	if len(s) != e.StateWidth() {
		return nil, fmt.Errorf(
			"agents: encoded state width %d, want %d", len(s), e.StateWidth())
	}

	// Hand back each partition's whole window, so an iteration that reads several
	// steps into the past resumes from the history it actually had.
	histories := make(map[int]*simulator.StateHistory, len(e.partitionWidths))
	offset := 2
	for index, width := range e.partitionWidths {
		histories[index] = simulator.NewStateHistoryFromWindow(
			s[offset:offset+e.windowWidths[index]], width, e.partitionDepths[index])
		offset += e.windowWidths[index]
	}
	e.simulation.SetParam(e.actionPartition, e.spec.ActionParam, e.spec.Actions[a])
	// Seeding from (scenario, state, action, sample) is what makes the transition
	// reproducible; ReentrantSimulation reseeds the iterations from it before
	// stepping.
	seed := mixSeed(e.spec.ScenarioSeed, s, a)
	if sampleSeed != 0 {
		seed = simulator.DeriveSeed(seed^sampleSeed, int(sampleSeed&0xffff))
	}
	advanced := e.simulation.RunWindows(simulator.ReentrantRun{
		Histories: histories,
		Seed:      &seed,
		Steps:     1,
	})

	next := make([]float64, e.StateWidth())
	next[0] = s[0] + 1
	writeOffset := 2
	for index := range e.partitionWidths {
		copy(next[writeOffset:], advanced[index])
		writeOffset += e.windowWidths[index]
	}
	// Discount by the step the reward was earned on, so a path's accumulated
	// value does not depend on the order the tree happened to visit it in.
	next[1] = s[1] + math.Pow(e.spec.Discount, s[0])*e.spec.Reward(e.rowsByName(next))
	return next, nil
}

// ApplySample implements StochasticEnvironment: one transition under an explicit
// sample seed, so a search can draw more than one successor of the same
// (state, action) and average over them instead of committing to the first.
//
// Apply is ApplySample with the sample seed fixed at zero. That is the whole
// difference between planning against a pinned scenario and planning against the
// distribution.
func (e *SimulationEnvironment) ApplySample(s []float64, a int, seed uint64) ([]float64, error) {
	return e.apply(s, a, seed)
}

// Terminal implements Environment: the episode ends at the horizon, scored by
// the accumulated reward normalised into [0,1].
func (e *SimulationEnvironment) Terminal(s []float64) ([]float64, bool) {
	if int(s[0]) < e.spec.Horizon {
		return nil, false
	}
	span := e.spec.MaxReturn - e.spec.MinReturn
	score := (s[1] - e.spec.MinReturn) / span
	return []float64{math.Min(1, math.Max(0, score))}, true
}

// Actor implements Environment. A planning problem has a single decision maker.
func (e *SimulationEnvironment) Actor([]float64) int { return 0 }

// Players implements Environment.
func (e *SimulationEnvironment) Players([]float64) int { return 1 }

// Progress scores an unfinished state on the same [0,1] scale Terminal uses, so
// a rollout that runs out of steps still contributes a value instead of no
// signal. Compose it with FromProgress.
//
// The proxy is the reward banked so far. That is deliberately conservative: it
// credits nothing for a position that is merely promising, so it under-rates a
// leaf whose payoff comes later. It beats the alternative of discarding the
// rollout, which leaves the search exploring on visit counts alone — the failure
// mode that bites when the horizon is longer than RolloutMaxSteps.
//
// Supply SimulationEnvironmentSpec.Progress to score positions instead of banked
// reward, which is what a domain proxy (a win probability, a margin) is for.
func (e *SimulationEnvironment) Progress(s []float64, player int) (float64, bool) {
	if e.spec.Progress != nil {
		return e.spec.Progress(e.rowsByName(s))
	}
	span := e.spec.MaxReturn - e.spec.MinReturn
	return math.Min(1, math.Max(0, (s[1]-e.spec.MinReturn)/span)), true
}

// Return reads the accumulated (discounted) reward out of an encoded state,
// which is the quantity a caller actually wants to report — the [0,1] score is
// an artefact of the UCB1 value scale.
func (e *SimulationEnvironment) Return(s []float64) float64 { return s[1] }

// mixSeed derives a transition seed from the scenario seed, the encoded state
// and the action, using an FNV-1a style mix over the state's bit patterns. Two
// paths arriving at the same state under the same action therefore share a noise
// draw, which is what makes the surrogate consistent.
func mixSeed(scenario uint64, s []float64, a int) uint64 {
	const prime = 0x100000001b3
	hash := uint64(0xcbf29ce484222325) ^ scenario
	for _, value := range s {
		hash = (hash ^ math.Float64bits(value)) * prime
	}
	return (hash ^ uint64(a+1)) * prime
}

var _ Environment[[]float64, int] = (*SimulationEnvironment)(nil)
