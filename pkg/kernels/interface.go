// Package kernels provides integration kernels for time-weighted aggregation
// and state-distance weighting in stochadex simulations. These kernels define
// how historical data points are weighted when computing aggregated statistics
// over time or when measuring similarity between states.
//
// Key Features:
//   - Time-based weighting kernels (exponential, periodic, constant)
//   - State-distance kernels (Gaussian, instantaneous)
//   - Custom kernel implementations for specialized weighting schemes
//   - Integration with aggregation functions for rolling statistics
//
// Mathematical Background:
// Integration kernels define weighting functions w(t-s) for aggregating
// historical data points. Common patterns include:
//   - Exponential decay: w(t-s) = exp(-λ(t-s))
//   - Gaussian weighting: w(t-s) = exp(-(t-s)²/(2σ²))
//   - State distance: w(x,y) = exp(-||x-y||²/(2σ²))
//
// Design Philosophy:
// Kernels are designed to be composable and efficient, providing standardized
// interfaces for time and state weighting. They enable flexible aggregation
// schemes while maintaining good performance characteristics.
//
// Usage Patterns:
//   - Rolling window statistics with time decay
//   - State similarity measurements for clustering
//   - Temporal aggregation with customizable weighting
//   - Feature engineering with distance-based weighting
package kernels

import "github.com/umbralcalc/stochadex/pkg/simulator"

// IntegrationKernel defines the interface for time/state weighting kernels
// used when aggregating over historical data in simulations.
//
// Integration kernels provide a standardized way to weight historical data points
// when computing aggregated statistics. They enable flexible temporal and spatial
// weighting schemes that can be customized for specific aggregation needs.
//
// Mathematical Concept:
// Integration kernels define weighting functions w(current, past) that determine
// how much influence a historical data point has on the current aggregation.
// Common patterns include:
//   - Time-based weighting: w(t_current, t_past) = f(t_current - t_past)
//   - State-based weighting: w(x_current, x_past) = f(||x_current - x_past||)
//   - Hybrid weighting: w(current, past) = f(time_diff, state_diff)
//
// Interface Methods:
//   - Configure: Initialize kernel with simulation settings (called once per partition)
//   - SetParams: Update kernel parameters from simulation context (called each step)
//   - Evaluate: Compute weight for a historical sample (called for each aggregation)
//
// Weight Properties:
//   - Weights must be non-negative: w(current, past) ≥ 0
//   - Weights should be normalized for consistent aggregation scales
//   - Zero weights indicate no influence from that historical sample
//
// Common Kernel Types:
//   - ExponentialIntegrationKernel: Exponential time decay w(t) = exp(-λt)
//   - GaussianStateIntegrationKernel: State distance weighting w(x,y) = exp(-||x-y||²/2σ²)
//   - InstantaneousIntegrationKernel: No weighting, w = 1 for current, 0 for past
//   - PeriodicIntegrationKernel: Periodic time weighting for seasonal patterns
//
// Example Usage:
//
//	kernel := &ExponentialIntegrationKernel{}
//	kernel.Configure(0, settings)
//	kernel.SetParams(params) // params contains decay rate λ
//
//	// Evaluate weight for a sample from 1.0 time units ago
//	weight := kernel.Evaluate(currentState, pastState, 5.0, 4.0)
//	// weight = exp(-λ * 1.0)
//
// Performance Considerations:
//   - Evaluate is called frequently during aggregation
//   - Implementations should be optimized for repeated calls
//   - Consider caching expensive computations in SetParams
//   - Avoid memory allocations in Evaluate method
//
// Related Types:
//   - See analysis.AppliedAggregation for usage in data aggregation
//   - See ExponentialIntegrationKernel for exponential decay weighting
//   - See GaussianStateIntegrationKernel for state-distance weighting
type IntegrationKernel interface {
	Configure(partitionIndex int, settings *simulator.Settings)
	SetParams(params *simulator.Params)
	Evaluate(
		currentState []float64,
		pastState []float64,
		currentTime float64,
		pastTime float64,
	) float64
}
