package solarfleet

// Fleet geometry and spatial coupling: the site metadata, the distance-decaying
// correlation kernel, and the Cholesky factor of the resulting covariance. All
// pure and data-free — lifted from the downstream repo's solarfleet/compose.py.
//
// These build the two site-dependent inputs the stochastic core reads as data: the
// per-site clear-sky plane-of-array driver (via geometry.go) and the flattened
// Cholesky factor L of the fleet covariance. Spreading sites apart is expressed
// entirely through these two — a different L and a different driver — which is why
// the dispersion claim is a data override, not a structural change.

import (
	"math"
	"time"

	"gonum.org/v1/gonum/mat"
)

// Illustrative defaults — a plausible GB rooftop-PV fleet, NOT calibrated
// posteriors. The downstream repo (https://github.com/umbralcalc/solar-fleet)
// owns real calibration against OCF uk_pv data.
const (
	// DefaultCloudVolatility is the per-site log-K innovation volatility (the one
	// scientifically-interesting driver BuildStub exposes): higher volatility
	// raises aggregate fleet-output variability.
	DefaultCloudVolatility = 0.8
	// DefaultReversion is the OU reversion speed (theta) of log clear-sky index.
	DefaultReversion = 0.3
	// DefaultCloudSpeed is the regional cloud advection speed, m/s.
	DefaultCloudSpeed = 6.0
	// DefaultC1 is the temporal-distance decay, 1/s (~half-correlation near two
	// hours' cloud travel).
	DefaultC1 = 6.0e-5
	// DefaultClearnessPower shapes the correlation decay (1 = plain exponential).
	DefaultClearnessPower = 1.0
	// DefaultNugget is diagonal jitter keeping the covariance Cholesky-factorable.
	DefaultNugget = 1e-9
	// DefaultKMax soft-caps the clear-sky index (systems can briefly exceed 1).
	DefaultKMax = 1.2
	// DefaultMu is the stationary mean of log clear-sky index (K ~ 0.8 typical).
	DefaultMu = -0.2
	// DefaultEta is the effective system efficiency (derate).
	DefaultEta = 0.85
	// DefaultKwp is the per-site nameplate capacity, kW at STC.
	DefaultKwp = 4.0
	// DefaultTilt / DefaultAzimuth are the panel orientation (degrees): south-facing
	// at a mid-latitude-optimal tilt.
	DefaultTilt    = 35.0
	DefaultAzimuth = 180.0
)

// FleetEpoch is the synthetic UTC start of the run's time axis (a summer day, so
// the daytime envelope is well developed). The clear-sky driver is a deterministic
// function of this and the step index; nothing is loaded.
var FleetEpoch = time.Date(2023, 6, 21, 0, 0, 0, 0, time.UTC)

// FleetStepMinutes is the real spacing between simulation steps used to advance the
// solar geometry (the OU dt stays a dimensionless 1 per step, matching the config).
const FleetStepMinutes = 30.0

// Site is one rooftop PV system: physical siting plus its log-K stationary mean and
// efficiency. Reversion and volatility are shared across the fleet (the CouplingKernel).
type Site struct {
	Name      string
	Latitude  float64
	Longitude float64
	Tilt      float64 // degrees from horizontal
	Azimuth   float64 // degrees clockwise from north (180 = due south)
	Kwp       float64 // nameplate capacity, kW at STC
	Mu        float64 // stationary mean of log clear-sky index
	Eta       float64 // effective system efficiency
}

// CouplingKernel is the distance-decaying spatial coupling of the full-covariance
// form: rho(i,j) = exp(-c1 * dist_m/cloud_speed) ^ clearness_power. Because the OU
// drift is isotropic, the stationary correlation of log K equals rho exactly —
// which is what makes the distance-decay real, unlike a single shared factor.
type CouplingKernel struct {
	CloudSpeed     float64
	C1             float64
	ClearnessPower float64
	Sigma          float64 // per-site log-K innovation volatility
	Nugget         float64
}

// DefaultKernel is the coupling kernel at the illustrative defaults, with a given
// innovation volatility (the swept driver).
func DefaultKernel(cloudVolatility float64) CouplingKernel {
	return CouplingKernel{
		CloudSpeed:     DefaultCloudSpeed,
		C1:             DefaultC1,
		ClearnessPower: DefaultClearnessPower,
		Sigma:          cloudVolatility,
		Nugget:         DefaultNugget,
	}
}

