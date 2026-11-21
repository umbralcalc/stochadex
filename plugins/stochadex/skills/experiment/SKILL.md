---
name: experiment
description: Run a loop of simulation experiments that starts from the user's diagram of a system. Read the diagram as a stochadex config, check and run it, test what the user expects of it, write the results back onto the diagram, and propose the next experiment; then repeat as the user changes the diagram or asks what would happen if. Use when the user shares a diagram, sketch or boxes-and-arrows description and wants it simulated, wants to explore or stress a model, try scenarios, compare designs or parameter values, or keep a diagram and a simulation in step. Also use to continue an experiment folder that already holds a diagram.md and model.yaml.
argument-hint: "[path to a diagram or an experiment folder]"
---

# Experiment on a diagram

The user has a picture of how a system works. This loop turns the picture into a simulation
that runs, finds out what it does, and puts that back on the picture, so each round of the
user's thinking starts from evidence. One pass round the loop is one **experiment**.

```text
   diagram ──▶ config ──▶ check ──▶ run ──▶ test claims
      ▲                                         │
      └──── next experiment ◀── updated diagram ┘
```

Two other skills in this plugin hold the detail: `diagram` (the notation, and how boxes and
arrows become partitions and reads) and `config` (writing and running the YAML). Read each
the first time you need it. The engine is the `stochadex` CLI: run `stochadex --version`
before anything else, and if it is missing follow the `config` skill's install section.

The diagram is the user's and the model is yours. The user steers by changing the diagram
or saying what they want to know. You keep the config, the runs and the record, and you never
make the model agree with an expectation by quietly changing it.

## The experiment folder

Keep everything for one system in one folder, which the user can read, diff and commit:

```text
<system>/
  diagram.md        the living diagram: Mermaid, the claims, the assumptions
  model.yaml        the config: the diagram's current state, as stochadex runs it
  claims.json       what the user expects, as tests
  experiments.md    the log: one entry per experiment, newest last
  original.*        the diagram as the user first gave it, never edited
  runs/NNN/         each experiment's config snapshot, logs and summary
```

If the folder already exists, read `diagram.md`, `experiments.md` and `model.yaml`, say in
two lines where things stand, and carry on from step 6.

## 1. Read the diagram

Follow the `diagram` skill. Save what the user gave as `original.*` (for a sketch pasted
into the chat, the transcription is the original). Then write `diagram.md`: the diagram in
the notation, with every node sorted into partition, parameter, input, macro or claim, and
every arrow given its timing.

Show it to the user with three short lists:

- **How I read it:** each box and arrow, in a line, where the reading is not obvious.
- **Assumed:** every rule, value, initial state, time step and run length you chose.
- **Claims:** what the user expects, in words, and how you will test each.

Ask at most a few questions, and only ones that would change the structure. Otherwise go on
with the assumptions marked; a first run that is wrong in a visible way gets a better answer
from the user than a question does.

## 2. Write the config

Write `model.yaml` following the `config` skill, with these fixed choices so that every
experiment can be varied from the command line without editing the file:

```yaml
run: {mode: ensemble, seeds: [1]}
outputs:
- {name: log, function: {type: json_log, path: "${RUN_DIR}/member-{seed}.log"}}
```

Name partitions as the diagram names its boxes. Put each parameter the user might vary in
`params:`, not inside an expression as a number, so `--set` can reach it.

Write each claim into `claims.json` (the format is in step 5). A claim comes from the user,
or from the diagram's own labels. If the user stated none, propose two or three that the
diagram implies (a stock that should not go negative, a balance that should hold, a level
that should settle) and say they are yours.

## 3. Check it

Run all of these, and go no further until each is clean:

```bash
export RUN_DIR=runs/001
stochadex --config model.yaml --check          # exit 0, or 78 with the fix named
stochadex graph -c model.yaml                  # the wiring the run will use
stochadex inspect --suggest -c model.yaml      # built-ins the expressions restate
```

- **`--check` fails:** the message names the partition and the fix. Fix the config. If the
  fix changes what the diagram says (an arrow has to become one step behind to break a
  loop), change the diagram too and tell the user.
- **`graph`:** compare it with `diagram.md` arrow by arrow, as the `diagram` skill's
  "Checking a config against a diagram" describes. Same nodes, same arrows, same timing.
- **`inspect --suggest`:** take each replacement it prints. They are checked to give the
  same numbers, and run faster. `[]` means there are none.

## 4. Run it

```bash
mkdir -p "$RUN_DIR" && cp model.yaml "$RUN_DIR/model.yaml"
stochadex --config model.yaml --set 'run.seeds=[1, 2, 3, 4, 5, 6, 7, 8]'
```

- A model with any randomness runs as an ensemble, eight seeds to begin with. One run of a
  random model is an anecdote. A model with none runs once (`run.seeds=[1]`).
- Vary an experiment with `--set` and record the exact command in the log. Edit
  `model.yaml` only when the model itself changes.
- A sweep of one parameter over several values is one experiment: one run per value, each
  into its own folder (`RUN_DIR=runs/002/gain-0.25`), one log entry with a row per value.
- The run's folder must exist first, and a failed run leaves no log behind. On a failure,
  read the one `stochadex:` line and the exit code (the `config` skill has the table).

## 5. Test it

Summarise, then test the claims:

