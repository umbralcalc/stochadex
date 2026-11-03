package discrete

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/rng"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// CategoricalStateTransitionIteration is essentially a state machine which
// transitions between states according to the event rate parameters.
//
// Update:
//
//	s = x[0],  r = transition_rates[o_s : o_s + m_s],  d = transitions_from_s
//	x[0] <- d[j]  with probability r[j] / (1/dt + sum(r)),  for j = 0..m_s-1
//	x[0] <- s     with probability (1/dt) / (1/dt + sum(r))
//	One U ~ U(0, 1) per step picks the outcome. m_s = len(transitions_from_s) and
//	o_s = m_0 + ... + m_(s-1): transition_rates holds the rates for every
//	transitions_from_<k> list concatenated in k order. Other elements of x are unchanged.
type CategoricalStateTransitionIteration struct {
	sampler    *rng.Sampler
	rateSlices [][]int
}

// Configure implements simulator.Iteration.
func (c *CategoricalStateTransitionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	seed := settings.Iterations[partitionIndex].Seed
	c.sampler = rng.New(seed)
	c.rateSlices = make([][]int, 0)
	i := 0
	transTotal := 0
	for {
		trans, ok := settings.Iterations[partitionIndex].Params.GetOk(
			"transitions_from_" + strconv.Itoa(i))
		if !ok {
			break
		}
		c.rateSlices = append(
			c.rateSlices,
			[]int{transTotal, transTotal + len(trans)},
		)
		i += 1
		transTotal += len(trans)
	}
}

// Iterate implements simulator.Iteration.
func (c *CategoricalStateTransitionIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	state := make([]float64, 0)
	state = append(state, stateHistories[partitionIndex].Values.RawRowView(0)...)
	cumulative := 1.0 / timestepsHistory.NextIncrement
	cumulatives := make([]float64, 0)
	cumulatives = append(cumulatives, cumulative)
	slices := c.rateSlices[int(state[0])]
	transitionRates := params.Get("transition_rates")[slices[0]:slices[1]]
	for _, rate := range transitionRates {
		cumulative += rate
		cumulatives = append(cumulatives, cumulative)
	}
	transitions := params.Get("transitions_from_" + strconv.Itoa(int(state[0])))
	event := c.sampler.Float64()
	if event*cumulative < cumulatives[0] {
		return state
	}
	for i, c := range cumulatives {
		if event*cumulative < c {
			state[0] = float64(transitions[i-1])
			return state
		}
	}
	state[0] = float64(transitions[len(transitions)-1])
	return state
}
