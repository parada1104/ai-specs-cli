# Exploration: tracker-only, Go-owned Ledger foundation

## Context

`ai-specs-cli` today tracks work state through prose and hooks, not through a
durable record: a `## Tracker` section inside a change's `proposal.md`, a
`tracker.none` exemption, a pre-tool-use warning/block hook, one `doctor` WARN
check, and the agent-facing brief rules. There is no reconciliation layer, no
work identity, no per-item evidence, and no lifecycle beyond "is a card_id
present".

This exploration maps the repository seams for a **tracker-only Ledger
foundation**: an authoritative, Go-owned record of one primary tracked item per
work identity (repository + current Git branch, enriched by change identity when
present), with local/remote/code/change/Git/PR evidence and explicit human
conflict resolution, evaluated at five checkpoints (work-start,
apply/implementation-start, PR/review, pre-merge, archive/close).

**Context, not behaviour.** The project-level Go strangler policy was merged
into the runtime brief (`AGENTS.md`): new authoritative logic, state machines,
predicates, and durable state belong in Go; Python on the active path may remain
only as a thin compatibility/acquisition/JSON bridge during the transition; keep
one authoritative grader per behaviour and add parity/contract tests at each
migrated seam; incremental, no drive-by rewrites. This is a project contract that
constrains *how* the ledger is built. It is not a recipe behaviour, not a
capability, and must not be materialized as a recipe config field.

## Locked scope (confirmed upstream, not reopened here)

- Tracker-only capability ledger. **No** generic multi-capability ledger
  framework.
- **No** worktree ledger now. Worktree identity is reconsidered only after the
  ask-mode and CWD-fix releases produce evidence.
- Ledger activates **only** when `resolve_bindings` yields a bound `tracker`.
  A declaration alone is *supply*; ambiguous/unbound is **dormant but visible**.
- One primary item per work identity; explicit human conflict resolution.
- Modes `always` / `ask` / `warn`; exact pre-merge posture.
- Provider-specific behaviour is an **adapter/compatibility** concern. Item
  fields and artifact sections must not be universalized onto one provider.
- Synthetic provider fixture is expected. No MCP/API write integration is
  required unless exploration proves it essential (it does not).
- The five-checkpoint lifecycle model is confirmed; this exploration does not
  reopen it.

---

## Findings

### A. Go build, distribution, and trust patterns (all directly reusable)

1. **There is exactly one Go module and one Go product today.**
   `catalog/recipes/worktree-flow/gate/go.mod` → `module ai-specs.dev/worktree-gate`,
   `go 1.22`, **zero third-party dependencies, no `go.sum`**. A second Go module
   would be a second toolchain contract, a second release asset matrix, a second
   trust root, and a second launcher — the single most expensive decision in
   this change.

2. **Build is reproducible and pinned.**
   `scripts/build-gate.sh` builds four targets
   (`darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`) with
   `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`, and
   `-ldflags "-s -w -X main.version=$VERSION"`. It warns when the active
   toolchain is not the canonical `go1.24.13`, and writes a native copy to
   `dist/worktree-gate-current` for the differential runners.

3. **One trust root, one verified acquisition path.**
   `catalog/recipes/worktree-flow/bin/SHA256SUMS` holds four digests (text only —
   **no committed binaries**). `lib/_internal/gate_binary.py` downloads from
   `https://github.com/parada1104/ai-specs-cli/releases/download/v<version>/<asset>`,
   verifies SHA-256 **before execution**, quarantines a mismatched candidate
   under `rejected/`, writes a sidecar `*.verified` receipt
   (`status/version/digest/selftest`), caches at
   `$AI_SPECS_HOME/cache/bin/worktree-gate/<cli-version>/<goos>-<goarch>/worktree-gate`,
   and **never raises**: every failure warns and fails open with a recorded
   `doctor` ERROR. Nothing is ever executed unverified.

4. **Release CI enforces the trust root.**
   `.github/workflows/release-worktree-gate.yml` runs `go vet`, `go test`,
   builds the matrix on tag push, emits `SHA256SUMS`, and diffs canonical forms
   against the committed file via `scripts/verify-gate-sums.sh` (comments/order
   never fail a release; only a real digest mismatch does). A separate `parity`
   job builds the host-native binary and runs `tests/test_worktree_gate_parity.py`.

