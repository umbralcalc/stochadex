package floodrisk

import (
	"math"

	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// RainfallRunoffIteration implements a lumped conceptual rainfall-runoff
// model for a single sub-catchment using PDM-style nonlinear runoff
// generation and parallel fast/slow flow stores.
//
// State vector: [soil_moisture_mm, total_flow_m3s, fast_flow_m3s, slow_flow_m3s]
//
// Parameters can be provided in two ways:
//
// Named params:
//   - field_capacity:       max soil moisture storage (mm)
//   - drainage_rate:        fraction of soil moisture draining per day
//   - et_rate:              evapotranspiration rate (mm/day)
//   - runoff_shape:         PDM exponent controlling nonlinear runoff generation
//   - fast_recession_rate:  recession constant for fast flow store (0–1)
//   - slow_recession_rate:  recession constant for slow baseflow store (0–1)
//   - catchment_area_km2:   catchment area for mm→m³/s conversion
//
// Vectorized (for inference wiring via params_from_upstream):
//   - model_params: [field_capacity, drainage_rate, et_rate,
//     runoff_shape, fast_recession_rate, slow_recession_rate, catchment_area_km2]
//
// Additional:
//   - upstream_partition: partition index providing rainfall
type RainfallRunoffIteration struct {
	upstreamPartitionIndex int
}

func (r *RainfallRunoffIteration) Configure(
	partitionIndex int,
	settings *simulator.Settings,
) {
	r.upstreamPartitionIndex = int(
		settings.Iterations[partitionIndex].Params.Map["upstream_partition"][0],
	)
}

func (r *RainfallRunoffIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	// Read parameters — vectorized or individual named params.
	var fieldCapacity, drainageRate, etRate float64
	var runoffShape, fastRecession, slowRecession, catchmentArea float64
	if mp, ok := params.GetOk("model_params"); ok {
		fieldCapacity = math.Max(mp[0], 1e-6)
		drainageRate = math.Max(mp[1], 0.0)
		etRate = math.Max(mp[2], 0.0)
		runoffShape = math.Max(mp[3], 1e-6)
		fastRecession = math.Min(math.Max(mp[4], 0.0), 1.0)
		slowRecession = math.Min(math.Max(mp[5], 0.0), 1.0)
		catchmentArea = math.Max(mp[6], 1e-6)
	} else {
		fieldCapacity = params.Map["field_capacity"][0]
		drainageRate = params.Map["drainage_rate"][0]
		etRate = params.Map["et_rate"][0]
		runoffShape = params.Map["runoff_shape"][0]
		fastRecession = params.Map["fast_recession_rate"][0]
		slowRecession = params.Map["slow_recession_rate"][0]
		catchmentArea = params.Map["catchment_area_km2"][0]
	}

	// Time step in days.
	dt := timestepsHistory.NextIncrement

	// Get rainfall from upstream partition (mm/day).
	rainfall := stateHistories[r.upstreamPartitionIndex].Values.At(0, 0)

	// Previous state.
	current := stateHistories[partitionIndex]
	soilMoisture := current.Values.At(0, 0)
	prevFastFlow := current.Values.At(0, 2)
	prevSlowFlow := current.Values.At(0, 3)

	// --- Soil moisture accounting ---

	// Net rainfall after ET losses (can't go negative).
	netRainfall := math.Max(rainfall-etRate, 0.0) * dt

	// PDM-style nonlinear runoff generation.
	// Runoff fraction increases nonlinearly with soil saturation,
	// representing partial-area runoff from saturated zones.
	saturation := math.Min(math.Max(soilMoisture/fieldCapacity, 0.0), 1.0)
	runoffFraction := 1.0 - math.Pow(1.0-saturation, runoffShape)
	directRunoff := netRainfall * runoffFraction
	infiltration := netRainfall - directRunoff

	// Add infiltration to soil store.
	soilMoisture += infiltration

	// Any excess over field capacity also becomes direct runoff.
	excess := math.Max(soilMoisture-fieldCapacity, 0.0)
	soilMoisture -= excess
	directRunoff += excess

	// Slow drainage from soil store.
	drainage := drainageRate * soilMoisture * dt
	soilMoisture -= drainage
	soilMoisture = math.Max(soilMoisture, 0.0)

	// --- Parallel flow routing ---

	// Convert mm → m³/s: mm * km² * 1000 / (86400 * dt)
	mmToM3s := catchmentArea * 1000.0 / (86400.0 * dt)

	// Fast store: receives direct runoff, responds quickly to rainfall.
	fastContribution := directRunoff * mmToM3s
	fastFlow := fastRecession*fastContribution + (1.0-fastRecession)*prevFastFlow

	// Slow store: receives soil drainage, provides baseflow.
	slowContribution := drainage * mmToM3s
	slowFlow := slowRecession*slowContribution + (1.0-slowRecession)*prevSlowFlow

	return []float64{soilMoisture, fastFlow + slowFlow, fastFlow, slowFlow}
}
