package observations

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
)

// FalseStaticPartialStateObservationIteration allows for false negatives or
// positives to be observed in some ongoing process of detections with binary encoding.
type FalseStaticPartialStateObservationIteration struct {
	bernoulliDist  *distuv.Bernoulli
	falsePositives float64
}

func (f *FalseStaticPartialStateObservationIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	f.bernoulliDist = &distuv.Bernoulli{
		P:   1.0,
		Src: rand.NewSource(settings.Seeds[partitionIndex]),
	}
	f.falsePositives = float64(settings.OtherParams[partitionIndex].
		IntParams["false_positives"][0])
}

func (f *FalseStaticPartialStateObservationIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	outputValues := make([]float64, 0)
	stateValues := params.FloatParams["values_to_observe"]
	probs := params.FloatParams["false_observation_probs"]
	for i, index := range params.IntParams["state_value_observation_indices"] {
		f.bernoulliDist.P = probs[i]
		value := stateValues[index] + (2.0 * (f.falsePositives - 0.5) * f.bernoulliDist.Rand())
		if value < 0.0 {
			value = 0.0
		} else if value > 1.0 {
			value = 1.0
		}
		outputValues = append(outputValues, value)
	}
	return outputValues
}
