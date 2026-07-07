package bizsurvival

import "math"

// MonthlyHazardsFromCumulativeSurvival builds 60 monthly discrete hazards from
// ONS-style cumulative survival fractions at 1–5 years after birth (indices 0–4
// = survival at end of years 1–5). surv[k] is the fraction still active after
// k+1 full years (e.g. surv[0]=0.946 ⇒ 94.6% one-year survival).
//
// Within each year of life, the hazard is piecewise constant across months so
// that Π_m (1-h_m) over 12 months equals the annual conditional survival ratio.
func MonthlyHazardsFromCumulativeSurvival(surv []float64) []float64 {
	if len(surv) < 5 {
		panic("bizsurvival: need 5 cumulative survival fractions (years 1–5)")
	}
	out := make([]float64, 60)
	prev := 1.0
	for y := 0; y < 5; y++ {
		ratio := surv[y] / prev
		if ratio <= 0 || ratio > 1 {
			ratio = math.Max(1e-12, math.Min(1.0, ratio))
		}
		monthlySurv := math.Pow(ratio, 1.0/12.0)
		h := 1.0 - monthlySurv
		for m := 0; m < 12; m++ {
			out[y*12+m] = h
		}
		prev = surv[y]
	}
	return out
}

// CumulativeSurvivalAfterMonths returns the theoretical fraction surviving after
// months steps given monthly hazards (same indexing as MonthlyHazardsFromCumulativeSurvival).
func CumulativeSurvivalAfterMonths(monthlyHazard []float64, months int) float64 {
	s := 1.0
	for m := 0; m < months && m < len(monthlyHazard); m++ {
		h := monthlyHazard[m]
		if h < 0 {
			h = 0
		}
		if h > 1 {
			h = 1
		}
		s *= 1.0 - h
	}
	return s
}
