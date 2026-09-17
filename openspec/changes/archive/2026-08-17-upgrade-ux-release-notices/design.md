# Design: upgrade-experience

## D1 — Notices live in `CHANGELOG.md`, not a dedicated file

**Decision.** Author notices as an `### Upgrade notes` subsection under each
version heading in `CHANGELOG.md`.

**Rejected: a dedicated `upgrade-notices.toml`.** Structured parsing is easier,
but it creates a second place that must be remembered at release time. The
failure mode of a forgotten notice is silent: the user upgrades and never learns
about the required action. `CHANGELOG.md` is already mandatory in the release
ritual — a notice authored beside the entry it belongs to cannot drift from it.

**Rejected: the GitHub Release body.** Requires network at upgrade time, and the
upgrade already has the authoritative bytes on disk after the fast-forward.

**Consequence.** One parser serves both the version summary and the notices,
because both read the same version-keyed sections.

## D2 — Notices are prose, and the constraint is structural

`ai-specs upgrade` runs against `~/.ai-specs`. It has no consumer project in
scope. It therefore *cannot* answer "does this project have a customized gate
hook?" — the question that decides whether `ai-specs sync --refresh-gates`
applies.

So notices are unconditional prose that name the next command. Anything
project-dependent is `doctor`'s job, and `doctor` already does it:
`lib/_internal/doctor.py:1075` emits the `--refresh-gates` guidance from real
project state.

This is not a simplification for its own sake — a conditional notice evaluated
by `upgrade` would be guessing.

The `0.22.0` notice, as it would be authored:

```markdown
### Upgrade notes
Run `ai-specs sync` in each project to acquire the verified Go worktree-gate
binary. Until you do, the gate falls back to the Bash implementation. Run
`ai-specs doctor` to confirm the resolved implementation; if it reports a
preserved customized gate, use `ai-specs sync --refresh-gates`.
```

## D3 — Crossed range is `(current, new]`, ordered by semver

The parser extracts `## [X.Y.Z] — DATE` headings, orders them by semantic
version, and selects those strictly greater than the pre-upgrade version and
less than or equal to the post-upgrade version.

- **Summary** renders newest first — a user reads "what did I just get".
- **Notices** replay oldest first — instructions apply in release order.

The two orderings are deliberate and differ.

## D4 — Partial clone, never shallow

`--depth` truncates history and breaks `git merge-base --is-ancestor`
(`upgrade.sh:149`, `:211`), which is the divergence guard protecting users from
a corrupted upgrade. Trading that guard for disk space is not acceptable.

`--filter=blob:none` keeps the full commit graph — every ancestry check keeps
working — and skips blob transfer for paths the sparse checkout excludes.

**Exclusion list**, derived from runtime reference counts (`lib/`: 41,
`VERSION`: 4, `bundled-skills`: 3, `templates`: 2, `catalog`: 1, `.git`: 1):

| Excluded | Files | Size |
|---|---|---|
| `openspec/` | 647 | 4147 KiB |
| `tests/` | 220 | 1655 KiB |
| `.github/` | 1 | 5 KiB |
| `tmp/` | 1 | 16 KiB |

The list is intentionally short and additive-only. A path is excluded only when
it has zero runtime references; anything uncertain stays in.

## D5 — Narrowing is best-effort and idempotent

Three states must all work:

1. **Fresh install, modern Git** — clone with filter and sparse cone.
2. **Existing full install** — narrow in place during upgrade, once. A second
   upgrade detects the narrowed state and does nothing.
3. **Old Git or any failure** — warn, leave a full checkout, complete the
   upgrade successfully.

Narrowing is never a precondition for upgrading. It is an optimization applied
opportunistically, in the same spirit as the existing mode-only dirt
remediation at `upgrade.sh:171-182`.

**Reversibility.** `git sparse-checkout disable` restores the full tree, so a
narrowed install is not a one-way door. This must be documented.

## D6 — Parser degradation

A changelog parse failure degrades to the existing plain
`Upgraded: <old> -> <new>` line. It never aborts an upgrade and never surfaces a
traceback. The upgrade already succeeded at that point; refusing to report it
would be strictly worse than reporting it plainly.

## D7 — Output shape

```
ai-specs upgrade
  checking installation
  fetching origin/main
  fast-forwarding 0.21.0 → 0.22.0
  refreshing TUI dependencies
  narrowing checkout

Upgraded: 0.21.0 → 0.22.0

What changed
  0.22.0 — autocontained Go worktree gate
    · Go worktree gate with committed SHA-256 trust root
    · subrepo planning context propagation
    · 6 fixes

Action required
  0.22.0
  Run `ai-specs sync` in each project to acquire the verified Go
  worktree-gate binary. …

Symlink integrity verified.
```

`Action required` is the one section that must survive compact mode — same
class as the existing `ℹ skipped AGENTS.md` notice in `sync-agent.sh`, which is
already exempt from filtering.

## D8 — Where the parser lives

A new `lib/_internal/changelog.py`, invoked from `upgrade.sh` like every other
Python helper. It exposes: parse sections, select a version range, extract the
notice subsection. Pure functions over text, so it is unit-testable without a
git fixture — which matters, because the alternative is asserting on rendered
terminal output.
