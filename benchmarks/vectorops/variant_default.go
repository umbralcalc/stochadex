//go:build !cblas

package main

// resultSuffix distinguishes the default pure-Go BLAS result file from the cblas one.
// Default build → vectorized_ops_go.json.
const resultSuffix = ""
