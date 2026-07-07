// Package measles is the data-free generative core of a sub-national measles
// transmission-risk model: a shared national importation latent seeds many local
// authorities (UTLAs) at once, and each UTLA runs a stochastic SIR / susceptible-
// depleting branching process whose local reproduction number R_local = R0·s is set
// by its effective susceptibility s (from MMR vaccine coverage). The shared latent
// is what makes outbreaks co-occur across areas.
//
// The NationalImportationIteration and JointOutbreakIteration in this package are
// bespoke simulator.Iteration extensions lifted verbatim from the downstream
// measles-risk-forecaster repo and staged here for the "should this be promoted into
// core?" question; the branching kernel (nextGeneration) and the coverage→
// susceptibility map are lifted alongside them. See card.md for the full spec and
// https://github.com/umbralcalc/measles-risk-forecaster for the calibration, spatial
// smoothing, censoring, and nowcast layers.
package measles
