package agents

import (
	"math"
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// MASTDefaultTau is the softmax temperature applied to MAST means when
// sampling. Smaller → more exploitative; larger → closer to uniform.
// 1.0 diverges from uniform within ~50 updates per key in practice.
const MASTDefaultTau = 1.0

// MASTSamplePrior is the score assigned to actions whose key has not
// yet been observed, so they retain selection probability early in
// search. 0.5 sits at the midpoint of the [0, 1] reward range — neither
// encouraged nor discouraged.
const MASTSamplePrior = 0.5

// MASTRolloutPartition is a stochadex iteration that runs one
// MAST-biased rollout per step. It reads the leaf state from an upstream
// MCTSTreePartition (within-step), reads the running aggregates from a
// MASTAggregationPartition (lag-1, via params_as_partitions), runs a
// playout sampling each ply via softmax over the aggregates, and emits
// the per-player scores plus the (key_idx, reward) path that the
// MASTAggregationPartition will absorb on the next step.
//
// Row layout (width = Players + 2 + 2 * MaxPath):
//
//	row[0 .. Players-1]                           per-player [0,1] scores
//	row[Players]                                  ok flag (1 = scores valid)
//	row[Players+1]                                num_path (length of valid pairs)
//	row[Players+2 .. Players+1+2*MaxPath]         (key_idx, reward) pairs,
//	                                              padded with zeros
//
// The (num_path, pairs...) suffix matches MASTAggregationParamUpdates
// so the downstream MASTAggregationPartition can read it as a single
// slice.
//
// Warm fields (must be set before Configure):
//   - Env: the typed Environment[S, A] to roll out against.
//   - Cfg: must have RolloutMaxSteps; the rollout function in Cfg is
//     ignored (this partition implements its own MAST-biased sampling).
//   - Decoder: decodes the leaf_state from the upstream-supplied slice.
//   - KeyToIdx: maps an action to a bounded index in [0, MaxKeys).
//   - MaxKeys: same K as the MASTAggregationPartition this is wired to.
//   - MaxPath: maximum number of (key, reward) updates emitted per
//     rollout. Paths longer than this are truncated.
//   - Players: per-player score vector length.
//   - Tau: softmax temperature (MASTDefaultTau if <= 0).
//   - Progress: optional [0,1] per-player progress proxy used to score
//     truncated rollouts.
type MASTRolloutPartition[S any, A any] struct {
	Env      Environment[S, A]
	Cfg      MCTSConfig[S, A]
	Decoder  func([]float64) (S, error)
	KeyToIdx func(A) int
	MaxKeys  int
	MaxPath  int
	Players  int
	Tau      float64
	Progress func(s S, player int) (float64, bool)

	seed uint64
}

// MASTRolloutRowWidth returns the required state_width for an
// MASTRolloutPartition with the given player count and path bound.
func MASTRolloutRowWidth(players, maxPath int) int {
	return players + 2 + 2*maxPath
}

// MASTRolloutScoresOffset returns the row offset of score slot i.
func MASTRolloutScoresOffset(i int) int { return i }

// MASTRolloutOkOffset returns the row offset of the ok flag.
func MASTRolloutOkOffset(players int) int { return players }

// MASTRolloutNumPathOffset returns the row offset of the num_path
// counter.
func MASTRolloutNumPathOffset(players int) int { return players + 1 }

// MASTRolloutPathOffset returns the row offset of the first
// (key_idx, reward) pair.
func MASTRolloutPathOffset(players int) int { return players + 2 }

// Configure implements simulator.Iteration.
func (m *MASTRolloutPartition[S, A]) Configure(partitionIndex int, settings *simulator.Settings) {
	if m.Env == nil {
		panic("agents.MASTRolloutPartition: Env required")
	}
	if m.Decoder == nil {
		panic("agents.MASTRolloutPartition: Decoder required")
	}
	if m.KeyToIdx == nil {
		panic("agents.MASTRolloutPartition: KeyToIdx required")
	}
	if m.MaxKeys <= 0 {
		panic("agents.MASTRolloutPartition: MaxKeys must be > 0")
	}
	if m.MaxPath <= 0 {
		panic("agents.MASTRolloutPartition: MaxPath must be > 0")
	}
	if m.Players <= 0 {
		panic("agents.MASTRolloutPartition: Players must be > 0")
	}
	is := settings.Iterations[partitionIndex]
	if is.StateWidth != MASTRolloutRowWidth(m.Players, m.MaxPath) {
		panic("agents.MASTRolloutPartition: StateWidth must equal MASTRolloutRowWidth(Players, MaxPath)")
	}
	if m.Tau <= 0 {
		m.Tau = MASTDefaultTau
	}
	m.seed = is.Seed
}

// Iterate implements simulator.Iteration.
func (m *MASTRolloutPartition[S, A]) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	out := make([]float64, MASTRolloutRowWidth(m.Players, m.MaxPath))
	cfg := m.Cfg
	cfg.applyDefaults()

	leafSlice, ok := params.GetOk(MCTSRolloutParamLeaf)
	if !ok || len(leafSlice) < 2 {
		return out
	}
	width := len(leafSlice) - 1
	if leafSlice[width] == 0 {
		return out // has_leaf=0 sentinel
	}
	encoded := make([]float64, width)
	copy(encoded, leafSlice[:width])
	leaf, err := m.Decoder(encoded)
	if err != nil {
		return out
	}

	// Read aggregates from the upstream MASTAggregationPartition's row 0
	// (lag-1, state-history mode) if wired; nil = uniform sampling.
	var aggregates []float64
	if upstreamSlice, present := params.GetOk(MASTAggregationParamPartition); present && len(upstreamSlice) > 0 {
		aggIdx := int(upstreamSlice[0])
		if aggIdx >= 0 && aggIdx < len(stateHistories) {
			aggregates = stateHistories[aggIdx].Values.RawRowView(0)
		}
	}

	step := uint64(timestepsHistory.CurrentStepNumber)
	seed := m.seed ^ step ^ uint64(partitionIndex+1)*0x9e3779b97f4a7c15
	rng := rand.New(rand.NewPCG(seed, ^seed))

	scores, valid, path := m.playout(leaf, aggregates, cfg.RolloutMaxSteps, rng)
	if !valid {
		return out
	}
	if len(scores) != m.Players {
		return out
	}
	copy(out[MASTRolloutScoresOffset(0):], scores)
	out[MASTRolloutOkOffset(m.Players)] = 1

	// Encode the path as a (num_path, pairs...) batch in the suffix.
	num := len(path)
	if num > m.MaxPath {
		num = m.MaxPath
	}
	out[MASTRolloutNumPathOffset(m.Players)] = float64(num)
	pathOff := MASTRolloutPathOffset(m.Players)
	for i := 0; i < num; i++ {
		out[pathOff+2*i] = float64(path[i].keyIdx)
		out[pathOff+2*i+1] = path[i].reward
	}
	return out
}

