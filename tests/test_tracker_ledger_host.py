"""RED/GREEN tests for the tracker-domain ledger host.

W1 split the tracker-ledger acquisition/JSON host out of the Plan Build artifact
guardian. These tests pin that the host:

  * grades the ``pre-merge`` and ``archive-close`` wire checkpoints on its own,
    without any Plan Build/OpenSpec artifact tree;
  * keeps the A9 ledger-mode mapping, evidence bridge, cold-cache fail-open, and
    fail-closed ``ask``/``block`` behavior;
  * exposes a CLI entry point that preserves ``--root`` and stage semantics.

The stub ``worktree-gate`` binary records its argv, so no host test re-implements
the Go predicate.
"""

from __future__ import annotations

import importlib.util
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "lib" / "_internal" / "tracker_ledger_host.py"

STUB_BINARY = """#!/usr/bin/env bash
printf '%s\\n' "$*" >> "${STUB_LOG}"
decision="${STUB_DECISION:-allow}"
reason="${STUB_REASON:-stub}"
checkpoint=""
wrote=0
while [ $# -gt 0 ]; do
  case "$1" in
    --checkpoint) checkpoint="$2"; shift 2 ;;
    --decide) printf 'DECIDE %s\\n' "$2" >> "${STUB_LOG}"; shift 2 ;;
    --write) printf 'WRITE %s\\n' "$2" >> "${STUB_LOG}"; wrote=1; shift 2 ;;
    --evidence) printf 'EVIDENCE %s\\n' "$2" >> "${STUB_LOG}"; shift 2 ;;
    *) shift ;;
  esac
done
if [ "$wrote" = 1 ] && [ "${STUB_WRITE_EXIT:-0}" = 2 ]; then
  exit 2
fi
printf '{"capability":"tracker","active":true,"checkpoint":"%s","mode":"warn","decision":"%s","reason":"%s","identity":{"common_dir":"","branch":"","change":null,"key":""},"item":null,"conflict":null,"prompt":null,"doctor":{"severity":"OK","name":"tracker-ledger","message":""}}\\n' "$checkpoint" "$decision" "$reason"
[ "$decision" = block ] && exit 2
exit 0
"""


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class TrackerLedgerHostTests(unittest.TestCase):
    """The tracker host grades checkpoints with no Plan Build dependency."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(MODULE_PATH, "tracker_ledger_host_test")

    def _root(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        # Deliberately no openspec/ tree: tracker checkpoints are independent.
        return Path(tmp.name)

    def _planning_root(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "openspec" / "changes" / "archive").mkdir(parents=True)
        return root

    def _active_light(self, root: Path, slug: str = "demo") -> Path:
        active = root / "openspec" / "changes" / slug
        active.mkdir(parents=True)
        (active / "tasks.md").write_text("Depth: light\n")
        (active / "proposal.md").write_text("# proposal\n")
        return active

    def _archive_light(self, root: Path, slug: str = "done") -> Path:
        archived = root / "openspec" / "changes" / "archive" / slug
        archived.mkdir(parents=True)
        (archived / "tasks.md").write_text("Depth: light\n")
        (archived / "proposal.md").write_text("# proposal\n")
        return archived

    def _stub(self, root: Path) -> tuple[Path, Path]:
        base = root / "stub"
        base.mkdir(exist_ok=True)
        log = base / "stub.log"
        binary = base / "worktree-gate"
        binary.write_text(STUB_BINARY)
        binary.chmod(0o755)
        return binary, log

    def _env(self, binary: Path | None, log: Path | None, *, decision: str = "block",
             **extra: str) -> dict:
        env = dict(os.environ)
        for key in ("TRACKER_LEDGER_MODE", "TRACKER_CARD_GATE_MODE", "AI_SPECS_HOME"):
            env.pop(key, None)
        if binary is not None and log is not None:
            env["WORKTREE_GATE_BIN"] = str(binary)
            env["STUB_LOG"] = str(log)
            env["STUB_DECISION"] = decision
        env.update(extra)
        return env

    def _run(self, root: Path, slug: str | None, stage: str, env: dict) -> subprocess.CompletedProcess:
        args = [slug] if slug is not None else []
        args += ["--root", str(root), "--stage", stage]
        return self._run_argv(args, env)

    def _run_argv(self, argv: list[str], env: dict) -> subprocess.CompletedProcess:
        # A new session has no controlling terminal, so the ask prompt's
        # /dev/tty open fails deterministically instead of blocking on a
        # developer's real terminal.
        return subprocess.run(
            [sys.executable, str(MODULE_PATH), *argv], capture_output=True, text=True,
            start_new_session=True,
            env=env,
        )

    def _logged(self, log: Path) -> list[str]:
        return log.read_text().splitlines() if log.exists() else []

    def _grade_lines(self, log: Path) -> list[str]:
        return [l for l in self._logged(log) if "--checkpoint" in l and "--write" not in l]

    def _write_lines(self, log: Path) -> list[str]:
        return [l for l in self._logged(log) if l.startswith("WRITE ")]

    # --- checkpoint availability without Plan Build ---

    def test_pre_merge_checkpoint_grades_without_plan_build(self):
        root = self._root()
        binary, log = self._stub(root)
        blockers = None
        env = self._env(binary, log, decision="allow")
        with mock.patch.dict(os.environ, env, clear=True):
            blockers = self.mod.ledger_blockers(root, "pre-merge")
        self.assertEqual(blockers, [], log.read_text())
        self.assertIn("--checkpoint pre-merge", self._grade_lines(log)[0])

    def test_archive_close_checkpoint_grades_without_plan_build(self):
        root = self._root()
        binary, log = self._stub(root)
        env = self._env(binary, log, decision="allow")
        with mock.patch.dict(os.environ, env, clear=True):
            blockers = self.mod.ledger_blockers(root, "archive-close")
        self.assertEqual(blockers, [], log.read_text())
        self.assertIn("--checkpoint archive-close", self._grade_lines(log)[0])

    # --- CLI preserves --root and stage wire values ---

    def test_cli_prearchive_stage_grades_archive_close(self):
        root = self._planning_root()
        self._active_light(root)
        binary, log = self._stub(root)
        r = self._run(root, "demo", "pre-archive", self._env(binary, log))
        self.assertEqual(r.returncode, 1, r.stderr)
        self.assertIn("--checkpoint archive-close", log.read_text())
        self.assertIn("tracker-ledger archive-close", r.stderr)

    def test_cli_premerge_stage_grades_pre_merge(self):
        root = self._planning_root()
        self._archive_light(root)
        binary, log = self._stub(root)
        r = self._run(root, "done", "pre-merge", self._env(binary, log))
        self.assertEqual(r.returncode, 1, r.stderr)
        self.assertIn("--checkpoint pre-merge", log.read_text())

    # --- direct --checkpoint lifecycle surface (W5) ---

    def test_cli_checkpoint_pre_merge_grades_without_plan_build(self):
        root = self._root()  # deliberately no openspec/ tree at all
        binary, log = self._stub(root)
        r = self._run_argv(
            ["demo", "--root", str(root), "--checkpoint", "pre-merge"],
            self._env(binary, log, decision="allow"),
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("--checkpoint pre-merge", self._grade_lines(log)[0])

    def test_cli_checkpoint_archive_close_grades_without_plan_build(self):
        root = self._root()  # deliberately no openspec/ tree at all
        binary, log = self._stub(root)
        r = self._run_argv(
            ["demo", "--root", str(root), "--checkpoint", "archive-close"],
            self._env(binary, log, decision="allow"),
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("--checkpoint archive-close", self._grade_lines(log)[0])
        self.assertEqual(
            self._write_lines(log), [],
            "the lifecycle host never infers a tracker close write",
        )

    def test_cli_default_checkpoint_stays_pre_merge(self):
        root = self._root()
        binary, log = self._stub(root)
        r = self._run_argv(
            ["demo", "--root", str(root)], self._env(binary, log, decision="allow")
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("--checkpoint pre-merge", self._grade_lines(log)[0])

    def test_cli_stage_alias_still_maps_to_wire_checkpoints(self):
        root = self._root()
        binary, log = self._stub(root)
        r = self._run_argv(
            ["demo", "--root", str(root), "--stage", "pre-archive"],
            self._env(binary, log, decision="allow"),
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("--checkpoint archive-close", self._grade_lines(log)[0])

    def test_cli_conflicting_checkpoint_and_stage_grade_nothing(self):
        root = self._root()
        binary, log = self._stub(root)
        r = self._run_argv(
            ["demo", "--root", str(root),
             "--checkpoint", "archive-close", "--stage", "pre-merge"],
            self._env(binary, log, decision="allow"),
        )
        self.assertNotEqual(r.returncode, 0, r.stderr)
        self.assertEqual(
            self._grade_lines(log), [],
            "a contradictory lifecycle request must not grade anything",
        )

    def test_openspec_archive_never_selects_the_archive_close_checkpoint(self):
        """An OpenSpec archive is not tracker item closure: the requested
        checkpoint decides, never the planning tree."""
        root = self._planning_root()
        self._archive_light(root, "done")
        binary, log = self._stub(root)
        r = self._run_argv(
            ["done", "--root", str(root), "--checkpoint", "pre-merge"],
            self._env(binary, log, decision="allow"),
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        grades = self._grade_lines(log)
        self.assertEqual(len(grades), 1, log.read_text())
        self.assertIn("--checkpoint pre-merge", grades[0])
        self.assertNotIn("--checkpoint archive-close", grades[0])
        self.assertEqual(self._write_lines(log), [])

    def test_runtime_docs_no_longer_route_tracker_checkpoints_to_the_guardian(self):
        for path in (ROOT / "docs" / "capabilities.md", ROOT / "docs" / "runtime-hooks.md"):
            text = path.read_text(encoding="utf-8")
            with self.subTest(doc=path.name):
                self.assertNotIn("pre-merge guardian", text)
                self.assertIn("tracker_ledger_host.py", text)

    # --- A9 ledger mode mapping ---

    def test_resolve_ledger_mode_a9_mapping(self):
        root = self._planning_root()
        (root / "ai-specs").mkdir()
        manifest = root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "ledger_mode = 'always'\ngate_mode = 'off'\n"
        )
        self.assertEqual(self.mod.resolve_ledger_mode(root), "always", "ledger_mode wins")
        manifest.write_text(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\ngate_mode = 'off'\n"
        )
        self.assertEqual(self.mod.resolve_ledger_mode(root), "off")
        manifest.write_text(
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\ngate_mode = 'always'\n"
        )
        self.assertEqual(self.mod.resolve_ledger_mode(root), "always")
        manifest.write_text("")
        self.assertEqual(self.mod.resolve_ledger_mode(root), "warn")
        with mock.patch.dict(os.environ, {"TRACKER_LEDGER_MODE": "ask"}):
            self.assertEqual(self.mod.resolve_ledger_mode(root), "ask")

    # --- fail-open / fail-closed behavior preserved ---

    def test_ask_without_tty_blocks_without_fabricating_a_decision(self):
        root = self._planning_root()
        self._archive_light(root)
        binary, log = self._stub(root)
        env = self._env(binary, log, decision="ask", STUB_REASON="needs-item")
        r = self._run(root, "done", "pre-merge", env)
        self.assertEqual(r.returncode, 1, r.stderr)
        self.assertIn("no terminal", r.stderr)
        self.assertNotIn("DECIDE", log.read_text())

    def test_ask_identity_unavailable_reports_and_proceeds(self):
        root = self._planning_root()
        self._archive_light(root)
        binary, log = self._stub(root)
        env = self._env(binary, log, decision="ask", STUB_REASON="identity_unavailable")
        r = self._run(root, "done", "pre-merge", env)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("identity_unavailable", r.stderr)
        self.assertNotIn("DECIDE", log.read_text())

    def test_cold_home_fails_open(self):
        root = self._planning_root()
        self._archive_light(root)
        env = self._env(None, None, AI_SPECS_HOME=str(root / "cold-home"))
        r = self._run(root, "done", "pre-merge", env)
        self.assertEqual(r.returncode, 0, r.stderr)

    # --- evidence bridge behavior preserved ---

    def test_tracker_none_never_becomes_a_host_write(self):
        root = self._planning_root()
        archived = self._archive_light(root, "done")
        (archived / "tracker.none").write_text(
            "\nno tracker card for this archive\n", encoding="utf-8"
        )
        binary, log = self._stub(root)
        r = self._run(root, "done", "pre-merge", self._env(binary, log, decision="allow"))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(
            self._write_lines(log), [],
            "the tracker.none file must never authorize the host to write an exemption",
        )
        grades = self._grade_lines(log)
        self.assertEqual(len(grades), 1, log.read_text())
        self.assertIn("--evidence", grades[0], "the host must pass bridge-built evidence")
        self.assertTrue((archived / "tracker.none").is_file(),
                        "the host never creates or deletes the human-authored file")

    def test_grade_line_carries_evidence_without_an_exemption(self):
        root = self._planning_root()
        self._archive_light(root)
        binary, log = self._stub(root)
        r = self._run(root, "done", "pre-merge", self._env(binary, log, decision="allow"))
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._write_lines(log), [])
        self.assertIn("--evidence", self._grade_lines(log)[0])

    def test_tracker_none_never_reaches_the_write_surface(self):
        root = self._planning_root()
        archived = self._archive_light(root, "done")
        (archived / "tracker.none").write_text("no tracker card\n", encoding="utf-8")
        binary, log = self._stub(root)
        env = self._env(binary, log, decision="allow", STUB_WRITE_EXIT="2")
        r = self._run(root, "done", "pre-merge", env)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._write_lines(log), [],
                         "a failing write surface proves the host issues no exempt write")
        self.assertEqual(len(self._grade_lines(log)), 1, log.read_text())

    def test_tracker_none_cannot_unlock_a_blocking_checkpoint(self):
        root = self._planning_root()
        archived = self._archive_light(root, "done")
        (archived / "tracker.none").write_text("no tracker card\n", encoding="utf-8")
        binary, log = self._stub(root)
        env = self._env(binary, log, decision="block", STUB_REASON="missing tracked item")
        r = self._run(root, "done", "pre-merge", env)
        self.assertEqual(r.returncode, 1, r.stderr)
        self.assertEqual(self._write_lines(log), [],
                         "writing the exemption the file describes is not the host's call")

    def test_evidence_survives_a_cold_cli_home(self):
        root = self._planning_root()
        self._archive_light(root)
        binary, log = self._stub(root)
        env = self._env(binary, log, decision="allow",
                        AI_SPECS_HOME=str(root / "cold-home-with-no-cache"))
        r = self._run(root, "done", "pre-merge", env)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("--evidence", self._grade_lines(log)[0],
                      "the bridge is sibling-loaded, so no cache is required")

    def test_ledger_blockers_grades_pr_review_with_evidence(self):
        root = self._planning_root()
        active = self._active_light(root, "demo")
        (active / "proposal.md").write_text(
            "## Tracker\n\n- **card_id**: `6aa703fdcf61a90ec702d58b`\n", encoding="utf-8"
        )
        binary, log = self._stub(root)
        env = self._env(binary, log, decision="allow")
        with mock.patch.dict(os.environ, env, clear=True):
            blockers = self.mod.ledger_blockers(root, "pr-review", slug="demo")
        self.assertEqual(blockers, [], log.read_text())
        self.assertIn("--evidence", self._grade_lines(log)[0])


if __name__ == "__main__":
    unittest.main()
