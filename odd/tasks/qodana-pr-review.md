# Feature: qodana-pr-review skill

Tracker: none (user-directed skill authoring, no card linked)

- [x] 1. Verify CLI version/flags — qodana 2026.2.2; `qodana-go-community` rejected,
      no community Go image exists; `qodana-go` needs a project QODANA_TOKEN.
- [x] 2. Author skill (SKILL.md + scripts/qodana-pr-review.sh) in ai-specs/skills/
- [x] 3. Real scan: merge-base baseline (227 existing) + PR-mode scan (0 new)
- [x] 4. Reporter extracted to scripts/sarif_new_issues.py with tests
      (fixed: baselineState is a top-level SARIF field); `--accept N` verified
      end-to-end in a throwaway clone (UNCHANGED 228 / NEW 1).
- [x] 5. Baseline kept local (.qodana/ git-ignored, ~1.5 MB, regenerated per worktree)
- [ ] 6. Commit, sync-agent, reload, dogfood the skill by opening the PR to development
