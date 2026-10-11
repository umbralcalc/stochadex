// Package keyboard provides interactive user input capabilities for stochadex simulations.
//
// Importing it registers the keyboard as a stream transport, so a config steers a run from
// the keyboard with a stream input, stream: {keyboard: {partition, keys}}, feeding
// params_from_input (see stream.go). That is the way to use it; UserInputIteration, which
// reads keys from inside an iteration, is kept for Go code that builds it directly.
// It enables real-time user interaction during simulation runs through keyboard events,
// allowing for dynamic control and parameter adjustment during execution.
//
// Key Features:
//   - Real-time keyboard event handling
//   - Configurable key-to-action mapping
//   - Timeout-based input handling
//   - Graceful exit and cleanup
//   - Extensible input channel abstraction
//
// Usage Patterns:
//   - Pause/resume simulations with keyboard shortcuts
//   - Adjust simulation parameters in real-time
//   - Trigger data export or analysis during runs
//   - Implement interactive debugging and monitoring
//   - Create user-controlled simulation scenarios
package keyboard
