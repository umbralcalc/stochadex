// Package analysis provides data analysis and aggregation utilities for simulation results.
// It includes functions for computing statistical measures, working with CSV data,
// creating dataframes, and performing grouped aggregations over time series data.
//
// Key Features:
//   - Statistical aggregation functions (mean, variance, covariance)
//   - CSV data import/export capabilities
//   - DataFrame integration for data manipulation
//   - Grouped aggregations with customizable kernels
//   - PostgreSQL integration for data storage
//   - Log file parsing and analysis
//
// Usage Patterns:
//   - Load simulation data from CSV files
//   - Compute rolling statistics with time-weighted kernels
//   - Export results to various formats
//   - Perform likelihood analysis and inference
//   - Online scalar OLS: ScalarRegressionStatsIteration and
//     NewScalarRegressionStatsPartition maintain sufficient statistics (and
//     closed-form α, β, σ²) for y on x with optional intercept; use
//     RegressionStatsCumulative or RegressionStatsWindow (fixed-length buffer).
//     Wire upstream scalars via ParamsFromUpstream keys ScalarRegressionParamY
//     and ScalarRegressionParamX. Row 0 of state history is the latest values,
//     consistent with other analysis replay partitions.
package analysis
