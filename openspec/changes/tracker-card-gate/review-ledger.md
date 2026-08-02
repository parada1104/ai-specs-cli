# Review Ledger: tracker-card-gate (Judgment Day)

- Change: `tracker-card-gate` — branch `feat/tracker-card-gate` @ `90ed152`
- Judges: `jd-judge-a` (13 rows) + `jd-judge-b` (1 row), blind, run 2026-08-02
- Policy: user triage — BLOCKER/CRITICAL/confirmed → fix; suspect → verified before deciding
- Status legend: open | refuted | fixed | verified | wont-fix | info

## Findings

| id | judge | lens | location | severity | triage | status |
|----|-------|------|----------|----------|--------|--------|
| JD-001 | A | risk | `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh:69,220-240` | CRITICAL | confirmed | fixed |
| JD-002 | A | reliability | `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh:535-546` | WARNING | confirmed | fixed |
| JD-003 | A | readability | `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh:72-204,285-302` | WARNING | confirmed | fixed |
| JD-004 | A | risk | `lib/_internal/trello_link.py:39-43,88-104` + gate twin `:433-455` | WARNING | confirmed | fixed |
| JD-005 | A | judgment-day | `lib/_internal/doctor.py:594-595` vs `specs/project-doctor/spec.md:11-13` | WARNING | confirmed | fixed |
| JD-006 | A | reliability | `lib/_internal/doctor.py:611-633`; `tests/test_doctor_tracker_card.py:89-157` | WARNING | confirmed | fixed |
| JD-007 | A | reliability | `tests/test_tracker_card_gate_hook.py:335-361` | SUGGESTION | confirmed | info |
| JD-008 | A | risk | `catalog/recipes/trello-mcp-workflow/recipe.toml:130` | WARNING | confirmed | fixed |
| JD-009 | A | resilience | `lib/_internal/doctor.py:598,611-612` | SUGGESTION | confirmed | info |
| JD-010 | A | readability | `lib/_internal/recipe-materialize.py:392-393` | SUGGESTION | confirmed | info |
| JD-011 | A | judgment-day | `openspec/config.yaml:120-129` vs `ai-specs/ai-specs.toml:64-68` | SUGGESTION | confirmed | info |
| JD-012 | A | readability | gate `:325-326`; `apply-progress.md:113`; `tests/evals/eval_harness_smoke.py:361-364,404` | SUGGESTION | confirmed | info |
| JD-013 | A | judgment-day | `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md:23,162` | SUGGESTION | suspect | wont-fix |
| B-001 | B | judgment-day | `openspec/config.yaml` tracking block; `trello_link.py:17-20,131-135`; gate `:61-65,155-157` | WARNING | confirmed | fixed |

## Evidence (condensed)

- **JD-001** — `segments()` splits only on tokens exactly `|`,`||`,`&&`,`;` after `shlex.split`; shlex treats `\n` as whitespace and leaves `x;` glued, so `cd sub; gh pr create --fill`, `cd sub\ngh pr create --fill`, `set -e\nopenspec archive …`, `git add -A; openspec archive …` all return rc 0 under `always` (probed). Multi-line Bash is the dominant agent shape → shell-mode enforcement largely inert. Design §4c requires per-segment tokenization. Full repro: `agent://JudgeA` JD-001.
- **JD-002** — `_emit_and_exit` comma-joins up to 3 deficient slugs into a singular phrase AND a filesystem path: `add openspec/changes/needs-card,second,third/tracker.none` — exempts nothing. Design §4e wants remediation naming slug(s) + exact fix. Probed with 3 deficient changes, `Write lib/foo.py`, mode=always.
- **JD-003** — first python heredoc (lines 59-350) carries ~150 dead lines: `marker_present`@165, `deficient_slugs`@195, `find_repo_root`@285 have zero call sites; the parser chain they pull in (`clean_value`, `extract_body`, `parse_section`, `is_valid_link`, `sanitize_basename`, `active_changes`, constants, `hashlib` import) is reachable only from them; `stamped_home = sys.argv[2]`@319 feeds only dead `marker_present`. Copies already diverged (dead `home.startswith("__TRACKER_CLI_HOME")` vs live `str(home).startswith(...)` @474). Design §1 promised ~20 lines; shipped is a third dead copy.
- **JD-004** — `_extract_tracker_body` is fence-aware when locating the heading but APPENDS fence lines; `_parse_section_body` parses every body line with no fence tracking → a fenced sample INSIDE a real `## Tracker` section validates (`proposal.md` with a ```markdown - **card_id**: `<24-hex>` ``` block → `is_valid_link()=True`, gate rc 0). Contradicts the module's own fence-protection docstring and the 1769ca9 fix intent.
- **JD-005** — `doctor.py:594-595` accepts a project-local `.recipe/…/bootstrap-ready` marker; project-doctor + tracker-card-gate delta specs name ONLY the runtime-cache path. design.md §3 step 5 explicitly locks the project-local fallback ("legacy … trivial seam for hermetic tests") — planning chain self-contradiction; delta specs never reconciled. Resolution: amend specs to name both locations (cache canonical + project-local fallback), keep production code.
- **JD-006** — doctor emits two `Severity.INFO` `tracker-card` checks (non-canonical card_id, missing url) mandated by spec scenario, with zero test coverage; INFO checks are appended inside the per-change loop before terminal OK, and `test_valid_tracker_ok` passes only because its fixture includes a url. Add tests: (a) valid card_id + no url → one INFO + terminal OK, exit 0; (b) valid non-24-hex card_id → INFO non-canonical, no WARN.
- **JD-008** — `tracker-card-gate-shell` hook description ends "(bash-bypass coverage)" but shell mode only detects `gh pr create` + archive shapes; `cat > lib/foo.py`, `printf 'x' > lib/foo.py`, `python3 -c open(...,'w')`, `sed -i` all rc 0 under `always` (probed). Gap is design-accepted (proposal High risk "Platform hook gaps"); the agent-facing description overclaims. Fix: reword description honestly.
- **B-001** — `openspec/config.yaml` `tracking:` declares the contract (artifact_section/required_fields/card_id validity) but gate/doctor/parser hardcode the same values and never read config → changing config cannot affect enforcement; declaration is misleading/drift-prone; apply-progress admits "declarative only". Fix: add a consistency check (test) pinning config.yaml `tracking:` board_id/gate_mode against recipe config `ai-specs/ai-specs.toml` and correct the header comment; JD-011 (info) partially addressed by this.
- **JD-013 (suspect)** — SKILL.md:162 logs tracker.none exemptions to project-local `.recipe/…/warnings.log`, a path the same skill (line 23) documents as legacy; 7 older references share the path (pre-existing convention). Verify before deciding; if confirmed misdirection, align the 8 references to the canonical location; the "exemption MUST be logged" spec scenario has no machine surface (design §4f keeps gate side-effect-free) — evaluate whether a doctor INFO surface is warranted.

## Triage decision (orchestrator, 2026-08-02)

- **Fix (confirmed, CRITICAL/WARNING):** JD-001, JD-002, JD-003, JD-004, JD-005, JD-006, JD-008, B-001 → `jd-fix-agent`.
- **Verify then decide (suspect):** JD-013.
- **Info (confirmed SUGGESTION, not scheduled):** JD-007, JD-009, JD-010, JD-011, JD-012.

- **JD-013 verification** — All eight `warnings.log` references use the pre-existing project-local `.recipe/trello-mcp-workflow/warnings.log` convention. The runtime cache marker is a separate bootstrap artifact location, so line 162 is not misdirected. Wont-fix; no machine surface added.