```bash
python3 "${CLAUDE_SKILL_DIR}/scripts/run_report.py" summarise "$RUN_DIR"/member-*.log
python3 "${CLAUDE_SKILL_DIR}/scripts/run_report.py" check --claims claims.json "$RUN_DIR"/member-*.log
```

`summarise` prints each series' first and final value, range and time-average across the
ensemble, with the spread of the final value across members, and reports any value that is
not finite. Its rows include the initial state, which for a derived partition is a
placeholder, so pass `--from-time` to leave out the start. `check` exits 1 when a claim
fails.
A claim is one object in the list in `claims.json`:

```json
[
  {"claim": "the reservoir never empties",
   "partition": "reservoir", "index": 0, "stat": "min", "op": ">", "value": 0},
  {"claim": "the level settles a little above the target",
   "partition": "reservoir", "stat": "mean", "from_time": 50,
   "op": "between", "value": [40, 55]},
  {"claim": "on average, what is released matches what flows in",
   "partition": "release", "stat": "mean", "from_time": 50,
   "op": "approx", "value": 4.0, "tol": 0.5, "members": "mean"}
]
```

`stat` is `first`, `final`, `min`, `max`, `mean`, `std` or `fraction` of one element of a
partition's state over the window `from_time` to `to_time`. `fraction` is the share of rows
that satisfy `"when": ["<=", 0]`, for a claim about how often: "rarely runs out" is
`"stat": "fraction", "when": ["<=", 0], "op": "<", "value": 0.05`. Give a claim about
settled behaviour a `from_time`, or the initial state and the run-in count against it. `op` is `<`, `<=`, `>`, `>=`, `between` or
`approx` (with `tol`). `members` says how an ensemble must satisfy it: `all` of them (the
default), their `mean`, or at least a fraction of them (`0.9`). For anything the format
can't say (a period of oscillation, a correlation), compute it from the logs yourself and
report the number.

Then test the model itself, whatever the claims say. These catch the errors a diagram can't
show:

- **Not finite, or outside what is physically possible** (a negative population, a
  probability above 1): a fault in the model, to fix before anything else is believed.
- **Step size:** when the model's rates are multiplied by `dt`, run again with half the
  step (`--set simulation.timestep_function.stepsize=...`, into its own `RUN_DIR`). If a
  time-average moves by more than its spread across seeds (`check` prints each claim's
  range across members), the result is an artefact of the step: shorten it until it stops
  moving, and say so. Keep the comparison like for like: a run that ends by
  `number_of_steps` needs twice the steps, a delay counted in steps needs twice the lag, and
  a quantity counted per step halves. When the step is itself part of the system (a daily
  review, a weekly order), it is not a numerical choice: skip the test and say why.
- **Seeds:** if a claim passes for some members and fails for others, report the fraction.
  That is the finding.

Every claim stays in `claims.json` for every later experiment, so a change that breaks an
earlier result shows up. When a claim fails, work out which of three things happened and say
which:

1. **The config doesn't do what the diagram says.** Your error: fix the config, rerun.
2. **An assumption you made is wrong.** Tell the user which one and what you would try.
3. **The system as drawn doesn't behave as expected.** This is a result, and often the most
   useful one. Report it. Do not tune parameters until the claim passes.

## 6. Update the diagram and the log

Update `diagram.md` as the `diagram` skill's "Writing results back onto the diagram"
describes: an observed line on each node, `failing` on nodes a failed claim is about,
`added` on anything new, and under the diagram the current claims with pass or fail and the
assumptions still open. Then append to `experiments.md`:

```markdown
## 003 — release gain halved
**Question:** does a gentler operating rule still hold the level?
**Change:** `--set 'partitions[name=release].params.gain=[0.25]'` (model.yaml unchanged)
**Run:** runs/003, 8 seeds, step 0.5, to t = 100
**Result:** level settles at 56.1 ± 1.3 (was 48.0 ± 0.8); never empties.
**Claims:** 2 of 3 pass. FAIL "settles between 40 and 55": 56.1.
**Reading:** the rule holds the level, 16 above target instead of 8; the claim's band
assumed the old gain. For the user to decide: widen the band or keep the gain.
```

Then tell the user, briefly: show the updated diagram, what held, what failed and why, what
is still assumed, and where the files are. Lead with the finding, not the process.

## 7. Propose the next experiment, and go round again

Offer two or three next experiments, each a question with what you would change to answer
it. Good ones, roughly in the order they pay off:

- **Settle an assumption** the result is sensitive to.
- **Vary a parameter** the user controls, across a range, and say where behaviour changes.
- **Stress it:** a larger shock, a longer run, an extreme initial state.
- **Change the structure:** add the feedback, delay or constraint the result suggests is
  missing, or remove a box that turned out not to matter.
- **Bring in data:** replay a measured series as an input in place of a simulated one, or
  estimate a parameter from data with a macro.

The user answers by picking one, asking something else, or editing the diagram. However it
comes, state the change as a difference to the diagram first (a node, an arrow, a label, a
claim), make that change in `diagram.md`, then make the same change to `model.yaml` or as
`--set` overrides, and go back to step 3 with the next run number. Change one thing per
experiment, so that a difference in the result has one cause.

If the user edits `diagram.md` or hands over a new version of their drawing, compare it with
the last one, list what changed, and treat each difference as the change to make.
