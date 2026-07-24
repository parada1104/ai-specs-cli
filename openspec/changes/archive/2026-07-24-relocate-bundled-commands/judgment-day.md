# Judgment Day — relocate-bundled-commands

**Target**: `change/relocate-bundled-commands` @ `f82b046f36666e7c759561ebfa3586bf4769d571`
**Mode**: judgment_day (dual blind adversarial review)
**Round**: 1

**Skills/agents**: `jd-judge-a`; `jd-judge-b`'s configured model (`kimi-k3`)
returned repeated `400 Upstream request failed` errors (confirmed
infrastructure issue via `/Users/robert/.omp/logs/http-400-requests/`, not a
content rejection) — substituted the general-purpose `reviewer` agent for the
second independent pass, given the identical blind-review brief and zero
visibility into Judge A's output. Documented here as a deviation from the
standing `jd-judge-a`/`jd-judge-b` mandate.

## Judge results

| Judge | Verdict | CRITICAL | WARNING | SUGGESTION | INFO |
|-------|---------|----------|---------|------------|------|
| A (`jd-judge-a`) | APPROVE | 0 | 0 | 2 | 1 |
| B (`reviewer`, substitute) | APPROVE_WITH_NITS (non-blocking) | 0 | 1 | 2 | 1 |

Both judges independently ran `./tests/validate.sh` themselves (1044/1044,
exit 0) rather than trusting `verify-report.md`'s claim.

## Confirmed findings (fixed in `f82b046`)

- **B-W1** `lib/init.sh:21` — stale comment described the lock as a
  "bundled-file SHA baseline"; post-change it is a `[meta]`+`[agents.*]`
  provenance stamp with no per-file bundled-command hashes. Reworded to match
  the corrected description already used elsewhere in the same file/change.
- **A-S1 / B-S1** (converging, both judges independently flagged) —
  `refresh-bundled.py`'s `refresh()` carried a dead `init_mode` parameter
  (threaded from `main()`, never read in the body once the `.new`-sidecar
  suppression it gated no longer exists). Removed from `refresh()`;
  `main()` still accepts and discards `--init` for CLI compatibility.
- **A-S2** `lib/_internal/project-cache.py:249` — local variable
  `commands_dir` shadowed the module-level `commands_dir()` function inside
  `remove_bundled_command_leftovers`. Renamed to `local_commands_dir`.
- **B-S2** `tests/test_lock.py` — coverage gap for the combined case (legacy
  `[commands]`/`[opted-out]` + a populated `[agents.*]` section) was
  independently repro-verified correct by Judge B but untested. Added
  `test_legacy_commands_opted_out_dropped_agents_preserved`.
- **A-I1 / B-I1** (converging) — `apply-progress.md`'s risk bullet claiming
  the repo's own dogfood lock "was intentionally left untouched" was stale
  (contradicted by the verify-phase self-migration, commit `9a3f1ce`).
  Updated to record the completed migration.

## Notable independent verifications (not just trust)

- Both judges re-derived the 3-tier merge precedence, the leftover
  byte-identical/lock-hash guard, the dead-code removal list, and the
  scenario→test mapping from the raw diff — not from `verify-report.md`'s
  table.
- Judge A caught and dismissed a false alarm from an earlier hub advisory
  claiming `command-merge.py` was not actually deleted (a stale relative-path
  read against the wrong checkout); confirmed via `git ls-tree` in-worktree
  that the file is genuinely absent from `HEAD` and has zero remaining
  references.
- Judge B manually reproduced the `[agents.*]`-survives-legacy-drop behavior
  rather than accepting the design claim at face value.

## Ledger

```yaml
target_identity: f82b046f36666e7c759561ebfa3586bf4769d571
round: 1
confirmed:
  - id: B-W1
    severity: WARNING
    location: lib/init.sh:21
    status: fixed
  - id: A-S1_B-S1
    severity: SUGGESTION
    location: lib/_internal/refresh-bundled.py
    status: fixed
  - id: A-S2
    severity: SUGGESTION
    location: lib/_internal/project-cache.py:249
    status: fixed
  - id: B-S2
    severity: SUGGESTION
    location: tests/test_lock.py
    status: fixed
info:
  - id: A-I1_B-I1
    location: openspec/changes/relocate-bundled-commands/apply-progress.md
    status: fixed
contradictions: []
deviation: jd-judge-b unavailable (provider 400s); substituted reviewer agent, blind
```

## Verdict

**APPROVED**. No CRITICAL findings from either judge. Zero blocking WARNING.
All confirmed findings fixed and re-verified green (1045/1045 tests,
`./tests/validate.sh` exit 0, independently re-run after fixes).

**Next**: archive, PR to `development`, Trello card #47 → Review.
