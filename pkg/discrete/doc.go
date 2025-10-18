// Package discrete provides implementations of discrete-time and event-based
// stochastic processes for simulation modeling. It includes counting processes,
// state transition models, and other discrete stochastic dynamics commonly
// used in queueing theory, epidemiology, finance, and system modeling.
//
// Key Features:
//   - Poisson processes for event counting and arrival modeling
//   - Bernoulli processes for binary outcomes and trials
//   - Binomial observation processes for sampling and measurement
//   - Categorical state transitions for discrete state spaces
//   - Cox processes for intensity-driven event modeling
//   - Hawkes processes for self-exciting event sequences
//
// Mathematical Background:
// Discrete stochastic processes typically model events, transitions, or
// counting phenomena. They are characterized by:
//   - Discrete state spaces (integers, categories, binary states)
//   - Event-driven dynamics (jumps, arrivals, transitions)
//   - Probability distributions for event occurrence
//   - Memory properties (Markovian vs. non-Markovian)
//
// Usage Patterns:
//   - Queueing systems (arrival processes, service times)
//   - Epidemiology (disease spread, infection events)
//   - Finance (default events, insurance claims)
//   - Network modeling (packet arrivals, connection events)
//   - Manufacturing (production events, quality control)
package discrete
