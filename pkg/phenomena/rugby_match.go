package phenomena

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// GetRugbyMatchPossessionMap returns a map from the integer possession Id to a
// string description.
func GetRugbyMatchPossessionMap() map[int]string {
	return map[int]string{0: "Home Team", 1: "Away Team"}
}

// GetRugbyMatchStateMap returns a map from the integer state Id to a
// string description.
func GetRugbyMatchStateMap() map[int]string {
	return map[int]string{
		0:  "Penalty",
		1:  "Free Kick",
		2:  "Goal",
		3:  "Drop Goal",
		4:  "Try",
		5:  "Kick Phase",
		6:  "Run Phase",
		7:  "Knock-on",
		8:  "Scrum",
		9:  "Lineout",
		10: "Ruck",
		11: "Maul",
	}
}

// GetRugbyMatchTransitionProbabilities returns a tuple of one slice to record the
// prospective next state to transition into from the current state and another slice
// to record the probability associated to each of these transitions in turn.
func GetRugbyMatchTransitionProbabilities(
	matchStateFrom int,
	matchStateTo int,
	state *simulator.State,
	otherParams *simulator.OtherParams,
	timestep int,
) ([]int64, []float64) {
	matchState := fmt.Sprintf("%d", int(state.Values.AtVec(0)))
	transitions := otherParams.IntParams["transitions_from_"+matchState]
	transitionProbs := otherParams.FloatParams["transition_probs_from_"+matchState]
	return transitions, transitionProbs
}

// RugbyMatchIteration defines an iteration for a model of a rugby match
// which was defined in this chapter of the book Diffusing Ideas:
// https://umbralcalc.github.io/diffusing-ideas/managing_a_rugby_match/chapter.pdf
type RugbyMatchIteration struct {
	unitUniformDist *distuv.Uniform
}

func (r *RugbyMatchIteration) Iterate(
	otherParams *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.TimestepsHistory,
) *simulator.State {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		if otherParams.FloatParams["rates"][i] > (otherParams.FloatParams["rates"][i]+
			(1.0/timestepsHistory.NextIncrement))*r.unitUniformDist.Rand() {
			values[i] = stateHistory.Values.At(0, i) + 1.0
		} else {
			values[i] = stateHistory.Values.At(0, i)
		}
	}
	return &simulator.State{
		Values: mat.NewVecDense(
			stateHistory.StateWidth,
			values,
		),
		StateWidth: stateHistory.StateWidth,
	}
}

// NewRugbyMatchIteration creates a new RugbyMatchIteration given a seed.
func NewRugbyMatchIteration(seed uint64) *RugbyMatchIteration {
	return &RugbyMatchIteration{
		unitUniformDist: &distuv.Uniform{
			Min: 0.0,
			Max: 1.0,
			Src: rand.NewSource(seed),
		},
	}
}
