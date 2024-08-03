package phenomena

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// StateTransitionIteration is essentially a state machine which transitions
// between states according to the event rate parameters.
type StateTransitionIteration struct {
	unitUniformDist *distuv.Uniform
	rateSlices      [][]int
}

func (s *StateTransitionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	seed := settings.Seeds[partitionIndex]
	rand.Seed(seed)

	s.unitUniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewSource(seed),
	}
	s.rateSlices = make([][]int, 0)
	i := 0
	transTotal := 0
	for {
		trans, ok :=
			settings.Params[partitionIndex]["transitions_from_"+strconv.Itoa(i)]
		if !ok {
			break
		}
		s.rateSlices = append(s.rateSlices, []int{transTotal, transTotal + len(trans)})
		i += 1
		transTotal += len(trans)
	}
}

func (s *StateTransitionIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	state := make([]float64, 0)
	state = append(state, stateHistories[partitionIndex].Values.RawRowView(0)...)
	cumulative := timestepsHistory.NextIncrement
	cumulatives := make([]float64, 0)
	cumulatives = append(cumulatives, cumulative)
	slices := s.rateSlices[int(state[0])]
	transitionRates := params["transition_rates"][slices[0]:slices[1]]
	for _, rate := range transitionRates {
		cumulative += 1.0 / rate
		cumulatives = append(cumulatives, cumulative)
	}
	transitions := params["transitions_from_"+strconv.Itoa(int(state[0]))]
	event := s.unitUniformDist.Rand()
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
