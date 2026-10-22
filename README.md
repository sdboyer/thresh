# thresh

Taps your shoulder when a changeset crosses a boundary that warrants stepping back.
It **notifies, never gates**, and every tap is a **fact about the diff** you can verify
by looking at it — never an opinion.

Rice's theorem says you can't decide anything interesting about what a program *means*.
thresh doesn't argue; it threshes. It separates signal from chaff by properties you can
actually check — a version tuple moved, a guard got bypassed, a reference stopped
resolving — and leaves judging the grain to you. (A threshold is just a boundary with a
name.)

It exists because an agent (small context) and the human supervising it (can't read
the firehose) both have to work narrowly. Thresh is externalized peripheral vision:
it watches the few boundaries in a repo whose crossing is *consequential*, and says so
at the moment you'd otherwise forget to look wider.

A tap fires for one of two reasons, and the label says which:

- **consequence** — the change is far-reaching *even if correct*. A clean, intended
  breaking change to a published contract still fires. No linter serves this cell.
- **wrongness** — the change is probably a mistake (a guard was bypassed).

## The model: a seam is `paths` + `kind`

```cue
// .thresh/thresh.cue  — data only; the schema is in the binary
package thresh

seams: lineages: {
	kind:   "published-language"
	paths:  ["exemplars/*.cue"]
	format: "thema"
}
seams: bindlineage: {
	kind:   "choke-point"
	paths:  ["*.go"]
	type:   "baseLineage"
	onlyIn: ["BindLineage"]
}
```

- **`paths`** are plain globs. Deliberately dumb — precision is never the glob's job.
- **`kind`** is an analyzer from the catalog below. It decides what to look for in the
  files under its paths and whether to fire.

That's everything a user specifies. Seams are keyed by name so files compose by CUE
unification and taps carry a stable name. Reuse a path set the CUE way (`_go: [...]`).

## The catalog

The deciding distinction between the two *consequence* kinds is whether the consumers of
a thing are **enumerable**: if you can't name them you owe **compatibility**; if you can,
you owe **consultation**.

| kind | fires when | fires on |
|---|---|---|
| `published-language` | a versioned contract changes; its format's compat checker says breaking (loud) or compatible (note). `format: "thema"` today; other formats are plugins. | consequence |
| `shared-kernel` | co-owned code changes (meaning-bearing changes only); the tap names who to `consult`. `watch` restricts it to changed lines mentioning given strings. | consequence |
| `choke-point` | a guarded `type` is constructed outside the functions in `onlyIn`, on changed lines. | wrongness |
| `declared-set` | a Go function matching `funcs`, or a CUE definition (at any depth) matching `defs`, is added without being in `declared` (loud), or a declared one disappears (note). Bodies are not its concern. | wrongness |
| `references-resolve` | a Go literal reference into CUE (`cue.Str/Def/Hid`) no longer resolves — checked across *all* Go files, since a CUE rename breaks files you never touched. | wrongness |

Comment-only and whitespace-only edits never fire, for any kind.

## Usage

```sh
go install .                 # from this checkout; puts `thresh` on PATH
thresh init               # scaffold .thresh/thresh.cue (every kind as an example)
thresh check              # uncommitted working tree vs HEAD (includes untracked files)
thresh check main..HEAD   # a commit range
thresh check --json
thresh explain            # render the seams as agent guidance; pipe into AGENTS.md
```

Output:

```
## Thresh
⚑ STEP BACK [published-language: lineages] exemplars/narrowing.cue
    breaking change: max version [1,0] → [2,0] (major bump). Consumers you don't control depend on this contract.
· [shared-kernel: lineage-kernel] lineage.cue
    co-owned kernel changed: #Lineage. Consult: every lineage author; the Go facade.
```

`check` exits 0 whether or not it found anything — signals are not failures. It exits
non-zero only when it *couldn't run* (invalid annotations, not a git repo).

## Connecting it to a repo and to Claude Code

1. **Annotate:** `thresh init`, keep the seams that apply. This is the real work and
   the only per-repo work. A repo with no `.thresh/` is simply ignored.
2. **Agent guidance:** `thresh explain > AGENTS.md` (or into a section of it). It is
   generated from the annotations, so it cannot drift from them.
3. **Automatic taps at end of turn:** install `hooks/run-thresh.sh` as a Stop hook —
   globally, once, in `~/.claude/settings.json` (instructions in the script). It is silent
   and free in any directory without `.thresh/`, never blocks, and surfaces a loud
   warning (never silence) if annotations are invalid.

Worktrees, cmux workspaces, other agents: nothing special. It's a CLI over the working
tree of whatever git repo you're in.

## Runtime requirements

`git`; `cue` (only to export annotations — the per-kind `#Seam` schema is embedded in the
binary and unified with your data at load, so bad kinds and typo'd fields are rejected
with a precise error); Go stdlib (`go/parser` for Go analysis; the target need not build).

## Layout

```
main.go           CLI: check / explain / init; loads + validates annotations
kinds.go          the uniform contract every kind satisfies, and the changeset (from git)
extract_text.go   format-agnostic extractors: touched(), changed lines, watched substrings
extract_go.go     Go extractors (go/parser): funcs, constructions, literal CUE references
extract_cue.go    CUE extractors: definitions + spans, labels, version tuples (thema)
kind_*.go         one file per kind; each is extract → trivial decision
explain.go        the consultative surface
init.go           the scaffold template
schema/seam.cue   the annotation contract (embedded); one closed definition per kind
hooks/            the Stop hook script
thresh_test.go    the scenarios: every kind, loud and silent, on a fresh fixture repo each
testdata/fixture/ a small synthetic repo the scenarios mutate (needs only git; cue for one test)
```

Verify with `go test ./...`.

See `AGENTS.md` for the design constraints that govern changes to this tool.
