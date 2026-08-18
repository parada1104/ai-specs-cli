# Exploration: runtime-brief ownership

## Trigger

A field report from a Parrotfy install: a repository with a hand-written
28k-character `CLAUDE.md` (Rails 8 migration gotchas, domain glossary, code
patterns). The reporter concluded `ai-specs sync` would destroy it, mitigated
with `[brief].render = false`, and wrote a guide for repositories that already
have agent documentation.

## What the report got wrong

**A pre-existing `CLAUDE.md` is not at risk.** In ai-specs, `CLAUDE.md` is not
rendered at all — it is a *relative symlink to `AGENTS.md`*, created by
`make_relative_symlink` (`lib/sync-agent.sh:435` at the time of the report).
That helper refuses to replace a non-symlink:

```bash
elif [[ -e "$link_path" ]]; then
    echo "    ✗ refuse to overwrite non-symlink: $link_path" >&2
    return 1
```

The failure propagates (`|| return $?`), and with `set -euo pipefail` the sync
aborts. The file survives and the command fails loudly.

The `<!-- ai-specs:runtime-brief -->` marker never applied to `CLAUDE.md`
either — it only ever guarded `AGENTS.md`. So the described mechanism is not
the one in play.

## What the report got right — on a different file

**`AGENTS.md` is the real exposure.** A hand-written `AGENTS.md` with no marker
*is* overwritten by `init` and `sync`. A repository that predates ai-specs is
unprotected by construction: it cannot contain a marker it has never heard of.

The reporter's instinct was correct and their mitigation was sound. Only the
file was wrong.

## The flag inconsistency, scoped

| Entry point | Passes `--preserve-if-runtime-brief` |
|---|---|
| `lib/init.sh:242` | yes |
| `lib/sync.sh:265` | yes |
| `lib/sync-agent.sh:372` | **no** |

Real, but narrower than reported: that render sits inside
`ensure_target_workspace()` and is only reached when
`TARGET_PATH != SOURCE_ROOT` — fan-out to subrepos. On the primary root,
`sync-agent` returns early and never renders. Still, protection that depends on
which entry point you came through is not protection.

## Why the marker is the wrong mechanism

`<!-- ai-specs:runtime-brief -->` is a **manual, binary opt-in**. It requires
the user to know it exists, and to place it by hand, before the first sync that
would destroy their file. The one moment it matters most — a repository ai-specs
has never touched — is precisely when it cannot be present.

A marker the user must know about and place by hand is not provenance. It is a
post-it.

## The capability already exists

`lib/_internal/util.py:552` — `classify_managed_override` — already answers the
right question from disk bytes, lock metadata, and would-write bytes:

| State | Meaning |
|---|---|
| `missing` | not on disk |
| `untracked` | on disk, **no lock entry** — we have never written it |
| `user_modified` | differs from the last bytes we wrote |
| `managed_current` | matches what we would write now |
| `managed_stale` | ours, untouched by the user, and out of date |

`untracked` is exactly the pre-existing hand-written `AGENTS.md`. The
classifier already distinguishes it, with no marker and no user action.

`lib/_internal/lock.py:139` — `set_managed_override` — records the last
CLI-written bytes, and `set_gate_baseline` (line 159) shows the established
pattern for a *generated* file: `kind`, `policy`, and an `auto` update rule.

This governance shipped in 0.20.0 (#172) for recipe overrides, and 0.22.0
extended it to gate hooks with an immutable cache-only backup. `AGENTS.md` is
the last high-value surface still guarded by the marker.

## Open questions carried into the proposal

1. What policy does a runtime brief take — `auto` like gates, or something
   stricter, given it can hold irreplaceable hand-written context?
2. What happens to a repository that already uses the marker deliberately?
3. Does `[brief].render = false` remain necessary once provenance exists?
4. What should the refusal message say, so the user knows what to do?

## Out of scope

- `CLAUDE.md` and other per-agent instruction slots: they are symlinks, already
  refuse to clobber, and are not rendered.
- Changing what the rendered brief contains.
- Any change to `make_relative_symlink`'s refusal behavior.
