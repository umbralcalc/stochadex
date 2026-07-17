package measles

import (
	"math/rand/v2"

	"gonum.org/v1/gonum/stat/distuv"
)

// Per-dose MMR efficacies used to convert coverage into effective susceptibility.
// Lifted from the downstream repo (pkg/measles/coverage.go).
const (
	DefaultMMR1Efficacy = 0.93
	DefaultMMR2Efficacy = 0.97
)

// SusceptibilityFromCoverage converts one- and two-dose MMR coverage (as fractions
// in [0,1]) into the effective susceptible fraction, given per-dose efficacies.
// Children with two doses are protected with probability e2; those with only one
// dose with probability e1; the rest are susceptible:
//
//	immune = c2*e2 + (c1 - c2)*e1   (c1 >= c2 expected; clamped if not)
//	s      = 1 - immune
//
// Lifted verbatim from the downstream repo (pkg/measles/coverage.go).
func SusceptibilityFromCoverage(c1, c2, e1, e2 float64) float64 {
	if c2 > c1 {
		c2 = c1 // two-dose coverage cannot exceed one-dose; guard data noise
	}
	immune := c2*e2 + (c1-c2)*e1
	s := 1 - immune
	if s < 0 {
		s = 0
	}
	return s
}

// effectivePool returns the reachable susceptible pool for one introduction:
// s·population·fraction, capped at a community-scale reachable ceiling. Lifted from
// the downstream repo (pkg/measles/transmission.go).
func effectivePool(s, population, fraction, reachableCeiling float64) float64 {
	pool := s * population * fraction
	if reachableCeiling > 0 && pool > reachableCeiling {
		return reachableCeiling
	}
	return pool
}

// nextGeneration draws the total number of secondary cases produced by the current
// generation of infectious cases, each with negative-binomial offspring (mean
// rLocal, dispersion k). Because the sum of n iid NegBin offspring counts is itself
// Poisson(Gamma(shape = n*k, rate = k/rLocal)), the whole generation is one Gamma
// draw plus one Poisson draw — exact, and O(1) in the generation size. This single
// step is the shared branching kernel of both bespoke iterations, so they cannot
// drift apart. Lifted verbatim from the downstream repo (pkg/measles/transmission.go).
func nextGeneration(infectious int, rLocal, dispersion float64, rng *rand.Rand) int {
	if infectious <= 0 || rLocal <= 0 {
		return 0
	}
	shape := float64(infectious) * dispersion
	rate := dispersion / rLocal
	lambda := distuv.Gamma{Alpha: shape, Beta: rate, Src: rng}.Rand()
	return int(distuv.Poisson{Lambda: lambda, Src: rng}.Rand())
}

// expectedTotalProgeny returns the analytic mean final size of a subcritical
// branching process seeded by one case: 1/(1-m) for m < 1. Used in tests to check
// the kernel against theory.
func expectedTotalProgeny(m float64) float64 { return 1.0 / (1.0 - m) }
