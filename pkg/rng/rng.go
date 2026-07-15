package rng

import (
	"math"
	"math/rand/v2"
)

// Sampler is an allocation-free source of random samples backed by a single owned
// math/rand/v2.Rand.
type Sampler struct {
	r *rand.Rand
}

// New returns a Sampler seeded deterministically from seed. It reproduces the source the
// iterations handed to distuv (rand.NewPCG(seed, seed)), so New(seed) yields a stream
// identical to a distuv distribution built with Src: rand.NewPCG(seed, seed).
func New(seed uint64) *Sampler {
	return &Sampler{r: rand.New(rand.NewPCG(seed, seed))}
}

// NewFromSource returns a Sampler drawing from src. Use it to reproduce iterations that
// derive a distribution's source from a master generator (e.g. rand.NewPCG(r.IntN(1e8),
// r.IntN(1e8))) rather than directly from the partition seed: wrap the same source and the
// stream is unchanged.
func NewFromSource(src rand.Source) *Sampler {
	return &Sampler{r: rand.New(src)}
}

// Rand returns the owned generator, for the rare caller that needs a *rand.Rand directly
// (e.g. to derive further sources). Draws taken from it advance the same stream.
func (s *Sampler) Rand() *rand.Rand { return s.r }

// Float64 returns a uniform sample in [0,1) — identical to
// distuv.Uniform{Min: 0, Max: 1, Src: ...}.Rand().
func (s *Sampler) Float64() float64 { return s.r.Float64() }

// Uniform returns a uniform sample in [min,max) — identical to
// distuv.Uniform{Min: min, Max: max, Src: ...}.Rand() (same rnd*(max-min)+min form, so
// bit-identical, not merely equal in distribution).
func (s *Sampler) Uniform(min, max float64) float64 { return s.r.Float64()*(max-min) + min }

// NormFloat64 returns a standard-normal sample — identical to
// distuv.Normal{Mu: 0, Sigma: 1, Src: ...}.Rand().
func (s *Sampler) NormFloat64() float64 { return s.r.NormFloat64() }

// Normal returns a Normal(mu, sigma) sample — identical to
// distuv.Normal{Mu: mu, Sigma: sigma, Src: ...}.Rand() (rnd*sigma+mu form).
func (s *Sampler) Normal(mu, sigma float64) float64 { return s.r.NormFloat64()*sigma + mu }

// Exponential returns an Exponential(rate) sample — identical to
// distuv.Exponential{Rate: rate, Src: ...}.Rand() (ExpFloat64()/rate form).
func (s *Sampler) Exponential(rate float64) float64 { return s.r.ExpFloat64() / rate }

// gammaSmallAlphaThresh is distuv.Gamma's shape threshold below which the
// Liu–Martin–Syring log-space method is used instead of Marsaglia–Tsang.
const gammaSmallAlphaThresh = 0.2

// Gamma returns a Gamma(alpha, beta) sample (shape alpha, rate beta) — identical to
// distuv.Gamma{Alpha: alpha, Beta: beta, Src: ...}.Rand(), reproducing its exact branch
// structure and draw order: exponential for alpha==1, the Liu–Martin–Syring log-space
// method for alpha<0.2, and Marsaglia–Tsang otherwise (with the alpha<1 boost). Panics on
// beta<=0 or alpha<=0, matching distuv.
func (s *Sampler) Gamma(alpha, beta float64) float64 {
	if beta <= 0 {
		panic("rng: gamma beta <= 0")
	}
	a, b := alpha, beta
	switch {
	case a <= 0:
		panic("rng: gamma alpha <= 0")
	case a == 1:
		// Generate from exponential.
		return s.r.ExpFloat64() / b
	case a < gammaSmallAlphaThresh:
		// Liu, Chuanhai, Martin, Ryan and Syring, Nick. "Simulating from a gamma
		// distribution with small shape parameter" (adjusted to work in log space).
		lambda := 1/a - 1
		lr := -math.Log1p(1 / lambda / math.E)
		for {
			e := s.r.ExpFloat64()
			var z float64
			if e >= -lr {
				z = e + lr
			} else {
				z = -s.r.ExpFloat64() / lambda
			}
			eza := math.Exp(-z / a)
			lh := -z - eza
			var lEta float64
			if z >= 0 {
				lEta = -z
			} else {
				lEta = -1 + lambda*z
			}
			if lh-lEta > -s.r.ExpFloat64() {
				return eza / b
			}
		}
	default: // a >= gammaSmallAlphaThresh
		// Marsaglia, George, and Wai Wan Tsang. "A simple method for generating gamma
		// variables." ACM TOMS 26.3 (2000): 363-372.
		d := a - 1.0/3
		m := 1.0
		if a < 1 {
			d += 1.0
			m = math.Pow(s.r.Float64(), 1/a)
		}
		c := 1 / (3 * math.Sqrt(d))
		for {
			x := s.r.NormFloat64()
			v := 1 + x*c
			if v <= 0.0 {
				continue
			}
			v = v * v * v
			u := s.r.Float64()
			if u < 1.0-0.0331*(x*x)*(x*x) {
				return m * d * v / b
			}
			if math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
				return m * d * v / b
			}
		}
	}
}

// Beta returns a Beta(alpha, beta) sample — identical to
// distuv.Beta{Alpha: alpha, Beta: beta, Src: ...}.Rand(): the ratio ga/(ga+gb) of two
// Gamma(·, 1) draws taken in order from this Sampler's stream.
func (s *Sampler) Beta(alpha, beta float64) float64 {
	ga := s.Gamma(alpha, 1)
	gb := s.Gamma(beta, 1)
	return ga / (ga + gb)
}

// Poisson returns a Poisson(lambda) sample — identical to
// distuv.Poisson{Lambda: lambda, Src: ...}.Rand(): the direct exponential-interarrival
// method for lambda<10, and Hörmann's PTRS transformed-rejection method for lambda>=10.
func (s *Sampler) Poisson(lambda float64) float64 {
	if lambda < 10.0 {
		// Direct method.
		var em float64
		t := 0.0
		for {
			t += s.r.ExpFloat64()
			if t >= lambda {
				break
			}
			em++
		}
		return em
	}
	// W. Hörmann. "The transformed rejection method for generating Poisson random
	// variables." Insurance: Mathematics and Economics 12.1 (1993): 39-45 (Algorithm PTRS).
	b := 0.931 + 2.53*math.Sqrt(lambda)
	a := -0.059 + 0.02483*b
	invalpha := 1.1239 + 1.1328/(b-3.4)
	vr := 0.9277 - 3.6224/(b-2)
	for {
		U := s.r.Float64() - 0.5
		V := s.r.Float64()
		us := 0.5 - math.Abs(U)
		k := math.Floor((2*a/us+b)*U + lambda + 0.43)
		if us >= 0.07 && V <= vr {
			return k
		}
		if k <= 0 || (us < 0.013 && V > us) {
			continue
		}
		lg, _ := math.Lgamma(k + 1)
		if math.Log(V*invalpha/(a/(us*us)+b)) <= k*math.Log(lambda)-lambda-lg {
			return k
		}
	}
}