// mastPathEntry records one (key, reward) pair to be absorbed by the
// downstream MASTAggregationPartition.
type mastPathEntry struct {
	keyIdx int
	reward float64
}

// mastStepRec records one ply within an in-progress rollout — the actor
// that took the action and the keyIdx of the action they took.
type mastStepRec struct {
	actor  int
	keyIdx int
}

// playout runs one MAST-biased rollout from leaf, returning the per-player
// scores, an ok flag, and the (key_idx, reward) path along the playout.
//
// rewards are scored from the actor's perspective: each path entry's
// reward is scores[actor_at_that_ply].
//
// aggregates is the MASTAggregationPartition's row (counts and sums per
// key); nil = uniform sampling. When aggregates is supplied and at least
// one key has been observed, sampling uses softmax over per-key means
// (with MASTSamplePrior for unobserved keys).
func (m *MASTRolloutPartition[S, A]) playout(
	leaf S,
	aggregates []float64,
	maxSteps int,
	rng *rand.Rand,
) (scores []float64, ok bool, path []mastPathEntry) {
	cur := leaf
	pathSteps := make([]mastStepRec, 0, 64)
	for i := 0; i < maxSteps; i++ {
		if termScores, done := m.Env.Terminal(cur); done {
			return termScores, true, m.attribute(pathSteps, termScores)
		}
		legal := m.Env.Legal(cur)
		if len(legal) == 0 {
			break
		}
		action := m.sample(legal, aggregates, rng)
		actor := m.Env.Actor(cur)
		keyIdx := m.KeyToIdx(action)
		pathSteps = append(pathSteps, mastStepRec{actor: actor, keyIdx: keyIdx})
		ns, applyErr := m.Env.Apply(cur, action)
		if applyErr != nil {
			return nil, false, nil
		}
		cur = ns
	}
	if m.Progress == nil {
		return nil, false, nil
	}
	progScores, hasSignal := m.applyProgress(cur)
	if !hasSignal {
		return nil, false, nil
	}
	return progScores, true, m.attribute(pathSteps, progScores)
}

