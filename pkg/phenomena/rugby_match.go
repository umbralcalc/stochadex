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
	nextState       int64
	lastState       int64
	playDirection   float64
	normalDist      *distuv.Normal
	unitUniformDist *distuv.Uniform
	exponentialDist *distuv.Exponential
	categoricalDist *distuv.Categorical
}

func (r *RugbyMatchIteration) getPossessionChange(
	state []float64,
	otherParams *simulator.OtherParams,
	timestepsHistory *simulator.TimestepsHistory,
) []float64 {
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
	if rate > (rate+(1.0/timestepsHistory.NextIncrement))*r.unitUniformDist.Rand() {
		state[1] = (1.0 - state[1])
	}
	return state
}

func (r *RugbyMatchIteration) getLongitudinalRunChange(
	state []float64,
	otherParams *simulator.OtherParams,
) []float64 {
	newLonState := state[2]
	attackerIndex := r.currentAttacker + int(15*state[1])
	defenderIndex := r.currentDefender + int(15*(1-state[1]))
	r.exponentialDist.Rate =
		otherParams.FloatParams["player_defensive_run_scales"][defenderIndex]
	newLonState -= r.playDirection * r.exponentialDist.Rand()
	r.exponentialDist.Rate =
		otherParams.FloatParams["player_attacking_run_scales"][attackerIndex]
	newLonState += r.playDirection * r.exponentialDist.Rand()
	// if the newLonState would end up moving over a tryline, just restrict
	// this movement so that it remains just within the field of play
	maxLon, _ := GetRugbyMatchPitchDimensions()
	if newLonState > maxLon {
		newLonState = maxLon - 0.5
	}
	if newLonState < 0.0 {
		newLonState = 0.5
	}
	state[2] = newLonState
	return state
}

func (r *RugbyMatchIteration) getLateralRunChange(
	state []float64,
	otherParams *simulator.OtherParams,
) []float64 {
	r.normalDist.Mu = 0.0
	r.normalDist.Sigma = otherParams.FloatParams["lateral_run_scale"][0]
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
	state[3] = newLatState
	return state
}

func (r *RugbyMatchIteration) getLongitudinalKickChange(
	state []float64,
	otherParams *simulator.OtherParams,
) []float64 {
	maxLon, _ := GetRugbyMatchPitchDimensions()
	// if this is a kick at goal or a drop goal don't move
	if ((r.lastState == 0) && (state[0] == 2)) ||
		((r.lastState == 5) && (state[0] == 3)) {
		return state
	}
	// if this is a kick in the field of play
	if (r.lastState == 5) && (state[0] != 3) && (state[0] != 9) {
		newLonState := state[2]
		// choose a kicker at random
		possibleKickers := []float64{9, 10, 11, 14, 15}
		r.currentAttacker = int(possibleKickers[int(rand.Intn(5))])
		var kickerIndex int
		if (r.currentAttacker == 9) || (r.currentAttacker == 10) {
			kickerIndex = (r.currentAttacker - 9) + 2*int(state[1])
			r.exponentialDist.Rate =
				otherParams.FloatParams["halves_kick_scales"][kickerIndex]
		} else {
			if r.currentAttacker == 11 {
				kickerIndex = 3 * int(state[1])
			} else {
				kickerIndex = (r.currentAttacker - 13) + 3*int(state[1])
			}
			r.exponentialDist.Rate =
				otherParams.FloatParams["back_three_kick_scales"][kickerIndex]
		}
		newLonState += r.playDirection * r.exponentialDist.Rand()
		if newLonState >= maxLon {
			newLonState = maxLon - 0.5
		}
		state[2] = newLonState
		return state
	}
	// if this is a kick to touch
	if (r.lastState == 5) && (state[0] == 9) {
		return state
	}
	return state
}

func (r *RugbyMatchIteration) getLateralKickChange(
	state []float64,
	otherParams *simulator.OtherParams,
) []float64 {
	_, maxLat := GetRugbyMatchPitchDimensions()
	// if this is a kick at goal or a drop goal don't move
	if ((r.lastState == 0) && (state[0] == 2)) ||
		((r.lastState == 5) && (state[0] == 3)) {
		return state
	}
	// if this is a kick in the field of play
	if (r.lastState == 5) && (state[0] != 3) && (state[0] != 9) {
		state[3] = maxLat * r.unitUniformDist.Rand()
		return state
	}
	// if this is a kick to touch
	if (r.lastState == 5) && (state[0] == 9) {
		if r.unitUniformDist.Rand() > 0.5 {
			state[3] = maxLat
		} else {
			state[3] = 0.0
		}
	}
	return state
}

func (r *RugbyMatchIteration) getKickAtGoalSuccess(
	state []float64,
	otherParams *simulator.OtherParams,
) bool {
	maxLon, maxLat := GetRugbyMatchPitchDimensions()
	success := r.unitUniformDist.Rand() <
		otherParams.FloatParams["goal_probabilities"][int(state[1])]
	midPitch := 0.5 * maxLat
	if success {
		// move ball back to halfway line for kickoff
		line50m := 0.5 * maxLon
		state[2] = line50m
		state[3] = midPitch
	} else {
		// move ball to 22 for a dropout (another kind of kickoff event)
		line22m := maxLon * (0.78*state[1] + 0.22*(1-state[1]))
		state[2] = line22m
		state[3] = midPitch
	}
	return success
}

