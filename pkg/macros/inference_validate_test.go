package macros

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/analysis"
)

// These validators are the guardrails on the Applied* specs: they run at
// config-assembly time and panic rather than return errors, because a bad spec
// is a programming error and a silently misshapen topology is worse than a
// loud stop. That makes their panic branches the whole point of the code, and
// an unexercised guardrail is one you cannot assume fires — so each is tested
// on both sides of its boundary.

// wantPanic runs fn and asserts it panicked with a message containing want.
func wantPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("expected a panic containing %q, got none", want)
			return
		}
		got, ok := r.(string)
		if !ok {
			if err, isErr := r.(error); isErr {
				got = err.Error()
			} else {
				t.Errorf("panicked with unexpected type %T: %v", r, r)
				return
			}
		}
		if !strings.Contains(got, want) {
			t.Errorf("panic = %q, want it to contain %q", got, want)
		}
	}()
	fn()
}

// wantLog runs fn with the standard logger captured and asserts what it wrote
// contains want; an empty want asserts it wrote nothing.
func wantLog(t *testing.T, want string, fn func()) {
	t.Helper()
	var buf bytes.Buffer
	flags, writer := log.Flags(), log.Writer()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(writer)
		log.SetFlags(flags)
	}()
	fn()
	got := buf.String()
	if want == "" {
		if got != "" {
			t.Errorf("expected no log output, got %q", got)
		}
		return
	}
	if !strings.Contains(got, want) {
		t.Errorf("log = %q, want it to contain %q", got, want)
	}
}

// wantNoPanic runs fn and fails if it panics.
func wantNoPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()
	fn()
}

func TestAssertWindowDataSourcesDeepEnough(t *testing.T) {
	refs := []analysis.DataRef{{PartitionName: "obs"}}

	t.Run("no depths map means the caller opted out of the check", func(t *testing.T) {
		wantNoPanic(t, func() {
			assertWindowDataSourcesDeepEnough(5, refs, nil)
		})
	})
	t.Run("no window data means there is nothing to check", func(t *testing.T) {
		wantNoPanic(t, func() {
			assertWindowDataSourcesDeepEnough(5, nil, map[string]int{"obs": 1})
		})
	})
	t.Run("a partition missing from the map is rejected", func(t *testing.T) {
		// Silently defaulting here would let a window read past the history
		// that was actually configured for it.
		wantPanic(t, "missing entry for window data partition \"obs\"", func() {
			assertWindowDataSourcesDeepEnough(5, refs, map[string]int{"other": 9})
		})
	})
	t.Run("a partition shallower than the window is rejected", func(t *testing.T) {
		wantPanic(t, "StateHistoryDepth 3 < Window.Depth 5", func() {
			assertWindowDataSourcesDeepEnough(5, refs, map[string]int{"obs": 3})
		})
	})
	t.Run("depth equal to the window is accepted silently", func(t *testing.T) {
		// Boundary: depth == window is exactly enough, not one short.
		wantLog(t, "", func() {
			wantNoPanic(t, func() {
				assertWindowDataSourcesDeepEnough(5, refs, map[string]int{"obs": 5})
			})
		})
	})
	t.Run("a partition deeper than the window is warned about", func(t *testing.T) {
		// Too deep cannot underflow, so it runs — and silently replays zeros.
		// The warning is the only signal the user gets, so it must fire.
		wantLog(t, "StateHistoryDepth 50 > Window.Depth 5", func() {
			wantNoPanic(t, func() {
				assertWindowDataSourcesDeepEnough(5, refs, map[string]int{"obs": 50})
			})
		})
	})
}

