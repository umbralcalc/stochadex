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
    card.md            # methodology card — the primary legible spec
    stub.go            # data-free SDK generative core (BuildStub)
    stub_test.go       # engine-CI test: harness + invariants + direction-of-response
    <iteration>.go     # bespoke simulator.Iteration implementations, beside the stub
```

The Go **package name** is a short identifier for the domain (`amr`, `floodrisk`) — it
need not equal the directory name, but must be a valid Go identifier (no hyphens).

## The three artifacts

### 1. Methodology card (`card.md`)

Because the stub is Go rather than declarative YAML, the card carries the primary legible
description of model structure. Do not let it degrade into a title and a link. Fixed
headings, in order:

- **Title** — `# <System> — <one-line mechanism>`, followed by a short blockquote noting
  this is the methodology card and the stub is the runnable demonstration.
- **System** — the real-world system, and a table of the stub's partitions
  (`| Partition | Iteration | State | Role |`).
- **Ingests** — what the model consumes. For the stub this is "nothing" (data-free); name
  what the *downstream* application ingests.
- **Assumptions** — the modelling choices a reader must accept.
- **Validity regime** — where the model is trustworthy, and where it stops being so.
- **Failure modes** — how it misleads when pushed out of regime.
- **Question answered** — the single question the model exists to answer, in italics.
- **Generative behaviour under test** — enumerate what `stub_test.go` asserts.
- **Bespoke extensions** — which iterations sit beside the stub, and the note on what a
  future promotion signal would look like.
- **Downstream** — a link to the project repo owning inference, data, and the decision layer.

### 2. Data-free SDK stub (`stub.go`)

- Expose a `BuildStub(...) *simulator.ConfigGenerator` constructor. Build partitions with
  `simulator.PartitionConfig`, wire them with `ParamsFromUpstream`, and set the run with
  `SimulationConfig` (`EveryStepOutputCondition`, `NumberOfStepsTerminationCondition`,
  `ConstantTimestepFunction`).
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
3. **Direction-of-parameter-response** — the headline claim: sweep the driver and assert
   the output moves the correct way. This is what makes the catalogue a real test surface
   rather than decoration — it is the assertion that would catch a sign error. Average
   over an ensemble (vary the seed) when a single realisation is too noisy to be reliable.

Keep runs short (small step counts / ensembles) so the suite stays sub-second per entry.

## Bespoke extensions

Custom `simulator.Iteration` implementations a model needs live *beside* its stub, lifted
from the downstream repo. Leave the downstream's data-fitting / calibration / inference
helpers downstream — only the generative iterations travel. The catalogue is the staging
ground for the "should this be promoted into core?" question: **an extension that recurs
across several entries, doing substantially the same job, is signalling it wants
promoting.** Do not design the promotion mechanism up front — let it emerge from the
recurrence once several stubs exist.

## Adding an entry

Use the `/new-model` skill, or by hand: create the folder, write the three artifacts,
add a row to the table in [`README.md`](README.md), and confirm
`go test ./models/<domain-name>/...` passes. Flagships are built deliberately; long-tail
models are catalogued opportunistically when next touched — never by forced retrofit.
