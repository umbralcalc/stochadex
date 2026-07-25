// Package analysis is the data layer around a simulation: getting time series
// into a *simulator.StateTimeStorage, addressing series inside one, and
// rendering them.
//
// It deliberately does not build simulation topologies. The Applied* specs that
// expand into multi-partition inference, aggregation and optimisation
// topologies live in pkg/macros, which imports this package for its vocabulary.
// The dependency runs one way:
//
//	pkg/api → pkg/macros → pkg/analysis → pkg/simulator
//
// # The vocabulary
//
// DataRef is the shared currency: a partition name plus optional value indices
// and time range, resolvable against a storage. Both the plotting helpers here
// and every windowed construction in pkg/macros are expressed in terms of it,
// which is what lets a config name a series once and use it for either.
// GroupedStateTimeStorage layers a grouping over a storage so aggregations can
// be taken per accepted value group.
//
// # Getting data in and out
//
//   - csv.go, logs.go       — load a storage from a CSV file or JSON log entries.
//   - postgres.go           — read a storage from PostgreSQL, write one back, and
//     PostgresDbOutputFunction to stream a live run into a table.
//   - partitions.go         — build a storage by running partitions, or append
//     partitions to an existing one (this is what the `data:`
//     tier of a YAML config resolves to).
//   - dataframe.go          — convert a partition to and from a gota DataFrame.
//
// # Rendering
//
// plot.go produces go-echarts line and scatter charts, from either a storage
// (via DataRef) or a DataFrame. ColourGenerator cycles a palette across series.
package analysis
