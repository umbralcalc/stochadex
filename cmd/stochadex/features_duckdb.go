//go:build duckdb_arrow

package main

func init() { features = append(features, "duckdb") }
