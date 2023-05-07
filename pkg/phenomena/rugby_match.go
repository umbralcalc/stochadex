package phenomena

import (
	"fmt"
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distuv"
)

// GetRugbyMatchPitchDimensions returns a tuple of pitch dimensions (Lon, Lat).
func GetRugbyMatchPitchDimensions() (float64, float64) {
	return 100.0, 70.0
}

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

// getPlayerFatigue is an internal method to retrieve a player's fatigue factor
func getPlayerFatigue(
	playerIndex int,
	otherParams *simulator.OtherParams,
	timestepsHistory *simulator.TimestepsHistory,
) float64 {
	return math.Exp(
		-otherParams.FloatParams["player_fatigue_rates"][playerIndex] *
			(timestepsHistory.Values.AtVec(0) -
				otherParams.FloatParams["player_start_times"][playerIndex]),
	)
}

// getScrumPossessionFactor is an internal method to retrieve the player weightings
// for the scrum possession transition probability
func getScrumPossessionFactor(
	state []float64,
	otherParams *simulator.OtherParams,
	timestepsHistory *simulator.TimestepsHistory,
) float64 {
	playersFactor := 0.0
	norm := 0.0
	for i := 0; i < 3; i++ {
		attackingFrontRowPos :=
			otherParams.FloatParams["front_row_scrum_possessions"][i+int(3*state[1])] *
				getPlayerFatigue(i+int(15*state[1]), otherParams, timestepsHistory)
		defendingFrontRowPos :=
			otherParams.FloatParams["front_row_scrum_possessions"][i+int(3*(1-state[1]))] *
				getPlayerFatigue(i+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingFrontRowPos
		norm += attackingFrontRowPos + defendingFrontRowPos
	}
	for i := 0; i < 2; i++ {
		attackingSecondRowPos :=
			otherParams.FloatParams["second_row_scrum_possessions"][i+int(2*state[1])] *
				getPlayerFatigue(i+3+int(15*state[1]), otherParams, timestepsHistory)
		defendingSecondRowPos :=
			otherParams.FloatParams["second_row_scrum_possessions"][i+int(2*(1-state[1]))] *
				getPlayerFatigue(i+3+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingSecondRowPos
		norm += attackingSecondRowPos + defendingSecondRowPos
	}
	playersFactor /= norm
	return playersFactor
}

// getLineoutPossessionFactor is an internal method to retrieve the player weightings
// for the lineout possession transition probability
func getLineoutPossessionFactor(
	state []float64,
	otherParams *simulator.OtherParams,
	timestepsHistory *simulator.TimestepsHistory,
) float64 {
	playersFactor := 0.0
	norm := 0.0
	for i := 0; i < 3; i++ {
		attackingFrontRowPos :=
			otherParams.FloatParams["front_row_lineout_possessions"][i+int(3*state[1])] *
				getPlayerFatigue(i+int(15*state[1]), otherParams, timestepsHistory)
		defendingFrontRowPos :=
			otherParams.FloatParams["front_row_lineout_possessions"][i+int(3*(1-state[1]))] *
				getPlayerFatigue(i+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingFrontRowPos
		norm += attackingFrontRowPos + defendingFrontRowPos
	}
	for i := 0; i < 2; i++ {
		attackingSecondRowPos :=
			otherParams.FloatParams["second_row_lineout_possessions"][i+int(2*state[1])] *
				getPlayerFatigue(i+3+int(15*state[1]), otherParams, timestepsHistory)
		defendingSecondRowPos :=
			otherParams.FloatParams["second_row_lineout_possessions"][i+int(2*(1-state[1]))] *
				getPlayerFatigue(i+3+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingSecondRowPos
		norm += attackingSecondRowPos + defendingSecondRowPos
	}
	for i := 0; i < 3; i++ {
		attackingBackRowPos :=
			otherParams.FloatParams["back_row_lineout_possessions"][i+int(3*state[1])] *
				getPlayerFatigue(i+5+int(15*state[1]), otherParams, timestepsHistory)
		defendingBackRowPos :=
			otherParams.FloatParams["back_row_lineout_possessions"][i+int(3*(1-state[1]))] *
				getPlayerFatigue(i+5+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingBackRowPos
		norm += attackingBackRowPos + defendingBackRowPos
	}
	playersFactor /= norm
	return playersFactor
}

// getMaulPossessionFactor is an internal method to retrieve the player weightings
// for the maul possession transition probability
func getMaulPossessionFactor(
	state []float64,
	otherParams *simulator.OtherParams,
	timestepsHistory *simulator.TimestepsHistory,
) float64 {
	playersFactor := 0.0
	norm := 0.0
	for i := 0; i < 3; i++ {
		attackingFrontRowPos :=
			otherParams.FloatParams["front_row_maul_possessions"][i+int(3*state[1])] *
				getPlayerFatigue(i+int(15*state[1]), otherParams, timestepsHistory)
		defendingFrontRowPos :=
			otherParams.FloatParams["front_row_maul_possessions"][i+int(3*(1-state[1]))] *
				getPlayerFatigue(i+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingFrontRowPos
		norm += attackingFrontRowPos + defendingFrontRowPos
	}
	for i := 0; i < 2; i++ {
		attackingSecondRowPos :=
			otherParams.FloatParams["second_row_maul_possessions"][i+int(2*state[1])] *
				getPlayerFatigue(i+3+int(15*state[1]), otherParams, timestepsHistory)
		defendingSecondRowPos :=
			otherParams.FloatParams["second_row_maul_possessions"][i+int(2*(1-state[1]))] *
				getPlayerFatigue(i+3+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingSecondRowPos
		norm += attackingSecondRowPos + defendingSecondRowPos
	}
	for i := 0; i < 3; i++ {
		attackingBackRowPos :=
			otherParams.FloatParams["back_row_maul_possessions"][i+int(3*state[1])] *
				getPlayerFatigue(i+5+int(15*state[1]), otherParams, timestepsHistory)
		defendingBackRowPos :=
			otherParams.FloatParams["back_row_maul_possessions"][i+int(3*(1-state[1]))] *
				getPlayerFatigue(i+5+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingBackRowPos
		norm += attackingBackRowPos + defendingBackRowPos
	}
	playersFactor /= norm
	return playersFactor
}

// getRuckPossessionFactor is an internal method to retrieve the player weightings
// for the ruck possession transition probability
func getRuckPossessionFactor(
	state []float64,
	otherParams *simulator.OtherParams,
	timestepsHistory *simulator.TimestepsHistory,
) float64 {
	playersFactor := 0.0
	norm := 0.0
	for i := 0; i < 3; i++ {
		attackingFrontRowPos :=
			otherParams.FloatParams["front_row_ruck_possessions"][i+int(3*state[1])] *
				getPlayerFatigue(i+int(15*state[1]), otherParams, timestepsHistory)
		defendingFrontRowPos :=
			otherParams.FloatParams["front_row_ruck_possessions"][i+int(3*(1-state[1]))] *
				getPlayerFatigue(i+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingFrontRowPos
		norm += attackingFrontRowPos + defendingFrontRowPos
	}
	for i := 0; i < 2; i++ {
		attackingSecondRowPos :=
			otherParams.FloatParams["second_row_ruck_possessions"][i+int(2*state[1])] *
				getPlayerFatigue(i+3+int(15*state[1]), otherParams, timestepsHistory)
		defendingSecondRowPos :=
			otherParams.FloatParams["second_row_ruck_possessions"][i+int(2*(1-state[1]))] *
				getPlayerFatigue(i+3+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingSecondRowPos
		norm += attackingSecondRowPos + defendingSecondRowPos
	}
	for i := 0; i < 3; i++ {
		attackingBackRowPos :=
			otherParams.FloatParams["back_row_ruck_possessions"][i+int(3*state[1])] *
				getPlayerFatigue(i+5+int(15*state[1]), otherParams, timestepsHistory)
		defendingBackRowPos :=
			otherParams.FloatParams["back_row_ruck_possessions"][i+int(3*(1-state[1]))] *
				getPlayerFatigue(i+5+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingBackRowPos
		norm += attackingBackRowPos + defendingBackRowPos
	}
	for i := 0; i < 2; i++ {
		attackingCentresPos :=
			otherParams.FloatParams["centres_ruck_possessions"][i+int(2*state[1])] *
				getPlayerFatigue(i+11+int(15*state[1]), otherParams, timestepsHistory)
		defendingCentresPos :=
			otherParams.FloatParams["centres_ruck_possessions"][i+int(2*(1-state[1]))] *
				getPlayerFatigue(i+11+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingCentresPos
		norm += attackingCentresPos + defendingCentresPos
	}
	playersFactor /= norm
	return playersFactor
}

// getRunPossessionFactor is an internal method to retrieve the player weightings
// for the run possession transition probability
func getRunPossessionFactor(
	state []float64,
	otherParams *simulator.OtherParams,
	timestepsHistory *simulator.TimestepsHistory,
) float64 {
	playersFactor := 0.0
	norm := 0.0
	for i := 0; i < 15; i++ {
		attackingPos :=
			otherParams.FloatParams["player_run_possessions"][i+int(3*state[1])] *
				getPlayerFatigue(i+int(15*state[1]), otherParams, timestepsHistory)
		defendingPos :=
			otherParams.FloatParams["player_run_possessions"][i+int(3*(1-state[1]))] *
				getPlayerFatigue(i+int(15*(1-state[1])), otherParams, timestepsHistory)
		playersFactor += defendingPos
		norm += attackingPos + defendingPos
	}
	playersFactor /= norm
	return playersFactor
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
	categoricalDist *distuv.Categorical
}

func (r *RugbyMatchIteration) getPossessionChange(
	state []float64,
	otherParams *simulator.OtherParams,
	timestepsHistory *simulator.TimestepsHistory,
) float64 {
	rate := otherParams.FloatParams["max_possession_change_rates"][int(state[0])]
	playersFactor := 1.0
	if state[0] == 6 {
		playersFactor = getRunPossessionFactor(state, otherParams, timestepsHistory)
	} else if state[0] == 8 {
		playersFactor = getScrumPossessionFactor(state, otherParams, timestepsHistory)
	} else if state[0] == 9 {
		playersFactor = getLineoutPossessionFactor(state, otherParams, timestepsHistory)
	} else if state[0] == 10 {
		playersFactor = getRuckPossessionFactor(state, otherParams, timestepsHistory)
	} else if state[0] == 11 {
		playersFactor = getMaulPossessionFactor(state, otherParams, timestepsHistory)
	}
	rate *= playersFactor
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
		otherParams.FloatParams["player_defensive_run_scales"][defenderIndex]
	newLonState -= r.exponentialDist.Rand()
	r.exponentialDist.Rate =
		otherParams.FloatParams["player_attacking_run_scales"][attackerIndex]
	newLonState += r.exponentialDist.Rand()
	// if the newLonState would end up moving over a tryline, just restrict
	// this movement so that it remains just within the field of play
	maxLon, _ := GetRugbyMatchPitchDimensions()
	if newLonState > maxLon {
		newLonState = maxLon - 0.5
	}
	if newLonState < 0.0 {
		newLonState = 0.5
	}
	return newLonState
}

func (r *RugbyMatchIteration) getLateralRunChange(
	state []float64,
	otherParams *simulator.OtherParams,
) float64 {
	index := r.currentAttacker + int(15*state[1])
	r.normalDist.Mu = 0.0
	r.normalDist.Sigma =
		otherParams.FloatParams["player_lateral_run_scales"][index]
	newLatState := state[3] + r.normalDist.Rand()
	// if the newLatState would end up moving out of bounds, just restrict
	// this movement so that it remains just within the field of play
	_, maxLat := GetRugbyMatchPitchDimensions()
	if newLatState > maxLat {
		newLatState = maxLat - 0.5
	}
	if newLatState < 0.0 {
		newLatState = 0.5
	}
	return newLatState
}

func (r *RugbyMatchIteration) getLongitudinalKickChange(
	lastState []float64,
	state []float64,
	otherParams *simulator.OtherParams,
) float64 {
	// if this is a kick at goal or a drop goal
	if ((lastState[0] == 0) && (state[0] == 2)) ||
		((lastState[0] == 5) && (state[0] == 3)) {

	}
	// if this is a kick in the field of play
	if (lastState[0] == 5) && (state[0] != 3) && (state[0] != 9) {

	}
	// if this is a kick to touch
	if (lastState[0] == 5) && (state[0] == 9) {

	}
	return 0.0
}

func (r *RugbyMatchIteration) getLateralKickChange(
	lastState []float64,
	state []float64,
	otherParams *simulator.OtherParams,
) float64 {
	// if this is a kick at goal or a drop goal
	if ((lastState[0] == 0) && (state[0] == 2)) ||
		((lastState[0] == 5) && (state[0] == 3)) {

	}
	// if this is a kick in the field of play
	if (lastState[0] == 5) && (state[0] != 3) && (state[0] != 9) {

	}
	// if this is a kick to touch
	if (lastState[0] == 5) && (state[0] == 9) {

	}
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
	// if the state hasn't changed then continue without doing anything else
	if lastState[0] == state[0] {
		return &simulator.State{
			Values: mat.NewVecDense(
				stateHistories[partitionIndex].StateWidth,
				state,
			),
			StateWidth: stateHistories[partitionIndex].StateWidth,
		}
	}
	// randomly select new attacking and defending player indices
	r.currentAttacker = int(r.categoricalDist.Rand())
	r.currentDefender = int(r.categoricalDist.Rand())
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
		state[2] = r.getLongitudinalKickChange(lastState, state, otherParams)
		state[3] = r.getLateralKickChange(lastState, state, otherParams)
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
	weights := make([]float64, 0)
	for i := 0; i < 15; i++ {
		weights = append(weights, 1.0)
	}
	catDist := distuv.NewCategorical(weights, rand.NewSource(seed))
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
		categoricalDist: &catDist,
	}
}
