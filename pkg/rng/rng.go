// Package rng provides a small, allocation-free random sampler for stochadex iterations.
//
// # Why this exists
//
// The stochastic iterations previously drew from gonum's distuv distributions, e.g.
// distuv.Normal{Mu: 0, Sigma: 1, Src: rand.NewPCG(seed, seed)}.Rand(). Each such .Rand()
// call copies the distribution value (value receiver) and, in effect, does
// rand.New(src).NormFloat64() — constructing a fresh math/rand/v2.Rand wrapper per call.
// gonum's math/rand/v2 backing means that wrapper does not escape and so is not heap
// allocated, but the copy + construct + nil-Src branch is still measurable overhead in a
// per-element, per-step hot loop.
//
// Sampler owns a single *math/rand/v2.Rand, created once (typically in Configure), and
// calls its methods directly. For the distributions here — standard normal, uniform, and
// exponential — this reproduces the exact stream distuv would have produced for the same
// seed (locked in by the TestStreamIdentical* tests), just without the per-call wrapper.
//
// # Scope
//
// Only the simple distributions live here, because they are the ones where owning the
// wrapper is both trivial and a strict win. Complex distributions (Gamma, Poisson,
// Binomial, Beta, Categorical) stay on distuv: their .Rand() cost is dominated by the
// sampling algorithm, not the wrapper, so there is nothing to gain by reimplementing them.
//
// # Concurrency
//
// A Sampler is NOT safe for concurrent use by multiple goroutines — matching the engine's
// per-partition model, where each partition owns its own iteration and therefore its own
// Sampler, and partitions never share one.
package rng

import "math/rand/v2"

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
