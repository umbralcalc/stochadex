package macros

import (
	"fmt"

	"github.com/umbralcalc/stochadex/pkg/analysis"
)

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
// will have at least depth rows of history when wired through
// analysis.AddPartitionsToStateTimeStorage (missing names default to depth 1).
// Call with the same windowSizeByPartition map passed to analysis.AddPartitionsToStateTimeStorage.
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
	}
}
