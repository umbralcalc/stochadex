package inference

import (
	"github.com/umbralcalc/stochadex/pkg/simulator"
	"golang.org/x/exp/rand"
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat/distmv"
)

// NormalLikelihoodDistribution assumes the real data are well described
// by a normal distribution, given the input mean and covariance matrix.
type NormalLikelihoodDistribution struct {
	Src          rand.Source
	defaultCov   []float64
	defaultCovOk bool
}

func (n *NormalLikelihoodDistribution) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	n.Src = rand.NewSource(settings.Iterations[partitionIndex].Seed)
	n.defaultCov, n.defaultCovOk =
		settings.Iterations[partitionIndex].Params.GetOk("default_covariance")
}

func (n *NormalLikelihoodDistribution) getDist(
	mean *mat.VecDense,
	covariance mat.Symmetric,
) *distmv.Normal {
	dist, ok := distmv.NewNormal(
		mean.RawVector().Data,
		covariance,
		n.Src,
	)
	if !ok {
		if n.defaultCovOk {
			dist, _ = distmv.NewNormal(
				mean.RawVector().Data,
				mat.NewSymDense(mean.Len(), n.defaultCov),
				n.Src,
			)
		} else {
			panic("covariance matrix is not positive-definite")
		}
	}
	return dist
}

func (n *NormalLikelihoodDistribution) EvaluateLogLike(
	mean *mat.VecDense,
	covariance mat.Symmetric,
	data []float64,
) float64 {
	dist := n.getDist(mean, covariance)
	return dist.LogProb(data)
}

func (n *NormalLikelihoodDistribution) GenerateNewSamples(
	mean *mat.VecDense,
	covariance mat.Symmetric,
) []float64 {
	dist := n.getDist(mean, covariance)
	return dist.Rand(nil)
}
