# Catalogue conventions

The thin spec for a domain-models catalogue entry. It documents the format the entries
follow rather than prescribing a speculative standard: keep it thin, and extend it only
when a new entry needs something the convention does not yet cover.

See [`README.md`](README.md) for *why* the catalogue exists. This file is *how* to add
to it.

## Folder layout

One directory per domain, named for the real-world system in `kebab-case` (matching the
downstream repo name where one exists):

```
models/
  <domain-name>/
    doc.go             # package doc comment: what the model is + the bespoke-extensions note
    card.md            # methodology card — the primary legible spec
    stub.go            # data-free SDK generative core (BuildStub)
    stub_test.go       # engine-CI test: harness + invariants + headline direction-of-response
    behaviour_test.go  # expected-behaviour suite: named, human-legible response claims
    declarative.yaml   # the same model stated as data — no Go, no compilation
    expression_equivalence_test.go  # proves the twin is the same model
    <iteration>.go     # bespoke simulator.Iteration implementations, beside the stub
```

The declarative twin is expected but not always possible — where the model resists the DSL,
its absence is itself a recorded finding (see §5 and [Bespoke extensions](#bespoke-extensions)).

The Go **package name** is a short identifier for the domain (`amr`, `floodrisk`) — it
need not equal the directory name, but must be a valid Go identifier (no hyphens).

Every entry carries a dedicated **`doc.go`** holding the package doc comment: a one-line
statement of what the model is, followed by the note that the `<iteration>.go` files are
bespoke extensions lifted from the downstream repo and staged here for the "should this be
promoted into core?" question (see [Bespoke extensions](#bespoke-extensions)). Keep the
comment in `doc.go` alone — do not also attach a package comment to a source file.

## The artifacts

### 1. Methodology card (`card.md`)

Because the stub is Go rather than declarative YAML, the card carries the primary legible
description of model structure. Do not let it degrade into a title and a link. Fixed
headings, in order:

- **Title** — `# <System> — <one-line mechanism>`, followed by a short blockquote noting
  this is the methodology card and the stub is the runnable demonstration.
- **System** — the real-world system, and a table of the stub's partitions
  (`| Partition | Iteration | State | Role |`).
- **Partition wiring** — *generated, do not hand-write.* A Mermaid dependency graph of the
  partitions, produced by `cmd/model-graphs` from the stub's `BuildStub` wiring and spliced
  in between `<!-- BEGIN/END generated: partition-wiring -->` markers (run
  `go generate ./cmd/model-graphs`). Solid arrows are within-step `params_from_upstream`
  wiring; dashed arrows from a shaded past-copy node are lag reads of a partition's
  committed state (drawn as separate nodes so the graph stays a DAG). `TestCardsUpToDate`
  fails CI if this section is stale.
- **Ingests** — what the model consumes. For the stub this is "nothing" (data-free); name
  what the *downstream* application ingests.
- **Assumptions** — the modelling choices a reader must accept.
- **Validity regime** — where the model is trustworthy, and where it stops being so.
- **Failure modes** — how it misleads when pushed out of regime.
- **Question answered** — the single question the model exists to answer, in italics.
- **Generative behaviour under test** — enumerate what `stub_test.go` asserts (harness,
  invariants, headline direction). Prose only — put no hand-typed numeric results here; the
  numbers live in the generated *Observed behaviour* block below.
- **Observed behaviour** — *generated, do not hand-write.* A table in which every row is one
  bound object: a named response claim, a link to the exact test subtest that enforces it
  (`Test…ExpectedBehaviour/<claim-id>`), and the number that test produced. Spliced between
  `<!-- BEGIN/END generated: observed-behaviour -->` markers by `cmd/model-graphs` from the
  model's `ObservedBehaviour()` (run `go generate ./cmd/model-graphs`). This is the model's
  claim↔test bond made mechanical: a claim cannot appear without a test that enforces it, nor
  carry a number the test did not produce. `TestCardsUpToDate` fails CI if any number drifts.
- **Bespoke extensions** — which iterations sit beside the stub, and the note on what a
  future promotion signal would look like.
- **Downstream** — a link to the project repo owning inference, data, and the decision layer.

### 2. Data-free SDK stub (`stub.go`)

- Expose a `BuildStub(...) *simulator.ConfigGenerator` constructor. Build partitions with
  `simulator.PartitionConfig`, wire them with `ParamsFromUpstream`, and set the run with
  `SimulationConfig` (`EveryStepOutputCondition`, `NumberOfStepsTerminationCondition`,
  `ConstantTimestepFunction`).
- **Wire cross-partition references by name** — `ParamsFromUpstream` for a within-step read,
  `ParamsAsPartitions` for a lag read of another partition's state history. Never pass a
  partition index as a raw numeric param (`"x_partition": {someIndexConst}`): it hides the
  dependency from `pkg/graph`, so the generated wiring diagram loses that edge, and it breaks
  under partition reordering. Also add the new entry to `cmd/model-graphs` so its card
  diagram is generated.
- **Every input is a literal constant** declared as an exported `Default*` const. No file
  I/O, no data loading, no inference, no decision/policy layer — the generative core only.
- Expose the **one scientifically-interesting driver** as a `BuildStub` parameter (the
  knob the CI test sweeps): e.g. prescribing rate (AMR), rainfall multiplier (floodrisk).
- These constants are *illustrative*, not calibrated posteriors. Say so in a comment and
  point to the downstream repo for real calibration.

### 3. Engine-CI test (`stub_test.go`)

The stub is an engine CI artifact; correctness is the point. Follow the house pattern
(`t.Run` subtests) with **three tiers**, weakest to strongest:

1. **`harness`** — pass `simulator.RunWithHarnesses(settings, implementations)` (NaN,
   state-width, params-mutation, history-integrity, statefulness-residue checks).
2. **`invariants`** — structural / physical properties that must hold every step
   (conservation, non-negativity, bounded state, components summing to a total).
3. **Headline direction-of-parameter-response** — the single most important claim: sweep
   the one driver `BuildStub` exposes and assert the output moves the correct way. This is
   the assertion that would catch a sign error in the model's reason to exist. Average over
   an ensemble (vary the seed) when a single realisation is too noisy to be reliable.

Keep runs short (small step counts / ensembles) so this file stays sub-second per entry.

### 4. Expected-behaviour suite (`behaviour_test.go`)

A stub is only *decision-ready* if we can state, and check, how its outputs respond to the
things a user might vary — and only *explainable* if each of those responses is a claim a
domain expert would recognise. The headline test above checks one such response; this file
makes the full set a first-class, enforced artifact. It is the difference between "the
model runs" and "the model behaves as claimed."

Every entry ships a `behaviour_test.go` whose subtests are **named, human-legible response
claims** — the subtest name *is* the behaviour, phrased as a plain sentence
(`higher_discharge_threshold_reduces_cycling`, `higher_evapotranspiration_lowers_flow`), so
the file reads as a behavioural specification of the model. Each claim varies an input,
holds the rest fixed, and asserts the output moves the way the name says (ensemble-averaged
where noisy). Claims come in two kinds, and an entry must cover both where both exist:

- **Decision-path responses (actionable levers).** Sweep the parameters and choices a
  *downstream decision-maker controls* — policy thresholds, asset sizing, an action
  selection — and assert the outcome moves as that decision-maker would expect. These are
  the crucial **(state, action) → outcome** paths: the model's decision-support surface.
  Cover *every* path a downstream decision depends on. Where the action is a discrete
  choice, drive the system into each branch and check that branch's signed effect directly
  (e.g. force the price above the discharge threshold and assert the battery becomes a net
  seller earning positive revenue). A wrong sign here is a wrong recommendation.

- **Structural-driver responses (non-actionable levers).** Sweep the parameters the *world*
  sets and the model does not act on — volatilities, physical efficiencies, rate constants,
  structural means and sensitivities — and assert the physically/economically correct sign.
  These are not decisions, but getting them right is what earns **out-of-sample
  credibility**: a model that responds correctly to a driver it was never tuned against is
  far more trustworthy when applied off-sample. Cover at least one structural driver per
  major mechanism in the model.

Some stubs are **purely structural** — their decision layer lives entirely downstream (e.g.
`floodrisk`, whose interventions are downstream NFM). That is legitimate: such an entry has
no actionable-lever claims, but must then be *comprehensive* on structural drivers and say
so in its card. Do not invent a fake in-stub "action" to satisfy the taxonomy.

Mechanics:

- **Vary params without bloating `BuildStub`.** `BuildStub` still exposes only the one
  headline driver. A behaviour test builds the generator, then reaches in to override any
  partition's params before `GenerateConfigs` — e.g. via a small `runStubOverride(...,
  override func(*simulator.ConfigGenerator))` helper that does
  `gen.GetPartition("<name>").Params.Map["<key>"] = []float64{v}` (and `gen.SetGlobalSeed`
  for ensemble variation when `BuildStub` takes no seed). This keeps the swept surface
  arbitrarily wide while the constructor stays minimal.
- **Ensemble-average noisy claims** (vary the seed) so each assertion is about the
  distribution, not one realisation; keep decision-path branch tests near-deterministic by
  driving the system hard into the branch (low noise, forced signal).
- **Budget.** Keep the whole suite within a few seconds (small step counts, ensembles of
  ~6–12); it runs in engine CI on every change.

The point is not exhaustive parameter coverage — it is that the crucial decision paths and
the credibility-bearing structural drivers each have a checked, named, sign-correct claim.

**Claim↔test binding (the reference pattern — `anglersim`).** So the card's claims cannot
drift from the tests that back them, define the claims *in code* as the single source of both
the assertions and the card numbers, rather than asserting in the test and re-typing numbers
into the card:

- Expose `func ObservedBehaviour() []cardgen.Claim` in a non-`_test.go` file (`behaviour.go`)
  so it is shared by the test and the card generator. Each `cardgen.Claim` carries a stable
  `ID` (the claim's contract, e.g. `climate_warming_reduces_density`), a plain-language
  `Statement`, a `Monotone` direction (+1/−1), and the ordered `Observations` (label + value)
  it produces. Put the ensemble helpers in `behaviour.go` too, not the test file.
- `behaviour_test.go` *consumes* `ObservedBehaviour()`: it runs one subtest per claim, named
  by `ID`, asserting the observations move in the claim's `Monotone` direction. The subtest
  name is therefore the claim id, and the test cannot pass while a claim's stated direction is
  false.
- Register the model's `ObservedBehaviour()` and its `cardgen.Binding` (test function name +
  file) in `cmd/model-graphs`; it renders the *Observed behaviour* block and
  `TestCardsUpToDate` guards it.

The result is one bond — claim → test → observed number — that fails CI in either direction:
break a claim's sign and the binding test fails; change a number without regenerating and the
card guard fails. New entries should adopt this from birth; older entries are being migrated
to it.

### 5. Declarative twin (`declarative.yaml` + `expression_equivalence_test.go`)

The same model, stated as data: partitions and a `general.ExpressionIteration` spec per
partition, loaded through `pkg/api` with no Go and no compilation step. See
[`anglersim/declarative.yaml`](anglersim/declarative.yaml) for the reference entry.

It earns its place twice over. It is the catalogue's answer to "can this engine be driven by
something that does not write Go?" — a question the stub, being Go, cannot answer. And
writing it is what triages the model for promotion (see [Bespoke extensions](#bespoke-extensions)):
a twin that *can* be written says the bespoke Go is a convenience; a twin that *cannot* says
the engine is missing something.

The twin must be the **same model**, not a rewrite that behaves similarly: same partition
names, same param keys, same state layout, same wiring. Two traps:

- **Match the Go's actual read semantics**, not its surface. `params_from_upstream` is a
  *within-step* read; the DSL's `upstreams:` alias is a *lag-1* state-history read. A model
  that resolves an index via `params_as_partitions` and then reads `stateHistories` is doing
  the lag-1 thing, so `upstreams:` is its faithful translation and `params_from_upstream`
  would be a different model. Say which, and why, in a YAML comment.
- **Draws must line up.** A draw's position in the stream is part of the model. Bindings are
  eager, and a scalar `where` is lazy — so a guarded draw belongs *inside* the `where`, which
  is what reproduces a Go early-return that skips the draw.

Where the Go hardcodes an index constant, state it as a mask param instead
(`anglersim`'s `warming_mask`/`nonneg_mask` replace a `tempIndex` constant). Pushing a
constant out into data is a genuine improvement, not a translation artifact.

#### How the twin is verified

**Both layers are required.** They are not alternatives and neither subsumes the other:

1. **Step-for-step** — randomised inputs through single `Iterate` calls on both sides. Catches
   a mis-stated formula, exactly and immediately.
2. **Whole-suite** — re-run `ObservedBehaviour()` against the declarative build. Catches what
   per-step agreement cannot: wrong wiring, wrong param values, wrong state layout, a missing
   partition.

Neither is optional, because each is blind where the other sees. A perturbed coefficient
survives step tests that feed their own randomised params, and only the suite catches it.
Conversely — and this is the one that should settle the argument — in
`antimicrobial-resistance`, **deleting the direct conversion term from the resistant drift, the
model's stated causal heart, passes the entire claim suite**: competitive release reproduces
the same directional response, so the claims cannot tell the two mechanisms apart. The step
test catches it instantly. A claim suite checks *direction*; it does not pin *mechanism*.

To make the suite layer possible, thread a `stubBuilder` through the behaviour helpers so
`ObservedBehaviour()` delegates to an `observedBehaviour(build)`; the claim suite is then
*pointed at* either assembly rather than restated for each. `ObservedBehaviour()` must keep
its no-arg signature — `cmd/model-index` detects it by AST.

Within each layer, use the **strongest oracle the model allows**, and say in the test header
which one you used and why. The two layers step down independently — a model can be exact
per-step and claim-level in the suite:

1. **Exact (~1e-16).** Available when the bespoke iteration draws from the same generator the
   evaluator does (`rng.New(seed)` is `rand.New(rand.NewPCG(seed, seed))`) *and* the
   expressions take the same number of draws per step. Then both models run on the same
   stream and equivalence is decidable directly. Assert a tight tolerance (1e-12), never
   bit-identity: compiled Go contracts `a + b*c` into an FMA, which rounds differently from
   the evaluator's separate operations. That residue is the FMA, not the model.
2. **Claim-level.** When the streams cannot be aligned — a model on `math/rand` v1, or one
   hand-rolling a sampler the engine implements differently — assert that every claim still
   `cardgen.Verify`s, and do *not* assert the numbers match. Know what you have bought: this
   covers direction, not mechanism, which is exactly why the step layer stays mandatory.
3. **Distributional.** Per-step moments with a tolerance justified as a Monte Carlo sampling
   bound. Say in a comment that it is a sampling bound, not a rounding bound.

Where the streams do not align, look for a **regime that recovers exactness** before settling
for moments. `antimicrobial-resistance` compares exactly at `noise_scale = 0`, where both
sides multiply their draw by zero and the streams stop mattering: the drift, the clamps and
the renormalisation are then decidable value by value, and only the noise itself is left to
distributional testing.

Two rules that matter more than the tests passing:

- **Step the oracle down, never the tolerance.** A fat tolerance that no longer tells
  equivalence from similarity is a test that has stopped working while still going green.
- **Never change the model to make the twin agree.** That inverts the point of the exercise,
  and it can silently falsify the card. If a model and the engine disagree, that is a finding
  to record, not a diff to apply.

Step-for-step tests must **exercise every branch and count the cases that hit each**, failing
if any count is zero — an untriggered branch is a comparison that looks stronger than it is.
Choose input ranges that actually reach the clips and guards, including ranges the model would
not ordinarily visit.

**Both layers are load-bearing; neither is redundant.** In `antimicrobial-resistance`, deleting
the direct conversion term from the resistant drift — the model's stated causal heart — passes
the *entire* claim suite, because competitive release reproduces the same directional response.
The step test catches it instantly. Conversely, a perturbed coefficient survives step tests that
feed their own randomised params, and is caught only by the suite. Where a model can only have
the claim-level oracle, know that you are covering direction and not mechanism.

**Mutation-test the twin; do not trust a green run.** Deliberately corrupt the YAML — perturb a
param, drop a term, mis-wire an upstream, unguard a draw — and confirm each is caught. This is
how the reference twin's own defects were found, and two traps recur:

- **A stepsize of 1 hides every `dt`.** At `dt = 1`, `* dt` and `sqrt(dt)` are no-ops, so a twin
  that dropped one agrees anyway. Every model here runs at a constant stepsize of 1, so step
  tests must **vary `dt`** — below, at, and above 1, since `sqrt(dt)` moves the opposite way
  either side.
- **Unknown YAML keys are ignored silently.** `yaml.v2` does not reject them, so a key that
  does nothing looks load-bearing. `state_width` sat in seven twins doing nothing —
  `PartitionConfig` has no such field, and the width comes from `init_state_values`.

Verify the mutation actually landed before believing it passed: a `sed` that silently matched
nothing reports the same green as a test that missed a real bug (BSD `sed` supports neither
`\s` nor GNU's `0,/re/`).

Where a model resists the DSL, the twin's **absence is the artifact**: record what could not
be expressed and why, as a category 2 finding below.

## Bespoke extensions

Custom `simulator.Iteration` implementations a model needs live *beside* its stub, lifted
from the downstream repo. Leave the downstream's data-fitting / calibration / inference
helpers downstream — only the generative iterations travel. The catalogue is the staging
ground for the "should this be promoted into core?" question.

Recurrence is one signal: **an extension that recurs across several entries, doing
substantially the same job, is signalling it wants promoting.** The **domain model index**
([`INDEX.md`](INDEX.md), generated by `cmd/model-index`) is where that becomes visible — it
lists every model's core-package usage and bespoke iterations, so a concept appearing beside
three or four entries is legible at a glance.

But recurrence is a slow signal: it cannot fire until several stubs exist. The declarative
twin (§5) gives a sharper one that fires on the *first* model, because it asks a question
with a decidable answer: **can `general.ExpressionIteration` express this?** That splits
promotion candidates into two categories which are not the same kind of thing and should not
be triaged the same way.

### Category 1 — standardisation candidate (the DSL *can* express it)

The evidence is a declarative twin that exists and whose claims hold. The bespoke Go is then
a convenience or a performance choice, **not a capability**: the engine can already do this,
just not as fast or not as tidily.

Promotion here is **optional, and must be earned by a measurement** — a benchmark showing the
evaluator is too slow for the job, or the same expression block recurring across entries.
"It would be nicer in Go" is not evidence. Absent a measurement, the twin is the answer and
no promotion is needed.

One sub-signal lives here and is easy to misread as an engine gap: a twin that had to fall
back to a weaker oracle (§5) because the bespoke hand-rolls what the engine already ships —
`math/rand` v1 where core has standardised on `pkg/rng`'s v2 PCG, or a hand-written sampler
beside `pkg/rng`'s. That is standardisation debt **in the model**, not a gap in core. It is
still a category 1 finding, and the fix is to the model, on its own merits and its own PR —
never as a side effect of making an equivalence test pass (see §5).

### Category 2 — capability gap (the DSL *cannot* express it)

The evidence is a twin that **cannot be written**, and the reason why. This is the strong
signal: something is genuinely missing from the engine, and one model is enough to prove it.
Two shapes, with very different costs:

- **A missing primitive.** The model needs a function the evaluator lacks, but which fits its
  existing shape — pure, elementwise, no draw-width question. Cheap and safe to promote
  straight into the evaluator. Seasonality (`sin`) and a threshold-exceedance probability
  (`erfc`, the primitive a Gaussian CDF is built from) arrived exactly this way, from
  `bathing-water-forecaster`. Prefer the mathematical primitive over the convenience wrapper:
  `erfc` rather than a `normal_cdf`, because it matches what compiled Go computes term for
  term, which is what keeps a twin exact.
- **A missing structure.** The model's update is not elementwise over the current row, so no
  set of functions rescues it: the shape is what is wrong. These are slow and expensive —
  each means either an operator that enlarges what the DSL *is*, or a core `Iteration`.
  Record them and let a second instance argue for the design. The catalogue has surfaced
  four, and they rhyme — all four are about *structured access* or *lane-wise control of
  draws*, which is exactly the axis a strictly elementwise evaluator gives up:

  | Gap | Found by | What it needs |
  |---|---|---|
  | Index shift — output *i* reads input *i−1*, with an absorbing boundary | `business-survival` (cohort/Leslie flow) | a shift, or a core aged-cohort iteration |
  | Lag-*N* history read — only the current row is exposed | `trywizard` (ageing yellow cards out ten rows back) | a `lag(alias, n)` accessor |
  | Slicing — no way to address a block within a flat vector | `trywizard` (9 coefficients at a stride-9 offset) | a slice/reshape primitive |
  | Draw ordering and lane-wise laziness — an expression takes all its Gammas before any Poisson, and a vector-guarded `where` draws in every lane | `measles-risk-forecaster` (per-area interleaved draws, inactive areas skipped) | per-lane control the evaluator does not have |

  The last one is worth reading twice: the DSL's own doc comment already predicts it ("a
  vector-guarded draw consumes randomness in every lane"). A limitation you documented is
  still a limitation — a model hitting it is evidence, not a duplicate of the note.

**Do not resolve a category 2 finding by changing the model to suit the DSL.** The finding is
the value; a model bent to fit tells you nothing. The promotion decision stays a maintainer
judgement.

## Adding an entry

Use the `/new-model` skill, or by hand: create the folder, write the artifacts (`doc.go`,
`card.md`, `stub.go`, `stub_test.go`, `behaviour_test.go`, and the bespoke `<iteration>.go`
files), add a row to the table in [`README.md`](README.md), and confirm
`go test ./models/<domain-name>/...` passes. Register the model in `cmd/model-graphs` (for
its card's generated blocks) and regenerate the generated artifacts —
`go generate ./cmd/model-graphs` (card wiring + observed-behaviour) and
`go generate ./cmd/model-index` (the domain model index, which auto-discovers the new stub). The `behaviour_test.go` expected-behaviour
suite (§4 above) is mandatory, not optional: an entry without it is incomplete. Flagships
are built deliberately; long-tail models are catalogued opportunistically when next touched
— never by forced retrofit.