func TestValidateWindowDataHistoryDepth(t *testing.T) {
	refs := []analysis.DataRef{{PartitionName: "obs"}}

	t.Run("a non-positive window depth is not a constraint", func(t *testing.T) {
		wantNoPanic(t, func() {
			ValidateWindowDataHistoryDepth(0, map[string]int{}, refs)
		})
	})
	t.Run("no data refs means there is nothing to check", func(t *testing.T) {
		wantNoPanic(t, func() {
			ValidateWindowDataHistoryDepth(4, map[string]int{}, nil)
		})
	})
	t.Run("an unlisted partition defaults to depth 1 and is rejected", func(t *testing.T) {
		// AddPartitionsToStateTimeStorage defaults absent names to depth 1, so
		// omitting a name is not the same as opting out — it is asking for 1.
		wantPanic(t, "has StateHistoryDepth 1", func() {
			ValidateWindowDataHistoryDepth(4, map[string]int{}, refs)
		})
	})
	t.Run("a partition shallower than the window is rejected", func(t *testing.T) {
		wantPanic(t, "Window.Depth is 4", func() {
			ValidateWindowDataHistoryDepth(4, map[string]int{"obs": 2}, refs)
		})
	})
	t.Run("depth equal to the window is accepted silently", func(t *testing.T) {
		wantLog(t, "", func() {
			wantNoPanic(t, func() {
				ValidateWindowDataHistoryDepth(4, map[string]int{"obs": 4}, refs)
			})
		})
	})
	t.Run("a partition deeper than the window is warned about", func(t *testing.T) {
		wantLog(t, "carry no information", func() {
			wantNoPanic(t, func() {
				ValidateWindowDataHistoryDepth(4, map[string]int{"obs": 2000}, refs)
			})
		})
	})
}

func TestValidateAppliedPosteriorWidths(t *testing.T) {
	// applied builds a 2-dimensional posterior spec, then lets each subtest
	// break exactly one width so the failure is attributable.
	applied := func(mean, sampler, cov []float64, justVariance bool,
	) AppliedPosteriorEstimation {
		return AppliedPosteriorEstimation{
			Mean:    PosteriorMean{Name: "mean", Default: mean},
			Sampler: PosteriorSampler{Name: "sampler", Default: sampler},
			Covariance: PosteriorCovariance{
				Name: "cov", Default: cov, JustVariance: justVariance,
			},
		}
	}

	t.Run("an empty mean has no dimension to validate against", func(t *testing.T) {
		wantPanic(t, "Mean.Default must be non-empty", func() {
			validateAppliedPosteriorWidths(
				applied(nil, []float64{0, 0}, []float64{1, 0, 0, 1}, false))
		})
	})
	t.Run("a sampler of the wrong dimension is rejected", func(t *testing.T) {
		wantPanic(t, "must match mean dimension 2, got 3", func() {
			validateAppliedPosteriorWidths(
				applied([]float64{0, 0}, []float64{0, 0, 0},
					[]float64{1, 0, 0, 1}, false))
		})
	})
	t.Run("a full covariance must be N squared", func(t *testing.T) {
		// The most valuable of these: a length-2 covariance for a
		// 2-dimensional posterior is the natural mistake, and it is only
		// wrong because JustVariance was not set.
		wantPanic(t, "must have length N²=4", func() {
			validateAppliedPosteriorWidths(
				applied([]float64{0, 0}, []float64{0, 0}, []float64{1, 1}, false))
		})
	})
	t.Run("a full covariance of N squared is accepted", func(t *testing.T) {
		wantNoPanic(t, func() {
			validateAppliedPosteriorWidths(
				applied([]float64{0, 0}, []float64{0, 0},
					[]float64{1, 0, 0, 1}, false))
		})
	})
	t.Run("JustVariance requires exactly N entries", func(t *testing.T) {
		// The mirror image: the same length-4 vector that is correct above is
		// wrong once JustVariance flips the expected shape.
		wantPanic(t, "JustVariance requires Default length 2", func() {
			validateAppliedPosteriorWidths(
				applied([]float64{0, 0}, []float64{0, 0},
					[]float64{1, 0, 0, 1}, true))
		})
	})
	t.Run("JustVariance with N entries is accepted", func(t *testing.T) {
		wantNoPanic(t, func() {
			validateAppliedPosteriorWidths(
				applied([]float64{0, 0}, []float64{0, 0}, []float64{1, 1}, true))
		})
	})
}
