package agents

import "math/rand/v2"

// MCTSRolloutFn is the single rollout signature used by the search. It runs a
// stochastic playout from s for at most maxSteps actions and returns:
//   - scores: per-player [0,1] score vector (length Players(s)).
//   - ok: false signals "no signal" — the caller counts the simulation as a
//     visit but does not credit any win, so UCB exploration still progresses.
//   - err: a hard failure (e.g. Apply rejected what it claimed was legal);
//     the caller treats this like ok=false.
//
// The seed argument is the rollout's full RNG seed; implementations should
// be deterministic given the same seed.
type MCTSRolloutFn[S any, A any] func(env Environment[S, A], s S, maxSteps int, seed uint64) (scores []float64, ok bool, err error)

// UniformRandomRollout returns a MCTSRolloutFn that plays uniformly random
// legal actions until either Terminal returns done or maxSteps is reached.
// On termination the env's Terminal scores are returned. On truncation
// (maxSteps reached without termination) the rollout returns ok=false —
// compose with FromProgress if you have a progress proxy to score truncated
// rollouts.
func UniformRandomRollout[S any, A any]() MCTSRolloutFn[S, A] {
	return func(env Environment[S, A], s S, maxSteps int, seed uint64) ([]float64, bool, error) {
		rng := rand.New(rand.NewPCG(seed, ^seed))
		cur := s
		for i := 0; i < maxSteps; i++ {
			if scores, done := env.Terminal(cur); done {
				return scores, true, nil
			}
			leg := env.Legal(cur)
			if len(leg) == 0 {
				return nil, false, nil
			}
			a := leg[rng.IntN(len(leg))]
			ns, err := env.Apply(cur, a)
			if err != nil {
				return nil, false, err
			}
			cur = ns
		}
		return nil, false, nil
	}
}

// FromProgress wraps an inner rollout so that truncated rollouts (ok=false
// from inner) are rescued by scoring the final state via the supplied
// progress function. progress returns a per-player [0,1] proxy of "how
// close is this player to winning" and an ok flag (false = no proxy
// available for that player; treated as 0).
//
// All-equal progress vectors carry no comparative signal and are treated as
// no signal (ok=false from the wrapper) so the search relies on UCB
// exploration alone — see the docstring on MCTSTree.backupVisits for the
// stall-move failure mode this avoids.
//
// FromProgress needs to know the final state of the inner rollout, so it
// re-runs the playout itself rather than wrapping inner. The inner rollout
// is therefore only consulted for early termination.
func FromProgress[S any, A any](
	inner MCTSRolloutFn[S, A],
	progress func(s S, player int) (float64, bool),
) MCTSRolloutFn[S, A] {
	return func(env Environment[S, A], s S, maxSteps int, seed uint64) ([]float64, bool, error) {
		// Try the inner rollout first; if it terminates with ok=true, use that.
		if scores, ok, err := inner(env, s, maxSteps, seed); err != nil || ok {
			return scores, ok, err
		}
		// Inner truncated. Replay deterministically using the same seed and
		// score the truncated terminal state via progress.
		rng := rand.New(rand.NewPCG(seed, ^seed))
		cur := s
		for i := 0; i < maxSteps; i++ {
			if scores, done := env.Terminal(cur); done {
				return scores, true, nil
			}
			leg := env.Legal(cur)
			if len(leg) == 0 {
				break
			}
			a := leg[rng.IntN(len(leg))]
			ns, err := env.Apply(cur, a)
			if err != nil {
				return nil, false, err
			}
			cur = ns
		}
		players := env.Players(cur)
		scores := make([]float64, players)
		any := false
		differs := false
		first := 0.0
		for p := 0; p < players; p++ {
			if v, ok := progress(cur, p); ok {
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
			return nil, false, nil
		}
		// All-equal scores carry no comparative signal and would mislead
		// the UCB tiebreaker into deterministically picking the first-listed
		// child of every node. Treat as "no signal".
		if !differs && first == 0 {
			return nil, false, nil
		}
		return scores, true, nil
	}
}

// WinnerToTerminal builds an Environment.Terminal-compatible result from a
// binary winner. For envs whose native terminal predicate is "winner int,
// done bool" rather than per-player scores, embed this in your Environment
// implementation:
//
//	func (g *MyGame) Terminal(s State) ([]float64, bool) {
//	    w, done := g.winnerOrDone(s)
//	    return agents.WinnerToTerminal(w, g.Players(s), done), done
//	}
func WinnerToTerminal(winner, players int, done bool) []float64 {
	if !done {
		return nil
	}
	scores := make([]float64, players)
	if winner >= 0 && winner < players {
		scores[winner] = 1
	}
	return scores
}
