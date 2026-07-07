package energybalancer

import (
	"github.com/umbralcalc/stochadex/pkg/continuous"
	"github.com/umbralcalc/stochadex/pkg/simulator"
)

// Default generative parameters for the flagship scenario: a 100 MW / 200 MWh
// grid-scale battery arbitraging the GB imbalance price (and, under an
// alternative policy, chasing carbon intensity). These are illustrative values
// chosen so the generative core runs with zero external inputs — NOT calibrated
// posteriors. In the downstream repo the residual-demand OU parameters are fitted
// from NESO half-hourly data (OLS + SMC) and the price / carbon / battery
// parameters are calibrated against Elexon, Carbon Intensity API, and BESS
// engineering data; see card.md.
//
// The residual-demand mean and the price / carbon coefficients are chosen so the
// baseline imbalance price (~£35/MWh) sits between its charge/discharge thresholds
// (£25 / £45) and the baseline carbon intensity (~175 gCO₂/kWh) sits between its
// thresholds (100 / 250): each battery only earns when volatility carries its
// signal across a threshold, which is exactly the effect the swept driver
// controls.
const (
	// Residual-demand OU generator (the data-free stand-in for NESO CSV replay).
	// stationary std ≈ sigma/sqrt(2*theta); with theta=0.5 that is ≈ sigma.
	DefaultResidualReversion    = 0.5     // theta (per hour) — ~1.4h reversion half-life
	DefaultResidualMeanMW       = 22500.0 // mu (MW) → baseline price ~£35/MWh
	DefaultBaselineVolatilityMW = 1000.0  // sigma at zero renewable penetration
	// Extra residual-demand volatility per unit of renewable penetration — the
	// generative expression of wind/solar intermittency. Higher penetration →
	// larger swings in net load.
	DefaultVolatilityPerPenetrationMW = 4000.0

	// Imbalance-price OU noise (mean-reverting intra-period volatility).
	DefaultPriceNoiseReversion  = 2.0 // theta
	DefaultPriceNoiseVolatility = 5.0 // sigma (£/MWh)

	// Carbon-intensity OU noise (mean-reverting measurement/mix volatility).
	DefaultCarbonNoiseReversion  = 2.0  // theta
	DefaultCarbonNoiseVolatility = 20.0 // sigma (gCO₂/kWh)

	// Structural imbalance-price response to residual demand.
	DefaultDemandSlope     = 0.002 // £/MWh per MW
	DefaultDemandIntercept = -10.0 // £/MWh

	// Structural carbon-intensity response to residual demand: high net load →
	// dirtier marginal plant. Baseline 0.012*22500 - 95 = 175 gCO₂/kWh.
	DefaultCarbonSlope     = 0.012 // gCO₂/kWh per MW
	DefaultCarbonIntercept = -95.0 // gCO₂/kWh

	// Price-threshold arbitrage policy.
	DefaultPriceHigh = 45.0 // discharge above (£/MWh)
	DefaultPriceLow  = 25.0 // charge below (£/MWh)

	// Carbon-threshold policy.
	DefaultCarbonHigh = 250.0 // discharge above (gCO₂/kWh) — displace gas
	DefaultCarbonLow  = 100.0 // charge below (gCO₂/kWh) — absorb clean energy

	// Battery (BESS) physical parameters — shared by both policy chains.
	DefaultPowerRatingMW       = 100.0
	DefaultEnergyCapacityMWh   = 200.0
	DefaultChargeEfficiency    = 0.92
	DefaultDischargeEfficiency = 0.92
	DefaultMinSoCFraction      = 0.1
	DefaultMaxSoCFraction      = 0.9

	// InitialSoCMWh seeds each battery at mid-capacity.
	InitialSoCMWh = 100.0

	// DefaultNumSteps is the flagship horizon: one week of half-hourly settlement
	// periods (7 days × 48).
	DefaultNumSteps = 336

	// Δ per step (hours): a half-hourly settlement period.
	stepSizeHours = 0.5
)

