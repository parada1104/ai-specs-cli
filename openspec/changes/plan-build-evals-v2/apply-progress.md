# Apply Progress — plan-build-evals-v2

| Task | Status | Notes |
|------|--------|-------|
| T1–T2 premerge guardian | done | `lib/_internal/premerge_guardian.py` + 7 unit tests |
| T3 VCS merge skills | done | git/gitlab/bitbucket; also repaired corrupted gitlab skill line-prefixes |
| T4 worktree-flow | done | leave-cwd + script-first in skill + worktree-clean command |
| T5 specs | done | recipe-evals promoted; plan-build-flow guardian req added |
| T6–T9 eval harness v2 | done | multi-runtime + natural AC3 + AC4/AC5 stubs |
| T10 live smoke | pending | opt-in |
| T11 validate | partial | related suites green; 2–3 flaky `test_init_tui` Ctrl-C PTY failures (rc=120≠1) pre-existing / unrelated |
| T12 PR | pending | after live optional / user ask |
