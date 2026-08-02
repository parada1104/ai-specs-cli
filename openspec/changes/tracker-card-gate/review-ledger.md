# Review Ledger: tracker-card-gate (Judgment Day)

- Change: `tracker-card-gate` — branch `feat/tracker-card-gate` @ `90ed152` (fixes through `f455483`)
- Judges: `jd-judge-a` + `jd-judge-b`, blind. Round 1 (initial sweeps) + Round 2 (scoped re-review of fix diff).
- Policy: user triage — BLOCKER/CRITICAL/confirmed → fix; suspect → verified before deciding
- Status legend: open | refuted | fixed | verified | wont-fix | info

## Findings (current status)

| id | judge | lens | location | severity | triage | status |
|----|-------|------|----------|----------|--------|--------|
| JD-001 | A | risk | `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh:69,220-240` | CRITICAL | confirmed | verified |
| JD-002 | A | reliability | `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh:535-546` | WARNING | confirmed | fixed |
| JD-003 | A | readability | `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh:72-204,285-302` | WARNING | confirmed | verified |
| JD-004 | A | risk | `lib/_internal/trello_link.py:39-43,88-104` + gate twin `:433-455` | WARNING | confirmed | verified |
| JD-005 | A | judgment-day | `lib/_internal/doctor.py:594-595` vs `specs/project-doctor/spec.md:11-13` | WARNING | confirmed | verified |
| JD-006 | A | reliability | `lib/_internal/doctor.py:611-633`; `tests/test_doctor_tracker_card.py:89-157` | WARNING | confirmed | verified |
| JD-007 | A | reliability | `tests/test_tracker_card_gate_hook.py:335-361` | SUGGESTION | confirmed | info |
| JD-008 | A | risk | `catalog/recipes/trello-mcp-workflow/recipe.toml:130` | WARNING | confirmed | verified |
| JD-009 | A | resilience | `lib/_internal/doctor.py:598,611-612` | SUGGESTION | confirmed | info |
| JD-010 | A | readability | `lib/_internal/recipe-materialize.py:392-393` | SUGGESTION | confirmed | info |
| JD-011 | A | judgment-day | `openspec/config.yaml:120-129` vs `ai-specs/ai-specs.toml:64-68` | SUGGESTION | confirmed | info |
| JD-012 | A | readability | gate `:325-326`; `apply-progress.md:113`; `tests/evals/eval_harness_smoke.py:361-364,404` | SUGGESTION | confirmed | info |
| JD-013 | A | judgment-day | `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md:23,162` | SUGGESTION | suspect | wont-fix |
| B-001 | B | judgment-day | `openspec/config.yaml` tracking block; `trello_link.py:17-20,131-135`; gate `:61-65,155-157` | WARNING | confirmed | verified |
| JD-014 | A (r2) | reliability | `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh:79-81` (`segments()`) | WARNING | confirmed | fixed |
| JD-015 | A (r2) | risk | `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh:79-81` (`segments()`) | WARNING | confirmed | fixed |
| JD-016 | A (r2) | judgment-day | `openspec/changes/tracker-card-gate/design.md:213` vs `recipe.toml:130` | SUGGESTION | confirmed | fixed |
| JD-017 | A (r2) | reliability | `tests/test_tracker_card_gate_hook.py:237-243` | SUGGESTION | confirmed | fixed |

## Round 1 evidence (condensed)

- **JD-001** — `segments()` split only on tokens exactly `|`,`||`,`&&`,`;` after `shlex.split`; shlex treats `\n` as whitespace and leaves `x;` glued → `cd sub; gh pr create --fill`, `cd sub\ngh pr create --fill`, `set -e\nopenspec archive …`, `git add -A; openspec archive …` all rc 0 under `always` (probed). Fixed by separator normalization; verified in round 2.
- **JD-002** — `_emit_and_exit` comma-joined up to 3 deficient slugs into a singular phrase AND a filesystem path: `add openspec/changes/needs-card,second,third/tracker.none` — exempts nothing. Design §4e wants remediation naming slug(s) + exact fix. Round-1 fix incomplete; REOPENED round 2 (see below).
- **JD-003** — first python heredoc carried ~150 dead lines (marker_present/deficient_slugs/find_repo_root + the parser chain, unused stamped_home/hashlib, leftover design comment); copies already diverged. Removed; verified round 2 (heredoc 1 now 90 lines, no dangling refs, 44 tests pass).
- **JD-004** — `_parse_section_body` was not fence-aware → fenced sample INSIDE a real `## Tracker` section validated. Fixed in both parsers; verified round 2 (fenced-inside → `{}`/`False`; plain + trailing-example still parse; `~~~` handled). Low-sev note: unclosed fence fail-closed, pre-existing.
- **JD-005** — doctor accepted project-local marker that delta specs excluded; design §3 step 5 locked the fallback. Specs amended to name both locations; verified round 2.
- **JD-006** — doctor INFO nudges (non-canonical card_id, missing url) untested. Tests added; verified round 2.
- **JD-008** — shell hook description overclaimed "(bash-bypass coverage)". Reworded; verified round 2 (see JD-016 for design.md drift).
- **B-001** — `tracking:` block declarative, not consumed → drift-prone. Consistency test added pinning config vs recipe config; header comment corrected; verified round 2.
- **JD-013 (suspect)** — SKILL.md:162 warnings.log path. Verified: all eight references use the pre-existing project-local `.recipe/trello-mcp-workflow/warnings.log` convention; runtime cache marker is a separate artifact → **wont-fix**, no machine surface added (design §4f).

