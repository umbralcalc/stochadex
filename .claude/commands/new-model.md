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
   the generative iterations travel. Add a package doc comment noting these are bespoke
   extensions staged for the "promote into core?" question.

4. **Stub** (`stub.go`) — write `BuildStub(...) *simulator.ConfigGenerator`:
   - Every input a literal exported `Default*` constant; no file I/O, no data, no inference.
   - Expose the one scientifically-interesting driver as a `BuildStub` parameter (the knob
     the test will sweep).
   - Note in a comment that the constants are illustrative, not calibrated posteriors, and
     point to the downstream repo.

5. **Test** (`stub_test.go`) — `t.Run` subtests in three tiers:
   - `harness`: `simulator.RunWithHarnesses(settings, implementations)` returns no error.
   - `invariants`: structural / physical properties that hold every step.
   - direction-of-parameter-response: sweep the driver, assert the output moves the correct
     way (average over a seed ensemble if a single run is too noisy). This is the assertion
     that would catch a sign error — make it meaningful, not "it runs."
   Keep runs sub-second (small step counts / ensembles).

6. **Write the methodology card** (`card.md`) with the fixed headings from
   `models/CONVENTIONS.md`: System (+ partition table) / Ingests / Assumptions / Validity
   regime / Failure modes / Question answered / Generative behaviour under test / Bespoke
   extensions / Downstream. The card carries the structural spec the Go code does not —
   keep it genuinely informative.

7. **Register** the entry: add a row to the table in `models/README.md`.

8. **Verify:** run `go build ./...`, `gofmt -l models/<domain-name>/` (expect empty), and
   `go test -count=1 ./models/<domain-name>/...`. Diagnose and fix any failures.

## Reference pattern

`models/antimicrobial-resistance/` (coupled compartments) and `models/floodrisk/`
(a rainfall → runoff cascade) — two different model shapes, same convention.