// site builds a Site at the default panel/parameter values.
func site(name string, lat, lon float64) Site {
	return Site{
		Name: name, Latitude: lat, Longitude: lon,
		Tilt: DefaultTilt, Azimuth: DefaultAzimuth, Kwp: DefaultKwp,
		Mu: DefaultMu, Eta: DefaultEta,
	}
}

// DefaultFleet is the illustrative dispersed fleet: four systems spread across
// Great Britain, so the distance-decay coupling has real range to act over.
func DefaultFleet() []Site {
	return []Site{
		site("london", 51.51, -0.13),
		site("reading", 51.46, -0.97),
		site("bristol", 51.45, -2.59),
		site("glasgow", 55.86, -4.25),
	}
}

// CompactFleet is the same four systems clustered within a few km (a single town):
// same capacity, but the coupling is near-total, so the aggregate is more variable.
// Used by the dispersion claim as the low-dispersion comparison point.
func CompactFleet() []Site {
	return []Site{
		site("a", 51.50, -0.12),
		site("b", 51.51, -0.13),
		site("c", 51.52, -0.14),
		site("d", 51.49, -0.11),
	}
}

// haversineKm is the great-circle distance in km between two lat/lon points (deg).
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	p1, p2 := radians(lat1), radians(lat2)
	dp := radians(lat2 - lat1)
	dl := radians(lon2 - lon1)
	a := math.Sin(dp/2)*math.Sin(dp/2) +
		math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

// CorrelationMatrix is the distance-decay correlation matrix rho(i,j) for a fleet.
func CorrelationMatrix(sites []Site, kernel CouplingKernel) *mat.SymDense {
	n := len(sites)
	rho := mat.NewSymDense(n, nil)
	for i := 0; i < n; i++ {
		rho.SetSym(i, i, 1.0)
		for j := i + 1; j < n; j++ {
			dM := haversineKm(sites[i].Latitude, sites[i].Longitude,
				sites[j].Latitude, sites[j].Longitude) * 1000.0
			td := dM / kernel.CloudSpeed
			r := math.Pow(math.Exp(-kernel.C1*td), kernel.ClearnessPower)
			rho.SetSym(i, j, r)
		}
	}
	return rho
}

// CholeskyFactorFlat returns the lower-triangular Cholesky factor L of the fleet
// covariance Sigma_ij = sigma^2 rho_ij (+ nugget), flattened row-major so it can be
// carried as a config param. All-zeros when sigma == 0 (a deterministic fleet).
func CholeskyFactorFlat(sites []Site, kernel CouplingKernel) []float64 {
	s := len(sites)
	flat := make([]float64, s*s)
	if kernel.Sigma == 0.0 {
		return flat
	}
	rho := CorrelationMatrix(sites, kernel)
	cov := mat.NewSymDense(s, nil)
	for i := 0; i < s; i++ {
		for j := i; j < s; j++ {
			v := kernel.Sigma * kernel.Sigma * rho.At(i, j)
			if i == j {
				v += kernel.Nugget
			}
			cov.SetSym(i, j, v)
		}
	}
	var chol mat.Cholesky
	if !chol.Factorize(cov) {
		panic("solarfleet: fleet covariance is not positive definite")
	}
	var lower mat.TriDense
	chol.LTo(&lower)
	for i := 0; i < s; i++ {
		for j := 0; j < s; j++ {
			flat[i*s+j] = lower.At(i, j)
		}
	}
	return flat
}

// ClearSkyDriverRows returns the per-site clear-sky POA driver as one row per step
// (row t is [poa_site0(t), ..., poa_siteS-1(t)]), over numSteps+1 rows so row 0 is
// the init row and steps 1..numSteps have a value. This is the deterministic series
// the from_storage clearsky partition replays.
func ClearSkyDriverRows(sites []Site, numSteps int) [][]float64 {
	rows := make([][]float64, numSteps+1)
	for t := 0; t <= numSteps; t++ {
		when := FleetEpoch.Add(time.Duration(float64(t)*FleetStepMinutes) * time.Minute)
		row := make([]float64, len(sites))
		for i, s := range sites {
			row[i] = ClearSkyPOA(s.Latitude, s.Longitude, when, s.Tilt, s.Azimuth)
		}
		rows[t] = row
	}
	return rows
}
