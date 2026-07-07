package bizsurvival

import (
	"math"
	"math/rand/v2"

	"github.com/umbralcalc/stochadex/pkg/simulator"
	"gonum.org/v1/gonum/stat/distuv"
)

// SingleLAPopulationIteration is a monthly Leslie-style model of a single
// local authority business stock: businesses are stratified by sector and
// age in months (60 buckets, 0..59). The top bucket aggregates all ages ≥59
// months.
//
// Params (all required unless noted):
//   - survival_fracs: 5 cumulative survival fractions (years 1–5), 0–1 scale.
//   - sector_hazard_scales: per-sector multipliers on the baseline monthly hazard.
//   - base_birth_rates: Poisson arrival rate per month for each sector.
//   - covariate_bank_rates: time series aligned to simulation months (indexed by
//     floor(current_time)); last value repeats if the run exceeds its length.
//   - covariate_claimants: same length as covariate_bank_rates (or length 1 for constant).
//   - rate_ref, claimant_ref: reference levels for elasticities (scalar params, length 1).
//   - birth_elasticity_rate, birth_elasticity_claimant: log-linear birth scaling.
//   - birth_elasticity_gdp: optional; use with covariate_gdp_growth (same indexing as bank rates).
//   - gdp_ref: optional reference level for GDP growth (% or index change); defaults to 0 if absent.
//   - death_elasticity_rate: log-linear hazard scaling (applied to all sectors).
//   - deterministic: optional [1] for mean-field (no Poisson/binomial noise); default stochastic.
//
// Optional policy layers (interventions — all default to no effect when absent):
//   - policy_birth_scale: scalar ≥0 multiplies births after economic birthMult (default 1).
//   - policy_death_hazard_scale: scalar ≥0 multiplies effective monthly hazard (after deathMult; default 1).
//   - policy_sector_birth_scale: length nSectors, per-sector birth multiplier (default 1 each).
//   - policy_sector_hazard_scale: length nSectors, per-sector hazard multiplier (default 1 each).
//   - policy_infant_hazard_scale: scalar applied only to the hazard for age 0→1 month (default 1).
//   - distress_hazard_boost: optional time series (same indexing as bank rates); death hazard
//     multiplier is further scaled by (1 + boost[t]) when boost ≥ 0 (leading-indicator / distress).
//
// Stochastic sampling uses the partition Seed from settings (re-initialised in Configure).
type SingleLAPopulationIteration struct {
	nSectors int
	nAges    int
	width    int

	monthlyHazard [][]float64

	poisson  distuv.Poisson
	binomial distuv.Binomial
	src      rand.Source

	deterministic bool

	scratch []float64
}

func (s *SingleLAPopulationIteration) Configure(partitionIndex int, settings *simulator.Settings) {
	is := settings.Iterations[partitionIndex]
	scales := is.Params.Get("sector_hazard_scales")
	surv := is.Params.Get("survival_fracs")

	s.nSectors = len(scales)
	s.nAges = 60
	s.width = s.nSectors * s.nAges
	if is.StateWidth != s.width {
		panic("bizsurvival: state_width must equal len(sector_hazard_scales)*60")
	}
	if len(is.InitStateValues) != s.width {
		panic("bizsurvival: init_state_values length must equal state_width")
	}

	base := MonthlyHazardsFromCumulativeSurvival(surv)
	s.monthlyHazard = make([][]float64, s.nSectors)
	for sec := 0; sec < s.nSectors; sec++ {
		row := make([]float64, 60)
		for m := range base {
			row[m] = base[m] * scales[sec]
			if row[m] < 0 {
				row[m] = 0
			}
			if row[m] > 1 {
				row[m] = 1
			}
		}
		s.monthlyHazard[sec] = row
	}

	seed := is.Seed
	s.src = rand.NewPCG(seed, seed)
	s.poisson = distuv.Poisson{Lambda: 1.0, Src: s.src}
	s.binomial = distuv.Binomial{N: 1, P: 0.5, Src: s.src}

	det, ok := is.Params.GetOk("deterministic")
	s.deterministic = ok && len(det) > 0 && det[0] >= 0.5

	if s.scratch == nil || len(s.scratch) != s.width {
		s.scratch = make([]float64, s.width)
	} else {
		for i := range s.scratch {
			s.scratch[i] = 0
		}
	}
}

func (s *SingleLAPopulationIteration) offset(sec, age int) int {
	return sec*s.nAges + age
}

func pickSeries(xs []float64, t int) float64 {
	if len(xs) == 0 {
		return 0
	}
	if t < 0 {
		t = 0
	}
	if t >= len(xs) {
		return xs[len(xs)-1]
	}
	return xs[t]
}

func (s *SingleLAPopulationIteration) covariateIndex(
	params *simulator.Params,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) int {
	t := int(timestepsHistory.Values.AtVec(0))
	if t < 0 {
		t = 0
	}
	rates := params.Get("covariate_bank_rates")
	if len(rates) == 0 {
		return 0
	}
	if t >= len(rates) {
		return len(rates) - 1
	}
	return t
}

