package api

import (
	"github.com/umbralcalc/stochadex/pkg/analysis"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// The aggregation macros: rolling windowed mean / variance / covariance of a data
// reference, weighted by an integration kernel.

// aggregationFields are the AppliedAggregation fields shared by the aggregation
// macros, plus the params: pass-through and the data-history window.
type aggregationFields struct {
	Name         string                  `yaml:"name"`
	Data         dataRefSpec             `yaml:"data"`
	Kernel       simulator.ComponentSpec `yaml:"kernel,omitempty"`
	DefaultValue float64                 `yaml:"default_value,omitempty"`
	Params       map[string][]float64    `yaml:"params,omitempty"`
	Window       int                     `yaml:"window,omitempty"`
}

func (f aggregationFields) applied() (analysis.AppliedAggregation, error) {
	applied := analysis.AppliedAggregation{
		Name:         f.Name,
		Data:         f.Data.resolve(),
		DefaultValue: f.DefaultValue,
	}
	if f.Kernel.IsData() {
		kernel, err := resolveKernel(f.Kernel)
		if err != nil {
			return applied, err
		}
		applied.Kernel = kernel
	}
	return applied, nil
}

func (f aggregationFields) window() int {
	if f.Window == 0 {
		return 1
	}
	return f.Window
}

// windowsFor pairs the macro partition (window 1) with its data source.
func (f aggregationFields) windows() map[string]int {
	return map[string]int{f.Name: 1, f.Data.PartitionName: f.window()}
}

type vectorMeanSpec struct {
	macroTypeField    `yaml:",inline"`
	aggregationFields `yaml:",inline"`
}

func (s *vectorMeanSpec) resolve(
	storage *simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, map[string]int, error) {
	applied, err := s.applied()
	if err != nil {
		return nil, nil, err
	}
	partition := analysis.NewVectorMeanPartition(applied, storage)
	applyParams(partition, s.Params)
	return []*simulator.PartitionConfig{partition}, s.windows(), nil
}

type vectorVarianceSpec struct {
	macroTypeField    `yaml:",inline"`
	Mean              dataRefSpec `yaml:"mean"`
	aggregationFields `yaml:",inline"`
}

func (s *vectorVarianceSpec) resolve(
	storage *simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, map[string]int, error) {
	applied, err := s.applied()
	if err != nil {
		return nil, nil, err
	}
	partition := analysis.NewVectorVariancePartition(s.Mean.resolve(), applied, storage)
	applyParams(partition, s.Params)
	windows := s.windows()
	windows[s.Mean.PartitionName] = s.window()
	return []*simulator.PartitionConfig{partition}, windows, nil
}

type vectorCovarianceSpec struct {
	macroTypeField    `yaml:",inline"`
	Mean              dataRefSpec `yaml:"mean"`
	aggregationFields `yaml:",inline"`
}

func (s *vectorCovarianceSpec) resolve(
	storage *simulator.StateTimeStorage,
) ([]*simulator.PartitionConfig, map[string]int, error) {
	applied, err := s.applied()
	if err != nil {
		return nil, nil, err
	}
	partition := analysis.NewVectorCovariancePartition(s.Mean.resolve(), applied, storage)
	applyParams(partition, s.Params)
	windows := s.windows()
	windows[s.Mean.PartitionName] = s.window()
	return []*simulator.PartitionConfig{partition}, windows, nil
}
