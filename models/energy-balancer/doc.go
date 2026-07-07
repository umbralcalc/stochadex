// Package energybalancer is the domain-models catalogue entry for GB electricity
// grid balancing: a data-free generative stub of the residual-demand → imbalance-
// price → battery-dispatch cascade that a grid-scale storage operator faces.
//
// The bespoke simulator.Iteration implementations in this package
// (ImbalancePriceIteration, CarbonThresholdDispatchIteration,
// PriceThresholdDispatchIteration, BatteryIteration, BatteryDegradationIteration,
// RevenueIteration, CarbonSavingsIteration) are lifted verbatim from the
// downstream energy-balancer repo's generative core. CarbonIntensityIteration is
// the one exception: it is the data-free generative counterpart of the downstream
// data-replay CarbonDataIteration (see carbon.go). They are staged here for the
// catalogue's recurring "should this be promoted into engine core?" question — a
// battery state-of-charge tracker or a threshold dispatch primitive recurring
// across other models would be the promotion signal, but that waits for the
// recurrence. The downstream repo owns data ingestion (NESO / Elexon / Carbon
// Intensity API), OU/SMC parameter inference, and the dispatch decision layer.
package energybalancer
