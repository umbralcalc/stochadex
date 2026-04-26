package agents

// Environment is the game/decision-process interface that MCTS searches
// over. Implementations are pure: Legal/Apply must not mutate s, and Apply
// must return a fresh value (the search clones aggressively).
//
// Terminal returns the per-player [0,1] score vector and a done flag. The
// score vector lets the env represent draws (0.5/0.5), graded scoring
// (Catan-like), and standard winner-takes-all (one-hot) without faking a
// "winner" int. For binary games, see WinnerToTerminal in rollout.go.
//
// Actor returns the index of the player whose decision creates the next
// edge from s; backups credit nodes by Actor at the parent. Players returns
// the total seat count and bounds the score vector length.
type Environment[S any, A any] interface {
	Legal(s S) []A
	Apply(s S, a A) (S, error)
	Terminal(s S) (scores []float64, done bool)
	Actor(s S) int
	Players(s S) int
}

// MCTSEdgeStat is per-action telemetry exposed at the root after a search.
// Useful for JSON reports or logging the search's distribution over moves.
type MCTSEdgeStat[A any] struct {
	Action       A       `json:"action"`
	Visits       int     `json:"visits"`
	MeanForActor float64 `json:"mean_for_actor"`
}
