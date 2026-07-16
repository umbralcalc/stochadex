// Package arrowstore is an opt-in, Apache Arrow-native storage + output function for the
// simulation output (egress) boundary. It is a SEPARATE Go module
// (github.com/umbralcalc/stochadex/pkg/arrowstore) so that Arrow and its dependency tree — which
// includes a gonum v0.17 requirement — stay entirely out of the core engine's go.mod. The
// engine module remains lean and WASM-clean; you opt in only by importing this module.
//
// What it is for:
// getting simulation output into Apache Arrow with minimal cost, so it can be handed to
// DuckDB / Polars / pandas (or written as Parquet/Feather) without a conversion pass. It is
// the egress foundation the analytical-sink integrations build on.
//
// It is NOT a general-purpose faster in-memory store. Two honest results from its benchmark
// (Apple M4, reproduced by BenchmarkAppend / BenchmarkToArrow):
//
//   - Append hot path, no materialisation: vs simulator.StateTimeStorage this trades a
//     one-heap-allocation-per-row jagged [][][]float64 for a growing contiguous
//     FixedSizeListBuilder. Allocation COUNT collapses to a constant in row count (a big
//     GC-pressure win — e.g. ~50 allocs vs ~2000 at 2000 rows), but wall-clock is only
//     comparable-to-better at wide state and is actually slower at mid widths, and transient
//     memory is higher (builder capacity doubling over-allocates). So if all you want is
//     [][]float64 in memory, keep simulator.StateTimeStorage — it is the right default.
//
//   - Getting to Arrow (the reason this exists): this wins decisively — ~2.2–2.7× faster,
//     far fewer allocations, and roughly half the memory of appending to StateTimeStorage
//     and then converting [][]float64 → Arrow — because it builds the Arrow arrays directly.
//
// So: reach for ArrowStateTimeStorage when the output is destined for the columnar/analytical
// world; keep the pure-Go StateTimeStorage otherwise.
//
// Concurrency mirrors simulator.StateTimeStorage exactly and MUST be preserved:
//   - one builder per partition index, written only by that partition's goroutine (lock-free);
//   - a single shared time column, deduplicated once globally under a mutex with an atomic
//     fast path (so the common "same timestamp across partitions this step" case skips the lock).
//
// Do not share a builder across goroutines, append out of the owning goroutine, or add a lock
// to the per-partition path — any of those trades the allocation win for contention.
//
// WASM note: the engine is WASM-clean; this module is not automatically so, since Arrow uses
// assembly SIMD. Build it for WASM with Arrow's noasm build tag if you need that target.
package arrowstore
