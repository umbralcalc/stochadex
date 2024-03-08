package objectives

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"
)

// NormalDataLinkingLogLikelihood assumes the real data are well described
// by a normal distribution, given the input mean and covariance matrix.
type NormalDataLinkingLogLikelihood struct {
	Src rand.Source
}

func (n *NormalDataLinkingLogLikelihood) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	n.Src = rand.NewSource(settings.Seeds[partitionIndex])
}

func (n *NormalDataLinkingLogLikelihood) Evaluate(
	mean *mat.VecDense,
	covariance mat.Symmetric,
	data []float64,
) float64 {
	dist, ok := distmv.NewNormal(
		mean.RawVector().Data,
		covariance,
		n.Src,
	)
	if !ok {
		return math.NaN()
	}
	return dist.LogProb(data)
}

func (n *NormalDataLinkingLogLikelihood) GenerateNewSamples(
	mean *mat.VecDense,
	covariance mat.Symmetric,
) []float64 {
	dist, ok := distmv.NewNormal(
		mean.RawVector().Data,
		covariance,
		n.Src,
	)
	if !ok {
		values := make([]float64, 0)
		for i := 0; i < mean.Len(); i++ {
			values = append(values, math.NaN())
		}
		return values
	}
	return dist.Rand(nil)
}
