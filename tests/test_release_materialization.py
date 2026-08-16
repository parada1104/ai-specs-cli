"""Isolated clean-materialization gate for a release candidate.

Evidence comes from init + sync of a temporary consumer project with
``AI_SPECS_HOME`` pointing at this tree. The candidate's dogfood
``ai-specs/.ai-specs.lock`` is snapshotted and must not change.
"""
from __future__ import annotations

import importlib.util
import os
import re
import subprocess
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
DOCTOR_PY = ROOT / "lib" / "_internal" / "doctor.py"
SHA256SUMS = ROOT / "catalog" / "recipes" / "worktree-flow" / "bin" / "SHA256SUMS"
DOGFOOD_LOCK = ROOT / "ai-specs" / ".ai-specs.lock"

ENABLED_AGENTS = ("claude", "cursor", "opencode", "pi", "omp")
CLI_BUNDLED_SKILL_IDS = (
    "harness-lifecycle",
    "harness-recipes",
    "harness-skills-deps",
    "skill-creator",
    "skill-sync",
)
GATE_PLATFORMS = (
    "worktree-gate-darwin-arm64",
    "worktree-gate-darwin-amd64",
    "worktree-gate-linux-amd64",
    "worktree-gate-linux-arm64",
)
# Doctor Check.render: ``f"{severity:5s}  {name:15s}  {message}"``, then
# ``report()`` prefixes two spaces. Summary "N ERROR" must not match.
DOCTOR_ERROR_LINE = re.compile(r"(?m)^\s*ERROR  ")
DIGEST_LINE = re.compile(
    r"^[0-9a-f]{64}  (worktree-gate-(?:darwin-arm64|darwin-amd64|linux-amd64|linux-arm64))$",
    re.MULTILINE,
)

CONSUMER_MANIFEST = """\
[project]
name = "clean-materialization"
subrepos = []

[agents]
enabled = ["claude", "cursor", "opencode", "pi", "omp"]

[recipes.worktree-flow]
enabled = true
[recipes.worktree-flow.config]
integration_branch = "development"

[recipes.git-pr-flow]
enabled = true
[recipes.git-pr-flow.config]
base_branch = "development"

[recipes.session-context]
enabled = true

[recipes.tdd-flow]
enabled = true
[recipes.tdd-flow.config]
test_command = "./tests/validate.sh"

[recipes.plan-build-flow]
enabled = true
"""


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def candidate_version() -> str:
    return (ROOT / "VERSION").read_text().strip()


def snapshot_dogfood_lock() -> bytes:
    if DOGFOOD_LOCK.is_file():
        return DOGFOOD_LOCK.read_bytes()
    return b""


def platform_output_relpaths(platform: dict, agents: tuple[str, ...]) -> list[str]:
    # No [mcp.*] in the representative manifest: doctor WARNs and sync
    # does not materialize MCP adapter files. Do not require them.
    keys = ("instructions_path", "skills_dir", "commands_dir")
    seen: list[str] = []
    for agent in agents:
        plat = platform[agent]
        for key in keys:
            value = plat.get(key) or ""
            if value and value not in seen:
                seen.append(value)
    return seen


class ReleaseMaterializationTests(unittest.TestCase):
    """Hermetic gate: isolated consumer project is the evidence surface."""

    @classmethod
    def setUpClass(cls):
        cls.doctor = load_module(DOCTOR_PY, "doctor_release_materialization")

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="release-materialization-")
        self.addCleanup(self.tmp.cleanup)
        self.workspace = Path(self.tmp.name) / "workspace"
        self.workspace.mkdir()

    def _env(self) -> dict:
        env = dict(os.environ, AI_SPECS_HOME=str(ROOT), AI_SPECS_GATE_OFFLINE="1")
        env.pop("AI_SPECS_GATE_BUILD", None)
        return env

    def _run_cli(self, *args: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            [str(CLI), *args, str(self.workspace)],
            capture_output=True,
            text=True,
            env=self._env(),
            check=False,
        )

    def test_sha256sums_declares_candidate_version_and_four_platforms(self):
        version = candidate_version()
        text = SHA256SUMS.read_text()
        self.assertIn(f"v{version}", text)
        for name in GATE_PLATFORMS:
            self.assertIn(name, text, f"SHA256SUMS missing {name}")
        found = set(DIGEST_LINE.findall(text))
        self.assertEqual(
            found,
            set(GATE_PLATFORMS),
            f"expected four digest lines, got {sorted(found)!r}\n{text}",
        )

    def test_isolated_init_sync_doctor_materializes_clean_consumer(self):
        version = candidate_version()
        lock_before = snapshot_dogfood_lock()

        proc = self._run_cli("init")
        self.assertEqual(proc.returncode, 0, proc.stderr)

        (self.workspace / "ai-specs" / "ai-specs.toml").write_text(CONSUMER_MANIFEST)

        proc = self._run_cli("sync")
        self.assertEqual(proc.returncode, 0, proc.stderr + proc.stdout)

        lock_path = self.workspace / "ai-specs" / ".ai-specs.lock"
        self.assertTrue(lock_path.is_file(), "temporary lock missing after sync")
        with lock_path.open("rb") as fh:
            lock = tomllib.load(fh)
        self.assertEqual(lock["meta"]["cli_version"], version)

        proc = self._run_cli("doctor")
        combined = proc.stdout + proc.stderr
        self.assertEqual(proc.returncode, 0, combined)
        error_lines = [ln for ln in combined.splitlines() if DOCTOR_ERROR_LINE.match(ln)]
        self.assertEqual(
            error_lines,
            [],
            "doctor reported ERROR severity:\n" + "\n".join(error_lines) + "\n\n" + combined,
        )

        self.assertTrue((self.workspace / "AGENTS.md").exists())

        platform = self.doctor.Doctor.PLATFORM
        for rel in platform_output_relpaths(platform, ENABLED_AGENTS):
            path = self.workspace / rel
            self.assertTrue(path.exists(), f"missing generated output: {rel}")

        skills_root = self.workspace / "ai-specs" / "skills"
        for skill_id in CLI_BUNDLED_SKILL_IDS:
            leftover = skills_root / skill_id
            self.assertFalse(
                leftover.exists(),
                f"CLI-bundled skill copied into project skills/: {leftover}",
            )

        gate = self.workspace / "ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh"
        self.assertTrue(gate.exists(), "worktree-flow launcher missing after sync")

        self.assertEqual(
            snapshot_dogfood_lock(),
            lock_before,
            "clean-materialization gate must not rewrite the candidate dogfood lock",
        )