func (r *RugbyMatchIteration) updateScoreAndBallLocation(
	state []float64,
	otherParams *simulator.OtherParams,
) []float64 {
	maxLon, maxLat := GetRugbyMatchPitchDimensions()
	// update either home team or away team scores with this index
	scorerIndex := int(5*state[1] + 4*(1-state[1]))
	line22m := maxLon * (0.78*state[1] + 0.22*(1-state[1]))
	midPitch := 0.5 * maxLat
	// update home team score
	if (state[0] == 2) || (state[0] == 3) {
		if r.getKickAtGoalSuccess(state, otherParams) {
			state[scorerIndex] += 3.0
		} else {
			// if unsuccessful with a penalty or drop goal, restart with dropout
			state[2] = line22m
			state[3] = midPitch
		}
	} else if state[0] == 4 {
		state[scorerIndex] += 5.0
		// always by default move the ball back to 22m line after a try is
		// scored, ready to kick at goal
		state[2] = line22m
		if r.getKickAtGoalSuccess(state, otherParams) {
			state[scorerIndex] += 2.0
		}
	}
	return state
}

func (r *RugbyMatchIteration) possessionChangeCanOccur(state []float64) bool {
	cantOccur := []float64{0, 1, 2, 3, 4, 7, 12}
	for _, value := range cantOccur {
		if value == state[0] {
			return false
		}
	}
	return true
}

func (r *RugbyMatchIteration) Iterate(
	otherParams *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.TimestepsHistory,
) *simulator.State {
	state := make([]float64, 0)
	state = append(state, stateHistories[partitionIndex].Values.RawRowView(0)...)
	r.playDirection = 1.0*state[1] - 1.0*(1-state[1])
	stateWidth := stateHistories[partitionIndex].StateWidth
	matchState := fmt.Sprintf("%d", int(state[0]))
	transitions := otherParams.IntParams["transitions_from_"+matchState]
	// if we are currently not planned to do anything, find the next transition
	if state[0] == float64(r.nextState) {
		// compute the cumulative rates and overall normalisation for transitions
		cumulative := 0.0
		cumulativeProbs := make([]float64, 0)
		transitionProbs := otherParams.FloatParams["transition_probs_from_"+matchState]
		for _, prob := range transitionProbs {
			cumulative += prob
			cumulativeProbs = append(cumulativeProbs, cumulative)
		}
		normalisation := cumulativeProbs[len(cumulativeProbs)-1]
		transitionEvent := r.unitUniformDist.Rand()
		for i, prob := range cumulativeProbs {
			if transitionEvent*normalisation < prob {
				if (i == 0) || (transitionEvent*normalisation >= cumulativeProbs[i-1]) {
					r.nextState = transitions[i]
					break
				}
			}
		}
	}
	// figure out if the next event should happen yet
	probDoNothing := 1.0 / (1.0 + timestepsHistory.NextIncrement*
		otherParams.FloatParams["background_event_rates"][r.nextState])
	event := r.unitUniformDist.Rand()
	if event < probDoNothing {
		// if the state hasn't changed then continue without doing anything else
		return &simulator.State{
			Values:     mat.NewVecDense(stateWidth, state),
			StateWidth: stateWidth,
		}
	} else {
		// else change the state
		r.lastState = int64(state[0])
		state[0] = float64(r.nextState)
	}
	// if at kickoff, reset the ball location to be central and continue
	if state[0] == 12 {
		maxLon, maxLat := GetRugbyMatchPitchDimensions()
		state[2] = 0.5 * maxLon
		state[3] = 0.5 * maxLat
		return &simulator.State{
			Values:     mat.NewVecDense(stateWidth, state),
			StateWidth: stateWidth,
		}
	}
	// if a knock-on has led to a scrum, change possession and continue
	if (r.lastState == 7) && (state[0] == 8) {
		state[1] = (1 - state[1])
		return &simulator.State{
			Values:     mat.NewVecDense(stateWidth, state),
			StateWidth: stateWidth,
		}
	}
	// randomly select new attacking and defending player indices
	r.currentAttacker = int(r.categoricalDist.Rand())
	r.currentDefender = int(r.categoricalDist.Rand())
	// handle scoring if there was a score event and then continue
	if (state[0] == 2) || (state[0] == 3) || (state[0] == 4) {
		state = r.updateScoreAndBallLocation(state, otherParams)
		return &simulator.State{
			Values:     mat.NewVecDense(stateWidth, state),
			StateWidth: stateWidth,
		}
	}
	// find out if there is a change in possession if possible
	if r.possessionChangeCanOccur(state) {
		state = r.getPossessionChange(state, otherParams, timestepsHistory)
	}
	// if the next phase is a run phase and we are entering this for the first time
	// then decide on what spatial movements the ball location makes as a result
	if state[0] == 6 {
		state = r.getLongitudinalRunChange(state, otherParams)
		state = r.getLateralRunChange(state, otherParams)
	}
	// similarly, if the next phase is a kick phase and we are entering this for
	// the first time then decide on what spatial movements the ball location makes
	if state[0] == 5 {
		state = r.getLongitudinalKickChange(state, otherParams)
		state = r.getLateralKickChange(state, otherParams)
	}
	return &simulator.State{
		Values:     mat.NewVecDense(stateWidth, state),
		StateWidth: stateWidth,
	}
}

// NewRugbyMatchIteration creates a new RugbyMatchIteration given a seed.
func NewRugbyMatchIteration(seed uint64) *RugbyMatchIteration {
	weights := make([]float64, 0)
	for i := 0; i < 15; i++ {
		weights = append(weights, 1.0)
	}
	catDist := distuv.NewCategorical(weights, rand.NewSource(seed))
	rand.Seed(seed)
	return &RugbyMatchIteration{
		nextState: 12,
		lastState: 12,
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
