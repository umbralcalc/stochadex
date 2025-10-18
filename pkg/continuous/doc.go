// Package continuous provides implementations of continuous-time stochastic processes
// for simulation modeling. It includes various diffusion processes, jump-diffusion
// models, and other continuous stochastic dynamics commonly used in mathematical
// finance, physics, and engineering applications.
//
// Key Features:
//   - Wiener processes (Brownian motion)
//   - Geometric Brownian motion for asset price modeling
//   - Ornstein-Uhlenbeck processes for mean-reverting dynamics
//   - Drift-diffusion and jump-diffusion processes
//   - Compound Poisson processes
//   - Gradient descent and optimization algorithms
//   - Cumulative time tracking utilities
//
// Mathematical Background:
// Continuous stochastic processes are typically described by stochastic differential
// equations (SDEs) of the form:
//
//	dX(t) = μ(X,t)dt + σ(X,t)dW(t)
//
// where μ is the drift, σ is the volatility, and W(t) is a Wiener process.
//
// Usage Patterns:
//   - Financial modeling (asset prices, interest rates, volatility)
//   - Physics simulation (particle dynamics, thermal motion)
//   - Engineering applications (noise modeling, signal processing)
//   - Machine learning (stochastic optimization, sampling)
package continuous
