// Package amr holds the antimicrobial-resistance domain model: a data-free,
// SDK-built simulation stub of the generative core plus the bespoke stochadex
// Iterations it depends on.
//
// The iterations in colonisation.go and infection.go are BESPOKE EXTENSIONS
// written against the core simulator.Iteration interface. They live beside the
// stub, not in the engine core, because that is where the catalogue stages the
// "should this be promoted into core?" question. They are lifted verbatim from
// the downstream project repo so the comparison across models stays honest — see
// card.md for the downstream pointer.
package amr
