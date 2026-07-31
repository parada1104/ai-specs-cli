# Tasks: worktree-gate bash-bypass coverage

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~320–420 |
| 400-line budget risk | Medium–High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 script dual-input + heuristics + gate tests → PR 2 recipe dual-hook + hooks-render/sync fixtures → PR 3 policy/docs/version + validate |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High
```

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Dual-input gate script + exhaustive shell heuristic / fail-open tests + path-mode regression | PR 1 | `worktree-gate.sh` + `tests/test_worktree_gate_hook.py`; independent of recipe dual-hook |
| 2 | Dual `[[provides.hooks]]` + render/sync fixture coverage | PR 2 | `recipe.toml` shell hook + `tests/test_hooks_render.py` (+ pipeline/recipe fixtures as needed) |
| 3 | Message parity polish (if not already in Unit 1), anti-fallback SKILL/brief, docs matrix, version `1.3.0`, `./tests/validate.sh` | PR 3 | docs/policy + final gate; base=PR 2 |

## Phase 1: Test helpers + dual-input shell extraction foundation

- [x] 1.1 RED: extend `tests/test_worktree_gate_hook.py` with `_shell_event(command, tool="Bash", cwd=None)` and `_cursor_shell_event(command, cwd=None)` helpers (design §9.1); add failing `test_shell_missing_command_fail_open` (shell-shaped event, empty `tool_input`, no path → exit 0) and `test_cursor_native_payload_command_extracted` scaffolding readiness for Cursor top-level `{command,cwd}` (design §3.1).
  - Files: `tests/test_worktree_gate_hook.py`
  - Spec: Shell Command Write-Bypass Detection — Missing command field fails open; Cursor top-level command acceptance
- [x] 1.2 Modify embedded python extractor in `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` to: parse JSON fail-open; prefer non-empty `file_path`/`notebook_path` → PATH mode single candidate; else extract command via precedence `tool_input.command` → `tool_input.script` → `tool_input.cmd` → top-level `command` → `script`; emit protocol `mode\ttool_name` + candidate lines (design §3.2). No heuristics yet → empty shell candidates → fail-open.
  - Files: `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`
  - Spec: Shell Command Write-Bypass Detection — dual-input + missing command fail-open
- [x] 1.3 GREEN: pass 1.1 missing-command case; run `./tests/run.sh tests/test_worktree_gate_hook.py` and confirm **all existing path-mode tests still pass unchanged** (regression guard).
  - Files: `tests/test_worktree_gate_hook.py`, `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`
  - Spec: Shell Command Write-Bypass Detection — path-mode behavior unchanged

## Phase 2: Unified candidate resolution (path-mode parity)

- [x] 2.1 RED: add a focused regression assertion in `tests/test_worktree_gate_hook.py` that path-write cases (`test_block_write_on_protected_branch_main_worktree`, linked-worktree allow, missing path, malformed JSON, ask/off modes) still match current exit codes and stderr shape (document as parity guard; may reuse existing tests without new names if they already cover — only add if refactor risks drift).
  - Files: `tests/test_worktree_gate_hook.py`
  - Spec: Shell Command Write-Bypass Detection — path-mode unchanged
- [x] 2.2 REFACTOR: extract shared bash `resolve_and_check` (or equivalent loop) in `worktree-gate.sh` so PATH mode feeds a one-element candidate list and SHELL mode will feed N candidates through the **same** nearest-existing-dir → git inside-work-tree → linked-worktree allow → protected-branch block path (design §3.4). Preserve `.claude/settings*` allowlist and `gate_mode` early `off` exit.
  - Files: `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`
  - Spec: Shell Command Write-Bypass Detection — same main-worktree + protected-branch check
- [x] 2.3 GREEN / TRIANGULATE: re-run full `tests/test_worktree_gate_hook.py`; path-mode suite must stay green byte-for-byte on exit codes and existing ask-hint semantics.
  - Files: `tests/test_worktree_gate_hook.py`
  - Spec: Shell Command Write-Bypass Detection — path-mode unchanged

## Phase 3: Pass 1 heuristics (redirection, tee, sed/perl -i, cp/mv) — RED → GREEN per pattern

- [x] 3.1 RED: add block cases in `tests/test_worktree_gate_hook.py` (checkout `main`, expect exit 2 + `worktree-gate` in stderr):
  - `test_shell_redirect_gt_blocks_protected_main` — `echo x > SRC`
  - `test_shell_redirect_append_blocks_protected_main` — `echo x >> SRC`
  - `test_shell_tee_blocks_protected_main` — `echo x | tee SRC` (and cover `tee -a` in same test or sibling)
  - `test_shell_sed_i_blocks_protected_main` — `sed -i 's/a/b/' SRC`
  - `test_shell_perl_i_blocks_protected_main` — `perl -i -pe 's/a/b/' SRC`
  - `test_shell_cp_dest_blocks_protected_main` — `cp /tmp/src.py SRC`
  - `test_shell_mv_dest_blocks_protected_main` — `mv /tmp/src.py SRC`
  - Include ≥1 relative-path SRC resolved via event `cwd` (design §9.1).
  - Files: `tests/test_worktree_gate_hook.py`
  - Spec: Shell Command Write-Bypass Detection — Redirection / tee / sed -i / (cp·mv implied by candidate list)
- [x] 3.2 GREEN: implement Pass 1 in embedded python — `shlex.split` (ValueError → skip Pass 1), segment on `| || && ;`, skip wrappers/`VAR=val`, extract `>`/`>>` (standalone or `^\d*>>?` glued), `tee` non-flag operands, last non-flag of `sed`/`perl` with `-i*`, last non-flag dest of `cp`/`mv`; scrub `/dev/*`, fd-dups, empty/`.`/`-`; dedupe (design §3.3 Pass 1 + scrubbing).
  - Files: `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`
  - Spec: Shell Command Write-Bypass Detection — heuristic candidate collection
- [x] 3.3 GREEN: pass all 3.1 block cases via `./tests/run.sh tests/test_worktree_gate_hook.py`.
  - Files: `tests/test_worktree_gate_hook.py`
  - Spec: scenarios Redirection write blocks; tee write blocks; sed -i write blocks

## Phase 4: Pass 2 interpreter-body heuristics (live exploit class)

- [x] 4.1 RED: add block cases in `tests/test_worktree_gate_hook.py`:
  - `test_shell_python_c_open_w_blocks_protected_main` — `python3 -c "open('SRC','w').write('x')"`
  - `test_shell_python_heredoc_write_text_blocks_protected_main` — live exploit heredoc `Path('SRC').write_text(...)`
  - `test_shell_node_writeFileSync_blocks_protected_main` — `node -e "require('fs').writeFileSync('SRC','x')"`
  - (Optional sibling if cheap) Ruby `File.write` block case per design §3.3.
  - Files: `tests/test_worktree_gate_hook.py`
  - Spec: Shell Command Write-Bypass Detection — Python heredoc write_text blocks on protected main worktree
- [x] 4.2 GREEN: implement Pass 2 regexes on raw `cmd` (always runs, including when shlex fails): Python `open(...,'w'|'a'|'x')`, `Path(...).write_text/bytes(`, Node `fs.writeFileSync|appendFileSync|writeFile|appendFile|createWriteStream`, Ruby `File.write` / write-mode `File.open` (design §3.3 Pass 2); union with Pass 1 candidates.
  - Files: `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`
  - Spec: Shell Command Write-Bypass Detection — interpreter write APIs
- [x] 4.3 GREEN: pass 4.1; keep Phase 3 green.
  - Files: `tests/test_worktree_gate_hook.py`
  - Spec: Python heredoc write_text blocks

## Phase 5: Fail-open / true-negative matrix (design §3.5 + §9.1)

- [x] 5.1 RED: add allow/fail-open cases in `tests/test_worktree_gate_hook.py` (expect exit 0 on protected main unless noted):
  - `test_shell_non_write_git_status_allowed` — `git status --porcelain`
  - `test_shell_non_write_ls_allowed` — `ls -la`
  - `test_shell_non_write_cat_allowed` — `cat SRC`
  - `test_shell_quoted_false_redirect_fail_open` — `echo 'a > b'`
  - `test_shell_redirect_dev_null_fail_open` — `echo x > /dev/null`
  - `test_shell_fd_dup_fail_open` — e.g. `foo 2>&1` / non-write fd form
  - `test_shell_ambiguous_python_variable_path_fail_open` — `python3 -c "open(dst,'w')"`
  - `test_shell_write_outside_repo_fail_open` — `echo x > /tmp/out.txt`
  - `test_shell_write_inside_linked_worktree_allowed` — write into `git worktree add` path
  - `test_shell_unbalanced_quote_non_write_fail_open` — `echo "unterminated`
  - `test_shell_read_only_heredoc_allowed` — python heredoc that only prints/reads (no write API)
  - Keep `test_shell_missing_command_fail_open` from Phase 1.
  - Files: `tests/test_worktree_gate_hook.py`
  - Spec: Ambiguous or unparseable fails open; Missing command; Write outside repo; Linked worktree allowed; Non-write shell allowed; Read-only heredoc allowed
- [x] 5.2 GREEN: adjust scrubbing/heuristics only if a true-negative falsely blocks; never widen patterns to chase completeness. Re-run gate hook suite.
  - Files: `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`, `tests/test_worktree_gate_hook.py`
  - Spec: fail-open matrix (design §3.5)
- [x] 5.3 TRIANGULATE: add `test_cursor_shell_redirect_blocks_protected_main` using `_cursor_shell_event("echo x > SRC")` → exit 2 (design §3.1 / §5 / §9.1 Cursor native shape).
  - Files: `tests/test_worktree_gate_hook.py`
  - Spec: Shell Command Write-Bypass Detection — top-level command acceptance

## Phase 6: Message + gate_mode parity for shell blocks

- [x] 6.1 RED: add `test_shell_block_message_names_bash_bypass_and_worktree_new` — always/default mode, high-confidence shell write → exit 2; stderr mentions bash/shell bypass risk and `/worktree-new` (or worktree creation guidance) per design §5 message template.
  - Files: `tests/test_worktree_gate_hook.py`
  - Spec: Ask-mode and message parity — Shell block message names bash-bypass and worktree creation
- [x] 6.2 RED: add `test_shell_gate_ask_includes_bypass_hint` — stamped/env `gate_mode=ask` + block-shaped shell → exit 2 and stderr contains `WORKTREE_GATE_MODE=off` identical in form to path ask hint; add `test_shell_gate_off_disables_shell_gating` — `off` + block-shaped shell → exit 0; add non-protected branch shell write → exit 0.
  - Files: `tests/test_worktree_gate_hook.py`
  - Spec: Ask-mode shell block includes same bypass hint; gate_mode=off disables shell gating
- [x] 6.3 GREEN: implement shell-mode stderr template (design §5); share the same `if [ "$gate_mode" = ask ]` hint emission as path mode; no new bypass surface.
  - Files: `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`
  - Spec: Ask-mode and message parity for shell blocks
- [x] 6.4 GREEN / REFACTOR: pass 6.1–6.2; ensure path-mode ask message still matches existing `test_gate_ask_blocks_with_bypass_hint`.
  - Files: `tests/test_worktree_gate_hook.py`, `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`
  - Spec: message parity

## Phase 7: Dual hook registration (`recipe.toml` + render tests)

- [x] 7.1 RED: extend `tests/test_hooks_render.py` with `SHELL_GATE_HOOK` fixture (`id="worktree-gate-shell"`, matcher `Bash|Shell|Execute|Terminal`, same `script_path` as file-write hook); add:
  - `test_omp_pi_render_both_worktree_gate_matchers` (or separate omp/pi tests) — two shims, matchers `Edit|Write|MultiEdit|NotebookEdit` and `Bash|Shell|Execute|Terminal`, both invoke same `worktree-gate.sh`, case-insensitive RegExp present
  - `test_cursor_renders_separate_shell_before_shell_execution` — file-write wrapper absent + warning; `worktree-flow-worktree-gate-shell.sh` present; `hooks.json` `beforeShellExecution` managed id for shell hook; shell matcher must not contain Edit/Write/MultiEdit/NotebookEdit
  - `test_claude_two_pretooluse_entries_share_script` — two managed PreToolUse entries, distinct ids, same script command path
  - Files: `tests/test_hooks_render.py`
  - Spec: Dual Hook Registration — omp/pi both matchers; Cursor separate shell-only beforeShellExecution
- [x] 7.2 GREEN: add second `[[provides.hooks]]` in `catalog/recipes/worktree-flow/recipe.toml`:
  ```toml
  id = "worktree-gate-shell"
  event = "pre-tool-use"
  script = "hooks/worktree-gate.sh"
  matcher = "Bash|Shell|Execute|Terminal"
  blocking = true
  ```
  Keep existing `worktree-gate` file-write entry verbatim. Do **not** change `lib/_internal/hooks-render.py` (design §4.2 / §12.3).
  - Files: `catalog/recipes/worktree-flow/recipe.toml`
  - Spec: Dual Hook Registration for Shell Matchers
- [x] 7.3 GREEN: pass 7.1 via `./tests/run.sh tests/test_hooks_render.py`.
  - Files: `tests/test_hooks_render.py`
  - Spec: Dual Hook Registration scenarios
- [x] 7.4 RED→GREEN: update `tests/test_worktree_flow_recipe.py` (and `tests/test_sync_pipeline.py` if it asserts hook ids) so recipe declares **two** hooks sharing `hooks/worktree-gate.sh`; after sync, one materialized script path and both managed shim ids for enabled harnesses (design §9.3).
  - Files: `tests/test_worktree_flow_recipe.py`, `tests/test_sync_pipeline.py` (as needed), `catalog/recipes/worktree-flow/recipe.toml`
  - Spec: Dual Hook Registration — shared script / two entries

## Phase 8: Anti-fallback SKILL + brief workflow_rules

- [x] 8.1 RED: add doc-content assertion test (prefer `tests/test_worktree_flow_recipe.py` or adjacent recipe test) that:
  - `[provides.brief].workflow_rules` contains anti-fallback language (blocked **or** errored structured write → never bash/shell/python/heredoc/redirect retry; create worktree / `/worktree-new`)
  - `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` contains the same rule class (section “Never shell-write around the gate” or equivalent)
  - Files: `tests/test_worktree_flow_recipe.py` (or new focused test module if project convention prefers)
  - Spec: Anti-Fallback Skill and Brief Guidance — SKILL.md and brief contain the anti-fallback rule
- [x] 8.2 GREEN: append design §7.1 rule string to `recipe.toml` `[provides.brief].workflow_rules`; add design §7.2 section to `SKILL.md`.
  - Files: `catalog/recipes/worktree-flow/recipe.toml`, `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md`
  - Spec: Anti-Fallback Skill and Brief Guidance
- [x] 8.3 GREEN: pass 8.1.
  - Files: tests + recipe/skill paths above
  - Spec: anti-fallback scenario

## Phase 9: Docs honesty matrix + version bump

- [x] 9.1 Update `docs/runtime-hooks.md`: shell-gating pattern, dual-hook Cursor rationale (`_matcher_targets_file_writes` → separate Bash-only hook), Cursor native payload note, residual heuristic + process-boundary gaps; per-harness table distinguishing structured file-write vs shell pre-exec (design §8 / §10). MUST NOT claim uniform/full bash prevention.
  - Files: `docs/runtime-hooks.md`
  - Spec: Honest per-harness shell-gate coverage documentation — Docs state residual gaps without overclaiming
- [x] 9.2 Update `catalog/recipes/worktree-flow/README.md` with shell coverage table + residual gaps; adjust `docs/recipes-catalog.md` worktree-flow blurb **only if** it currently implies file-tools-only or absolute gate coverage (design §10). Touch `tests/test_recipes_catalog.py` only if blurb assertions break.
  - Files: `catalog/recipes/worktree-flow/README.md`, `docs/recipes-catalog.md` (conditional), `tests/test_recipes_catalog.py` (conditional)
  - Spec: Honest per-harness shell-gate coverage documentation
- [x] 9.3 Bump `catalog/recipes/worktree-flow/recipe.toml` `version` **1.2.4 → 1.3.0**; assert in recipe tests (`version == "1.3.0"`). Update any other recipe metadata fields only if required by existing validators.
  - Files: `catalog/recipes/worktree-flow/recipe.toml`, `tests/test_worktree_flow_recipe.py`
  - Spec: (design §4.1 version / proposal success criteria); Dual Hook Registration delivery

## Phase 10: Final validation

- [x] 10.1 Run full gate + render + recipe suites: `./tests/run.sh tests/test_worktree_gate_hook.py tests/test_hooks_render.py tests/test_worktree_flow_recipe.py` (plus `test_sync_pipeline.py` / `test_recipes_catalog.py` if touched).
  - Files: tests listed
  - Spec: all ADDED scenarios in `specs/worktree-flow/spec.md`
- [x] 10.2 Cross-check every delta-spec scenario (16) against implemented tests; list any intentional residual (platform subagent/MCP, Cursor no pre-file-write) as docs-only.
  - Files: `openspec/changes/worktree-gate-bash-coverage/specs/worktree-flow/spec.md`
  - Spec: full delta coverage map
- [x] 10.3 Run `./tests/validate.sh` (py_compile + `bash -n` + tests) as the **last** gate; fix drift only.
  - Files: whole tree as needed
  - Spec: proposal success criterion — validate.sh passes

## Out of scope (do not task)

- Post-hoc revert / dirty-tree rollback after shell
- Renderer mixed-matcher Cursor fix / `hooks-render.py` changes
- plan-build-gate bash hole (follow-up)
- Closing OpenCode subagent/MCP or pi/omp child-process hook gaps
- Fully general shell parser / OS sandbox
