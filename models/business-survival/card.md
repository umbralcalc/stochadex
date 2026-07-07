# Local-authority business survival — a monthly sector×age Leslie register under support policy

> **Methodology card.** This is the primary human- and agent-legible description of
> the model. The runnable stub beside it ([`stub.go`](stub.go)) is the type-checked
> generative demonstration; this card carries the structure, assumptions, and
> validity regime that the Go code does not spell out.

## System

The business demography of a single UK local authority: the standing **register** of
active businesses, stratified by **sector** and by **age in months**, evolving under
monthly **formation** (new businesses arriving), **exit hazards** (businesses failing),
macroeconomic covariates, and **support-policy multipliers**. Businesses are born into
an age-0 bucket per sector, age one month per step, and exit at a monthly hazard derived
from an ONS-style cumulative survival curve; a 60th "top" bucket aggregates all ages ≥59
months. The quantities of interest are the **standing register stock** (how many
businesses are alive) and **five-year cohort survival** (what fraction of a birth cohort
is still active after 60 months), and how each responds to support interventions and to
the economic weather.

The generative core is a single partition — the whole demographic engine lives in one
bespoke iteration:

| Partition | Iteration | State | Role |
|---|---|---|---|
| `population` | `SingleLAPopulationIteration` | `[count per (sector, age-month)]`, width `nSectors×60` | Monthly Leslie register: per-sector Poisson formation, age progression, ONS-derived exit hazards, macro + policy multipliers |

**Formation.** Each sector draws new businesses from a Poisson process at a base monthly
rate, scaled by an economic birth multiplier (log-linear in Bank Rate, claimant count and
optional GDP growth) and by policy birth multipliers (a global `policy_birth_scale` and a
per-sector `policy_sector_birth_scale`).

**Exit hazards.** A cumulative survival curve at years 1–5 (ONS business demography) is
converted to a piecewise-constant monthly hazard (`hazard.go`), applied per sector via a
binomial thinning of each age bucket. The effective hazard is scaled by a per-sector
structural multiplier (`sector_hazard_scales`), an economic death multiplier (log-linear
in Bank Rate), a distress boost, and policy hazard multipliers — a global
`policy_death_hazard_scale`, a per-sector `policy_sector_hazard_scale`, and a first-year
`policy_infant_hazard_scale` acting only on the age 0→1 transition.

**The swept driver.** `BuildStub` exposes one knob, `hazardScale`, which sets
`policy_death_hazard_scale`: the intensity of a support package that lowers business exits
(`< 1`) or an adverse shock that raises them (`> 1`). Every other input is a literal
`Default*` constant. The register starts empty and fills from formation, approaching a
quasi-steady stock of `formation × mean-lifetime` — which is exactly what the hazard
multiplier moves.

## Ingests (in the stub: nothing)

The stub is **data-free** — every input is a literal constant in [`stub.go`](stub.go),
with `hazardScale` exposed as the one swept driver. In the downstream application the
baseline **survival curve** is taken from **ONS business demography** (by LA / sector /
cohort); **per-LA monthly formation** is built from the **Companies House** live register
joined to **ONS NSPL** postcode geography; the **macroeconomic covariates** are **Bank of
England** Bank Rate and **NOMIS** claimant counts; and the **birth / hazard elasticities**
are estimated from a panel first-difference regression and **SMC** calibration. All of
that ingestion and calibration stays downstream — the stub *generates* formation and
survival structurally from constants where the downstream *fits* them from data.

## Assumptions

- **The register is a single-LA, sector×age-in-months Leslie process.** State is a count
  per `(sector, age-month)` bucket, 60 age buckets with the last aggregating all ages ≥59
  months; there is no size band, no legal-form or turnover dimension.
- **Exit hazards come from an annual ONS survival curve, made piecewise-constant within
  each year of life.** A single national-style curve is shared across sectors until a
  structural or policy multiplier differentiates them; the top bucket applies the year-5
  monthly hazard indefinitely (a geometric tail).
- **Formation is independent Poisson per sector**, scaled multiplicatively by economics and
  policy; there is no capacity constraint, no feedback from stock to formation, and no
  inter-sector coupling.
- **Economics enters log-linearly** through fixed elasticities on Bank Rate, claimant count
  and optional GDP growth, relative to reference levels. In the stub the elasticities
  default to zero (macro-neutral) and covariates are held constant, isolating demography
  and policy; behaviour tests activate the macro channels.
- **Support policies are static multipliers** on births and hazards (global, per-sector, and
  first-year infant), not a modelled programme with take-up, cost, or displacement. The
  scenario overlays (recession / expansion) and portfolio definitions are a downstream
  decision layer and are **not** in the stub.
- **Counts are treated as continuous in mean-field mode** (`deterministic`) and as
  Poisson/binomial integers otherwise; the stub's headline run is stochastic.
- **Monthly steps**, constant Δ = 1 month.

## Validity regime

