package phenomena

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/stat/distuv"
	"scientificgo.org/special"
)

// FractionalBrownianMotionIntegral computes the integral term in the fractional
// Brownian motion process defined here: https://en.wikipedia.org/wiki/Fractional_Brownian_motion
func FractionalBrownianMotionIntegral(
	currentTime float64,
	nextTime float64,
	hurstExponent float64,
	numberOfIntegrationSteps int,
) float64 {
	integralStepSize := (nextTime - currentTime) / float64(numberOfIntegrationSteps)
	a := []float64{hurstExponent - 0.5, 0.5 - hurstExponent}
	b := []float64{hurstExponent + 0.5}
	integralValue := 0.0
	// implements the trapezium rule in a loop over the steps
	// between the current and the next point in time
	for t := 0; t < numberOfIntegrationSteps; t++ {
		t1 := currentTime + float64(t)*integralStepSize
		t2 := t1 + integralStepSize
		functionValue1 := (math.Pow(t1-currentTime, hurstExponent-0.5) /
			math.Gamma(hurstExponent+0.5)) *
			special.HypPFQ(a, b, 1.0-t1/currentTime)
		functionValue2 := (math.Pow(t2-currentTime, hurstExponent-0.5) /
			math.Gamma(hurstExponent+0.5)) *
			special.HypPFQ(a, b, 1.0-t2/currentTime)
		integralValue += 0.5 * (functionValue1 + functionValue2) * integralStepSize
	}
	return integralValue
}

// FractionalBrownianMotionIteration defines an iteration for fractional
// Brownian motion.
type FractionalBrownianMotionIteration struct {
	unitNormalDist           *distuv.Normal
	numberOfIntegrationSteps int
}

func (f *FractionalBrownianMotionIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	f.unitNormalDist = &distuv.Normal{
		Mu:    0.0,
		Sigma: 1.0,
		Src:   rand.NewSource(settings.Seeds[partitionIndex]),
	}
	f.numberOfIntegrationSteps = int(
		settings.OtherParams[partitionIndex].
			IntParams["number_of_integration_steps"][0],
	)
}

func (f *FractionalBrownianMotionIteration) Iterate(
	params *simulator.OtherParams,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	stateHistory := stateHistories[partitionIndex]
	values := make([]float64, stateHistory.StateWidth)
	for i := range values {
		values[i] = stateHistory.Values.At(0, i) +
			math.Sqrt(params.FloatParams["variances"][i]*
				timestepsHistory.NextIncrement)*f.unitNormalDist.Rand()*
				FractionalBrownianMotionIntegral(
					timestepsHistory.Values.AtVec(0),
					timestepsHistory.Values.AtVec(0)+
						timestepsHistory.NextIncrement,
					params.FloatParams["hurst_exponents"][i],
					f.numberOfIntegrationSteps,
				)/timestepsHistory.NextIncrement
	}
	return values
}
