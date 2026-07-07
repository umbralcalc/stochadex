# stochadex: domain-models catalogue — implementation brief

**For:** an agent working on the stochadex repo (and its downstream project repos) locally.
**Author's intent:** this is a working spec, not a suggestion list. The rationale for each
choice is included deliberately — do not silently reverse a decision because a simpler path
looks available; the simpler paths were considered and rejected for the reasons given.

---

## Background & goal

The stochadex is a general stochastic simulation engine (the *forward / generative* model).
Downstream project repos apply it to specific real-world domains — they own data ingestion,
simulation-based inference, calibration, and the decision/policy layer.

We are replacing the old `template` folder (a cookie-cutter scaffold) with a **domain-models
catalogue**. The template was deleted because cookie-cutter scaffolding pushes *frozen
structure downstream* — each application diverges from a snapshot and upstream learning never
flows back. The catalogue inverts this: applications teach the core what good domain models
look like, and recurring needs surface for promotion into the core.

### The organising principle (do not violate)

**The repo boundary follows the generative/inferential split.**
- The engine repo owns the **forward model** — the thing that *simulates*.
- Downstream repos own **inference, data ingestion, calibration, and the decision layer** —
  everything that makes a specific application *work* against real stakes.

Every design decision below traces back to this line.

### What each domain gets (three artifacts)

1. **Methodology card** — the primary human- and agent-legible description. Structural and
   methodological content: what real-world system it represents, what it ingests, its
   assumptions, its validity regime, its failure modes, and the question it answers.
2. **SDK-based, data-free simulation stub** — the executable, type-checked demonstration of
   the generative core, wired into the engine's own CI.
3. **Downstream pointer** — a link to the project repo where inference, data, and the
   decision layer live.

### Why SDK stubs, not YAML config

Both simulation-declaration paths exist in stochadex: (1) API + YAML config, and (2) the SDK.
The YAML path *can* declare runtime dependencies on extended functionality implemented against
the core SDK interfaces, so it is not less expressive. **However, the stub is an engine CI
artifact, and correctness matters more than declarative readability there.** The YAML path has
occasional type-checking gaps; SDK-declared stubs get compile-time checking for free and the
SDK configuration surface is very clean. For a test surface, that is decisive.

**Consequence — weight moves from stub to card.** Because the stub is now Go rather than a
declarative YAML file, it no longer doubles as the non-executable structural spec. The
**methodology card therefore carries the primary legible description of model structure.** Keep
the card genuinely informative; do not let it degrade into a title and a link.

### Bespoke extensions

Some models need custom functionality written against the core SDK interfaces that does not
(yet) belong in the engine core. That code lives **beside the model's stub** in the catalogue.
This is deliberate: the catalogue becomes the staging ground for the "should this be in core?"
question. **An extension that recurs across several models' stubs, doing substantially the same
job, is signalling it wants promoting into the core.** Recurrence is the promotion signal. Do
not design the promotion mechanism up front — let it emerge once several stubs exist.

---

## Phase 0 — Ground-truth the design against the real code (BLOCKING, do first)

This phase can invalidate later phases. Do not skip it or run it in parallel with building.

1. **Verify nothing imports `template` or `scripts`.** The author believes this is true —
   confirm it with an actual search across the engine repo and, if feasible, the downstream
   repos. The only thing in `scripts` should be a git hook that updated the template; if so it
   is dead once the template goes.
2. **Confirm the SDK simulation-declaration surface** is stable enough to build stubs against,
   and note the canonical way a simulation is constructed via the SDK (this becomes the stub
   convention).
3. **The real risk — separability.** For two or three downstream projects, check that the
   **data-free generative core is cleanly separable** from the inference / data-ingestion /
   decision layers as the code is *actually written* (not as it ideally would be). If a
   project's forward model is tangled with its data ingestion, extracting a clean stub is more
   work than expected — you want to discover that here, on one model, not on the eighth.

**Deliverable of Phase 0:** a short written finding on (a) import-safety of the deletion,
(b) the SDK stub construction pattern to standardise on, and (c) how clean the
generative/inference separation is per project inspected. **If separation is messy, stop and
report back before proceeding** — the stub design may need revisiting.

---

## Phase 1 — Delete the dead scaffolding

- Remove `template` and `scripts` in a single commit.
- Commit message must record *why*: cookie-cutter scaffolding pushes frozen structure
  downstream and blocks upstream learning; being replaced by an upstream-learning catalogue.
- **No replacement in this commit.** Keep the deletion legible in history; do not muddle it
  with the new folder's introduction.

---

## Phase 2 — Define the format by building ONE model end-to-end

Do not write an abstract template first. The format is a *residue* of having built one.

1. **Pick the single strongest flagship model** — the one doing the most credibility work,
   ideally one whose downstream separation Phase 0 confirmed is clean.
2. Build all three artifacts for it:
   - **Methodology card**: what system, ingests what, assumptions, validity regime, failure
     modes, question answered.
   - **SDK-based, data-free simulation stub**: the generative core only, no data, no inference.
   - **Downstream pointer**: link to the project repo.
3. **Wire the stub into the engine CI** with at least one *meaningful* assertion about
   generative behaviour — structure, a conservation property, or correct direction of
   parameter response. Not merely "it runs." This is what makes the catalogue a real test
   surface, not decoration.
4. If the model needs a bespoke extension, place it beside the stub and note it.

**Goal of this phase:** surface the real problems of card content and stub convention once, by
building, before committing to a template.

---

## Phase 3 — Freeze the conventions

Only after Phase 2 works end-to-end, write the thin spec:

- **Card template** — the fixed set of headings (system / ingests / assumptions / validity
  regime / failure modes / question answered, adjusted by whatever Phase 2 revealed).
- **Stub convention** — where stubs live, naming, what the CI test must assert, and where
  bespoke extensions sit relative to the stub.
- **Folder layout** for the domain-models catalogue.

Keep it thin. It documents what was built, it is not a speculative standard.

---

## Phase 4 — Roll out across the portfolio, prioritised

- Do the remaining **flagship models** next (the two or three carrying real credibility weight).
- The **long tail** is done opportunistically — when a model is next touched for other reasons.
- **Full retrofit of all models is explicitly NOT a blocking goal.** New domain entries adopt
  the format from birth; flagships are done deliberately; everything else upgrades when touched.
  This is the rule that keeps the effort from dying in unrewarding retrofit work — respect it.

---

## Phase 5 — Link the blog, both directions

- Each **card points to its narrative derivation** (the corresponding blog post).
- Each **blog post points to its card + runnable stub**.
- Adopt the forward-going discipline: **a new blog post's structured residue *is* its model
  card**, authored together so the two cannot drift out of sync. The post is the narrative
  derivation; the card is its structured residue; the downstream repo owns making it true.

---

## Deliberately parked (do not build up front)

- **Agent-discovery / skill-metadata format** — the machine-readable "when and why to reach
  for this model" layer for agent discovery. Belongs around Phase 4 or later. **Re-check the
  current conventions for agent tool/skill discovery when you reach it** — this area moves
  fast and should not be built to stale assumptions.
- **Promotion mechanism** for recurring bespoke extensions into core — let it emerge from the
  recurrence signal once several stubs exist; do not design it in advance.

---

## Critical-path note

Phase 0 is genuinely blocking: if the generative/inference separation is messy in the real
downstream code, the SDK-stub design needs revisiting before rollout. That is exactly why
Phase 2 builds *one* model rather than all of them — the format is proven against reality on a
single case before being multiplied.