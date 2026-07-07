package rugby

import (
	"github.com/umbralcalc/stochadex/pkg/general"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Match state indices.
const (
	StateIdxHomeScore        = 0
	StateIdxAwayScore        = 1
	StateIdxScoreDiff        = 2
	StateIdxHomeActiveYellow = 3
	StateIdxAwayActiveYellow = 4
	StateIdxHalf             = 5
	MatchStateWidth          = 6
)

// YellowCardMinutes is the duration a yellow card is active.
const YellowCardMinutes = 10

// MatchStateFunction derives match state from the cumulative score_events,
// conversion_events, and card_events partitions. It reads:
//   - "score_values": latest state of score_events [home_tries, away_tries, home_penalties, away_penalties]
//   - "conversion_values": latest state of conversion_events [home_conv, away_conv]
//   - "card_values": latest state of card_events [home_yellows, away_yellows]
//
// It also uses the card_events state history to compute active yellow cards
// (cards received in the last YellowCardMinutes steps).
func MatchStateFunction(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	scoreValues := params.Get("score_values")
	convValues := params.Get("conversion_values")
	cardValues := params.Get("card_values")

	homeTries := scoreValues[0]
	awayTries := scoreValues[1]
	homePenalties := scoreValues[2]
	awayPenalties := scoreValues[3]
	homeConv := convValues[0]
	awayConv := convValues[1]

	homeScore := homeTries*5.0 + homeConv*2.0 + homePenalties*3.0
	awayScore := awayTries*5.0 + awayConv*2.0 + awayPenalties*3.0
	scoreDiff := homeScore - awayScore

	// Active yellow cards: count cards in last YellowCardMinutes.
	// cardValues is cumulative, so active = current - value_from_10_steps_ago.
	cardPartitionIndices := params.Get("card_partition")
	cardPartitionIndex := int(cardPartitionIndices[0])
	cardHistory := stateHistories[cardPartitionIndex]
	historyDepth := cardHistory.StateHistoryDepth
	lookback := YellowCardMinutes
	if lookback > historyDepth-1 {
		lookback = historyDepth - 1
	}
	homeActiveYellow := cardValues[0] - cardHistory.Values.At(lookback, 0)
	awayActiveYellow := cardValues[1] - cardHistory.Values.At(lookback, 1)

	// Half: 0 for first half (time < 40), 1 for second half.
	half := 0.0
	if timestepsHistory.Values.AtVec(0) >= 40.0 {
		half = 1.0
	}

	return []float64{
		homeScore,
		awayScore,
		scoreDiff,
		homeActiveYellow,
		awayActiveYellow,
		half,
	}
}

// NewMatchStatePartition creates a partition that derives match state from
// score_events, conversion_events, and card_events partitions.
func NewMatchStatePartition() *simulator.PartitionConfig {
	return &simulator.PartitionConfig{
		Name: "match_state",
		Iteration: &general.ValuesFunctionIteration{
			Function: MatchStateFunction,
		},
		Params: simulator.NewParams(make(map[string][]float64)),
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"score_values":      {Upstream: "score_events"},
			"conversion_values": {Upstream: "conversion_events"},
			"card_values":       {Upstream: "card_events"},
		},
		ParamsAsPartitions: map[string][]string{
			"card_partition": {"card_events"},
		},
		InitStateValues:   make([]float64, MatchStateWidth),
		StateHistoryDepth: 1,
		Seed:              0,
	}
}
