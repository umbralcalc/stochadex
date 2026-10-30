package api

import (
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// peakHeap runs once while sampling the live heap, and returns the most it
// held above where it started. Sampling stops the world, so it runs outside
// any timed loop.
func peakHeap(run func()) uint64 {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	base := stats.HeapAlloc
	var peak atomic.Uint64
	done := make(chan struct{})
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			var now runtime.MemStats
			runtime.ReadMemStats(&now)
			if now.HeapAlloc > base && now.HeapAlloc-base > peak.Load() {
				peak.Store(now.HeapAlloc - base)
			}
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}()
	run()
	close(done)
	<-sampled
	return peak.Load()
}

// BenchmarkMacroConfigs is PLAN 2.12's performance gate: every shipped macros:
// config run through the macro tier, and through its expansion under each
// execution strategy of its run. Expanding happens once, as loading would, so
// both sides time running. Each reports its peak heap, the most of five
// sampled runs, as peak-B. Results are recorded in docs/performance.md.
func BenchmarkMacroConfigs(b *testing.B) {
	b.Chdir("../..")
	strategies := []struct {
		name     string
		strategy simulator.ExecutionStrategy
	}{
		{"default", nil},
		{"inline", &simulator.InlineExecution{}},
		{"persistent_worker", &simulator.PersistentWorkerExecution{}},
	}
	for _, path := range oracleConfigs(b) {
		name := strings.TrimSuffix(filepath.Base(path), ".yaml")
		measure := func(b *testing.B, run func() error) {
			if err := run(); err != nil {
				b.Skipf("does not run: %v", err)
			}
			// One sampled run can miss a short run's peak, so take the most of
			// several.
			var peak uint64
			for range 5 {
				peak = max(peak, peakHeap(func() { _ = run() }))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if err := run(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(peak), "peak-B")
		}
		b.Run(name+"/macro", func(b *testing.B) {
			config, err := LoadConfig(path)
			if err != nil {
				b.Fatal(err)
			}
			measure(b, func() error {
				_, err := RunToStorage(config)
				return err
			})
		})
		for _, strategy := range strategies {
			b.Run(name+"/expanded_"+strategy.name, func(b *testing.B) {
				config, err := LoadConfig(path)
				if err != nil {
					b.Fatal(err)
				}
				expanded, err := Expand(config)
				if err != nil {
					b.Fatal(err)
				}
				if strategy.strategy != nil {
					expanded.Main.Simulation.ExecutionStrategy = strategy.strategy
				}
				measure(b, func() error {
					_, err := RunToStorage(expanded)
					return err
				})
			})
		}
	}
}
