"""Integration tests for the plan-build-flow plan-build-gate.sh runtime hook.

Drives the script with normalized stdin-JSON events and asserts the exit-code
contract: 0 allow / 2 block / fail-open. The gate blocks production edits when
no active change folder (openspec/changes/<slug>/tasks.md, outside archive/)
exists.
"""
from __future__ import annotations

import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
GATE = ROOT / "catalog" / "recipes" / "plan-build-flow" / "hooks" / "plan-build-gate.sh"


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(cwd), *args], check=True,
                   capture_output=True, text=True)


class PlanBuildGateHookTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.repo = Path(self.tmp.name) / "repo"
        self.repo.mkdir()
        _git(self.repo, "init", "-q")
        _git(self.repo, "config", "user.email", "t@t.t")
        _git(self.repo, "config", "user.name", "t")
        (self.repo / "README.md").write_text("x\n")
        _git(self.repo, "add", "-A")
        _git(self.repo, "commit", "-qm", "init")

    def _seed_change(self, slug: str = "demo-change") -> None:
        d = self.repo / "openspec" / "changes" / slug
        d.mkdir(parents=True)
        (d / "tasks.md").write_text("# tasks\n")

    def _seed_archived_change(self, slug: str = "old-change") -> None:
        d = self.repo / "openspec" / "changes" / "archive" / slug
        d.mkdir(parents=True)
        (d / "tasks.md").write_text("# tasks\n")

    def _event(self, tool: str, file_path: str) -> dict:
        return {
            "event": "pre-tool-use",
            "tool_name": tool,
            "tool_input": {"file_path": file_path},
            "cwd": str(self.repo),
        }

    def _run(
        self,
        event: dict,
        *,
        extra_env: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess:
        env = dict(os.environ)
        env.pop("PLAN_BUILD_GATE_MODE", None)
        env.pop("PLAN_BUILD_GATE_PATHS", None)
        if extra_env:
            env.update(extra_env)
        return subprocess.run(
            ["bash", str(GATE)],
            input=json.dumps(event),
            capture_output=True, text=True, env=env,
        )

    # 1. Production write, no change folder → block (exit 2).
    def test_block_production_write_without_change_folder(self):
        r = self._run(self._event("Write", str(self.repo / "src" / "app.py")))
        self.assertEqual(r.returncode, 2, r.stderr)
        self.assertIn("plan-build-gate", r.stderr)

    # 2. Production write, active change folder present → allow.
    def test_allow_production_write_with_change_folder(self):
        self._seed_change()
        r = self._run(self._event("Write", str(self.repo / "src" / "app.py")))
        self.assertEqual(r.returncode, 0, r.stderr)

    # 3. Writing the plan itself is never blocked.
    def test_allow_writing_plan_artifacts(self):
        target = self.repo / "openspec" / "changes" / "new-slug" / "tasks.md"
        r = self._run(self._event("Write", str(target)))
        self.assertEqual(r.returncode, 0, r.stderr)

    # 4. Non-production path (tests) → allow even without a change folder.
    def test_allow_non_production_path(self):
        r = self._run(self._event("Write", str(self.repo / "tests" / "t.py")))
        self.assertEqual(r.returncode, 0, r.stderr)

    # 5. Gitignored agent config on a production tree → always allow.
    def test_allow_claude_settings(self):
        target = self.repo / ".claude" / "settings.json"
        r = self._run(self._event("Write", str(target)))
        self.assertEqual(r.returncode, 0, r.stderr)

    # 6. Missing file_path → fail-open allow.
    def test_missing_file_path_fail_open(self):
        r = self._run({"event": "pre-tool-use", "tool_name": "Write", "tool_input": {}})
        self.assertEqual(r.returncode, 0, r.stderr)

    # 7. Malformed JSON on stdin → fail-open allow.
    def test_malformed_stdin_fail_open(self):
        env = dict(os.environ)
        env.pop("PLAN_BUILD_GATE_MODE", None)
        r = subprocess.run(["bash", str(GATE)], input="not json",
                           capture_output=True, text=True, env=env)
        self.assertEqual(r.returncode, 0, r.stderr)

    # 8. Only an ARCHIVED change folder does not count → still block.
    def test_archived_change_folder_does_not_count(self):
        self._seed_archived_change()
        r = self._run(self._event("Write", str(self.repo / "lib" / "core.py")))
        self.assertEqual(r.returncode, 2, r.stderr)

    # 9. The gate is non-bypassable: there is no on/off/ask mode. Setting the
    #    (now-removed) mode env var must NOT open the gate.
    def test_no_mode_bypass(self):
        r = self._run(
            self._event("Write", str(self.repo / "src" / "app.py")),
            extra_env={"PLAN_BUILD_GATE_MODE": "off"},
        )
        self.assertEqual(r.returncode, 2, r.stderr)

    # 11. PLAN_BUILD_GATE_PATHS override redefines production dirs.
    def test_custom_production_paths_override(self):
        # 'app' is now production; 'src' is not.
        blocked = self._run(
            self._event("Write", str(self.repo / "app" / "x.py")),
            extra_env={"PLAN_BUILD_GATE_PATHS": "app"},
        )
        self.assertEqual(blocked.returncode, 2, blocked.stderr)
        allowed = self._run(
            self._event("Write", str(self.repo / "src" / "x.py")),
            extra_env={"PLAN_BUILD_GATE_PATHS": "app"},
        )
        self.assertEqual(allowed.returncode, 0, allowed.stderr)

    # 12. Not inside a git repo → fail-open allow.
    def test_outside_git_repo_fail_open(self):
        outside = Path(self.tmp.name) / "loose"
        outside.mkdir()
        r = self._run({
            "event": "pre-tool-use",
            "tool_name": "Write",
            "tool_input": {"file_path": str(outside / "src" / "x.py")},
            "cwd": str(outside),
        })
        self.assertEqual(r.returncode, 0, r.stderr)


if __name__ == "__main__":
    unittest.main()
