// Package lob is the catalogue stub for a limit-order-book microsimulation: a
// single trading venue's order book as a shared latent-activity driver feeding
// stochastic order flow, a deterministic matching engine, and a derived set of
// market observables. It is used to ask how the book's stability — spread, depth,
// and the coupling between arrivals and resting depth — responds to the model's
// structural knobs, above all the activity-dependent arrival-damping exponent.
//
// The four partitions ([activity.go], [flows.go], [book.go], [observables.go]) are
// wired together in [stub.go]. Unlike the rest of the catalogue, none of this Go is
// "lifted" from the downstream repo, because the downstream repo (cryptobook) has no
// bespoke Go at all: its entire forward model — every model in the paper — is written
// as stochadex configuration, run by the engine with no toolchain. So this stub is a
// faithful Go re-expression of that config's generative core, and [declarative.yaml]
// is the config itself. That inverts the usual catalogue relationship: here the
// declarative twin is the primary artifact the downstream actually runs, and the Go is
// the convenience form. It makes the promotion triage decidable a priori and in the
// affirmative — a serious, market-calibrated model already runs as pure data — so this
// entry has no promotion-candidate extensions, only two spellings of one model that
// the equivalence test holds bit-identical.
//
// The specific model here is cfg/lob_counts_split.yaml — the "counts route" model that
// cryptobook found best (it reproduces all three of the market's pooled correlation
// signatures and was the one to pass an out-of-sample test), in the four-partition
// modular form cfg/lob_split.yaml established. The downstream repo's data ingestion
// (a live Binance depth/trade recorder), its calibration of the damping exponent to a
// market correlation, and its simulation-based inference are inference concerns and stay
// downstream; this stub ships the illustrative (best-model) parameters as constants.
//
// This family of order-book configs is also what drove two engine primitives into core
// — scan (a fold across each's lanes) and a zero-width slice — surfaced by a queue-
// position model too structured for the elementwise evaluator; see models/CONVENTIONS.md.
package lob
