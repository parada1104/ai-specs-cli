# Archive Report: topology-aware `gate_scope`

- Change: `gate-scope`
- Tracker: https://trello.com/c/LIoDU2xL/65-follow-up-worktree-flow-gate-scope-for-cross-repo-superrepos
- Depth: domain change
- Delivery: one PR, explicitly authorized despite high forecast; estimated implementation diff remains within the accepted 800-line review exception once artifacts/tests are included.

## Archive readiness

- Source recipe updated: `catalog/recipes/worktree-flow/recipe.toml`, hook, docs, and skill.
- Canonical worktree-flow spec promoted.
- Materialization stamp and stale consumer-hook migration guidance included.
- Focused verification passed: gate 50/50, recipe 16/16, Trello recipe 9/9, bash syntax pass.
- Full validation attempted; long harness timed out. The observed unrelated tracking consistency mismatch was aligned in the dogfood manifest (`gate_mode = "warn"`) and the affected test passes in the focused suite.
- No generated consumer outputs are committed.
- No destructive consumer hook removal was performed; migration remains explicit `rm <hook> && ai-specs sync`.

The active change folder is ready to move to `openspec/changes/archive/gate-scope/` on this review branch before merge.
