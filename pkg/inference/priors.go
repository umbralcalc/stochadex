package inference

import (
	"fmt"
	"math"
	"math/rand/v2"
)

// Prior defines a 1D prior distribution for a model parameter.
type Prior interface {
	Sample(rng *rand.Rand) float64
	LogPDF(x float64) float64
	InSupport(x float64) bool
}

// UniformPrior is a uniform distribution on [Lo, Hi].
type UniformPrior struct {
	Lo, Hi float64
}

func (p *UniformPrior) Sample(rng *rand.Rand) float64 {
	return p.Lo + rng.Float64()*(p.Hi-p.Lo)
}

func (p *UniformPrior) LogPDF(x float64) float64 {
	if x < p.Lo || x > p.Hi {
		return math.Inf(-1)
	}
	return -math.Log(p.Hi - p.Lo)
}

func (p *UniformPrior) InSupport(x float64) bool {
	return x >= p.Lo && x <= p.Hi
}

// TruncatedNormalPrior is a normal distribution truncated to [Lo, Hi].
type TruncatedNormalPrior struct {
	Mu, Sigma float64
	Lo, Hi    float64
}

func (p *TruncatedNormalPrior) Sample(rng *rand.Rand) float64 {
	for {
		x := rng.NormFloat64()*p.Sigma + p.Mu
		if x >= p.Lo && x <= p.Hi {
			return x
		}
	}
}

func (p *TruncatedNormalPrior) LogPDF(x float64) float64 {
	if x < p.Lo || x > p.Hi {
		return math.Inf(-1)
	}
	d := (x - p.Mu) / p.Sigma
	// Unnormalised (ignoring truncation constant, which is the same for all x in support)
	return -0.5*d*d - math.Log(p.Sigma) - 0.5*math.Log(2*math.Pi)
}

func (p *TruncatedNormalPrior) InSupport(x float64) bool {
	return x >= p.Lo && x <= p.Hi
}

// HalfNormalPrior is a half-normal distribution (x >= 0) with scale sigma.
type HalfNormalPrior struct {
	Sigma float64
}

func (p *HalfNormalPrior) Sample(rng *rand.Rand) float64 {
	return math.Abs(rng.NormFloat64()) * p.Sigma
}

func (p *HalfNormalPrior) LogPDF(x float64) float64 {
	if x < 0 {
		return math.Inf(-1)
	}
	d := x / p.Sigma
	return math.Log(2) - 0.5*d*d - math.Log(p.Sigma) - 0.5*math.Log(2*math.Pi)
}

func (p *HalfNormalPrior) InSupport(x float64) bool {
	return x >= 0
}

// LogNormalPrior is a log-normal distribution: log(x) ~ N(mu, sigma^2).
type LogNormalPrior struct {
	Mu, Sigma float64
}

func (p *LogNormalPrior) Sample(rng *rand.Rand) float64 {
	return math.Exp(rng.NormFloat64()*p.Sigma + p.Mu)
}

func (p *LogNormalPrior) LogPDF(x float64) float64 {
	if x <= 0 {
		return math.Inf(-1)
	}
	logX := math.Log(x)
	d := (logX - p.Mu) / p.Sigma
	return -0.5*d*d - logX - math.Log(p.Sigma) - 0.5*math.Log(2*math.Pi)
}

func (p *LogNormalPrior) InSupport(x float64) bool {
	return x > 0
}

// Prior type codes for params-based configuration.
const (
	PriorTypeUniform         = 0
	PriorTypeTruncatedNormal = 1
	PriorTypeHalfNormal      = 2
	PriorTypeLogNormal       = 3
)

// PriorParamsStride is the number of float64 values per prior in the
// prior_params encoding. Each prior type uses a subset:
//
//	Uniform (0):         [lo, hi, 0, 0]
//	TruncatedNormal (1): [mu, sigma, lo, hi]
//	HalfNormal (2):      [sigma, 0, 0, 0]
//	LogNormal (3):       [mu, sigma, 0, 0]
const PriorParamsStride = 4

// DecodePriors builds a []Prior from params-encoded type codes and
// parameter values. prior_types has length d, prior_params has length
// 4*d (PriorParamsStride per prior).
func DecodePriors(priorTypes, priorParams []float64) []Prior {
	d := len(priorTypes)
	priors := make([]Prior, d)
	for i := range d {
		pp := priorParams[i*PriorParamsStride : (i+1)*PriorParamsStride]
		switch int(priorTypes[i]) {
		case PriorTypeUniform:
			priors[i] = &UniformPrior{Lo: pp[0], Hi: pp[1]}
		case PriorTypeTruncatedNormal:
			priors[i] = &TruncatedNormalPrior{
				Mu: pp[0], Sigma: pp[1], Lo: pp[2], Hi: pp[3],
			}
		case PriorTypeHalfNormal:
			priors[i] = &HalfNormalPrior{Sigma: pp[0]}
		case PriorTypeLogNormal:
			priors[i] = &LogNormalPrior{Mu: pp[0], Sigma: pp[1]}
		default:
			panic(fmt.Sprintf("unknown prior type code: %v", priorTypes[i]))
		}
	}
	return priors
}

// EncodePriors converts a []Prior into params-compatible slices
// (prior_types and prior_params) for YAML configuration.
func EncodePriors(priors []Prior) (priorTypes, priorParams []float64) {
	d := len(priors)
	priorTypes = make([]float64, d)
	priorParams = make([]float64, d*PriorParamsStride)
	for i, p := range priors {
		pp := priorParams[i*PriorParamsStride : (i+1)*PriorParamsStride]
		switch v := p.(type) {
		case *UniformPrior:
			priorTypes[i] = PriorTypeUniform
			pp[0], pp[1] = v.Lo, v.Hi
		case *TruncatedNormalPrior:
			priorTypes[i] = PriorTypeTruncatedNormal
			pp[0], pp[1], pp[2], pp[3] = v.Mu, v.Sigma, v.Lo, v.Hi
		case *HalfNormalPrior:
			priorTypes[i] = PriorTypeHalfNormal
			pp[0] = v.Sigma
		case *LogNormalPrior:
			priorTypes[i] = PriorTypeLogNormal
			pp[0], pp[1] = v.Mu, v.Sigma
		default:
			panic(fmt.Sprintf("unknown prior type: %T", p))
		}
	}
	return
}
