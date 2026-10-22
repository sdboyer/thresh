---
name: thresh-sweep
description: Resolve thresh taps nobody has given a verdict on. Reads each pending tap's transcript and the git history that followed it, decides whether the tap was useful by thresh's rubric, and records `thresh verdict --by sweep` with evidence. Use when asked to sweep, review, or resolve pending thresh taps.
---

# Sweeping pending thresh taps

thresh logs every tap it emits. The agent that was tapped is meant to record whether
the tap helped; this sweep resolves the ones it didn't. You are recording judgments
about taps, which feed `thresh stats`. Never edit code or `.thresh/` annotations
during a sweep.

## Install

Copy this directory to `~/.claude/skills/thresh-sweep/`.

## Procedure

1. `thresh pending --json` (add `--repo DIR` to limit it to one repo). Each entry has
   the tap's `id`, `kind`, `seam`, `file`, `level`, latest `message`, `base` commit,
   `repo` (`root`, `remote`, `toplevel`), `firstSeen`/`lastSeen`, and the `sessions`
   (with transcript paths) that saw it.
2. **Skip taps still in flight.** If `lastSeen` is within the last hour, or the
   working tree at `toplevel` still produces the tap (`thresh check --repo <toplevel>`
   prints `#<id>`), the work isn't settled. Leave it for a later sweep.
3. For each remaining tap, gather evidence:
   - **The transcript(s)** in `sessions`: find where the tap was delivered (search for
     `#<id>`) and read what the agent did afterwards. Did it change course, consult
     someone, revisit a version or compat decision, or explicitly confirm the crossing
     was intended?
   - **What landed**: in `toplevel`, look at commits after `base` that touch `file`
     (`git log --oneline <base>..HEAD -- <file>`, and the diffs). Did the change that
     produced the tap land as-is, change, or get dropped?
   - **Whether the tap was true**: compare `message` with the diff. If it misstated
     the diff (wrong version, a construction that isn't one, a reference that does
     resolve), that is an extractor bug.
4. Apply the rubric and record exactly one verdict per tap:

   ```sh
   thresh verdict <id> --by sweep --useful yes|no|unclear --evidence "..."
   thresh verdict <id> --by sweep --inaccurate --evidence "..."
   ```

   - **yes**: the tap caused or confirmed something observable: the diff changed, a
     co-owner was consulted, a version or compatibility decision was revisited, or the
     agent explicitly checked that the crossing was intended.
   - **no**: the tap was true, but nothing followed and nothing needed to.
   - **unclear**: it can't be determined: the transcript is gone, the work was
     abandoned, or the evidence points both ways. Prefer `unclear` over guessing.
   - **--inaccurate**: the tap misstated the diff. Say exactly what was wrong; this is
     a bug report against thresh.

   Evidence must be checkable: cite the transcript moment ("after the tap, the agent
   reverted the v2 bump") or the commit (`abc1234 drops sneaky.go`), not a sentiment.
5. Finish with `thresh stats` and report, briefly: how many taps you resolved and how,
   how many you skipped as in flight, and every `--inaccurate` with its evidence.
