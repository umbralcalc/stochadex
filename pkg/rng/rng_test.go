package rng

import (
	"math/rand/v2"
	"testing"

	"gonum.org/v1/gonum/stat/distuv"
)

// Sampler is a bit-identical, drop-in replacement for the distuv.X{Src}.Rand() calls the
// iterations used to make. These tests are the contract: if a future gonum version changes
// a distribution's algorithm, the mismatch surfaces here rather than as silent output drift
// in every stochastic iteration.

const streamLen = 100_000

func TestStreamIdenticalNormal(t *testing.T) {
	for _, seed := range []uint64{1, 42, 12345} {
		s := New(seed)
		d := &distuv.Normal{Mu: 0, Sigma: 1, Src: rand.NewPCG(seed, seed)}
		for i := 0; i < streamLen; i++ {
			if got, want := s.NormFloat64(), d.Rand(); got != want {
				t.Fatalf("seed %d, draw %d: NormFloat64=%v, distuv.Normal=%v", seed, i, got, want)
			}
		}
	}
}

func TestStreamIdenticalNormalMuSigma(t *testing.T) {
	const mu, sigma = 1.5, 0.3
	s := New(7)
	d := &distuv.Normal{Mu: mu, Sigma: sigma, Src: rand.NewPCG(7, 7)}
	for i := 0; i < streamLen; i++ {
		if got, want := s.Normal(mu, sigma), d.Rand(); got != want {
			t.Fatalf("draw %d: Normal(%v,%v)=%v, distuv=%v", i, mu, sigma, got, want)
		}
	}
}

func TestStreamIdenticalUniform(t *testing.T) {
	s01 := New(3)
	d01 := &distuv.Uniform{Min: 0, Max: 1, Src: rand.NewPCG(3, 3)}
	sab := New(99)
	dab := &distuv.Uniform{Min: -2, Max: 5, Src: rand.NewPCG(99, 99)}
	for i := 0; i < streamLen; i++ {
		if got, want := s01.Float64(), d01.Rand(); got != want {
			t.Fatalf("draw %d: Float64=%v, distuv.Uniform{0,1}=%v", i, got, want)
		}
		if got, want := sab.Uniform(-2, 5), dab.Rand(); got != want {
			t.Fatalf("draw %d: Uniform(-2,5)=%v, distuv.Uniform{-2,5}=%v", i, got, want)
		}
	}
}

func TestStreamIdenticalExponential(t *testing.T) {
	for _, rate := range []float64{1.0, 2.5, 0.3} {
		s := New(21)
		d := &distuv.Exponential{Rate: rate, Src: rand.NewPCG(21, 21)}
		for i := 0; i < streamLen; i++ {
			if got, want := s.Exponential(rate), d.Rand(); got != want {
				t.Fatalf("rate %v, draw %d: Exponential=%v, distuv=%v", rate, i, got, want)
			}
		}
	}
}

// TestNewFromSourceMatchesDistuv covers the derived-source path used by iterations that
// seed a distribution from a master generator (e.g. DriftJumpDiffusion).
func TestNewFromSourceMatchesDistuv(t *testing.T) {
	m1 := rand.New(rand.NewPCG(5, 5))
	m2 := rand.New(rand.NewPCG(5, 5))
	s := NewFromSource(rand.NewPCG(uint64(m1.IntN(1e8)), uint64(m1.IntN(1e8))))
	d := &distuv.Normal{Mu: 0, Sigma: 1, Src: rand.NewPCG(uint64(m2.IntN(1e8)), uint64(m2.IntN(1e8)))}
	for i := 0; i < streamLen; i++ {
		if got, want := s.NormFloat64(), d.Rand(); got != want {
			t.Fatalf("draw %d: NewFromSource NormFloat64=%v, distuv=%v", i, got, want)
		}
	}
}

// BenchmarkNormal quantifies the win: Sampler.NormFloat64 vs distuv.Normal{Src}.Rand().
func BenchmarkNormal(b *testing.B) {
	b.Run("sampler", func(b *testing.B) {
		s := New(1)
		b.ReportAllocs()
		var sink float64
		for i := 0; i < b.N; i++ {
			sink += s.NormFloat64()
		}
		_ = sink
	})
	b.Run("distuv", func(b *testing.B) {
		d := &distuv.Normal{Mu: 0, Sigma: 1, Src: rand.NewPCG(1, 1)}
		b.ReportAllocs()
		var sink float64
		for i := 0; i < b.N; i++ {
			sink += d.Rand()
		}
		_ = sink
	})
}
