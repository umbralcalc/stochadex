// Package simulator provides the core simulation engine and infrastructure
// for stochadex simulations. It includes the main simulation loop, state management,
// partition coordination, and execution control mechanisms.
//
// Key Features:
//   - Partition-based simulation architecture
//   - Concurrent execution with goroutine coordination
//   - State history management and time tracking
//   - Configurable termination and output conditions
//   - Flexible timestep control
//   - Storage and persistence utilities
//
// Architecture:
// The simulator uses a partition-based approach where simulations are divided
// into independent partitions that can be executed concurrently. Each partition
// maintains its own state history and can communicate with other partitions
// through defined interfaces.
//
// Usage Patterns:
//   - Configure and run complex multi-partition simulations
//   - Manage simulation state across multiple timesteps
//   - Coordinate concurrent execution of simulation components
//   - Store and retrieve simulation results and intermediate states
//   - Implement custom termination and output conditions
//
// Input from outside a run:
// A run driven step by step (PartitionCoordinator.NewStepper) can take input
// from outside — a user's action, a live feed — only through
// PartitionCoordinator.InjectParams, between steps. Input never reaches a
// partition inside Iterate, so iterations stay pure functions of their params,
// histories and seed, and a run with no injections is reproducible exactly.
package simulator
