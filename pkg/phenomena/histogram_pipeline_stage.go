package phenomena

import (
	"strconv"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// HistogramPipelineStageIteration evolves a table of entity counts as its
// state as well as both checking for entities entering and indicating that
// entities are leaving this stage for another one.
type HistogramPipelineStageIteration struct {
	unitUniformDist *distuv.Uniform
}

func (h *HistogramPipelineStageIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	seed := settings.Seeds[partitionIndex]
	rand.Seed(seed)

	h.unitUniformDist = &distuv.Uniform{
		Min: 0.0,
		Max: 1.0,
		Src: rand.NewSource(seed),
	}
}

func (h *HistogramPipelineStageIteration) Iterate(
	params simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	state := make([]float64, 0)
	state = append(state, stateHistories[partitionIndex].Values.RawRowView(0)...)
	for _, index := range params["upstream_partitions"] {
		if entity := params["entity_from_partition_"+
			strconv.Itoa(int(index))][0]; entity >= 0.0 {
			state[int(entity)] += 1
			params["entity_from_partition_"+strconv.Itoa(int(index))][0] = -1.0
		}
	}
	downstreams := params["downstream_partitions"]
	numEntityTypes := len(params["entity_dispatch_probs"])
	cumulative := timestepsHistory.NextIncrement
	cumulatives := make([]float64, 0)
	cumulatives = append(cumulatives, cumulative)
	for _, rate := range params["downstream_flow_rates"] {
		cumulative += 1.0 / rate
		cumulatives = append(cumulatives, cumulative)
	}
	event := h.unitUniformDist.Rand()
	if event*cumulative < cumulatives[0] {
		// minus number indicates nothing sent this step
		for i := range downstreams {
			state[numEntityTypes+i] = -1.0
		}
		return state
	}
	entityCumulative := 0.0
	entities := make([]int, 0)
	entityCumulatives := make([]float64, 0)
	probs := params["entity_dispatch_probs"]
	for i := 0; i < numEntityTypes; i++ {
		prob := state[i] * probs[i]
		if prob == 0 {
			continue
		}
		entityCumulative += prob
		entities = append(entities, i)
		entityCumulatives = append(entityCumulatives, entityCumulative)
	}
	if len(entityCumulatives) == 0 {
		return state
	}
	whichEntity := entities[len(entities)-1]
	entityEvent := h.unitUniformDist.Rand()
	for i, c := range entityCumulatives {
		if entityEvent*entityCumulative < c {
			whichEntity = entities[i]
			break
		}
	}
	state[whichEntity] -= 1
	for i, c := range cumulatives {
		if event*cumulative < c {
			state[numEntityTypes-1+i] = float64(whichEntity)
			return state
		}
	}
	state[len(state)-1] = float64(whichEntity)
	return state
}
