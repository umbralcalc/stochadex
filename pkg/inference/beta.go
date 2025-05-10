package inference

import (
	"math"

	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/floats"
	"gonum.org/v1/gonum/stat/distuv"
	"scientificgo.org/special"
)

// BetaLikelihoodDistribution assumes the real data are well
// described by a beta distribution, given the input alpha
// and beta parameters.
type BetaLikelihoodDistribution struct {
	Src   rand.Source
	alpha []float64
	beta  []float64
}

func (b *BetaLikelihoodDistribution) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	b.Src = rand.NewPCG(
		settings.Iterations[partitionIndex].Seed,
		settings.Iterations[partitionIndex].Seed,
	)
	b.alpha = make([]float64, settings.Iterations[partitionIndex].StateWidth)
}

func (b *BetaLikelihoodDistribution) SetParams(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) {
	if alpha, ok := params.GetOk("alpha"); ok {
		b.alpha = alpha
		b.beta = params.Get("beta")
	} else if mean, ok := params.GetOk("mean"); ok {
		precision := params.Get("precision")
		floats.MulTo(b.alpha, mean, precision)
		floats.SubTo(b.beta, precision, b.alpha)
	} else {
		panic("beta likelihood error: either 'alpha' or 'mean' must be set")
	}
}

func (b *BetaLikelihoodDistribution) EvaluateLogLike(
	data []float64,
) float64 {
	dist := &distuv.Beta{Alpha: 1.0, Beta: 1.0, Src: b.Src}
	logLike := 0.0
	for i, alpha := range b.alpha {
		dist.Alpha = alpha
		dist.Beta = b.beta[i]
		logLike += dist.LogProb(data[i])
	}
	return logLike
}

func (b *BetaLikelihoodDistribution) GenerateNewSamples() []float64 {
	samples := make([]float64, 0)
	dist := &distuv.Beta{Alpha: 1.0, Beta: 1.0, Src: b.Src}
	for i, alpha := range b.alpha {
		dist.Alpha = alpha
		dist.Beta = b.beta[i]
		samples = append(samples, dist.Rand())
	}
	return samples
}

func (b *BetaLikelihoodDistribution) EvaluateLogLikeMeanGrad(
	data []float64,
) []float64 {
	logLikeGrad := make([]float64, 0)
	var prec, mean, x float64
	for i, alpha := range b.alpha {
		x = data[i]
		prec = alpha + b.beta[i]
		mean = alpha / prec
		logLikeGrad = append(
			logLikeGrad,
			prec*(math.Log(x/(1.0-x))+
				special.Digamma((1.0-mean)*prec)-
				special.Digamma(mean*prec)),
		)
	}
	return logLikeGrad
}
