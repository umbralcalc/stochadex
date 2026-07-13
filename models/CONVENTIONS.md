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
    <iteration>.go     # bespoke simulator.Iteration implementations, beside the stub
```

The Go **package name** is a short identifier for the domain (`amr`, `floodrisk`) — it
need not equal the directory name, but must be a valid Go identifier (no hyphens).

Every entry carries a dedicated **`doc.go`** holding the package doc comment: a one-line
statement of what the model is, followed by the note that the `<iteration>.go` files are
bespoke extensions lifted from the downstream repo and staged here for the "should this be
promoted into core?" question (see [Bespoke extensions](#bespoke-extensions)). Keep the
comment in `doc.go` alone — do not also attach a package comment to a source file.

## The three artifacts

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

## Bespoke extensions

Custom `simulator.Iteration` implementations a model needs live *beside* its stub, lifted
from the downstream repo. Leave the downstream's data-fitting / calibration / inference
helpers downstream — only the generative iterations travel. The catalogue is the staging
ground for the "should this be promoted into core?" question: **an extension that recurs
across several entries, doing substantially the same job, is signalling it wants
promoting.** Do not design the promotion mechanism up front — let it emerge from the
recurrence once several stubs exist.

## Adding an entry

Use the `/new-model` skill, or by hand: create the folder, write the artifacts (`doc.go`,
`card.md`, `stub.go`, `stub_test.go`, `behaviour_test.go`, and the bespoke `<iteration>.go`
files), add a row to the table in [`README.md`](README.md), and confirm
`go test ./models/<domain-name>/...` passes. The `behaviour_test.go` expected-behaviour
suite (§4 above) is mandatory, not optional: an entry without it is incomplete. Flagships
are built deliberately; long-tail models are catalogued opportunistically when next touched
— never by forced retrofit.
