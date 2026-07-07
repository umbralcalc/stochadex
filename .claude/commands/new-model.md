Add a new domain model to the catalogue in `models/`.

If $ARGUMENTS is provided, treat it as the domain name and/or a description of the
real-world system to model (and, if given, the downstream project repo). Otherwise, ask
the user: what real-world system does this model represent, and where does its downstream
repo live?

Read `models/CONVENTIONS.md` first — it is the spec this skill implements. Use the
existing entries `models/antimicrobial-resistance/` and `models/floodrisk/` as the
reference pattern to copy.

## Steps

1. **Locate the generative core.** If there is a downstream repo, find its data-free
   forward model — the partitions that *simulate*, not the data ingestion, calibration,
   inference, or decision layers. Confirm it separates cleanly; if the forward model is
   tangled with data loading, surface that before proceeding.

2. **Create the folder** `models/<domain-name>/` (kebab-case, matching the downstream repo
   name where one exists). Choose a short Go package identifier (no hyphens).

3. **Bespoke iterations** (`<iteration>.go`) — lift the custom `simulator.Iteration`
   implementations the model needs from the downstream repo, into this folder, verbatim.
   Leave the downstream's data-fitting / calibration / inference helpers downstream — only
   the generative iterations travel. Put the package doc comment in a dedicated **`doc.go`**
   (one line on what the model is, plus the note that these iterations are bespoke extensions
   staged for the "promote into core?" question) — keep it there alone, not also on a source
   file.

4. **Stub** (`stub.go`) — write `BuildStub(...) *simulator.ConfigGenerator`:
   - Every input a literal exported `Default*` constant; no file I/O, no data, no inference.
   - Expose the one scientifically-interesting driver as a `BuildStub` parameter (the knob
     the test will sweep).
   - Note in a comment that the constants are illustrative, not calibrated posteriors, and
     point to the downstream repo.

5. **Test** (`stub_test.go`) — `t.Run` subtests in three tiers:
   - `harness`: `simulator.RunWithHarnesses(settings, implementations)` returns no error.
   - `invariants`: structural / physical properties that hold every step.
   - headline direction-of-parameter-response: sweep the one driver `BuildStub` exposes,
     assert the output moves the correct way (average over a seed ensemble if a single run
     is too noisy). This is the assertion that would catch a sign error — make it
     meaningful, not "it runs."
   Keep runs sub-second (small step counts / ensembles).

6. **Expected-behaviour suite** (`behaviour_test.go`) — MANDATORY (CONVENTIONS §4). A set
   of `t.Run` subtests whose *names are plain-language response claims* (e.g.
   `higher_discharge_threshold_reduces_cycling`), each varying one input and asserting the
   output moves as the name says. Cover both:
   - **Decision-path responses (actionable levers):** sweep the params/choices a downstream
     decision-maker controls (policy thresholds, sizing, action selection) — every crucial
     `(state, action) → outcome` path. For discrete actions, drive the system into each
     branch and check that branch's signed outcome directly.
   - **Structural-driver responses (non-actionable levers):** sweep params the world sets
     (volatilities, efficiencies, rate constants) and assert the physically/economically
     correct sign — this earns out-of-sample credibility.
   Vary params by reaching into the generator (`gen.GetPartition(name).Params.Map[key] =
   ...`) rather than bloating `BuildStub`; ensemble-average noisy claims; keep the suite to
   a few seconds. Purely-structural stubs (decisions all downstream, e.g. `floodrisk`) have
   no actionable claims but must be comprehensive on structural drivers and say so.

7. **Write the methodology card** (`card.md`) with the fixed headings from
   `models/CONVENTIONS.md`: System (+ partition table) / Ingests / Assumptions / Validity
   regime / Failure modes / Question answered / Generative behaviour under test / Bespoke
   extensions / Downstream. The card carries the structural spec the Go code does not —
   keep it genuinely informative.

8. **Register** the entry: add a row to the table in `models/README.md`.

9. **Verify:** run `go build ./...`, `gofmt -l models/<domain-name>/` (expect empty), and
   `go test -count=1 ./models/<domain-name>/...`. Diagnose and fix any failures — and treat
   a surprising behaviour-suite result as a finding to understand (it may be a real bug in
   the stub or a wrong assumption in the test), not just a threshold to tune away.

## Reference pattern

`models/antimicrobial-resistance/` (coupled compartments) and `models/floodrisk/`
(a rainfall → runoff cascade) — two different model shapes, same convention.
