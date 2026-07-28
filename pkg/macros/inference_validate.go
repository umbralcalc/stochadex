package macros

import (
	"fmt"
	"log"

	"github.com/umbralcalc/stochadex/pkg/analysis"
)

// warnIfWindowDataHistoryTooDeep flags a window data partition whose replay
// StateHistoryDepth exceeds Window.Depth. Too deep is as broken as too shallow,
// but silently: general.FromHistoryIteration walks the replay buffer from row
// StateHistoryDepth-2 down to StateHistoryDepth-Depth-1, so a depth of exactly
// Window.Depth consumes the buffer, while anything larger anchors the window in
// rows that are still zero-filled (the buffer starts as zeros and gains one real
// row per outer step). The likelihood is then computed against zeros for most or
// all of the run — it comes out near-constant and carries no information about
// the parameters, so anything downstream of it (a posterior, an ES reward)
// silently freezes at its prior rather than failing.
func warnIfWindowDataHistoryTooDeep(depth int, partitionName string, d int) {
	if d <= depth {
		return
	}
	log.Printf(
		"macros: WARNING window data partition %q has StateHistoryDepth %d > Window.Depth %d; "+
			"the window will replay zero-filled rows for the first %d steps and lag the data "+
			"by %d steps thereafter, so the likelihood will be near-constant and carry no "+
			"information — set the history depth equal to Window.Depth (%d)",
		partitionName, d, depth, d-2, d-depth, depth,
	)
}

func assertWindowDataSourcesDeepEnough(
	depth int,
	data []analysis.DataRef,
	depths map[string]int,
) {
	if depths == nil || len(data) == 0 {
		return
	}
	for _, ref := range data {
		d, ok := depths[ref.PartitionName]
		if !ok {
			panic(fmt.Sprintf(
				"macros: WindowDataHistoryDepth missing entry for window data partition %q",
				ref.PartitionName,
			))
		}
		if d < depth {
			panic(fmt.Sprintf(
				"macros: window data partition %q StateHistoryDepth %d < Window.Depth %d",
				ref.PartitionName, d, depth,
			))
		}
		warnIfWindowDataHistoryTooDeep(depth, ref.PartitionName, d)
	}
}

func validateAppliedPosteriorWidths(applied AppliedPosteriorEstimation) {
	n := len(applied.Mean.Default)
	if n == 0 {
		panic("macros: AppliedPosteriorEstimation.Mean.Default must be non-empty")
	}
	if len(applied.Sampler.Default) != n {
		panic(fmt.Sprintf(
			"macros: PosteriorSampler.Default length must match mean dimension %d, got %d",
			n, len(applied.Sampler.Default),
		))
	}
	if applied.Covariance.JustVariance {
		if len(applied.Covariance.Default) != n {
			panic(fmt.Sprintf(
				"macros: PosteriorCovariance.JustVariance requires Default length %d (per-dimension variance), got %d",
				n, len(applied.Covariance.Default),
			))
		}
		return
	}
	if want := n * n; len(applied.Covariance.Default) != want {
		panic(fmt.Sprintf(
			"macros: full posterior covariance Default must have length N²=%d (N=%d), got %d",
			want, n, len(applied.Covariance.Default),
		))
	}
}

// ValidateWindowDataHistoryDepth checks that each window data source partition
// will have exactly depth rows of history when wired through
// analysis.AddPartitionsToStateTimeStorage (missing names default to depth 1).
// Call with the same windowSizeByPartition map passed to analysis.AddPartitionsToStateTimeStorage.
// Too few rows panics; too many logs a warning (see warnIfWindowDataHistoryTooDeep
// — an oversized depth voids the likelihood instead of underflowing).
func ValidateWindowDataHistoryDepth(
	windowDepth int,
	windowSizeByPartition map[string]int,
	dataRefs []analysis.DataRef,
) {
	if windowDepth <= 0 || len(dataRefs) == 0 {
		return
	}
	for _, ref := range dataRefs {
		w, ok := windowSizeByPartition[ref.PartitionName]
		if !ok {
			w = 1
		}
		if w < windowDepth {
			panic(fmt.Sprintf(
				"macros: window data partition %q has StateHistoryDepth %d via analysis.AddPartitionsToStateTimeStorage but Window.Depth is %d (need depth >= %d)",
				ref.PartitionName, w, windowDepth, windowDepth,
			))
		}
		warnIfWindowDataHistoryTooDeep(windowDepth, ref.PartitionName, w)
	}
}
