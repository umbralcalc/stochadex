package analysis

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// StateTimeHistories is a collection of simulator state histories for
// named partitions that have a cumulative timestep value associated to
// each row in the history.
type StateTimeHistories struct {
	StateHistories   map[string]*simulator.StateHistory
	TimestepsHistory *simulator.CumulativeTimestepsHistory
}
