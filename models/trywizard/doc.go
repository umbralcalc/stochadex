// Package rugby is the catalogue stub for trywizard: an event-based simulation
// of a rugby union match as a system of coupled stochastic counting processes,
// used to ask how substitution timing changes the scoreline and win probability.
//
// The generative core is a set of bespoke pieces lifted verbatim from the
// downstream trywizard repo's pkg/match: the log-linear rate functions
// ([rate_function.go]), the Bernoulli conversion iteration ([conversion.go]),
// the derived match-state function ([state_function.go]), and the substitution
// strategy → covariate machinery ([substitution.go]). They are wired together
// with the engine's own discrete.CoxProcessIteration and general.* iterations in
// [stub.go]. They live here — beside the stub rather than in engine core —
// because the catalogue is the staging ground for the "should this be promoted
// into core?" question: a generic log-linear (Poisson-GLM) rate primitive
// recurring across other models would be the signal to promote, but that waits
// for the recurrence.
//
// The downstream repo's data ingestion (the SportDevs API client), the
// adaptive-bandwidth kernel smoothing of baseline rates, and the warm-start SGD
// training of the Poisson-GLM coefficients are inference / ingestion concerns and
// were left downstream. This stub replaces the fitted, time-varying baseline with
// constant intercepts and ships illustrative (uncalibrated) coefficients.
package rugby