5. **The runtime seam is a thin Bash launcher + stamped placeholders.**
   `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` resolves, in order:
   `$WORKTREE_GATE_BIN` → project pin `<recipe_root>/bin/worktree-gate` →
   version-keyed cache → *no binary* (one stderr line + `exit 0`). Sync stamps
   `__WORKTREE_GATE_MODE__`, `__WORKTREE_GATE_SCOPE__`,
   `__WORKTREE_REPO_TOPOLOGY__`, `__WORKTREE_GATE_IMPL__`,
   `__WORKTREE_GATE_VERSION__`, and `__TRACKER_CLI_HOME__`
   (`lib/_internal/recipe-materialize.py` `GATE_MODE_PLACEHOLDERS` and friends).
   The launcher never rebuilds and never downloads on the hot path.

6. **The binary's own contract is already the shape a ledger needs.**
   `gate/main.go`: exit `0` allow, exit `2` block, `--version`, `--selftest`
   (compiles every regexp, proves `git` is invocable), `--explain` emitting a
   **JSON diagnostic on stdout** consumed by parity/doctor tooling, and a
   non-destructive fail-open rule on flag-parse errors. Destructive modes are the
   exception and refuse (`--cleanup` returns `2` on a version-skewed binary).

7. **Go already owns the Git-fact layer.** `gate/gitfacts.go` provides memoized
   per-invocation Git facts (`gitMemo`, `gitCommon`, `--path-format=absolute`
   with a legacy fallback), i.e. work identity derivation (toplevel + branch) has
   a proven, tested Go precedent in this repo.

8. **Two build-verification hosts hardcode the gate path and must be touched by
   any new Go module.**
   `tests/run.sh`: `go -C catalog/recipes/worktree-flow/gate test ./...`;
   `tests/validate.sh`: `gofmt -l catalog/recipes/worktree-flow/gate` (skipped
   with a WARNING when `go` is absent). A second module that is not added to
   both files would ship unchecked and unformatted.

### B. Python call sites that can remain thin bridges

The strangler policy permits Python to stay as an **acquisition / compatibility /
JSON bridge**. Concretely:

| Site | Today | Bridge role under the ledger |
|---|---|---|
| `lib/_internal/gate_binary.py` | acquires + verifies + caches the Go binary | keep verbatim — this *is* the sanctioned acquisition bridge |
| `lib/_internal/recipe-materialize.py` | `resolve_bindings()` (line ~701), `build_resolved_config()` (~987), resolved-config write (~1297-1310) | the binding *producer*; stays Python because it reads the catalog and is already the single source of the auto-bound map |
| `lib/_internal/premerge_guardian.py` | archive + verify-evidence gates; `--root` **required**, `--stage pre-archive\|pre-merge` | host that must invoke the Go ledger verdict; its own artifact/evidence logic is untouched |
| `lib/_internal/doctor.py` | `_check_tracker_card_link()` (line ~678), `_check_worktree_gate()` | consume Go ledger diagnostics; stop being a second grader |
| `lib/_internal/trello_link.py` | canonical `## Tracker` parser; `is_valid_link()` = non-empty `card_id` | today's only tracker predicate — a **second grader candidate** the ledger must supersede or parity-test |
| `catalog/recipes/*/hooks/*.sh` | Bash + embedded `python3` heredocs | launchers/stamps only; the ledger verdict must not live here |

**Critical finding — the binding map is not durable.** `lib/sync.sh` writes
resolved-config to a `mktemp` file (`RESOLVED_CONFIG_TEMP`) consumed by
`agents-render.py` and `sync-agent.sh`, then deleted by an EXIT trap. At agent
runtime **nothing on disk records which recipe was bound to `tracker`**. The
ledger's activation predicate ("bound `tracker`") therefore has no offline
witness today. Three ways out: re-derive bindings in Go (new TOML + catalog
reader in Go), persist the already-computed auto-bound map as a durable cache
artifact, or shell out to the Python resolver (a bridge, but on the hot path of
every hook).

