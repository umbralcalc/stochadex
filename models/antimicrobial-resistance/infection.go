package amr

import (
	"math"
	"math/rand"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// InfectionProcessIteration models the stochastic transition from colonisation
// to bloodstream infection (BSI). It reads colonisation fractions from an
// upstream partition and produces BSI incidence counts for susceptible and
// resistant strains.
//
// State: [susceptible_bsi_count, resistant_bsi_count]
//
// Params:
//   - infection_probability: per-timestep probability that a colonised patient develops BSI
//   - patient_population: total number of patients in the ward/trust
//   - colonisation_partition: index of the colonisation dynamics partition
type InfectionProcessIteration struct {
	colonisationPartitionIndex int
	rng                        *rand.Rand
}

func (inf *InfectionProcessIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	inf.colonisationPartitionIndex = int(
		settings.Iterations[partitionIndex].Params.Map["colonisation_partition"][0],
	)
	inf.rng = rand.New(rand.NewSource(
		int64(settings.Iterations[partitionIndex].Seed),
	))
}

func (inf *InfectionProcessIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	infectionProb := params.Map["infection_probability"][0]
	population := params.Map["patient_population"][0]

	// Read colonisation fractions from upstream
	colonisation := stateHistories[inf.colonisationPartitionIndex]
	fracS := colonisation.Values.At(0, 0)
	fracR := colonisation.Values.At(0, 1)

	dt := timestepsHistory.NextIncrement

	// Expected number of colonised patients
	nColonisedS := population * fracS
	nColonisedR := population * fracR

	// BSI events as Poisson draws: rate = infection_probability * n_colonised * dt
	lambdaS := infectionProb * nColonisedS * dt
	lambdaR := infectionProb * nColonisedR * dt

	bsiS := float64(poissonSample(inf.rng, lambdaS))
	bsiR := float64(poissonSample(inf.rng, lambdaR))

	return []float64{bsiS, bsiR}
}

// poissonSample draws from a Poisson distribution with the given mean lambda.
// Uses Knuth's method for small lambda and a normal approximation for large lambda.
func poissonSample(rng *rand.Rand, lambda float64) int {
	if lambda <= 0 {
		return 0
	}
	if lambda < 30 {
		// Knuth's algorithm
		L := math.Exp(-lambda)
		k := 0
		p := 1.0
		for {
			k++
			p *= rng.Float64()
			if p < L {
				return k - 1
			}
		}
	}
	// Normal approximation for large lambda
	return int(math.Max(0, math.Round(lambda+math.Sqrt(lambda)*rng.NormFloat64())))
}
