// Package bathingwater holds the bathing-water-forecaster domain model: a
// data-free, SDK-built simulation stub of the generative core of a bathing-water
// pollution-exceedance forecaster — a shared regional "wet-week" anomaly (an
// Ornstein–Uhlenbeck process) coupled to many designated bathing sites, each
// mapping its latent log-concentration to an exceedance probability against the
// statutory E. coli threshold.
//
// The iteration in concentration.go is a BESPOKE EXTENSION written against the
// core simulator.Iteration interface. It lives beside the stub, not in the engine
// core, because that is where the catalogue stages the "should this be promoted
// into core?" question. It is lifted from the downstream project repo, where the
// same latent-concentration → exceedance-probability step is expressed as an inline
// closure over a general.ValuesFunctionIteration in the regional partition graph
// (internal/compose); it is promoted here to a named iteration that computes its
// own seasonal term and self-drives, so the stub runs with zero external inputs.
// The shared anomaly uses the engine's own continuous.OrnsteinUhlenbeckIteration.
//
// The downstream data-fitting concerns — censored maximum-likelihood fitting,
// empirical-Bayes pooling, and the sequential-Monte-Carlo particle filter that
// infers the anomaly from partly-censored observations — are inference and stay
// downstream. See card.md.
package bathingwater
