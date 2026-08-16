// Package solarfleet is the domain-models catalogue entry for aggregate domestic
// solar-PV fleet output across a region: a deterministic solar-geometry backbone
// driving a distance-coupled stochastic clear-sky-index field, aggregated to a
// fleet total. Its flagship is dispersion-smoothing — spreading sites apart lowers
// aggregate output variability at fixed capacity — which is real here because the
// coupling derives from geography (the full-covariance form), not a scalar per-site
// loading.
//
// Like limit-order-book, this is a born-declarative entry: the downstream repo's
// forward model is itself pure stochadex config, so [declarative.yaml] is the form
// it actually runs and the bespoke iterations here ([fleetsky.go], [aggregate.go])
// are a faithful re-expression of it, held exact by expression_equivalence_test.go.
// [geometry.go] and [fleet.go] port the deterministic physics (NOAA solar position,
// Meinel clear-sky, plane-of-array transposition) and the distance-decay Cholesky
// coupling; these are lifted from the downstream repo and staged here for the
// "should this be promoted into core?" question — see models/CONVENTIONS.md.
//
// The engine owns this generative and forward model; the downstream repo
// (https://github.com/umbralcalc/solar-fleet) owns inference, data, calibration
// against real OCF uk_pv data, and the capacity-siting decision layer.
package solarfleet
