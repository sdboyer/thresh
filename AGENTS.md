# Working on thresh

Read `README.md` for what the tool is and the model. This file is the set of
constraints that make the tool worth trusting. They were arrived at deliberately, several
of them by discarding plausible alternatives; **do not relax them to make a feature fit.**
If a change needs one of these relaxed, stop and raise it instead.

## Non-negotiables

1. **Notify, never gate.** `check` exits 0 whether or not it found signals. The hook never
   blocks. Signals are not failures. Nothing here is CI.

2. **Every signal is a verifiable fact about the diff.** A version number, a line, a
   function name, a file that did or didn't change. If a reader could not confirm a tap by
   looking at the diff, it is not a signal. **No kind may fire on a judgment**, including a
   model's judgment. One judgment-kind in the catalog forces the reader to second-guess every
   tap, which destroys the tool's entire value. "Does this Go code *mean* what the CUE says"
   is not a kind and never will be (Rice's theorem — a non-trivial property of a general
   program). That concern lives elsewhere: push the rule into a decidable layer (CUE), use
   a mechanical proxy that *is* a kind (`references-resolve`, `declared-set`), or route it
   to a separate judgment channel whose output is never emitted as a thresh signal.

3. **The admission bar for a new kind** — it must pass all three, and be stateable in one
   sentence of the form *"over files matching P, it extracts X and fires when Y"*:
   - a **mechanical fact** about the changeset (decidable; no inference of severity or intent);
   - about **consequence or wrongness**, and labeled as one of the two;
   - **not already owned** by the target's own verification (thema's compat check is
     correctness, delegated; thresh reads its result, never recomputes it).

4. **`paths` are globs. Period.** Precision is an extractor's job. Never add selection
   smarts to the config (no "set of lineages" as a path; that is a projection, done in code).
   Generous globs plus a discriminating extractor is the intended pattern.

5. **Extractors are the library; kinds are glue.** All variability lives in
   `extract_<format>.go`. A kind is extract → trivial decision; if a kind is growing logic,
   the logic belongs in an extractor. Shared precision is fixed in one place: `touched()`
   (comment/whitespace suppression) is the canonical example — fix it there, every kind
   benefits. A new *language* means new extractors; that is the real cost. A new kind over
   existing extractors should be small.

6. **Naming.** Use a DDD term only when the check *is* the concept (`published-language`,
   `shared-kernel`). When the check is narrower, name the check (`choke-point`, not
   "anti-corruption-layer"). The user-facing vocabulary is exactly: seam, paths, kind, the
   kind names, tap, and verdict. Internal words (extent, projection, predicate, extractor, node/edge) stay
   internal.

7. **The schema is the single source.** `schema/seam.cue` is embedded and unified with
   each repo's data at load. One **closed** definition per kind, so misplaced fields and
   typos are rejected. Repos never import it and never restate it. Adding a field to a kind
   means: schema definition, `seam` struct in `kinds.go`, the kind, `explain.go`, `init.go`.

8. **Enumerable vs. non-enumerable consumers** is the distinction between the two
   consequence kinds. `published-language` requires a declared or checkable compatibility
   signal (a `format` with a compat extractor); without one, it is a `shared-kernel`.
   Severity is **read, never inferred**: `shared-kernel` is flat on purpose.

9. **Verdicts are a separate judgment channel.** The tap log (`taplog.go`, `review.go`)
   records whether a tap helped. Those verdicts are judgments, so they never suppress,
   reweight, reorder or otherwise affect a signal. `check`'s output and exit code never
   depend on the log: a log that can't be written or read is a warning, and every tap
   still prints.

## Dropped on purpose — do not reintroduce

- **File-level co-change (`coupled`)**: hopelessly noisy at file granularity; no scope
  reducer exists. `references-resolve` is the element-level replacement.
- **Inferred severity** for kernel/definition changes.
- **Judgment or LLM-backed kinds** (see 2).
- **Config-side selection smarts** (see 4).

## Verifying a change

```sh
go test ./...
```

`thresh_test.go` runs each scenario on a fresh git repo built from `testdata/fixture/` (a
small synthetic project: a versioned contract, a CUE kernel, a Go binding with a choke
point and a declared check set, `go.mod`/`cue.mod`). It asserts both the loud cases and the
silence cases. The silence cases are not optional — a kind you can't picture staying quiet
is one nobody would leave switched on. **Any new kind must add at least one loud scenario
and one silence scenario.** `fixtureSeams` in the test and `testdata/fixture/.thresh/thresh.cue`
must declare the same seams; `TestAnnotationsValidate` enforces it. Scenario edits use
`replace()`, which fails loudly on fixture drift rather than passing vacuously.

The suite is self-contained on purpose: it must never depend on an external checkout.
Real-world pilots (thema was the first) are for discovering kinds and bugs, not for the
test loop.

## Known follow-ups (don't let these block, don't skip past them silently)

- Extractor return types are loose; tidying them is wanted before the library grows.
- `published-language` compat plugins beyond thema (`buf breaking`, `oasdiff`, …).
- `references-resolve`: a softer note when a *referenced* CUE element changes and the Go
  side didn't.
- Globs are `filepath.Match` (no `**`).
- In-process CUE (cuelang Go API) only if schema work needs it; shelling out is deliberate.
- Dogfood: a `.thresh/` for this repo.
- TypeScript extractors. The ynab-amazon pilot (first TS repo) has two real choke points
  thresh can't express because `choke-point` reads Go only: all YNAB writes go through one
  function that holds a dry-run gate, and credentials are read in one function. That repo
  enforces both with ESLint for now; TS construction/call-site extractors would let
  `choke-point` own them.
- The tap log is read in full on every `check --new` (each edit, via the delivery hook).
  Fine at current volumes; compaction or an index when it isn't.
- A worktree-mode run and a range-mode run over the same change resolve different bases,
  so they log the same crossing under different tap IDs.
- `thresh check` fails with "git diff failed: exit status 128" in a repo with no commits
  (unborn HEAD). It should diff against the empty tree, or say plainly that there's nothing
  to compare against yet.
