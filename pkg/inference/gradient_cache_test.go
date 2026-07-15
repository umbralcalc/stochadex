package inference

import (
	"math/rand/v2"
	"testing"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// These tests guard the covariance/scale-factorisation cache on the multivariate likelihood
// gradients: the factorisation is computed once per SetParams and reused across a data
// batch. Two properties must hold:
//   1. Reuse — repeated calls within one parameterisation return identical gradients. A
//      corrupted or in-place-mutated cache would drift; this is the specific guard for
//      Wishart, whose inverse is reused across calls and must stay intact.
//   2. Invalidation — SetParams recomputes the factorisation, so a new covariance/scale
//      produces a new gradient rather than a stale one.

func paramsWith(kv map[string][]float64) *simulator.Params {
	p := simulator.NewParams(make(map[string][]float64))
	for k, v := range kv {
		p.Set(k, v)
	}
	return &p
}

func equalExact(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNormalGradientCacheReuseAndInvalidation(t *testing.T) {
	mean := []float64{35.0, 3.6, 1.0}
	covA := []float64{1.0, 0.0, 0.0, 0.0, 3.0, 0.0, 0.0, 0.0, 2.0}
	covB := []float64{2.0, 0.5, 0.0, 0.5, 4.0, 0.0, 0.0, 0.0, 1.5}
	rows := [][]float64{{34, 3, 1.5}, {36, 4, 0.5}, {35.5, 3.2, 1.1}}
	newDist := func(cov []float64) *NormalLikelihoodDistribution {
		d := &NormalLikelihoodDistribution{Src: rand.NewPCG(1, 1)}
		d.SetParams(paramsWith(map[string][]float64{"mean": mean, "covariance_matrix": cov}), 0, nil, nil)
		return d
	}

	// Reference: a fresh distribution factorises independently for each row.
	wantA := make([][]float64, len(rows))
	for i, row := range rows {
		wantA[i] = append([]float64(nil), newDist(covA).EvaluateLogLikeMeanGrad(row)...)
	}

	// Reuse: one SetParams(covA), then repeated interleaved batch calls must match.
	d := newDist(covA)
	for pass := 0; pass < 3; pass++ {
		for i, row := range rows {
			if got := d.EvaluateLogLikeMeanGrad(row); !equalExact(got, wantA[i]) {
				t.Fatalf("covA reuse pass %d row %d: got %v want %v", pass, i, got, wantA[i])
			}
		}
	}

	// Invalidation: SetParams(covB) recomputes — row0's gradient becomes covB's, not covA's.
	wantB0 := newDist(covB).EvaluateLogLikeMeanGrad(rows[0])
	d.SetParams(paramsWith(map[string][]float64{"mean": mean, "covariance_matrix": covB}), 0, nil, nil)
	gotB0 := d.EvaluateLogLikeMeanGrad(rows[0])
	if !equalExact(gotB0, wantB0) {
		t.Fatalf("after SetParams(covB): got %v want %v", gotB0, wantB0)
	}
	if equalExact(gotB0, wantA[0]) {
		t.Fatalf("cache not invalidated: covB gradient equals covA gradient")
	}
}

func TestTGradientCacheReuseAndInvalidation(t *testing.T) {
	mean := []float64{35.0, 3.6, 1.0}
	covA := []float64{1.0, 0.0, 0.0, 0.0, 3.0, 0.0, 0.0, 0.0, 2.0}
	covB := []float64{2.0, 0.5, 0.0, 0.5, 4.0, 0.0, 0.0, 0.0, 1.5}
	rows := [][]float64{{34, 3, 1.5}, {36, 4, 0.5}}
	newDist := func(cov []float64) *TLikelihoodDistribution {
		d := &TLikelihoodDistribution{Src: rand.NewPCG(1, 1)}
		d.SetParams(paramsWith(map[string][]float64{
			"degrees_of_freedom": {8.0}, "mean": mean, "covariance_matrix": cov}), 0, nil, nil)
		return d
	}
	wantA := make([][]float64, len(rows))
	for i, row := range rows {
		wantA[i] = append([]float64(nil), newDist(covA).EvaluateLogLikeMeanGrad(row)...)
	}
	d := newDist(covA)
	for pass := 0; pass < 3; pass++ {
		for i, row := range rows {
			if got := d.EvaluateLogLikeMeanGrad(row); !equalExact(got, wantA[i]) {
				t.Fatalf("covA reuse pass %d row %d: got %v want %v", pass, i, got, wantA[i])
			}
		}
	}
	wantB0 := newDist(covB).EvaluateLogLikeMeanGrad(rows[0])
	d.SetParams(paramsWith(map[string][]float64{
		"degrees_of_freedom": {8.0}, "mean": mean, "covariance_matrix": covB}), 0, nil, nil)
	if gotB0 := d.EvaluateLogLikeMeanGrad(rows[0]); !equalExact(gotB0, wantB0) || equalExact(gotB0, wantA[0]) {
		t.Fatalf("t-dist cache invalidation: got %v (covB want %v, covA %v)", gotB0, wantB0, wantA[0])
	}
}

func TestWishartGradientCacheReuseAndInvalidation(t *testing.T) {
	scaleA := []float64{7.0, 0.0, 0.0, 0.0, 2.7, 0.0, 0.0, 0.0, 1.8}
	scaleB := []float64{5.0, 0.5, 0.0, 0.5, 3.0, 0.0, 0.0, 0.0, 2.5}
	rows := [][]float64{
		{6.0, 0.1, 0.0, 0.1, 2.5, 0.0, 0.0, 0.0, 1.5},
		{8.0, 0.0, 0.2, 0.0, 3.0, 0.0, 0.2, 0.0, 2.0},
	}
	newDist := func(scale []float64) *WishartLikelihoodDistribution {
		d := &WishartLikelihoodDistribution{Src: rand.NewPCG(1, 1)}
		d.SetParams(paramsWith(map[string][]float64{
			"degrees_of_freedom": {45.0}, "scale_matrix": scale}), 0, nil, nil)
		return d
	}
	// Reference per row (independent factorise + inverse each time).
	wantA := make([][]float64, len(rows))
	for i, row := range rows {
		wantA[i] = append([]float64(nil), newDist(scaleA).EvaluateLogLikeMeanGrad(row)...)
	}
	// Reuse across passes: if the cached inverse were scaled in place, pass 2 would drift.
	d := newDist(scaleA)
	for pass := 0; pass < 3; pass++ {
		for i, row := range rows {
			if got := d.EvaluateLogLikeMeanGrad(row); !equalExact(got, wantA[i]) {
				t.Fatalf("scaleA reuse pass %d row %d: got %v want %v", pass, i, got, wantA[i])
			}
		}
	}
	// Invalidation.
	wantB0 := newDist(scaleB).EvaluateLogLikeMeanGrad(rows[0])
	d.SetParams(paramsWith(map[string][]float64{
		"degrees_of_freedom": {45.0}, "scale_matrix": scaleB}), 0, nil, nil)
	if gotB0 := d.EvaluateLogLikeMeanGrad(rows[0]); !equalExact(gotB0, wantB0) || equalExact(gotB0, wantA[0]) {
		t.Fatalf("wishart cache invalidation: got %v (scaleB want %v, scaleA %v)", gotB0, wantB0, wantA[0])
	}
}
