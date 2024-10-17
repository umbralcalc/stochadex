package discrete

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// CategoricalStateTransitionIteration is essentially a state machine which
// transitions between states according to the event rate parameters.
type CategoricalStateTransitionIteration struct {
	unitUniformDist *distuv.Uniform
	rateSlices      [][]int
}

func (c *CategoricalStateTransitionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	seed := settings.Seeds[partitionIndex]
	rand.Seed(seed)

	c.unitUniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewSource(seed),
	}
	c.rateSlices = make([][]int, 0)
	i := 0
	transTotal := 0
	for {
		trans, ok :=
			settings.Params[partitionIndex].GetOk("transitions_from_" + strconv.Itoa(i))
		if !ok {
			break
		}
		c.rateSlices = append(c.rateSlices, []int{transTotal, transTotal + len(trans)})
		i += 1
		transTotal += len(trans)
	}
}

func (c *CategoricalStateTransitionIteration) Iterate(
	params simulator.Params,
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
	event := c.unitUniformDist.Rand()
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
