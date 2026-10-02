---
name: qodana-pr-review
description: >
  PR-mode code review with Qodana before opening a pull request in this repo.
  Runs `qodana scan` with the Go linter (qodana-go, native mode) over the
  current worktree, reporting only problems NEW relative to `development` via
  a baseline SARIF, then drives a fix / baseline / continue decision with the
  user before opening the PR toward development. Trigger: before opening a PR,
  "qodana review", "review con qodana", pre-PR quality check.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Before opening a PR from a feature branch or worktree"
    - "User asks for a Qodana review or pre-PR quality check"
---

# Qodana PR Review

Static-review gate before opening a PR toward `development`. Runs Qodana in
PR mode: only problems introduced by this branch are reported.

## Critical rules

1. **PR base is always `development`.** Never open a PR toward `main`.
2. **Project token required.** The Go linter (`qodana-go`) is a licensed
   linter: it needs `QODANA_TOKEN` = the **project token** of a qodana.cloud
   project (JWT with `"token_type":"project"`). A *personal API token* is
   declined ("token was declined by Qodana Cloud server"). The Free plan
   grants the license ("Licensed to Free Customer").
   - There is **no Community Go edition**: `qodana-go-community` is rejected
     as a `--linter` value by CLI 2026.2.2 and `jetbrains/qodana-go-community`
     does not exist on Docker Hub. Do not "fix" the script back to it.
   - Token lookup order: `QODANA_TOKEN` env → `ai-specs.env` of this worktree
     → `ai-specs.env` of the main checkout (worktrees under `.worktrees/`
     inherit its direnv `.envrc`) → `~/.qodana/token` → `.qodana/token`.
     All are git-ignored. Never commit or echo the token.
3. **Native mode by default** (`--within-docker=false`, no Docker). Set
   `QODANA_DOCKER=1` to run in Docker instead. `--image` always forces Docker.
4. **Ask before acting on findings.** After the report, ask the user with the
   ask-user tool. Never silently fix, baseline, or ignore new issues.
5. Everything under `.qodana/` is git-ignored, including the baseline
   (~1.5 MB). It is regenerated per worktree from the merge-base with
   `development` (~1 min), so accepted issues live as long as the branch;
   once merged they are part of `development` and of every later baseline.

## Files

| File | Role |
|------|------|
| `scripts/qodana-pr-review.sh` | Resolves CLI + token, ensures baseline, runs PR-mode scan, prints report |
| `scripts/sarif_new_issues.py` | Reports `baselineState == "new"` results; `--accept` moves selected ones to the baseline |
| `tests/test_qodana_pr_review_report.py` | Contract tests for the reporter (repo `tests/`) |

## Flow

1. **Preflight** — verify, do not assume (the CLI may still be installing):
   ```bash
   qodana --version && qodana scan --help
   ```
   The script also tries `/opt/homebrew/bin/qodana` and `~/homebrew/bin/qodana`,
   and waits/retries up to `QODANA_MAX_WAIT_SECS` (default 900s) while
   `brew install jetbrains/utils/qodana` finishes. If flags differ from the
   ones the script uses (`-l`, `--within-docker`, `-i`, `-o`, `-b`,
   `--save-report`), stop and adapt the script before running.

2. **Run the PR-mode scan** from the worktree root (~1 min per scan once the
   linter is cached; the first run downloads it):
   ```bash
   ./ai-specs/skills/qodana-pr-review/scripts/qodana-pr-review.sh [base-branch]
   ```
   - If `.qodana/baseline.sarif.json` is missing, it scans the merge-base of
     `development` and `HEAD` in a temporary local clone and saves that SARIF
     as the baseline.
   - Scans the worktree with `-b .qodana/baseline.sarif.json` and prints the
     new issues, numbered `#N`, grouped by Qodana severity
     (Critical/High/Moderate/Low/Info) with `file:line`, rule and message.

3. **Present and ask.** Show the counts per severity and the numbered list,
   then ask with `ask_user_question` (multiSelect) which issues go where:
   - **Fix now** → fix the selected issues, then re-run step 2 to confirm.
   - **Send to baseline** → accept the selected issue numbers:
     ```bash
     python3 ai-specs/skills/qodana-pr-review/scripts/sarif_new_issues.py \
       --accept 2,3 .qodana/results/qodana.sarif.json .qodana/baseline.sarif.json
     ```
     Re-run step 2; accepted issues now count as `UNCHANGED`. Mention them
     in the PR description (the baseline itself is not committed).
   - **Continue as-is** → proceed with the remaining new issues unresolved;
     mention them in the PR description.

4. **Open the PR toward `development`** (only after the decision; pushing and
   opening the PR need the user's explicit go-ahead per repo policy):
   ```bash
   git push -u origin "$(git branch --show-current)"
   gh pr create --base development --head "$(git branch --show-current)" ...
   ```
   Check `--base development` is present before running. Include the Qodana
   summary (new / fixed / baselined) in the PR body.

## Example

```
User: revisa con qodana antes de abrir el PR

Agent: ./ai-specs/skills/qodana-pr-review/scripts/qodana-pr-review.sh
  === Qodana PR review: 2 new issue(s) vs baseline ===
    High: 2
    #1 [High] GoUnhandledErrorResult  catalog/recipes/worktree-flow/gate/qodana_probe.go:5
            Unhandled error
    #2 [High] GoUnusedFunction  catalog/recipes/worktree-flow/gate/qodana_probe.go:5
            Unused function 'qodanaProbe'

Agent → ask_user_question: "¿Qué hacemos con cada issue nuevo?"
  [Arreglar ahora] [Mandar al baseline] [Continuar igual]
User: arreglar #1, baseline #2

Agent: fixes #1 (handles the error), then
  python3 .../sarif_new_issues.py --accept 2 .qodana/results/qodana.sarif.json .qodana/baseline.sarif.json
  re-runs the scan → "0 new issue(s)", commits the fix,
  then (after user OK) gh pr create --base development --head <branch>
```
