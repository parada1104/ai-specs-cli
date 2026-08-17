# Design: provenance-governed runtime brief

## D1 — Reuse `classify_managed_override`, do not invent a second classifier

`lib/_internal/util.py:552` already computes exactly the states this needs,
from the same three inputs: disk bytes, the lock entry, and the would-write
bytes. Recipe overrides and gate hooks are governed by it today.

A parallel classifier for one file would be a second definition of "who owns
this", which is the failure mode this change exists to remove.

## D2 — The migration problem, and why it decides the whole design

No project has a lock entry for `AGENTS.md` today. Applied naively, **every
existing project classifies `untracked` and its brief stops updating** — a
regression affecting everyone, in the name of protecting a minority.

The tempting fix is to adopt the file on first sight and record the current
disk bytes as the baseline. **That is unsafe.** If the user had edited a
generated brief, adopting their bytes as our baseline makes the next sync see
`managed_stale` and overwrite the edit. Adoption would cause the exact data
loss this change prevents, one sync later.

### There is no reliable retroactive signature

The renderer emits `# <name> Runtime Brief` and a blockquote intro, then
`## Project`. Recognizable, but not probative — a hand-written `AGENTS.md` can
contain the same headings. **We cannot prove after the fact that a file is
ours.** Any migration rule based on shape is a guess, and a wrong guess here
destroys irreplaceable content.

### Chosen rule

On a target with no lock entry:

| Condition | Action |
|---|---|
| disk bytes **exactly equal** would-write bytes | provably current and ours → record the baseline, proceed silently |
| anything else | **preserve**, print the detected state and the remedy |

Exact match is the only condition under which ownership is proven rather than
inferred, so it is the only one that adopts silently.

### The cost, stated plainly

A project whose brief is stale relative to its manifest — config changed since
the last sync — gets a one-time preserve and a message instead of an automatic
update. That cohort is not small.

That cost is accepted deliberately: a visible, reversible, one-command
interruption is a better failure than silently overwriting a file we cannot
prove we wrote. The message names both exits, so the interruption ends in one
step.

**Rejected: adopt-on-first-sight.** Cheaper migration, but converts a
user-edited brief into a baseline and overwrites it on the following sync.
Trading a certain silent data loss for a smoother upgrade is the wrong trade
for this surface.

## D3 — Policy is `never-force`, not `auto`

Gate hooks use `auto`: a matching baseline may be force-updated. Gates are
reproducible — a lost customization can be re-applied.

A runtime brief can hold irreplaceable hand-written context. `user_modified`
therefore never updates automatically, with no exception, and no
`--refresh`-style flag in this change. The user's exits are explicit and
documented, not a force flag whose blast radius is a file they cannot
reconstruct.

## D4 — One decision function, three callers

`init.sh`, `sync.sh` and `sync-agent.sh` each call `agents-render.py`. The
decision moves **inside** the renderer, so the guard cannot depend on which
flags a caller happened to pass — the defect that let `sync-agent.sh:372` drift
from the other two.

`--preserve-if-runtime-brief` stays accepted and still preserves on a marker,
so existing callers and existing projects keep working. It becomes one input to
the decision rather than the whole of it.

## D5 — The marker keeps working, and keeps meaning "mine, always"

An existing `<!-- ai-specs:runtime-brief -->` still preserves the file
unconditionally. It stops being the *only* protection and becomes the explicit,
permanent opt-out — useful precisely because provenance answers "have we
written this", while the marker answers "the user has claimed this".

## D6 — A refusal must be actionable

A skipped write prints the detected state and both exits:

```
  ℹ AGENTS.md left unchanged (not written by ai-specs)
    to let ai-specs manage it:  ai-specs sync --adopt-brief
    to keep it yours forever:   add <!-- ai-specs:runtime-brief --> at the top
```

A silent no-op is as bad as a silent overwrite: in both cases the user does not
learn what happened.

`--adopt-brief` is a deliberate, one-time, user-initiated declaration. It is
safe *because* the user issues it, which is the consent that automatic adoption
lacks.

## D7 — `doctor` reports the state

`doctor` already reports override ownership. It gains the brief's
classification, so a project sitting in `untracked` after upgrade is
discoverable without waiting for someone to read sync output.

## D8 — Failure mode

Any error while classifying — unreadable lock, unreadable file — resolves to
**preserve**. When ownership cannot be determined, not writing is the only safe
answer.
