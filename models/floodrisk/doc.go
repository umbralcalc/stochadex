// Package floodrisk holds the flood-risk domain model: a data-free, SDK-built
// simulation stub of the generative core (stochastic rainfall driving a
// rainfall-runoff cascade) plus the bespoke stochadex Iterations it depends on.
//
// The iterations in rainfall.go and runoff.go are BESPOKE EXTENSIONS written
// against the core simulator.Iteration interface. They live beside the stub, not
// in the engine core, because that is where the catalogue stages the "should this
// be promoted into core?" question. They are lifted from the downstream project
// repo; the data-fitting / calibration helpers that accompany them there
// (parameter estimation from observed series) are inference concerns and stay
// downstream — see card.md.
package floodrisk
