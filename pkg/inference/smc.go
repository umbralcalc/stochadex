package inference

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// SMCResult holds the posterior estimates from SMC inference.
type SMCResult struct {
	ParamNames     []string
	PosteriorMean  []float64
	PosteriorStd   []float64
	PosteriorCov   []float64   // d*d flattened row-major
	LogMarginalLik float64
	Predictions    [][]float64 // [T][N] predicted values per particle
	ParticleParams [][]float64 // [N][d] final round particle parameters
	ParticleLogLik []float64   // [N] final round cumulative log-likelihoods
	Weights        []float64   // [N] normalised importance weights
}

// SMCProposalIteration generates N particle proposals at each step.
// On step 1 it draws from the prior. On subsequent steps it draws
// from a multivariate normal centred on the previous posterior,
// read from the posterior partition's state history.
//
// State layout: [particle_params(N*d)] flattened row-major.
// State width: N*d.
//
// Params:
//
//	num_particles:      [N]
//	prior_types:        [type codes]
//	prior_params:       [4 values per prior]
//	posterior_partition: [partition_index] (via params_as_partitions)
//	verbose:            [0 or 1]
type SMCProposalIteration struct {
	Priors []Prior

	rng                  *rand.Rand
	numParticles         int
	nParams              int
	posteriorPartitionIdx int
	verbose              bool
}

func (s *SMCProposalIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	iterParams := settings.Iterations[partitionIndex].Params
	seed := settings.Iterations[partitionIndex].Seed
	s.rng = rand.New(rand.NewPCG(seed, seed+1))
	s.verbose = iterParams.GetIndex("verbose", 0) > 0
	s.numParticles = int(iterParams.GetIndex("num_particles", 0))
	if s.numParticles == 0 {
		panic("SMCProposalIteration: num_particles must be set")
	}
	if s.Priors == nil {
		priorTypes, ok1 := iterParams.GetOk("prior_types")
		priorParams, ok2 := iterParams.GetOk("prior_params")
		if ok1 && ok2 {
			s.Priors = DecodePriors(priorTypes, priorParams)
		} else {
			panic("SMCProposalIteration: priors must be set")
		}
	}
	s.nParams = len(s.Priors)
	s.posteriorPartitionIdx = int(iterParams.GetIndex("posterior_partition", 0))
}

func (s *SMCProposalIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	round := timestepsHistory.CurrentStepNumber
	N := s.numParticles
	d := s.nParams

	particleParams := make([][]float64, N)
	if round == 1 {
		for p := range N {
			pp := make([]float64, d)
			for j, prior := range s.Priors {
				pp[j] = prior.Sample(s.rng)
			}
			particleParams[p] = pp
		}
	} else {
		prevPosterior := stateHistories[s.posteriorPartitionIdx].Values.RawRowView(0)
		proposalMean := prevPosterior[:d]
		proposalCov := regulariseCov(
			prevPosterior[d:d+d*d], d, s.Priors,
		)
		particleParams = sampleMultivariateNormal(
			s.rng, N, proposalMean, proposalCov, s.Priors,
		)
	}

	if s.verbose {
		fmt.Printf("Round %d: drawing %d particles...\n", round, N)
	}

	state := make([]float64, N*d)
	for p := range N {
		copy(state[p*d:(p+1)*d], particleParams[p])
	}
	return state
}

// SMCPosteriorIteration computes importance-weighted posterior
// statistics from particle log-likelihoods and parameters received
// via params_from_upstream channels.
//
// State layout: [posterior_mean(d) | posterior_cov(d²) | log_marginal_lik(1)]
// State width: d + d² + 1.
//
// Params:
//
//	num_particles:     [N]
//	num_params:        [d]
//	particle_loglikes: [N values] (via params_from_upstream)
//	particle_params:   [N*d flat] (via params_from_upstream)
//	verbose:           [0 or 1]
type SMCPosteriorIteration struct {
	ParamNames []string

	numParticles int
	nParams      int
	verbose      bool
}

func (s *SMCPosteriorIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	iterParams := settings.Iterations[partitionIndex].Params
	s.numParticles = int(iterParams.GetIndex("num_particles", 0))
	s.nParams = int(iterParams.GetIndex("num_params", 0))
	s.verbose = iterParams.GetIndex("verbose", 0) > 0
	if s.numParticles == 0 || s.nParams == 0 {
		panic("SMCPosteriorIteration: num_particles and num_params must be set")
	}
	if len(s.ParamNames) == 0 {
		s.ParamNames = make([]string, s.nParams)
		for i := range s.nParams {
			s.ParamNames[i] = fmt.Sprintf("param_%d", i)
		}
	}
}

func (s *SMCPosteriorIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	N := s.numParticles
	d := s.nParams

	logLiks := params.Get("particle_loglikes")
	proposalFlat := params.Get("particle_params")

	particleParams := make([][]float64, N)
	for p := range N {
		particleParams[p] = make([]float64, d)
		copy(particleParams[p], proposalFlat[p*d:(p+1)*d])
	}

	result := ComputePosterior(s.ParamNames, particleParams, logLiks, nil)

	if s.verbose {
		fmt.Printf("  Log marginal likelihood: %.4f\n", result.LogMarginalLik)
		for i, name := range s.ParamNames {
			fmt.Printf("  %-22s mean=%.4f std=%.4f\n",
				name, result.PosteriorMean[i], result.PosteriorStd[i])
		}
	}

	state := make([]float64, d+d*d+1)
	copy(state[:d], result.PosteriorMean)
	copy(state[d:d+d*d], result.PosteriorCov)
	state[d+d*d] = result.LogMarginalLik
	return state
}

