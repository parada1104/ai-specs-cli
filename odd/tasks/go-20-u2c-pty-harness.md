# Go 20 / U2c support — PTY harness: resume stage search after the consumed marker

Separate unit from U2c (`odd/tasks/go-20-u2c-doctor.md`, branch `change/go-20-u2c-doctor` @ 4ff3943, Trello #187). Branch `change/go-20-u2c-pty-harness` cut from `epic/go-single-binary` @ 9ed25e1. PR target: `epic/go-single-binary`, never `development`. U2c is stacked on this branch after it merges. Trello card #188 https://trello.com/c/WdwHCJlx.

## Specs

- S1 — Symptom, verbatim from the full validate of U2c 4ff3943: `FAIL: test_dep_gate_hands_the_terminal_back (test_init_tui.TestConfigureRecipesStep)`, `command timed out after 90s`. The test passes alone and in its module, and fails intermittently under full-suite conditions.
- S2 — Cause, to be stated only by the deterministic fixture (`tests/test_pty_harness.py`): when one child write carries two prompt markers, `_spawn_pty` must still answer the second one. Before this unit, the next search began at `len(output)` at write time, so a marker that arrived in the same chunk as the matched one was skipped and its stage never fired.
- S3 — Repair boundary: only `_spawn_pty` in `tests/test_init_tui.py`. After a match, the next search resumes at the end of the consumed needle. A `needle is None` stage keeps its old semantics (write immediately).
- S4 — Preserved behaviour: the target test's `qqq` payload and its no-echo assertion stay unchanged; repeated prompts keep matching in order. No timeout is raised to make the test pass.
- S5 — Out of scope: any change to the wizard, to the terminal raw-mode code, to doctor, or to U2c.

## Tasks

- **T1** — RED: `tests/test_pty_harness.py` fixture prints `Skipped dep-tail` and `Write ai-specs.toml ...` in one write, then reads a line. Stages: `qqq`, then newline. Covers: S2.
- **T2** — GREEN: resume the search after the consumed needle in `_spawn_pty`. Covers: S3, S4.
- **T3** — Gates: `tests.test_pty_harness`, `tests.test_init_tui` (32), `tests.test_hub_tui` (15, same helper), target test. Covers: S4.
- **T4** — Full `./tests/validate.sh` once on the committed candidate. Covers: S1.

## Log

- L1 — Causal evidence (deterministic): `python3 -m unittest tests.test_pty_harness` against the original helper: `command timed out after 8s; output: b'Skipped dep-tail\r\r\nWrite ai-specs.toml with these choices? (Y/n) qqq'` (RED, exit 1). Against the repaired helper: `Ran 1 test in 0.475s OK` (GREEN).
- L2 — Gates on the repaired helper: target test OK (2.3s); `tests.test_init_tui` 32 tests OK (51s); `tests.test_hub_tui` 15 tests OK (15s).
- L3 — Previous attempts that are INCOMPLETE and are NOT proof: (a) a sampling probe that kept the PTY slave open in the parent changed the handshake and almost every run timed out (invalid instrument); (b) a second probe was interrupted after 3 partial runs, all at the same stage; (c) a third probe, 5 runs, showed the same failing stage with the next needle already in the buffer at write time. These support the mechanism above and were never a complete sample. The fixture in L1 is the evidence of the cause.
- L4 — Earlier observations, kept for the record: 9/10 quiet and 7/10 loaded runs of the original test passed. The loaded batch ran with 12 unisolated `yes` processes started by this session, stopped by PID; it is an incident, not normal-use evidence.
- L5 — Risk: the helper is shared by `test_init_tui` and `test_hub_tui`; both modules were run against the repair (L2).
