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
// Kernels define weighting functions K(t, s) that determine how much influence
// a data point at time s has on the aggregated value at time t. Common patterns:
//   - Exponential kernels: K(t, s) = exp(-λ(t-s)) for time decay
//   - Gaussian kernels: K(x, y) = exp(-||x-y||²/2σ²) for state similarity
//   - Periodic kernels: K(t, s) = cos(2π(t-s)/T) for cyclical patterns
//
// Usage Patterns:
//   - Weight historical data for rolling window statistics
//   - Define similarity measures between simulation states
//   - Implement custom aggregation schemes with domain-specific weighting
//   - Create time-decay functions for forgetting old information
package kernels
