# Rugby match — coupled counting processes turning event rates into a scoreline

> **Methodology card.** This is the primary human- and agent-legible description of
> the model. The runnable stub beside it ([`stub.go`](stub.go)) is the type-checked
> generative demonstration; this card carries the structure, assumptions, and
> validity regime that the Go code does not spell out.

## System

A single 80-minute rugby union match between a **home** and an **away** side, generated
event-by-event as a system of coupled stochastic **counting processes** running one
minute per step. Scoring events (tries, penalties) and discipline events (yellow cards)
arrive as **Cox processes** whose per-minute intensities come from a **log-linear**
(Poisson-GLM) model: `rate = baseline · exp(intercept + Σ βⱼ·covariateⱼ)`. The covariates
are **substitution indicators** — one per position group per side, switching from 0 to 1
at the minute that group's replacements come on — so "fresh legs" shift a side's scoring
rate. Each new try triggers a **Bernoulli conversion** attempt, and a derived
**match-state** partition accumulates the scoreline (try = 5, conversion = 2, penalty = 3),
tracks active yellow cards (a 10-minute sin-bin), and flags the half. The quantities of
interest are the **final scoreline** and **home win probability**, and how they respond to
**substitution timing**.

The generative core is eight partitions:

| Partition | Iteration | State | Role |
|---|---|---|---|
| `baseline_rates` | `general.ParamValuesIteration` | `[6 baseline rates]` | Constant zero baseline (data-free stand-in for the kernel-smoothed series) |
| `sub_covariates` | `general.FromStorageIteration` | `[8 sub indicators]` | Per-minute binary substitution covariates replayed from the strategy |
| `score_rates` | `general.ValuesFunctionIteration` (`ScoreEventRateFunction`) | `[4 rates]` | Log-linear try / penalty rates (home, away) |
| `card_rates` | `general.ValuesFunctionIteration` (`CardEventRateFunction`) | `[2 rates]` | Log-linear yellow-card rates (home, away) |
| `score_events` | `discrete.CoxProcessIteration` | `[home_try, away_try, home_pen, away_pen]` | Cumulative scoring-event counts |
| `card_events` | `discrete.CoxProcessIteration` | `[home_yellow, away_yellow]` | Cumulative yellow-card counts |
| `conversion_events` | `ConversionIteration` | `[home_conv, away_conv]` | Bernoulli conversion per new try |
| `match_state` | `general.ValuesFunctionIteration` (`MatchStateFunction`) | `[home_score, away_score, diff, home_active_yellow, away_active_yellow, half]` | Derived scoreline / cards / half |

**Wiring.** The chain is feed-forward within each step: `baseline_rates` and
`sub_covariates` feed `score_rates` / `card_rates` (within-step `params_from_upstream`);
the rate partitions feed the Cox `score_events` / `card_events`; `score_events` feeds
`conversion_events` (which also reads the previous step's try counts from `score_events`'
state history to detect *new* tries) and `match_state`. `match_state` reads the
`card_events` history 10 steps back to count still-active yellow cards.

**The swept driver.** `BuildStub` exposes one knob, `homeSubMinute`: the minute the home
side empties its bench (all four position groups). Earlier substitution leaves the fresh-legs
covariate on for more of the match, and — with the illustrative positive coefficients —
lifts the home side's try and penalty rates. Every other input is a literal `Default*`
constant; the away side's timing is held fixed so home timing varies in isolation.

## Ingests (in the stub: nothing)

The stub is **data-free** — every input is a literal constant in [`stub.go`](stub.go), with
`homeSubMinute` exposed as the one swept driver. In the downstream application the
**intercepts and covariate coefficients** are a Poisson-GLM fitted by warm-start stochastic
gradient descent to real **SportDevs** match-event streams (tries, penalties, cards, and the
substitution timings that generate the covariates), and the **baseline rates** are
adaptive-bandwidth **kernel-smoothed** per minute from multi-game data (capturing effects
like the late-match rise in yellow cards). All of that fitting and smoothing stays downstream;
the stub replaces the time-varying baseline with a constant zero baseline (so rates fall back
to `exp(intercept + covariates)`) and ships illustrative coefficients.

## Assumptions

- **Events are Cox / inhomogeneous-Poisson counting processes** on one-minute steps: at most
  the modelled increment per channel per minute, with no explicit inter-event clustering
  beyond what the rate model encodes.
- **Rates are log-linear in substitution covariates** (a Poisson GLM with log link). The only
  covariates are binary substitution indicators per position group per side; there is no
  score-state feedback (being behind does not change the rate), no red cards, no weather, no
  fatigue beyond the substitution switch.
- **The stub's baseline is constant** — a single per-minute intercept per channel. The real
  time-of-match profile (e.g. more cards late) lives in the downstream kernel-smoothed
  baseline and is deliberately omitted here.
- **Substitutions only ever raise the substituting side's rates** in the shipped coefficients
  (positive covariate effects); the sign and size are illustrative, not fitted. A side's subs
  do not affect the opponent's rates.
- **Conversions are independent Bernoulli trials** at a fixed per-side probability, fired once
  per new try; penalty *goals* are folded into the penalty channel (3 points each) rather than
  modelled as separate kicks.
- **A yellow card removes no player from the rate model** — the sin-bin is tracked in
  `match_state` for reporting but does not feed back into the scoring intensities (a modelling
  simplification the downstream can extend).
- **One-minute steps**, constant Δ = 1, over an 80-minute match; the half flips at minute 40.

## Validity regime

