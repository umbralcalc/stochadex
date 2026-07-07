// Package homark is the data-free generative core of a single-local-authority UK
// housing-market model: a mean-reverting bank rate and a stochastic planning-supply
// pipeline drive a log-price drift–diffusion against a log-earnings drift–diffusion,
// yielding a price-to-earnings affordability ratio.
//
// The StochasticPipelineIteration and AffordabilityFromLogsIteration in this package
// are bespoke simulator.Iteration extensions lifted verbatim from the downstream
// homark repo and staged here for the "should this be promoted into core?" question;
// they are not part of the engine's public API. See card.md for the full spec and
// https://github.com/umbralcalc/homark for the inference, data, and decision layers.
package homark