// sample draws one action via softmax over the supplied aggregates
// (interpreted as the MASTAggregationPartition's row layout). Falls back
// to uniform when aggregates is nil or no observations exist yet.
func (m *MASTRolloutPartition[S, A]) sample(
	legal []A,
	aggregates []float64,
	rng *rand.Rand,
) A {
	if aggregates == nil {
		return legal[rng.IntN(len(legal))]
	}
	any := false
	for k := 0; k < m.MaxKeys; k++ {
		if aggregates[MASTAggregationCountSlot(k)] > 0 {
			any = true
			break
		}
	}
	if !any {
		return legal[rng.IntN(len(legal))]
	}
	weights := make([]float64, len(legal))
	maxLogit := math.Inf(-1)
	for i, a := range legal {
		k := m.KeyToIdx(a)
		mean := MASTSamplePrior
		if k >= 0 && k < m.MaxKeys {
			c := aggregates[MASTAggregationCountSlot(k)]
			if c > 0 {
				mean = aggregates[MASTAggregationSumSlot(k)] / c
			}
		}
		l := mean / m.Tau
		weights[i] = l
		if l > maxLogit {
			maxLogit = l
		}
	}
	sum := 0.0
	for i := range weights {
		weights[i] = math.Exp(weights[i] - maxLogit)
		sum += weights[i]
	}
	if sum <= 0 {
		return legal[rng.IntN(len(legal))]
	}
	r := rng.Float64() * sum
	cum := 0.0
	for i, w := range weights {
		cum += w
		if r <= cum {
			return legal[i]
		}
	}
	return legal[len(legal)-1]
}

// attribute converts an in-memory path of (actor, keyIdx) records into
// (keyIdx, reward) pairs ready to emit. Reward for each entry is
// scores[entry.actor]; entries with out-of-range actors are dropped.
func (m *MASTRolloutPartition[S, A]) attribute(steps []mastStepRec, scores []float64) []mastPathEntry {
	out := make([]mastPathEntry, 0, len(steps))
	for _, st := range steps {
		if st.actor < 0 || st.actor >= len(scores) {
			continue
		}
		out = append(out, mastPathEntry{keyIdx: st.keyIdx, reward: scores[st.actor]})
	}
	return out
}

// applyProgress evaluates the optional Progress function across players,
// returning the score vector and a "has comparative signal" flag (false
// if all values are zero, which carries no signal — see the MCTSTree docs).
func (m *MASTRolloutPartition[S, A]) applyProgress(s S) (scores []float64, hasSignal bool) {
	if m.Progress == nil {
		return nil, false
	}
	scores = make([]float64, m.Players)
	any := false
	differs := false
	first := 0.0
	for p := 0; p < m.Players; p++ {
		if v, ok := m.Progress(s, p); ok {
			scores[p] = v
			if !any {
				first = v
			} else if v != first {
				differs = true
			}
			any = true
		}
	}
	if !any {
		return nil, false
	}
	if !differs && first == 0 {
		return nil, false
	}
	return scores, true
}