// BuildStub constructs the data-free generative core of the GB grid-balancing
// model: a mean-reverting residual-demand process drives a structural imbalance
// price and a structural carbon intensity (co-moving, since both respond to net
// load). Two identical batteries then arbitrage the same market under two
// different threshold policies — one price-driven, one carbon-driven — so their
// cycling, revenue and carbon savings can be compared under identical conditions.
//
// This is the generative core only — no data ingestion, no OU/SMC inference, no
// sophisticated dispatch-optimisation decision layer. The one scientifically-
// interesting driver, renewablePenetration ∈ [0, 1], enters as the residual-
// demand volatility knob (0 = a calm, low-renewable grid; 1 = a wind-dominated
// 2030-style grid with large net-load swings). It is exactly the parameter the
// CI test sweeps to check the model's headline claim: more renewable
// intermittency raises battery cycling for both policies.
//
// Partitions (declaration order):
//
//	Shared signals
//	  residual_demand   OU net-load process (data-free stand-in for NESO replay)
//	  price_noise       OU mean-reverting price noise (mu=0)
//	  carbon_noise      OU mean-reverting carbon-intensity noise (mu=0)
//	  price             structural imbalance price:   slope·demand + intercept + noise
//	  carbon_intensity  structural carbon intensity:  slope·demand + intercept + noise
//	Price-threshold policy chain
//	  price_dispatch, price_battery, price_efc, price_revenue, price_co2_saved
//	Carbon-threshold policy chain
//	  carbon_dispatch, carbon_battery, carbon_efc, carbon_revenue, carbon_co2_saved
func BuildStub(renewablePenetration float64, numSteps int, seed uint64) *simulator.ConfigGenerator {
	residualVolatility := DefaultBaselineVolatilityMW +
		renewablePenetration*DefaultVolatilityPerPenetrationMW

	residualDemand := &simulator.PartitionConfig{
		Name:      "residual_demand",
		Iteration: &continuous.OrnsteinUhlenbeckExactGaussianIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"thetas": {DefaultResidualReversion},
			"mus":    {DefaultResidualMeanMW},
			"sigmas": {residualVolatility},
		}),
		InitStateValues:   []float64{DefaultResidualMeanMW},
		StateHistoryDepth: 1,
		Seed:              seed,
	}

	priceNoise := &simulator.PartitionConfig{
		Name:      "price_noise",
		Iteration: &continuous.OrnsteinUhlenbeckExactGaussianIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"thetas": {DefaultPriceNoiseReversion},
			"mus":    {0.0},
			"sigmas": {DefaultPriceNoiseVolatility},
		}),
		InitStateValues:   []float64{0.0},
		StateHistoryDepth: 1,
		Seed:              seed + 1,
	}

	carbonNoise := &simulator.PartitionConfig{
		Name:      "carbon_noise",
		Iteration: &continuous.OrnsteinUhlenbeckExactGaussianIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"thetas": {DefaultCarbonNoiseReversion},
			"mus":    {0.0},
			"sigmas": {DefaultCarbonNoiseVolatility},
		}),
		InitStateValues:   []float64{0.0},
		StateHistoryDepth: 1,
		Seed:              seed + 2,
	}

	price := &simulator.PartitionConfig{
		Name:      "price",
		Iteration: &ImbalancePriceIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"demand_slope":     {DefaultDemandSlope},
			"demand_intercept": {DefaultDemandIntercept},
		}),
		ParamsAsPartitions: map[string][]string{
			"demand_partition": {"residual_demand"},
			"noise_partition":  {"price_noise"},
		},
		InitStateValues:   []float64{DefaultDemandSlope*DefaultResidualMeanMW + DefaultDemandIntercept},
		StateHistoryDepth: 1,
		Seed:              0,
	}

	carbon := &simulator.PartitionConfig{
		Name:      "carbon_intensity",
		Iteration: &CarbonIntensityIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"carbon_slope":     {DefaultCarbonSlope},
			"carbon_intercept": {DefaultCarbonIntercept},
		}),
		ParamsAsPartitions: map[string][]string{
			"demand_partition": {"residual_demand"},
			"noise_partition":  {"carbon_noise"},
		},
		InitStateValues:   []float64{DefaultCarbonSlope*DefaultResidualMeanMW + DefaultCarbonIntercept},
		StateHistoryDepth: 1,
		Seed:              0,
	}

	gen := simulator.NewConfigGenerator()
	gen.SetPartition(residualDemand)
	gen.SetPartition(priceNoise)
	gen.SetPartition(carbonNoise)
	gen.SetPartition(price)
	gen.SetPartition(carbon)

	// Two symmetric policy chains arbitrage the same market: dispatch → battery →
	// {efc, revenue, co2_saved}. addPolicyChain wires each one against its signal.
	// The specs are constructed fresh per call (not shared package-level vars) so a
	// caller that overrides one chain's params can never alias into another run.
	addPolicyChain(gen, &policySpec{
		prefix:   "price",
		dispatch: &PriceThresholdDispatchIteration{},
		dispatchParams: map[string][]float64{
			"price_high":      {DefaultPriceHigh},
			"price_low":       {DefaultPriceLow},
			"power_rating_mw": {DefaultPowerRatingMW},
		},
		dispatchAsPartitions: map[string][]string{
			"price_partition": {"price"},
		},
	})
	addPolicyChain(gen, &policySpec{
		prefix:   "carbon",
		dispatch: &CarbonThresholdDispatchIteration{},
		dispatchParams: map[string][]float64{
			"carbon_high":     {DefaultCarbonHigh},
			"carbon_low":      {DefaultCarbonLow},
			"power_rating_mw": {DefaultPowerRatingMW},
		},
		dispatchAsPartitions: map[string][]string{
			"carbon_partition": {"carbon_intensity"},
		},
	})

	gen.SetSimulation(&simulator.SimulationConfig{
		OutputCondition: &simulator.EveryStepOutputCondition{},
		TerminationCondition: &simulator.NumberOfStepsTerminationCondition{
			MaxNumberOfSteps: numSteps,
		},
		TimestepFunction: &simulator.ConstantTimestepFunction{Stepsize: stepSizeHours},
		InitTimeValue:    0.0,
	})
	return gen
}

