// Package mctstest provides reusable Environment fixtures for testing
// pkg/mcts and any package that builds on it (pkg/analysis, downstream
// consumers).
//
// Tic-tac-toe is the canonical fixture: small enough to be obvious, large
// enough that random play loses, and known-deterministic at endgame —
// from a "win in one" position MCTS must pick the winning move; from a
// "block in one" position it must block. Both assertions pass with very
// few simulations because rollouts terminate within a handful of plies.
//
// This package is named with the conventional "test" suffix
// (httptest, fstest, iotest) but contains regular Go code so it can be
// imported from _test.go files in other packages.
package agentstest