**Second critical finding — checker duplication is already the failure mode.**
The `## Tracker` validity rule exists in three places: `trello_link.py`,
a hand-copied twin heredoc inside `tracker-card-gate.sh`, and `doctor.py`. Any
new ledger predicate must land as **one authoritative grader** (Go) with the
existing copies either delegating or parity-tested, per the merged policy.

### C. Recipe / materialization / binding seam

9. **`resolve_bindings(catalog_dir, enabled_ids, manifest_bindings)` is the
   activation source of truth** (`recipe-materialize.py`): step 1 validates
   explicit `[[bindings]]` (recipe enabled, recipe declares the capability,
   duplicates fatal); step 2 auto-binds every capability declared by **exactly
   one** enabled recipe. Ambiguity leaves the capability **unbound** and warns
   ("Add an explicit `[[bindings]]` entry to resolve") — which matches the
   locked semantics: declaration is supply, ambiguity is dormant.

10. **Capability vocabulary is already documented.**
    `docs/capabilities.md` defines `tracker` = "Work-state tracking (cards/issues,
    status, dependencies)" with `trello-mcp-workflow` as the current provider and
    `jira-*`/`github-issues-*` named as future providers. The ledger must speak
    `tracker`, never Trello.

11. **`trello-mcp-workflow` declares `tracker` plus four provider-scoped
    capabilities** (`trello-session-bootstrap`, `trello-card-linking`,
    `trello-state-sync`, `trello-progress-comment`) and carries real
    provider-shaped configuration (`board_id` regex, `default_list`,
    `epic_list`, `board_isolation` tool allow/deny lists, `gate_mode`). A
    provider-neutral ledger core must read this through an **adapter**, and must
    not promote `board_id`/list names into ledger item fields.

12. **Activation precedent to reuse (not to copy blindly).**
    `tracker-card-gate.sh` activates via `marker_present()`: project-local
    `.recipe/trello-mcp-workflow/bootstrap-ready` **or** the canonical cache path
    `<AI_SPECS_HOME>/cache/projects/<key>/.recipe/trello-mcp-workflow/bootstrap-ready`,
    with `<key> = sha256(realpath(project_root))[:12] + "-" + sanitize(basename)`
    (`project-cache.py`). Two known sharp edges: the key is derived from
    **realpath**, so every worktree is a different cache key (an unsynced
    worktree is "inactive"); and a *marker* proves "bootstrapped", not "bound".

13. **`openspec/config.yaml` already declares a `tracking:` block** with
    `tracker: trello`, `board_id`, `artifact_section: "## Tracker"`,
    `required_fields: [card_id, url]`, `gate_mode: warn` — described in-file as
    "a declaration for SDD phase agents and file-format consumers", explicitly
    **not** enforcement. That is a natural place to declare the ledger's
    integration without making it authoritative.

### D. The five checkpoint hosts as they exist today

| Checkpoint | Current host | Mechanism |
|---|---|---|
| work-start | `plan-build-flow` skill prose; `plan-build-gate.sh` | blocks production writes when no `openspec/changes/*/tasks.md` exists; resolves the central planning root through proven submodule topology |
| apply / implementation-start | `tracker-card-gate.sh` (`kind = path`) | exit `2` when a production path is written while an active change lacks `## Tracker`; `warn` mode degrades to stderr |
| PR / review | `tracker-card-gate.sh` (`kind = shell`, `pr_create`) + `catalog/recipes/git-pr-flow/commands/pr-create.md` | shell-action detection for `gh pr create`; the command's step 6 is an explicit **STOP — do not merge** |
| pre-merge | `lib/_internal/premerge_guardian.py` (both stages) + `git-merge-workflow` skill | `--stage pre-archive` then `--stage pre-merge`; tier minima + verify-evidence fields (Full: strict `PASS`, `ready_for_archive: true`, one `Criterion N: PASS` row per `## Success Criteria` bullet) |
| archive / close | archive-tail (`openspec/changes/<slug>/` → `openspec/changes/archive/YYYY-MM-DD-<slug>/`) + `premerge_guardian --stage pre-archive` | dated ISO archive, ambiguity/near-match fail closed, symlink candidates rejected |

