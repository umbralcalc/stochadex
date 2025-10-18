// Package inference provides statistical inference and likelihood modeling
// capabilities for stochadex simulations. It includes probability distributions,
// likelihood functions, gradient computation, and Bayesian inference utilities
// for parameter estimation and model validation.
//
// Key Features:
//   - Probability distribution implementations (Beta, Gamma, Normal, Poisson, etc.)
//   - Likelihood function evaluation and gradient computation
//   - Bayesian inference with posterior estimation
//   - Parameter estimation and optimization support
//   - Model comparison and validation utilities
//
// Mathematical Background:
// The package implements various probability distributions and their associated
// likelihood functions for use in Bayesian inference. Key distributions include:
//   - Normal distributions for continuous variables
//   - Poisson distributions for count data
//   - Gamma distributions for positive continuous variables
//   - Beta distributions for bounded continuous variables
//
// Usage Patterns:
//   - Fit models to observed data using maximum likelihood estimation
//   - Perform Bayesian parameter estimation with prior distributions
//   - Compare models using likelihood ratios or information criteria
//   - Validate model assumptions through posterior predictive checks
package inference
