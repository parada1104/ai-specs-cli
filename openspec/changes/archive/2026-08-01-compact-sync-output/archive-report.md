# Archive Report: compact-sync-output

## Summary

`ai-specs sync` and `ai-specs sync-agent` now default to compact per-step output
(`syncing <label>`), with `-v`/`--verbose` restoring full detail. Warnings/notices/errors
always pass through; a failing step always prints its complete unfiltered output in
either mode. The public-root fan-out double-sync regression (introduced by this branch's
own pre-commit WIP, never released) is fixed with a restored terminal `exit 0`.

## Timeline

1. **Retro-planning (P0)** — 192 lines of uncommitted production code were found in the
   worktree with no change folder, no tests, no tier classification. `proposal.md`,
   `specs/sync-output-verbosity/spec.md`, and `tasks.md` were written to reconstruct the
   contract, and the existing code was committed as-is per the retro-planning note.
2. **Human authorization** — given explicitly in chat ("sí, continuemos con el cambio").
3. **P1–P4 (TDD implementation)** — fan-out regression fix, verbosity contract, nested
   framing/marker hygiene, and `inherit_errexit` risk review. Commits `56d1f1f`, `daad3aa`.
4. **P5 (docs and close)** — README CLI table, CHANGELOG entry, `./tests/validate.sh`
   green (1187 tests), manual dogfood smoke test. Commits `060c843`, `f799196`.
5. **Round 1 verify + judges** — `sdd-verify` returned PASS with minor gaps. Two
   independent blind adversarial reviews (`jd-judge-a`, `jd-judge-b`) both found the same
   **BLOCKING** defect the verify pass missed: `ai-specs sync-agent` compact mode leaked
   raw `✓` detail lines from `flatten-resolved-skills`/`merge-commands`/subrepo
   `gitignore-render`, which ran outside `run_step`. Plus secondary findings H2 (untested
   `-v` forwarding), M1 (unclassified `·` line), M2 (untested compact-visibility notice),
   M3/F5 (false "byte-identical" verbose claim), L1/L2 (changelog/docs precision).
6. **Remediation (P6)** — every BLOCKING/high/medium/low finding fixed with RED-then-GREEN
   TDD, 7 commits (`0e68ebb` .. `2219a34`). `./tests/validate.sh` green (1194 tests, +7).
7. **Round 2 re-verify** — both `jd-judge-a` and `jd-judge-b` independently re-ran their
   own round-1 findings against the remediation diff, including empirical repro of the
   fixed fan-out compact output and the byte-identical verbose replay. Both returned
   **PASS**, no new issues introduced by the remediation.

## Outcome

All P0–P6 tasks closed. Two accepted nits carried forward, explicitly out of scope for
this change (unchanged by it, no regression):
- `run_step` temp files (`mktemp` out/err) are not `trap`-protected; the leak window is
  effectively unreachable given the surrounding `set -e`/`&&` structure (proposal risk 4).
- A pre-existing `if ...; then ...; fi` around `hooks-render.py` (no `else`) silently
  swallows a non-zero hooks-render exit; unrelated to this change's scope.

## Verification Evidence

- `./tests/validate.sh`: 1194 tests, OK (post-remediation).
- `tests/test_sync_output_verbosity.py`: 22 tests, OK.
- Empirical repro (round 2, `jd-judge-a`): standalone and 3-target public-root fan-out
  `ai-specs sync-agent` in compact mode produce zero leaked `✓`/`·`/`⇢`/`▸` lines, exactly
  one `✓ sync-agent complete` footer.
- Manual dogfood smoke test (`ai-specs sync .` and `ai-specs sync . -v`) against this
  repo's own project state, per `dogfood-verification-isolation`; generated `AGENTS.md`
  reverted, no dogfood state committed.

## Full Commit Range (development..HEAD before archive commit)

```
56d1f1f refactor(sync-agent): extract sync_one_agent with explicit failure returns
daad3aa feat(sync): compact step output with -v/--verbose opt-in
060c843 docs(openspec): check off P0 commit task for compact-sync-output
f799196 docs(sync): document -v/--verbose and add changelog entry
0e68ebb fix(sync-agent): route flatten/merge/gitignore through run_step
7ba66a7 test(sync): cover -v fan-out detail from parent and children
175fc59 docs(sync): classify template-skipped as compact noise
2d7c68b test(sync): assert skipped AGENTS.md notice survives compact
7d5aadf fix(sync): make verbose step replay byte-identical
260ec01 docs(sync): clarify fan-out note and add -v to usage synopses
2219a34 docs(openspec): record P6 post-judge remediation for compact-sync-output
```

Ready for PR against `development`.