Two consequences worth naming: (a) the guards are **fail-open** on any
parse/lookup/git/python3 error, and `openspec/**` writes are never blocked, so a
ledger must respect that same blast-radius discipline; (b) `premerge_guardian.py`
ships in the CLI install (`~/.ai-specs`), **not** in the project — so a Go
verdict reached from that host crosses an install boundary and must still work
when the project cache is cold.

### E. Cache and atomic-state conventions

14. **Project cache layout is keyed and documented** (`project-cache.py`):
    `$AI_SPECS_HOME/cache/projects/<sha256(realpath)[:12]>-<sanitized-basename>/`
    with `meta.toml`, `.recipe/`, `.deps/`, `.bundled/`, `commands/`,
    `resolved-skills/`, and a cache-only immutable `backups/<sha256(rel_path)>/<content_sha>.sh`
    namespace for gate snapshots.
15. **Atomic-write convention is `tempfile.mkstemp` in the destination directory
    + `os.replace`** (`gate_binary.py::_write_verification_record`), and the
    verification receipt is itself a small `key=value` text file whose absence
    means "not verified". Mutable state in this repo is small, textual, and
    either content-hash-named or version-keyed — never a database.
16. **There is no existing ledger-like durable store.** Grepping `ledger` finds
    only change-folder artifacts (`review-ledger.md`, `judgment-ledger.md`) in
    `openspec/changes/**` — prose, not state. Nothing to reuse; the store shape
    is a genuine design decision.
17. **Locking exists but is narrow.** `lib/_internal/lock.py` + a project
    `ai-specs/.ai-specs.lock` govern materialized-asset provenance, and the
    `sync-lock` spec covers concurrent sync. A ledger that both a hook and a
    human can write needs its own concurrency answer; the repo precedent is
    content-addressed names plus atomic replace over lock files.

### F. Test and fixture precedents

18. **Runner and validator are the whole bar.** `./tests/run.sh`
    (Go tests + `unittest discover`) and `./tests/validate.sh` (`py_compile`,
    `bash -n`, `gofmt -l`, then `run.sh`). `openspec/config.yaml` sets
    `strict_tdd: true`, `test_command: ./tests/run.sh`,
    `validation.command: ./tests/validate.sh`; the brief requires RED→GREEN
    discipline and a full-suite run before commit.
19. **Synthetic provider fixtures already exist as a sanctioned pattern.**
    Internal `test-*` recipes live in `tests/fixtures/recipes/` (never in
    `catalog/`), gated by `AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES`, and assembled
    with `tests/_fixture_catalog.py::cli_home_with_fixtures()` /
    `populate_catalog()`. A synthetic *tracker provider* recipe fits this exact
    seam — no MCP, no network, no live board.
20. **The parity-corpus pattern is the strongest precedent for a new Go
    predicate.** `tests/fixtures/worktree-gate-corpus/*.json` holds 16 pinned
    hook-event fixtures; `tests/test_worktree_gate_parity.py` drives the Go binary
    from `dist/worktree-gate-current`. A ledger fixture (`work-identity + evidence
    → expected verdict + expected conflict`) can be a corpus of the same shape.
21. **Hook tests stamp placeholders into a temp copy.**
    `tests/test_tracker_card_gate_hook.py` builds a temp git repo, substitutes
    `__TRACKER_CARD_GATE_MODE__` / `__TRACKER_CLI_HOME__`, writes the
    project-local `bootstrap-ready` marker as the hermetic seam, and asserts
    exit codes with JSON on stdin. `tests/test_premerge_guardian.py` covers the
    guardian's tier/evidence matrix directly.
22. **Go test style in this repo**: stdlib-only table-driven tests beside the
    source (`gate/*_test.go`), no fixtures framework, `t.TempDir()` for
    filesystem (per the `go-testing` skill), and `go vet` in CI.

---

## Options considered

### Option A — Second Go module + second binary `tracker-ledger`