// PosteriorStateWidth returns the state width for SMCPosteriorIteration.
func PosteriorStateWidth(nParams int) int {
	return nParams + nParams*nParams + 1
}

// ComputePosterior computes posterior statistics from weighted particles.
func ComputePosterior(
	paramNames []string,
	particleParams [][]float64,
	logLiks []float64,
	predictions [][]float64,
) *SMCResult {
	N := len(particleParams)
	nParams := len(particleParams[0])

	logMarginalLik := LogSumExp(logLiks) - math.Log(float64(N))

	logWeights := make([]float64, N)
	copy(logWeights, logLiks)
	logZ := LogSumExp(logWeights)
	weights := make([]float64, N)
	for i := range N {
		weights[i] = math.Exp(logWeights[i] - logZ)
	}

	postMean := make([]float64, nParams)
	for p := range N {
		for j := range nParams {
			postMean[j] += weights[p] * particleParams[p][j]
		}
	}

	postCov := make([]float64, nParams*nParams)
	for p := range N {
		for i := range nParams {
			di := particleParams[p][i] - postMean[i]
			for j := range nParams {
				dj := particleParams[p][j] - postMean[j]
				postCov[i*nParams+j] += weights[p] * di * dj
			}
		}
	}

	postStd := make([]float64, nParams)
	for j := range nParams {
		postStd[j] = math.Sqrt(postCov[j*nParams+j])
	}

	return &SMCResult{
		ParamNames:     paramNames,
		PosteriorMean:  postMean,
		PosteriorStd:   postStd,
		PosteriorCov:   postCov,
		LogMarginalLik: logMarginalLik,
		Predictions:    predictions,
		Weights:        weights,
	}
}

// sampleMultivariateNormal draws N samples from a multivariate normal,
// rejecting any samples that fall outside prior support.
func sampleMultivariateNormal(
	rng *rand.Rand,
	n int,
	mean []float64,
	covFlat []float64,
	priors []Prior,
) [][]float64 {
	d := len(mean)

	L := choleskyDecomp(covFlat, d)
	if L == nil {
		L = make([]float64, d*d)
		for i := range d {
			v := covFlat[i*d+i]
			if v <= 0 {
				v = 1.0
			}
			L[i*d+i] = math.Sqrt(v)
		}
	}

	samples := make([][]float64, n)
	for i := range n {
		for {
			z := make([]float64, d)
			for j := range d {
				z[j] = rng.NormFloat64()
			}
			x := make([]float64, d)
			for row := range d {
				x[row] = mean[row]
				for col := 0; col <= row; col++ {
					x[row] += L[row*d+col] * z[col]
				}
			}
			valid := true
			for j, p := range priors {
				if !p.InSupport(x[j]) {
					valid = false
					break
				}
			}
			if valid {
				samples[i] = x
				break
			}
		}
	}
	return samples
}

// choleskyDecomp computes the lower Cholesky factor of a d×d symmetric
// positive-definite matrix (row-major flat). Returns nil if not PD.
func choleskyDecomp(a []float64, d int) []float64 {
	L := make([]float64, d*d)
	for i := range d {
		for j := 0; j <= i; j++ {
			sum := 0.0
			for k := 0; k < j; k++ {
				sum += L[i*d+k] * L[j*d+k]
			}
			if i == j {
				val := a[i*d+i] - sum
				if val <= 0 {
					return nil
				}
				L[i*d+j] = math.Sqrt(val)
			} else {
				L[i*d+j] = (a[i*d+j] - sum) / L[j*d+j]
			}
		}
	}
	return L
}

// regulariseCov adds a minimum diagonal to prevent posterior covariance
// collapse between importance sampling rounds.
func regulariseCov(cov []float64, d int, priors []Prior) []float64 {
	reg := make([]float64, len(cov))
	copy(reg, cov)
	for i := range d {
		var priorVar float64
		switch p := priors[i].(type) {
		case *UniformPrior:
			span := p.Hi - p.Lo
			priorVar = span * span / 12.0
		case *TruncatedNormalPrior:
			priorVar = p.Sigma * p.Sigma
		case *HalfNormalPrior:
			priorVar = p.Sigma * p.Sigma
		case *LogNormalPrior:
			priorVar = p.Sigma * p.Sigma
		default:
			priorVar = 1.0
		}
		floor := 0.01 * priorVar
		if reg[i*d+i] < floor {
			reg[i*d+i] = floor
		}
	}
	return reg
}

// LogSumExp computes log(sum(exp(x))) with numerical stability.
func LogSumExp(x []float64) float64 {
	if len(x) == 0 {
		return math.Inf(-1)
	}
	maxVal := x[0]
	for _, v := range x[1:] {
		if v > maxVal {
			maxVal = v
		}
	}
	if math.IsInf(maxVal, -1) {
		return math.Inf(-1)
	}
	sum := 0.0
	for _, v := range x {
		sum += math.Exp(v - maxVal)
	}
	return maxVal + math.Log(sum)
}

// WeightedQuantiles computes weighted quantiles from particle values.
func WeightedQuantiles(values, weights, probs []float64) []float64 {
	n := len(values)
	indices := make([]int, n)
	for i := range n {
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool {
		return values[indices[i]] < values[indices[j]]
	})

	cumWeight := make([]float64, n)
	cumWeight[0] = weights[indices[0]]
	for i := 1; i < n; i++ {
		cumWeight[i] = cumWeight[i-1] + weights[indices[i]]
	}
	total := cumWeight[n-1]
	for i := range n {
		cumWeight[i] /= total
	}

	result := make([]float64, len(probs))
	for q, p := range probs {
		for i := range n {
			if cumWeight[i] >= p {
				result[q] = values[indices[i]]
				break
			}
		}
	}
	return result
}