## Round 2 re-review (2026-08-02, judges A + B on diff `90ed152..f455483`)

Verdicts: JD-001 verified, JD-003 verified, JD-004 verified, JD-005 verified, JD-006 verified, JD-008 verified, B-001 verified, JD-013 wont-fix stands, **JD-002 REOPENED**. No other regressions confirmed.

- **JD-002 reopen** — persists at n=2: `... or add openspec/changes/aaa,bbb/tracker.none with a reason` (rc 2, mode=always, hermetic). Cause: `printf '%s' "$deficient" | tr ',' '\n' | wc -l` counts newlines on a string with no trailing newline → 0 for one slug, 1 for two → singular `path_hint` branch (gate `:389-391`) fires exactly at n=2 and is unreachable at n=1. Consequence: the n=1 exact-path remediation (design.md:416-417 locked example) is never emitted. `paste -sd ', ' -` also cycles delimiters → 3+ slugs render `aaa,bbb ccc`.
- **JD-014 (new, regression)** — `cmd.replace("\n", " ; ")` turns heredoc body lines into segments → `cat > docs/x.md <<'EOF'\ngh pr create --fill\nEOF` blocks under `always` (rc 0 → 2), a docs write shell-mode is not supposed to gate (docs/ outside prod_dirs). Fix: drop heredoc bodies before lexing, or split only outside heredoc regions.
- **JD-015 (new, bypass)** — `shlex.shlex` defaults to `commenters='#'`; with newlines already replaced there is no line terminator, so `# create the PR\ngh pr create --fill`, `echo hi  # note\ngh pr create --fill`, `set -e\n# archive\nopenspec archive needs-card` → rc 0 under `always`. One-line fix: `lexer.commenters = ""`.
- **JD-016 (new, docs drift)** — design.md:213 still pins the old "(bash-bypass coverage)" hook description; design.md:416-417 still shows the pre-JD-002 remediation string. Reconcile with shipped strings.
- **JD-017 (new, vacuous test)** — the JD-002 guard `assertNotRegex(..., r"openspec/changes/[^/]*,[^/]*/tracker\.none")` is vacuous: the fixture seeds exactly one deficient slug so the singular path_hint branch never fires. Fix with JD-002 (seed 2 deficient changes + assert n=1 exact path).

## Triage decision (orchestrator, 2026-08-02)

- **Round 1 fix set:** JD-001, JD-002, JD-003, JD-004, JD-005, JD-006, JD-008, B-001 (confirmed CRITICAL/WARNING) → applied via `jd-fix-agent` (commits ceb9b51..f455483).
- **Round 2 fix set (round 3 pass):** JD-002 (reopen, WARNING confirmed), JD-014 (regression, WARNING confirmed), JD-015 (bypass, WARNING confirmed); JD-016 + JD-017 (confirmed SUGGESTION, folded in: docs drift + the vacuous guard test for JD-002).
- **Info (confirmed SUGGESTION, not scheduled):** JD-007, JD-009, JD-010, JD-011, JD-012.
- **Wont-fix:** JD-013 (verified convention, documented above).

## Round 2 notes

- Non-blocking observations from judge A: newlines inside quoted args are rewritten to ` ; ` before lexing (nil impact today, flagged for future extraction code); `2>&1` lexes as `['2>','&','1']` (detection unaffected); cosmetic blank lines at gate `:142-145` and `:380` from the JD-003 removal.