New module (e.g. `catalog/recipes/trello-mcp-workflow/ledger/` or a shared CLI
Go root), own launcher, own `SHA256SUMS`, own release workflow job, own cache
namespace, new entries in `build-gate.sh`, `verify-gate-sums.sh`,
`release-worktree-gate.yml`, `tests/run.sh`, `tests/validate.sh`,
`gate_binary.py`, and `doctor`.

- Pro: clean separation; tracker ledger not coupled to `worktree-flow`.
- Con: duplicates ~6 trust/verification seams, doubles the release surface, and
  creates a **second place** where "release is unverifiable" can silently happen.
  The repo's own history shows how expensive this seam is (`card-46-asset-freshness`,
  `2026-08-18-worktree-cleanup-go` deliberately chose **one** binary and added a
  command mode instead of a second asset).
- Verdict: **reject** as the smallest path. Revisit only if the ledger proves it
  must be distributed independently of the gate binary's lifecycle.

### Option B — Ledger as a new mode of the existing Go module/binary

Add a ledger mode (`--ledger`, plus JSON output) to
`catalog/recipes/worktree-flow/gate`, reusing the existing launcher, cache,
receipt, digest, release, and parity machinery verbatim. The worktree launcher
already stamps a version and resolves the verified binary; the ledger mode is
reached through the same resolution.

- Pro: one module, one toolchain pin, one trust root, one release job, one
  format/vet/test host; parity + fixture corpus patterns apply unchanged;
  "one authoritative grader" is trivially true.
- Con: a tracker concern physically living inside the `worktree-flow` recipe
  tree/packaging is a layering smell — the ledger must activate on `tracker`
  binding, which `worktree-flow` does not own. Mitigable by relocating the Go
  module to a CLI-owned neutral path (e.g. `go/gate/`) as part of this change —
  but that relocation is itself a trust-root-touching move and should be judged
  on its own merits, not smuggled in.
- Verdict: **recommended shape**, with the physical location recorded as an open
  decision (see Unknowns U1).

### Option C — Go verdict engine behind the existing Python hosts (bridge-only)

Keep `premerge_guardian.py`, `doctor.py`, and the shell hooks as **hosts**; they
acquire the verified binary and call the ledger verdict, consuming JSON. No new
Python grader; existing `trello_link.py` validity becomes a documented legacy
alias or a parity-tested shadow.

- Pro: smallest behavioural diff at the checkpoints; respects "no drive-by
  rewrites"; keeps the CLI-install guardian boundary intact; keeps the
  fail-open/warn semantics the hooks already implement.
- Con: a hook that shells out to the Go binary must already know whether the
  ledger is active — which is the binding-durability problem (Finding B/§C).
- Verdict: **this is the delivery mechanism** for Option B, not an alternative.

### Option D — Persist a ledger-state document and make the checkpoints read-only

Ledger verdicts are computed and written by sync/CLI, checkpoints merely read the
document.

- Rejected as the foundation: stale state would contradict live Git facts
  (branch renamed, PR merged, change archived), and it inverts the "one
  authoritative grader" rule by letting a writer, not a predicate, decide.

---

## Recommended smallest path

**One Go module mode + durable activation witness + host-level invocation, with
a synthetic tracker fixture.**

1. **Ledger logic in Go, in one place.** Implement the work-identity derivation
   (repository + current branch, enriched by change identity), the evidence
   model, the conflict predicate, the lifecycle state machine, and the
   checkpoint verdicts as a new mode of the single existing Go module, with
   `--selftest` extended to cover the new invariant set and a JSON diagnostic on
   stdout (the `--explain` precedent). No second binary, no second trust root.
2. **Activation witness, produced by the existing Python bridge.** Because the
   auto-bound capability map is currently a temp file (Finding B), have sync
   persist the already-computed resolved bindings (or a minimal
   `tracker_bound` + provider-id witness) into the project cache using the
   `bootstrap-ready` location convention and the `mkstemp`+`os.replace` atomic
   convention. Python keeps producing it; Go keeps only reading it. Dormant
   states (declared-not-bound, ambiguous) must be **visible**, not silent — the
   natural surfaces are `doctor` and the offline-degradation line the existing
   launcher already prints.
