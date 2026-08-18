# Proposal: provenance-governed runtime brief

## Tracker

- card_id: `#81`
- url: https://trello.com/c/tR60h8lX/81-runtime-brief-ownership-bring-override-ownership-governance-to-agentsmd

## Depth

**Full.** A data-loss surface, and a contract against existing override-ownership
governance rather than a bounded edit.

## Why

`AGENTS.md` is the only high-value generated surface still protected by a
manual marker. A repository that predates ai-specs cannot contain
`<!-- ai-specs:runtime-brief -->`, so its hand-written `AGENTS.md` is
overwritten on the first `init` or `sync` — silently, with no backup and no
prompt.

Protection also depends on the entry point: `init` and `sync` pass
`--preserve-if-runtime-brief`; `sync-agent` does not.

The machinery to fix this properly already exists and is already trusted
elsewhere in the product. `classify_managed_override`
(`lib/_internal/util.py:552`) distinguishes `untracked` — on disk, never
written by us — from `managed_stale` — ours, untouched, out of date. That
distinction is exactly what the marker is a poor manual proxy for.

This is not three bugs. It is one capability, shipped in 0.20.0 for recipe
overrides and extended to gate hooks in 0.22.0, never applied to one surface.

## What changes

1. **Provenance for the brief.** Record the last CLI-written `AGENTS.md` bytes
   in the lock, the way managed overrides and gate hooks already are.
2. **Classify before writing.** Render only when the classification permits it:
   - `missing` → write;
   - `untracked` → **never overwrite** (the pre-existing hand-written file);
   - `user_modified` → **never overwrite**;
   - `managed_current` → no-op;
   - `managed_stale` → update, because we wrote it and nobody touched it.
3. **One guard for every entry point.** `init`, `sync` and `sync-agent` take
   the same path, so protection cannot depend on how the user arrived.
4. **An actionable refusal.** A skipped write says which state was detected and
   what the user can do, instead of silently doing nothing.
5. **Keep the marker working.** An existing `<!-- ai-specs:runtime-brief -->`
   still preserves the file, so nobody who adopted it is broken by this change.

## Success criteria

1. A hand-written `AGENTS.md` with no marker and no lock entry survives `init`,
   `sync` and `sync-agent`, and the user is told why it was left alone.
2. A generated `AGENTS.md` the user has since edited is never overwritten.
3. A generated `AGENTS.md` nobody has touched still updates normally, so
   ordinary projects see no behavior change.
4. All three entry points reach the identical decision for identical inputs.
5. An existing marker still preserves the file.
6. `[brief].render = false` continues to work unchanged.

## Non-goals

- `CLAUDE.md` and the other per-agent instruction slots: symlinks, not renders,
  and `make_relative_symlink` already refuses to replace a regular file.
- Changing the content of the rendered brief.
- Merging user content with generated content. This change decides **whether**
  to write, never how to combine.
- Removing the marker or `[brief].render`.

## Capabilities

### Modified

- `runtime-brief` — the write decision becomes provenance-governed instead of
  marker-governed.

## Affected areas

| Area | Impact | Description |
|---|---|---|
| `lib/_internal/agents-render.py` | Modified | Classification-driven write decision |
| `lib/_internal/lock.py` | Modified | A brief-baseline recorder beside the gate one |
| `lib/init.sh`, `lib/sync.sh`, `lib/sync-agent.sh` | Modified | One shared guard |
| `lib/_internal/doctor.py` | Modified | Report the brief's ownership state |
| `docs/`, wiki quickstart | Modified | Guidance for repos with existing agent docs |

## Risks

| Risk | Mitigation |
|---|---|
| A project mid-adoption has an `AGENTS.md` we wrote but never recorded, so it classifies `untracked` and stops updating | Treat the first sync after upgrade as an adoption point; decide the migration rule explicitly in design, and surface it in doctor |
| Silent no-op is as bad as silent overwrite | Every refusal prints the detected state and the remedy |
| Behavior change for ordinary projects | `managed_stale` must keep updating with no prompt; covered by an explicit success criterion |