- Intended for **relative, distributional** questions ("how does *this* substitution timing or
  plan shift the scoreline and win probability?"), not absolute score forecasting of a named
  fixture. With uncalibrated coefficients the *levels* are illustrative; the *responses* are
  the point.
- Trustworthy for the **direction and rough shape** of the substitution-timing → scoring →
  win-probability relationship, and for the signs of the structural drivers (intercepts,
  conversion probability, effect size, card rate).
- The counting processes are correct as **cumulative, non-decreasing** counts, and conversions
  are bounded by tries by construction — the invariants the CI test pins down.
- The home edge in the shipped intercepts is deliberately **small**, so at symmetric
  substitution timing the match is near even (home win probability ≈ 0.53) and the substitution
  lever moves it materially — the intended experimental surface.

## Failure modes

- **Uncalibrated coefficients give plausible-looking but wrong magnitudes.** The structure
  guarantees the signs and monotonic responses, not the scoreline; absolute points and win
  probabilities are meaningless without the downstream fit.
- **The constant baseline erases within-match timing.** Any question about *when* in the match
  events cluster (late-game cards, second-half surges) is outside the stub — that signal is in
  the downstream kernel-smoothed baseline only.
- **The one-event-per-minute Cox increment caps intensity.** If intercepts are pushed very high
  the per-minute rate saturates against the unit step, understating scoring for extreme
  parameters — keep intercepts in the realistic (small per-minute rate) regime.
- **No score-state or player-count feedback.** A side never plays differently when behind, and a
  sin-binned player still "scores" at full rate — so the stub cannot capture comeback dynamics
  or the cost of indiscipline.
- **Substitution effects are assumed beneficial and one-directional.** Real fitted coefficients
  can be negative or noisy; reading the stub's positive fresh-legs effect as established fact
  would over-state the value of early substitution.

## Question answered

*For a rugby match generated from log-linear event rates, in which direction — and roughly how
much — do the scoreline and the home win probability move as the home side changes when (and how
fully) it uses its bench, and how do the structural rate, conversion, and card drivers move the
outcome?*

## Generative behaviour under test

[`stub_test.go`](stub_test.go) asserts, beyond "it runs":
1. **Harness** — no NaNs, correct state widths, no `params` mutation, no statefulness residue
   across a repeated run (`simulator.RunWithHarnesses`).
2. **Structural invariants** — every cumulative counting process (`score_events`,
   `card_events`, `conversion_events`) is non-decreasing; scores are non-negative; the half
   indicator is binary and reaches the second half by the final step; and conversions never
   exceed the tries that trigger them.
3. **Correct direction of parameter response** — an earlier home substitution (minute 20 vs 70)
   raises the ensemble-mean home try count, averaged over a 24-member ensemble. (Observed sweep
   over `homeSubMinute` 15 → 35 → 55 → 75, 200-run ensemble: home tries 6.08 → 5.37 → 4.64 →
   4.01; home score 47.6 → 42.5 → 37.4 → 32.9 against a flat away 35.8/36.0; home win
   probability 0.70 → 0.64 → 0.53 → 0.42. Away scoring is flat because the away timing is held
   fixed — the response is attributable to the home lever alone.)

The **expected-behaviour suite** ([`behaviour_test.go`](behaviour_test.go)) makes the
decision-readiness explicit — each subtest is a named, plain-language response claim:

- *Decision-path responses (actionable levers a coach / analyst controls):* an earlier home
  substitution raises the home **win probability** (the metric that matters); substituting more
  position groups raises home tries; and — the symmetric mirror — an earlier *away* substitution
  raises away tries. These are the `(substitution decision) → outcome` paths the model exists to
  inform; a wrong sign is a wrong recommendation.
- *Structural-driver responses (non-actionable; out-of-sample credibility):* a higher try
  intercept raises home tries; a higher conversion probability raises the home score (the
  tries → points channel); a stronger per-group substitution coefficient produces a bigger
  scoring gain from the same timing; the small home-advantage intercept makes home outscore away
  under *symmetric* substitutions; and a higher yellow-card intercept raises cards (the
  independent discipline channel). These span every mechanism — the score rate, the conversion
  step, the covariate effect size, the home edge, and the card process — none of which the stub
  was tuned against.

## Bespoke extensions (staged beside the stub)

`ScoreEventRateFunction` / `CardEventRateFunction` and their `computeRates` core
([`rate_function.go`](rate_function.go)), `ConversionIteration` ([`conversion.go`](conversion.go)),
`MatchStateFunction` ([`state_function.go`](state_function.go)), and the `SubstitutionStrategy`
→ covariate machinery ([`substitution.go`](substitution.go)) are custom `simulator.Iteration` /
generative code lifted verbatim from the downstream repo's `pkg/match`. The event counting and
signal-holding partitions reuse the engine's own `discrete.CoxProcessIteration`,
`general.ParamValuesIteration`, `general.FromStorageIteration` and `general.ValuesFunctionIteration`,
so no bespoke generator is needed there.

These lifted pieces live here rather than in engine core because the catalogue is the staging
ground for the "should this be promoted into core?" question — a generic **log-linear
(Poisson-GLM) rate primitive** (`baseline · exp(intercept + Σβ·x)`) recurring across other models
would be the signal to promote, but that waits for the recurrence. The data-fitting helpers that
accompany these iterations downstream — the SportDevs API client, the adaptive-bandwidth kernel
smoothing of baseline rates, and the warm-start SGD training of the coefficients — are inference /
ingestion concerns and were left downstream.

## Downstream

Data ingestion (the SportDevs rugby API), baseline kernel smoothing, Poisson-GLM training, and
the substitution-counterfactual evaluation / win-probability decision layer live in the project
repo:

**https://github.com/umbralcalc/trywizard**