3. **Existing hosts invoke the verdict; the verdict stays Go.** `doctor`,
   the two shell hooks, and `premerge_guardian.py` call the Go ledger and consume
   JSON. Existing `## Tracker` validity logic in `trello_link.py` /
   `tracker-card-gate.sh` becomes a parity-tested shadow or delegates.
4. **Synthetic provider fixture drives everything.** A `test-*` fixture recipe in
   `tests/fixtures/recipes/` providing `tracker` (per Finding F/§19) plus a
   pinned JSON corpus (work identity + evidence + expected verdict/conflict),
   mirroring `tests/fixtures/worktree-gate-corpus/`. MCP/API writes are **not**
   required: the ledger records evidence, it does not perform board edits.
5. **Build/verify plumbing updated in the same work unit.** Whatever the module
   location, `tests/run.sh`, `tests/validate.sh`, `scripts/build-gate.sh`,
   `scripts/verify-gate-sums.sh`, and the release workflow must learn about it in
   the same commit that adds the mode, or the new logic ships unchecked.

Estimated shape: one Go module (new files, no new dependency), one durable
witness writer on the Python side, three host integrations, one fixture recipe,
one corpus. That is already at or beyond the 400-line review budget — the
delivery strategy decision belongs to the tasks phase, not this exploration.

---

## Unknowns that product questions must resolve

- **U1 — Physical home of the ledger Go code.** New mode inside
  `catalog/recipes/worktree-flow/gate` (fastest, layering smell), or relocate the
  module to a CLI-owned neutral path (cleaner, trust-root-touching)? The answer
  decides every path in findings A2/A8.
- **U2 — Work-identity key definition.** "Repository + current Git branch" is
  ambiguous for: detached HEAD, an unborn branch, a worktree of the same branch,
  a branch renamed mid-change, and branch reuse after an item closed. What is the
  canonical string, and what happens when it cannot be derived?
- **U3 — Where the ledger record lives.** Project-committed file
  (reviewable, travels with the change, conflicts on merge), project-local
  gitignored file (worktree-safe, invisible to review), CLI cache keyed by
  project realpath (today's convention, but **per-worktree**, and an unsynced
  worktree is empty), or Engram (already in use, not reviewable, not authoritative
  per the conflict policy). This is the single highest-leverage unanswered
  question; the brief's precedence rules make Engram explicitly *not* final
  authority.
- **U4 — "One primary item" semantics over time.** Same repo+branch over
  multiple closed items: reuse the item, replace it, or keep a history? Does
  closing an item release the identity?
- **U5 — Conflict resolution surface.** Local vs remote vs code vs change vs Git
  vs PR can disagree. Which side wins by default, what exactly does `ask` ask,
  and is human resolution recorded as an event (append-only) or as a resolved
  field (mutable state)? The `ask` mode precedent in `worktree-flow`
  (`gate_mode = ask` presents three destinations and waits) is the form to
  follow.
- **U6 — Mode granularity and the exact pre-merge posture.** `always|ask|warn`
  per checkpoint, or one mode for the ledger? Is pre-merge `always` (block) and is
  a `size:exception`-style escape needed? Who may override, and with what
  artifact?
- **U7 — What "dormant but visible" renders as.** Named in the brief, a `doctor`
  INFO/WARN, an offline note from the launcher, or a runtime brief line? Silent
  dormancy would contradict the locked requirement.
- **U8 — Provider adapter boundary.** Which fields are core
  (item id, provider id, url, state, evidence refs) versus adapter payload, and
  is the adapter section opaque JSON or declared? Given `board_id`/`default_list`/
  `board_isolation`, an opaque payload is the cheaper first cut, but it must not
  leak into the artifact section or the `## Tracker` contract.
- **U9 — Ledger vs the `## Tracker` artifact section.** Does the ledger
  supersede the section as the validity source (with the section becoming
  presentation), or do both remain graders? Per the merged policy only one may be
  authoritative; the other becomes presentation or a parity shadow.
- **U10 — Evidence retention and size.** Append-only events for an
  unknown-length history inside a committed file will grow review load. Is there
  a compaction rule, and does compaction belong in this foundation?
