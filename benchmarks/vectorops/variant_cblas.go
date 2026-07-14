//go:build cblas

package main

// resultSuffix distinguishes the cblas (system C BLAS) result file from the default one.
// `-tags cblas` build → vectorized_ops_go_cblas.json.
const resultSuffix = "_cblas"