func (s *SingleLAPopulationIteration) economicMultipliers(
	params *simulator.Params,
	tIndex int,
) (birthMult, deathMult float64) {
	rates := params.Get("covariate_bank_rates")
	claimants := params.Get("covariate_claimants")
	rate := pickSeries(rates, tIndex)
	claimant := pickSeries(claimants, tIndex)

	rRef := params.GetIndex("rate_ref", 0)
	cRef := params.GetIndex("claimant_ref", 0)
	eBR := params.GetIndex("birth_elasticity_rate", 0)
	eBC := params.GetIndex("birth_elasticity_claimant", 0)
	eDR := params.GetIndex("death_elasticity_rate", 0)

	gdpTerm := 0.0
	if gdpSeries, ok := params.GetOk("covariate_gdp_growth"); ok && len(gdpSeries) > 0 {
		g := pickSeries(gdpSeries, tIndex)
		gRef := 0.0
		if gRefSlice, ok2 := params.GetOk("gdp_ref"); ok2 && len(gRefSlice) > 0 {
			gRef = gRefSlice[0]
		}
		eGDP := 0.0
		if eSlice, ok3 := params.GetOk("birth_elasticity_gdp"); ok3 && len(eSlice) > 0 {
			eGDP = eSlice[0]
		}
		gdpTerm = eGDP * (g - gRef)
	}

	birthMult = math.Exp(eBR*(rate-rRef) + eBC*(math.Log(claimant/cRef)) + gdpTerm)
	deathMult = math.Exp(eDR * (rate - rRef))
	if birthMult < 0 || math.IsNaN(birthMult) {
		birthMult = 1
	}
	if deathMult < 0 || math.IsNaN(deathMult) {
		deathMult = 1
	}
	return birthMult, deathMult
}

func policyScalarParam(params *simulator.Params, key string, def float64) float64 {
	xs, ok := params.GetOk(key)
	if !ok || len(xs) == 0 {
		return def
	}
	v := xs[0]
	if v < 0 || math.IsNaN(v) {
		return def
	}
	return v
}

func (s *SingleLAPopulationIteration) policyPerSector(
	params *simulator.Params,
	key string,
	sec int,
) float64 {
	xs, ok := params.GetOk(key)
	if !ok || len(xs) != s.nSectors {
		return 1.0
	}
	v := xs[sec]
	if v <= 0 || math.IsNaN(v) {
		return 1.0
	}
	return v
}

func (s *SingleLAPopulationIteration) poissonSample(lambda float64) float64 {
	if lambda <= 0 {
		return 0
	}
	s.poisson.Lambda = lambda
	return s.poisson.Rand()
}

func (s *SingleLAPopulationIteration) binomialSample(nFloat float64, prob float64) float64 {
	n := int(math.Round(nFloat))
	if n <= 0 {
		return 0
	}
	if prob <= 0 {
		return 0
	}
	if prob >= 1 {
		return float64(n)
	}
	s.binomial.N = float64(n)
	s.binomial.P = prob
	return s.binomial.Rand()
}

func (s *SingleLAPopulationIteration) Iterate(
	params *simulator.Params,
	partitionIndex int,
	stateHistories []*simulator.StateHistory,
	timestepsHistory *simulator.CumulativeTimestepsHistory,
) []float64 {
	hist := stateHistories[partitionIndex]
	tIdx := s.covariateIndex(params, timestepsHistory)
	birthMult, deathMult := s.economicMultipliers(params, tIdx)
	if boost, ok := params.GetOk("distress_hazard_boost"); ok && len(boost) > 0 {
		b := pickSeries(boost, tIdx)
		if !math.IsNaN(b) && b > 0 {
			deathMult *= 1.0 + b
		}
	}
	policyBirth := policyScalarParam(params, "policy_birth_scale", 1.0)
	policyDeath := policyScalarParam(params, "policy_death_hazard_scale", 1.0)
	infantHazard := policyScalarParam(params, "policy_infant_hazard_scale", 1.0)

	// Clear scratch (next state).
	for i := range s.scratch {
		s.scratch[i] = 0
	}

	for sec := 0; sec < s.nSectors; sec++ {
		secBirth := s.policyPerSector(params, "policy_sector_birth_scale", sec)
		secHazard := s.policyPerSector(params, "policy_sector_hazard_scale", sec)
		baseBirth := params.GetIndex("base_birth_rates", sec)
		lambda := baseBirth * birthMult * policyBirth * secBirth
		var births float64
		if s.deterministic {
			births = lambda
		} else {
			births = s.poissonSample(lambda)
		}
		s.scratch[s.offset(sec, 0)] = births

		// Age 1..58: inflow from younger bucket.
		for age := 1; age <= 58; age++ {
			prev := hist.Values.At(0, s.offset(sec, age-1))
			h := s.monthlyHazard[sec][age-1] * deathMult * policyDeath * secHazard
			if age == 1 {
				h *= infantHazard
			}
			if h < 0 {
				h = 0
			}
			if h > 1 {
				h = 1
			}
			pSurv := 1.0 - h
			var moved float64
			if s.deterministic {
				moved = prev * pSurv
			} else {
				moved = s.binomialSample(prev, pSurv)
			}
			s.scratch[s.offset(sec, age)] += moved
		}

		// Top bucket (59): from bucket 58 and survivors already in 59.
		h58 := s.monthlyHazard[sec][58] * deathMult * policyDeath * secHazard
		if h58 < 0 {
			h58 = 0
		}
		if h58 > 1 {
			h58 = 1
		}
		p58 := 1.0 - h58
		prev58 := hist.Values.At(0, s.offset(sec, 58))
		var into59 float64
		if s.deterministic {
			into59 = prev58 * p58
		} else {
			into59 = s.binomialSample(prev58, p58)
		}

		h59 := s.monthlyHazard[sec][59] * deathMult * policyDeath * secHazard
		if h59 < 0 {
			h59 = 0
		}
		if h59 > 1 {
			h59 = 1
		}
		p59 := 1.0 - h59
		prev59 := hist.Values.At(0, s.offset(sec, 59))
		var stay float64
		if s.deterministic {
			stay = prev59 * p59
		} else {
			stay = s.binomialSample(prev59, p59)
		}
		s.scratch[s.offset(sec, 59)] = into59 + stay
	}

	out := make([]float64, s.width)
	copy(out, s.scratch)
	return out
}