- Intended for **relative, ranking** questions ("which support lever, and how much, moves
  register stock or five-year survival, and in which direction?"), not absolute business
  counts or official-statistic reproduction. The stock scale is arbitrary (driven by the
  illustrative formation rates); compare **across interventions**, not to real register
  totals.
- Trustworthy for the **direction and rough shape** of the hazard → lifetime → stock and the
  support → survival relationships. At the baseline hazard multiplier the isolated cohort
  reproduces the ONS **five-year survival ≈ 0.384** by construction, which anchors the
  survival metric even though absolute stock is uncalibrated.
- A short **spin-up** is implicit: the register starts empty and fills over the first months
  before reaching quasi-steady stock; the tests average over the back half of the run.
- Sector heterogeneity is only as rich as the `sector_hazard_scales` / per-sector policy
  vectors make it — the stub ships a homogeneous baseline, so sector-level claims are about
  *responses to a differentiating driver*, not calibrated sector differences.

## Failure modes

- **Uncalibrated parameters give plausible-looking but wrong magnitudes.** The structure
  guarantees sign and monotonicity (lower hazard → more stock, higher survival), not level —
  absolute stock and even absolute survival away from the baseline multiplier depend on
  calibration.
- **Extreme multipliers clamp the hazard.** A large `policy_death_hazard_scale`, distress
  boost, or rate-elasticity product can push the monthly hazard to its `[0, 1]` clamp,
  flattening the response — the model saturates rather than extrapolating linearly.
- **The geometric top bucket overstates old-business persistence.** All ages ≥59 months share
  one hazard forever, so very long-lived firms decay too slowly; the stub is not a vehicle
  for the far tail of the age distribution.
- **No stock feedback or displacement.** Formation never responds to crowding, and one
  sector's or neighbour's policy never diverts activity from another — so the stub cannot
  speak to net additionality, only gross per-lever response.
- **Survival vs stock can move differently.** Formation-side levers raise stock without
  moving cohort survival, and vice versa; reading one metric as a proxy for the other
  misranks portfolios (the very distinction the downstream evaluator reports separately).

## Question answered

*For a single local authority, in which direction — and roughly how much — do the standing
business register and five-year cohort survival move as a support package changes the exit
hazard (and as formation support, first-year support, sector targeting, and the economic
weather change), so that support portfolios can be ranked?*

## Generative behaviour under test

[`stub_test.go`](stub_test.go) asserts, beyond "it runs":
1. **Harness** — no NaNs, correct state width, no `params` mutation, no statefulness residue
   across a repeated run (`simulator.RunWithHarnesses`).
2. **Structural invariants** — every sector×age bucket is a non-negative business count of
   the correct width at every step, and (from an empty register) formation leaves a strictly
   positive standing stock.
3. **Correct direction of parameter response** — a supportive hazard multiplier
   (`hazardScale = 0.85`) yields a higher back-half register stock than an adverse one
   (`1.15`), averaged over an 8-member stochastic ensemble. (Deterministic back-half stock
   over the sweep `hazardScale` 0.80 → 0.90 → 1.00 → 1.10 → 1.20: 1493 → 1418 → 1348 → 1285
   → 1226 — monotone. The isolated five-year cohort survival over 0.80 → 1.00 → 1.20:
   0.4657 → 0.3840 → 0.3164, with the baseline exactly matching the ONS ≈0.384 benchmark.)

The **expected-behaviour suite** ([`behaviour_test.go`](behaviour_test.go)) makes the
decision-readiness explicit — each subtest is a named, plain-language response claim, run in
deterministic mean-field mode so each signed effect is exact:

- *Decision-path responses (actionable support levers a downstream controls):* a lower
  death-hazard scale raises five-year cohort survival (the headline lever on the signature
  metric); formation support raises register stock; lower first-year (infant) hazard raises
  cohort survival; a **sector-targeted** formation subsidy raises *that* sector's stock; and
  sector-targeted hazard relief raises *that* sector's stock. The last two verify the
  `(sector, action) → outcome` path directly — a wrong sign there is a mis-targeted
  recommendation.
- *Structural-driver responses (non-actionable; out-of-sample credibility):* a worse
  baseline ONS survival curve lowers stock; a higher Bank Rate (with a negative birth
  elasticity) suppresses formation; a higher claimant count suppresses formation; a higher
  Bank Rate (with a positive death elasticity) lowers cohort survival; a positive distress
  boost lowers cohort survival; and a higher structural sector hazard lowers that sector's
  stock. These cover every mechanism — formation, baseline demography, both macro channels,
  the distress channel, and sector heterogeneity — the model was not tuned against.

## Bespoke extensions (staged beside the stub)

`SingleLAPopulationIteration` ([`single_la_population.go`](single_la_population.go)) and its
survival→hazard helper ([`hazard.go`](hazard.go)) are custom `simulator.Iteration` /
generative code lifted verbatim from the downstream repo's `pkg/population`. They live here
rather than in engine core because the catalogue is the staging ground for the "should this
be promoted into core?" question — a generic **aged-cohort / Leslie** primitive (age-bucket
progression with per-bucket hazards and a boundary aggregating bucket) recurring across other
models would be the signal to promote, but that waits for the recurrence.

The data-fitting helpers that accompany this iteration downstream — the ONS survival-curve
loader, the panel first-difference regression and elasticity mapping, and the SMC
forward-statistic iterations (`ScaledCohortSurvivalIteration`,
`PopulationSurvivalBirthMomentsIteration`) — are inference / ingestion concerns and were
left downstream. The scenario overlays and support portfolios are a decision layer and
likewise stay downstream; here they appear only as raw param multipliers the behaviour suite
sweeps.

## Downstream

Data ingestion (ONS / Companies House / NSPL / NOMIS / Bank of England), panel and SMC
calibration, the support-portfolio and macro-scenario decision layer, and the Monte-Carlo
policy evaluator live in the project repo:

**https://github.com/umbralcalc/business-survival**
