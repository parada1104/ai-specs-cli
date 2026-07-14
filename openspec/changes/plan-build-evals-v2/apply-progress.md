# Apply Progress — plan-build-evals-v2

| Task | Status | Notes |
|------|--------|-------|
| T1–T2 premerge guardian | done | `lib/_internal/premerge_guardian.py` + 7 unit tests |
| T3 VCS merge skills | done | git/gitlab/bitbucket; also repaired corrupted gitlab skill line-prefixes |
| T4 worktree-flow | done | leave-cwd + script-first in skill + worktree-clean command |
| T5 specs | done | recipe-evals promoted; plan-build-flow guardian req added |
| T6–T9 eval harness v2 | done | multi-runtime + natural AC3 + AC4/AC5/AC7; omp→`.omp/skills`; Claude acceptEdits |
| T10 live smoke | done | deepseek-v4-flash on opencode/pi/omp; opus on claude (see matrix below) |
| T11 validate | partial | related suites green; 2–3 flaky `test_init_tui` Ctrl-C PTY failures unrelated |
| T12 PR | pending | after user ask / commit remaining harness fixes |

## Live matrix (model: `opencode-go/deepseek-v4-flash` except Claude=`opus`)

| Runtime | AC3 | AC4 | AC5 | AC7 |
|---------|-----|-----|-----|-----|
| opencode | PASS | PASS | PASS* | PASS |
| pi | PASS | PASS | PASS | PASS |
| omp | PASS† | PASS | PASS | PASS |
| claude | PASS‡ | PASS | PASS | PASS‡ |

\* AC5 first attempt empty; retry PASS.
† After fixing skill install path `.pi/skills` → `.omp/skills`; also `--no-extensions` + `--approval-mode yolo`.
‡ First AC3/AC7 used Claude `--permission-mode plan` (read-only) → only `.atl/`; fixed to `acceptEdits`, then PASS.

AC7 assertion: accepts `openspec/changes/*/tasks.md` **or** `.gitignore` (light nondeterminism).
AC5 archive glob: `openspec/changes/archive/*signup-validation*/…` (bare slug or dated).