// policySpec describes one threshold-policy battery chain. The dispatch
// iteration and its threshold params vary; everything downstream (battery,
// degradation, revenue, carbon savings) is identical across policies.
type policySpec struct {
	prefix         string              // partition-name prefix, e.g. "price"
	dispatch       simulator.Iteration // the threshold dispatch iteration
	dispatchParams map[string][]float64
	// dispatchAsPartitions names the shared-signal partitions the dispatch reads
	// (e.g. "price" or "carbon_intensity"), resolved to indices by name so they
	// survive reordering and are visible to pkg/graph.
	dispatchAsPartitions map[string][]string
}

// addPolicyChain appends the five partitions of one policy's battery chain,
// wiring the battery to its dispatch (within-step) and the accumulators to the
// battery / shared price / shared carbon signals (lag-1 via state history).
func addPolicyChain(gen *simulator.ConfigGenerator, spec *policySpec) {
	dispatchName := spec.prefix + "_dispatch"
	batteryName := spec.prefix + "_battery"

	gen.SetPartition(&simulator.PartitionConfig{
		Name:               dispatchName,
		Iteration:          spec.dispatch,
		Params:             simulator.NewParams(spec.dispatchParams),
		ParamsAsPartitions: spec.dispatchAsPartitions,
		InitStateValues:    []float64{0.0},
		StateHistoryDepth:  1,
		Seed:               0,
	})

	gen.SetPartition(&simulator.PartitionConfig{
		Name:      batteryName,
		Iteration: &BatteryIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"energy_capacity_mwh":  {DefaultEnergyCapacityMWh},
			"power_rating_mw":      {DefaultPowerRatingMW},
			"charge_efficiency":    {DefaultChargeEfficiency},
			"discharge_efficiency": {DefaultDischargeEfficiency},
			"min_soc_fraction":     {DefaultMinSoCFraction},
			"max_soc_fraction":     {DefaultMaxSoCFraction},
		}),
		// Dispatch flows in within-step; battery applies it to the current SoC.
		ParamsFromUpstream: map[string]simulator.NamedUpstreamConfig{
			"dispatch_mw": {Upstream: dispatchName, Indices: []int{0}},
		},
		InitStateValues:   []float64{InitialSoCMWh, 0.0},
		StateHistoryDepth: 1,
		Seed:              0,
	})

	// Accumulators reference the battery by name so they stay correct regardless
	// of where the chain lands in declaration order.
	gen.SetPartition(&simulator.PartitionConfig{
		Name:      spec.prefix + "_efc",
		Iteration: &BatteryDegradationIteration{},
		Params: simulator.NewParams(map[string][]float64{
			"energy_capacity_mwh": {DefaultEnergyCapacityMWh},
		}),
		ParamsAsPartitions: map[string][]string{
			"battery_partition": {batteryName},
		},
		InitStateValues:   []float64{0.0},
		StateHistoryDepth: 1,
		Seed:              0,
	})

	gen.SetPartition(&simulator.PartitionConfig{
		Name:      spec.prefix + "_revenue",
		Iteration: &RevenueIteration{},
		Params:    simulator.NewParams(map[string][]float64{}),
		ParamsAsPartitions: map[string][]string{
			"battery_partition": {batteryName},
			"price_partition":   {"price"},
		},
		InitStateValues:   []float64{0.0},
		StateHistoryDepth: 1,
		Seed:              0,
	})

	gen.SetPartition(&simulator.PartitionConfig{
		Name:      spec.prefix + "_co2_saved",
		Iteration: &CarbonSavingsIteration{},
		Params:    simulator.NewParams(map[string][]float64{}),
		ParamsAsPartitions: map[string][]string{
			"battery_partition": {batteryName},
			"carbon_partition":  {"carbon_intensity"},
		},
		InitStateValues:   []float64{0.0},
		StateHistoryDepth: 1,
		Seed:              0,
	})
}
