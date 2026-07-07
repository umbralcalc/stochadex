package homark

import (
	"math"
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// StochasticPipelineIteration models the housing supply pipeline with per-unit
// binomial draws for completions and attrition each step.
//
// State: [pipeline_stock] — current units in pipeline.
//
// Params:
//   - completion_rate: probability each unit completes per step [0,1]
//   - attrition_rate:  probability each remaining unit lapses per step [0,1]
//   - approval_rate:   units/step entering pipeline (scalar inflow; wirable from upstream)
//
// Each step:
//
//	completions ~ Binomial(floor(stock), completion_rate)
//	attritions  ~ Binomial(floor(stock − completions), attrition_rate)
//	new_stock    = max(0, stock − completions − attritions + approval_rate)
//
// Lifted verbatim from the downstream homark repo (pkg/housing/pipeline.go).
type StochasticPipelineIteration struct {
	binomialDist *distuv.Binomial
}

func (s *StochasticPipelineIteration) Configure(partitionIndex int, settings *simulator.Settings) {
	s.binomialDist = &distuv.Binomial{
		N: 0,
		P: 1.0,
		Src: rand.NewPCG(
			settings.Iterations[partitionIndex].Seed,
			settings.Iterations[partitionIndex].Seed,
		),
	}
}

func (s *StochasticPipelineIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	_ *simulator.CumulativeTimestepsHistory,
) []float64 {
	stock := stateHistories[partitionIndex].Values.At(0, 0)
	completionRate := params.Get("completion_rate")[0]
	attritionRate := params.Get("attrition_rate")[0]
	approvalRate := params.Get("approval_rate")[0]

	n := math.Floor(stock)
	if n < 0 {
		n = 0
	}

	var completions float64
	if n > 0 && completionRate > 0 {
		s.binomialDist.N = n
		s.binomialDist.P = completionRate
		completions = s.binomialDist.Rand()
	}

	remaining := n - completions
	var attritions float64
	if remaining > 0 && attritionRate > 0 {
		s.binomialDist.N = remaining
		s.binomialDist.P = attritionRate
		attritions = s.binomialDist.Rand()
	}

	newStock := stock - completions - attritions + approvalRate
	if newStock < 0 {
		newStock = 0
	}
	return []float64{newStock}
}
