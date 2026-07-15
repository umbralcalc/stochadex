// Package rng provides a small, allocation-free random sampler for stochadex
// iterations and likelihood distributions. It is a drop-in, bit-identical
// replacement for the gonum distuv distribution draws the engine previously
// made, tuned to avoid their per-call overhead in hot loops.
//
// Key Features:
//   - A single owned math/rand/v2.Rand per Sampler, seeded once from a partition seed
//   - Simple draws: standard normal, uniform, exponential (Float64/NormFloat64/…)
//   - Compound draws: gamma, beta, and Poisson variates for jump and likelihood models
//   - Bit-identical streams to distuv for the same seed, guaranteed by stream-identity tests
//   - No per-call allocation and no per-call distribution-value copy or wrapper construction
//
// Why this exists:
// The stochastic iterations previously drew from gonum's distuv distributions, e.g.
// distuv.Normal{Mu: 0, Sigma: 1, Src: rand.NewPCG(seed, seed)}.Rand(). Each such call
// copies the distribution (distuv methods have value receivers) and, internally, does
// rand.New(src) to build a fresh math/rand/v2.Rand wrapper — and for the compound
// distributions it also binds method values (rnd.Float64, rnd.NormFloat64, …) that are
// then called indirectly. gonum's math/rand/v2 backing means the wrapper does not escape
// and so is not heap allocated, but the copy, the construction, and the indirection are
// all measurable in a per-element, per-step hot loop. A Sampler owns one *math/rand/v2.Rand
// and calls its methods directly, so all of that overhead disappears.
//
// Mathematical Background:
// Each method reproduces the exact algorithm the corresponding distuv distribution uses,
// consuming the underlying source in the same order, so for a given seed it yields the
// identical stream (not merely the same distribution). The compound draws mirror gonum's
// choices directly:
//   - Gamma: exponential for shape α=1; the Liu–Martin–Syring log-space method for α<0.2;
//     Marsaglia–Tsang otherwise (with the α<1 boost m = U^(1/α))
//   - Beta(α, β): ratio X/(X+Y) of two Gamma(·, 1) draws from the same stream
//   - Poisson: the direct exponential-interarrival method for λ<10; Hörmann's PTRS
//     transformed-rejection method for λ≥10
//
// Scope:
// Only the distributions where owning the generator is a clean, strict win live here.
// Binomial (a three-branch algorithm used in a single observation process) and Categorical
// (a stateful precomputed sampling heap) stay on distuv: the copied-algorithm maintenance
// cost outweighs the small per-draw saving, and neither sits in a tight per-element loop.
//
// Usage Patterns:
//   - Create a Sampler in an iteration's Configure, seeded from the partition Seed, and
//     keep it on the iteration struct; call its methods in Iterate
//   - Use it for likelihood-distribution sampling (GenerateNewSamples) while keeping distuv
//     for the log-likelihood evaluation, which needs distuv's LogProb, not randomness
//   - Use NewFromSource to reproduce iterations that derive a distribution's source from a
//     master generator rather than directly from the partition seed
//
// Concurrency:
// A Sampler is NOT safe for concurrent use by multiple goroutines. This matches the
// engine's per-partition model: each partition owns its own iteration and therefore its
// own Sampler, and partitions never share one.
package rng
