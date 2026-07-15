# arrowstore — opt-in Apache Arrow egress

An Arrow-native `simulator.OutputFunction` + storage for the simulation **output boundary**.
It is a **separate Go module** so that Apache Arrow (and its dependency tree, including a
gonum v0.17 requirement) stays entirely out of the core engine's `go.mod` — the engine stays
lean and WASM-clean; you opt in only by importing this module.

```bash
go get github.com/umbralcalc/stochadex/pkg/arrowstore
```

## When to use it

Reach for it when simulation output is destined for the **columnar / analytical world** —
DuckDB, Polars, pandas, or Parquet/Feather on disk. It builds the Arrow arrays directly, so
there is no `[][]float64` → Arrow conversion pass.

**It is not a general-purpose faster store.** If you only want `[][]float64` in memory, the
pure-Go `simulator.StateTimeStorage` is the right default.

```go
store := arrowstore.NewArrowStateTimeStorage()
impl.OutputFunction = &arrowstore.ArrowStateTimeStorageOutputFunction{Store: store}
// ... run the coordinator ...
store.Finalize()
rec := store.Record() // *arrow.Record: time column + one FixedSizeList column per partition
```

## Measured (Apple M4, `go test -bench=. -benchmem`)

Two questions, two honest answers — vs `simulator.StateTimeStorage`:

| scenario | metric | current | arrowstore |
|---|---|---|---|
| **append only** (in-memory), width 256 × 2000 rows | allocs | 2031 | **51** |
| | ns | 425µs | **331µs** |
| | bytes | **4.3 MB** | 8.6 MB |
| append only, width 64 × 2000 | ns | **133µs** | 183µs |
| **get output into Arrow** (interchange), width 256 × 2000 | ns | 856µs | **324µs (2.6×)** |
| | allocs | 4071 | **63** |
| | bytes | 17 MB | **8.6 MB** |

- **Append allocation count collapses to a constant** (a real GC-pressure win) — but wall-clock
  is only comparable-to-better at wide state, and transient memory is higher (builder capacity
  doubles). So this is not a drop-in append speedup.
- **Getting to Arrow wins decisively** (~2.2–2.7× faster, far fewer allocations, ~half the
  memory) because it skips the conversion — which is the whole point.

## Concurrency

Mirrors `simulator.StateTimeStorage` exactly: one builder per partition index written only by
that partition's goroutine (lock-free), plus a single shared time column deduplicated under a
mutex with an atomic fast path. Do not share a builder, append off the owning goroutine, or add
a lock to the per-partition path.

## WASM

The engine is WASM-clean; this module is not automatically so (Arrow uses assembly SIMD). Build
it for `js/wasm` with Arrow's `noasm` build tag if you need that target.
