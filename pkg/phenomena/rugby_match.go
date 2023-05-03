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
		12: "Kickoff",
	}
}

// RugbyMatchIteration defines an iteration for a model of a rugby match
// which was defined in this chapter of the book Diffusing Ideas:
// https://umbralcalc.github.io/diffusing-ideas/managing_a_rugby_match/chapter.pdf
type RugbyMatchIteration struct {
	currentAttacker int
	currentDefender int
	normalDist      *distuv.Normal
	unitUniformDist *distuv.Uniform
	exponentialDist *distuv.Exponential
}

func (r *RugbyMatchIteration) getPossessionChange(
	state []float64,
	otherParams *simulator.OtherParams,
	timestepsHistory *simulator.TimestepsHistory,
) float64 {
	rate := 0.0
	newPossessionState := state[1]
	if rate > (rate+(1.0/timestepsHistory.NextIncrement))*r.unitUniformDist.Rand() {
		newPossessionState = (1.0 - state[1])
	}
	return newPossessionState
}

func (r *RugbyMatchIteration) getLongitudinalRunChange(
	state []float64,
	otherParams *simulator.OtherParams,
) float64 {
	newLonState := state[2]
	attackerIndex := r.currentAttacker + int(15*state[1])
	defenderIndex := r.currentDefender + int(15*(1-state[1]))
	r.exponentialDist.Rate =
		otherParams.FloatParams["player_defensive_run_param"][defenderIndex]
	newLonState -= r.exponentialDist.Rand()
	r.exponentialDist.Rate =
		otherParams.FloatParams["player_attacking_run_param"][attackerIndex]
	newLonState += r.exponentialDist.Rand()
	return newLonState
}

func (r *RugbyMatchIteration) getLateralRunChange(
	state []float64,
	otherParams *simulator.OtherParams,
) float64 {
	index := r.currentAttacker + int(15*state[1])
	r.normalDist.Mu = 0.0
	r.normalDist.Sigma = otherParams.FloatParams["player_lateral_run_sigma"][index]
	return state[3] + r.normalDist.Rand()
}

func (r *RugbyMatchIteration) getLongitudinalKickChange(
	state []float64,
	otherParams *simulator.OtherParams,
) float64 {
	return 0.0
}

func (r *RugbyMatchIteration) getLateralKickChange(
	state []float64,
	otherParams *simulator.OtherParams,
) float64 {
	return 0.0
}

func (r *RugbyMatchIteration) Iterate(
	otherParams *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.TimestepsHistory,
) *simulator.State {
	state := stateHistories[partitionIndex].Values.RawRowView(0)
	lastState := state
	matchState := fmt.Sprintf("%d", int(state[0]))
	transitions := otherParams.IntParams["transitions_from_"+matchState]
	transitionProbs := otherParams.FloatParams["transition_probs_from_"+matchState]
	// compute the cumulative probabilities and overall normalisation for transitions
	cumulative := 0.0
	cumulativeProbs := make([]float64, 0)
	normalisation := (1.0 / timestepsHistory.NextIncrement)
	for i, prob := range transitionProbs {
		normalisation +=
			prob * otherParams.FloatParams["background_event_rates"][transitions[i]]
		cumulative += prob
		cumulativeProbs = append(cumulativeProbs, cumulative)
	}
	// find out what the next transition is
	event := r.unitUniformDist.Rand()
	for i, prob := range cumulativeProbs {
		if event*normalisation < prob {
			if i == 0 {
				state[0] = float64(transitions[i])
			} else if event*normalisation >= cumulativeProbs[i-1] {
				state[0] = float64(transitions[i])
			}
		}
	}
	// find out if there is a change in possession
	state[1] = r.getPossessionChange(state, otherParams, timestepsHistory)
	// if the next phase is a run phase and we are entering this for the first time
	// then decide on what spatial movements the ball location makes as a result
	if (lastState[0] != 6) && (state[0] == 6) {
		state[2] = r.getLongitudinalRunChange(state, otherParams)
		state[3] = r.getLateralRunChange(state, otherParams)
	}
	// similarly, if the next phase is a kick phase and we are entering this for
	// the first time then decide on what spatial movements the ball location makes
	if (lastState[0] != 5) && (state[0] == 5) {
		state[2] = r.getLongitudinalKickChange(state, otherParams)
		state[3] = r.getLateralKickChange(state, otherParams)
	}
	return &simulator.State{
		Values: mat.NewVecDense(
			stateHistories[partitionIndex].StateWidth,
			state,
		),
		StateWidth: stateHistories[partitionIndex].StateWidth,
	}
}

// NewRugbyMatchIteration creates a new RugbyMatchIteration given a seed.
func NewRugbyMatchIteration(seed uint64) *RugbyMatchIteration {
	return &RugbyMatchIteration{
		normalDist: &distuv.Normal{
			Mu:    0.0,
			Sigma: 1.0,
			Src:   rand.NewSource(seed),
		},
		unitUniformDist: &distuv.Uniform{
			Min: 0.0,
			Max: 1.0,
			Src: rand.NewSource(seed),
		},
		exponentialDist: &distuv.Exponential{
			Rate: 1.0,
			Src:  rand.NewSource(seed),
		},
	}
}