- **U11 — Change-identity enrichment.** How is the active change bound to the
  item when several change folders are active, or when a change is archived
  mid-item (identity moves from `changes/<slug>/` to
  `changes/archive/YYYY-MM-DD-<slug>/`)? The archive-aware resolver precedent is
  `tests/_change_paths.py::change_dir`.

---

## Risks

- **Second distribution seam** (Option A) silently unverifiable: mitigated by
  reusing the single module/trust root; this is the repo's own recorded lesson.
- **Checker proliferation**: the `## Tracker` predicate already exists three
  times. Adding a fourth authoritative copy would contradict the merged
  policy — mitigate with one Go grader plus parity tests at each legacy seam.
- **Hot-path cost**: hooks run on every edit. Re-deriving catalog + TOML bindings
  in Go on each invocation is wasteful; the durable witness removes that cost.
- **Per-worktree cache key** (`realpath`-derived) means an unsynced worktree
  reads as unbound → ledger silently dormant in exactly the workflow the repo
  prescribes (worktrees for every artifact-writing change). This is the highest
  practical risk and it argues for a **project-local** durable witness as the
  primary and the cache path only as fallback.
- **Fail-open vs the locked "exact pre-merge posture"**: today's guards are
  fail-open on any parse/IO error, and `openspec/**` writes are never blocked.
  If pre-merge is meant to be strict, the deliberate exemption must be written
  down, or a cold cache becomes a merge-eligibility change.
- **`premerge_guardian.py` lives in the CLI install**, not the project. A Go
  verdict reached from there must not assume the project cache is warm or that
  `AI_SPECS_HOME` is discoverable.
- **Unverified-bytes regression risk**: any launcher that "tries the local pin
  first" must keep the digest/receipt contract; the launcher must never compile.
- **Committing a growing ledger** into `openspec/changes/**` will collide with
  archive-tail's dated move and with `premerge_guardian`'s folder minima if the
  file is placed carelessly.
- **Synthetic-provider realism gap**: a fixture provider can prove predicate
  behaviour but cannot prove that real Trello evidence maps cleanly; the adapter
  boundary (U8) is where that gap will surface later.

---

## Evidence index

- Go build/distribution/trust: `catalog/recipes/worktree-flow/gate/go.mod`,
  `scripts/build-gate.sh`, `scripts/verify-gate-sums.sh`,
  `catalog/recipes/worktree-flow/bin/SHA256SUMS`,
  `catalog/recipes/worktree-flow/bin/README.md`,
  `.github/workflows/release-worktree-gate.yml`, `lib/_internal/gate_binary.py`,
  `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`,
  `catalog/recipes/worktree-flow/gate/main.go`,
  `catalog/recipes/worktree-flow/gate/gitfacts.go`.
- Build/verify hosts that must learn a new Go module: `tests/run.sh`,
  `tests/validate.sh`.
- Binding + materialization: `lib/_internal/recipe-materialize.py`
  (`resolve_bindings`, `build_resolved_config`, gate placeholders),
  `lib/sync.sh` (`RESOLVED_CONFIG_TEMP`), `lib/_internal/project-cache.py`,
  `docs/capabilities.md`, `catalog/recipes/trello-mcp-workflow/recipe.toml`,
  `catalog/recipes/worktree-flow/recipe.toml`, `openspec/config.yaml`.
- Checkpoint hosts: `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`,
  `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh`,
  `lib/_internal/trello_link.py`, `lib/_internal/premerge_guardian.py`,
  `lib/_internal/doctor.py`, `catalog/recipes/git-pr-flow/commands/pr-create.md`,
  `tests/_change_paths.py`.
- Tests/fixtures: `tests/_fixture_catalog.py`,
  `tests/fixtures/worktree-gate-corpus/`, `tests/fixtures/recipes/`,
  `tests/test_worktree_gate_parity.py`, `tests/test_tracker_card_gate_hook.py`,
  `tests/test_premerge_guardian.py`, `catalog/recipes/worktree-flow/gate/*_test.go`.
- Prior art in the change archive:
  `openspec/changes/archive/2026-08-02-tracker-card-gate/`,
  `openspec/changes/archive/2026-08-18-worktree-cleanup-go/explore.md`,
  `openspec/changes/archive/card-46-asset-freshness/`.
