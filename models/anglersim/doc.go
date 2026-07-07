// Package anglersim holds the anglersim domain model: a data-free, SDK-built
// simulation stub of the generative core of a climate-driven brown trout
// population-dynamics model (a stochastic Ricker density process forced by an
// environmental covariate process) plus the bespoke stochadex Iterations it
// depends on.
//
// The iterations in ricker.go and covariates.go are BESPOKE EXTENSIONS written
// against the core simulator.Iteration interface. They live beside the stub, not
// in the engine core, because that is where the catalogue stages the "should this
// be promoted into core?" question. RickerIteration is lifted verbatim from the
// downstream project repo; the data-fitting / calibration / SMC-inference helpers
// that accompany it there (fitting the Ricker parameters from electrofishing
// density series) are inference concerns and stay downstream. The covariate
// forcing there is a bootstrap resample from observed EA hydrology / water-quality
// records; ClimateCovariatesIteration is a data-free generative stand-in for that
// supply so the stub runs with zero external inputs — see card.md.
package anglersim
