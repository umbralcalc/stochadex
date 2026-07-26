package agents

// Default hyperparameters used when MCTSConfig fields are zero.
const (
	MCTSDefaultSimulations     = 120
	MCTSDefaultRolloutMaxSteps = 220
	MCTSDefaultMaxTreeDepth    = 14
	MCTSDefaultExploration     = 1.41
)

// MCTSConfig holds UCT hyperparameters and the rollout driver.
//
// Rollout is the single graded rollout signature: it returns a per-player
// score vector in [0,1] and an ok flag (false signals "no signal" — caller
// counts the visit but skips the win credit). For binary winner games or
// games using a Progress proxy, build the rollout from helpers in
// rollout.go (UniformRandomRollout, OneHotFromWinner, FromProgress).
//
// Progress is an optional per-state, per-player [0,1] value proxy used by
// the FromProgress rollout adapter to score truncated rollouts.
type MCTSConfig[S any, A any] struct {
	Simulations     int
	Exploration     float64
	MaxTreeDepth    int
	RolloutMaxSteps int
	Rollout         MCTSRolloutFn[S, A]
	Progress        func(s S, player int) (float64, bool)

	// ChanceWideningFactor and ChanceWideningExponent tune progressive widening
	// in MCTSChanceTree: a chance node holds up to
	// ceil(factor * visits^exponent) sampled outcomes. Ignored by MCTSTree,
	// which has no chance nodes. Zero selects the package defaults.
	//
	// A larger factor or exponent averages over more outcomes per node but
	// spreads the same visits more thinly, so each average is noisier.
	ChanceWideningFactor   float64
	ChanceWideningExponent float64
}

// ApplyDefaults fills in zero-valued hyperparameters with the package
// defaults, mutating the receiver. Called once at the start of each search
// run; safe to call multiple times.
//
// Exported so external packages building on MCTSConfig can share the
// defaults logic without duplicating it.
func (c *MCTSConfig[S, A]) ApplyDefaults() { c.applyDefaults() }

// applyDefaults is the internal implementation used by the in-package
// hot paths (MCTSTree.RunOne, MCTSTree.SelectLeaf) to avoid the indirection of
// the exported wrapper.
func (c *MCTSConfig[S, A]) applyDefaults() {
	if c.Simulations <= 0 {
		c.Simulations = MCTSDefaultSimulations
	}
	if c.RolloutMaxSteps <= 0 {
		c.RolloutMaxSteps = MCTSDefaultRolloutMaxSteps
	}
	if c.MaxTreeDepth <= 0 {
		c.MaxTreeDepth = MCTSDefaultMaxTreeDepth
	}
	if c.Exploration <= 0 {
		c.Exploration = MCTSDefaultExploration
	}
}
