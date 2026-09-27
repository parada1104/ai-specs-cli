"""Black-box tests for the brief render policy (lib/_internal/brief-render-policy.py).

No test may import ``lib/_internal`` modules. The policy module ships its own
CLI (``python3 brief-render-policy.py <toml> [--validate]`` → prints
true/false); every policy-value test drives it at the process boundary, and
the dead-recipe-fragment tests drive the ``ai-specs doctor`` verb.
"""
from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home, populate_catalog  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]


def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy (see test_sync_pipeline)."""
    home = isolated_home(base)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    return home


_HOMES: dict[str, Path] = {}


def _home_for(project: Path) -> Path:
    base = project.parent
    key = str(base)
    if key not in _HOMES:
        _HOMES[key] = _make_home(base)
    return _HOMES[key]


def _run_script(script: Path, *args: str, env_home: Path | None = None) -> subprocess.CompletedProcess:
    """Run an internal script at the process boundary, stdin closed."""
    env = {
        "PATH": os.environ.get("PATH", ""),
        "HOME": os.environ.get("HOME", "/tmp"),
        "TMPDIR": os.environ.get("TMPDIR", "/tmp"),
        "LC_ALL": "C",
        "LANG": "C",
    }
    if env_home is not None:
        env["AI_SPECS_HOME"] = str(env_home)
        env["AI_SPECS_NO_NETWORK"] = "1"
    return subprocess.run(
        ["python3", str(script), *args],
        text=True, capture_output=True, check=False, input="", env=env,
    )


class BriefRenderPolicyTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls._tmp = tempfile.TemporaryDirectory(prefix="ai-specs-policy-")
        cls.addClassCleanup(cls._tmp.cleanup)
        cls.base = Path(cls._tmp.name)
        cls.home = _make_home(cls.base)
        cls.policy = cls.home / "lib" / "_internal" / "brief-render-policy.py"

    def _write_toml(self, content: str) -> Path:
        path = self.base / f"{self.id().rsplit('.', 1)[-1]}.toml"
        path.write_text(content)
        self.addCleanup(lambda: path.unlink(missing_ok=True))
        return path

    def _run_policy(self, toml: Path, *extra: str):
        return _run_script(self.policy, str(toml), *extra, env_home=self.home)

    # -- policy verdicts through the script's own CLI -----------------------

    def test_no_brief_table_defaults_true(self):
        # TRIAGE: ai-specs sync — no verb prints the [brief].render policy
        # verdict; the default-enabled contract is observed at the policy
        # script's own CLI boundary.
        toml = self._write_toml("[project]\nname = 'x'\n")
        result = self._run_policy(toml)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "true")

    def test_brief_without_render_defaults_true(self):
        # TRIAGE: ai-specs sync — no verb exposes the bare-[brief] default;
        # observed at the policy script's own CLI boundary.
        toml = self._write_toml("[brief]\n")
        result = self._run_policy(toml)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "true")

    def test_render_true(self):
        # TRIAGE: ai-specs sync — no verb prints the policy verdict for an
        # explicit render = true; observed at the policy script's own CLI.
        toml = self._write_toml("[brief]\nrender = true\n")
        result = self._run_policy(toml)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "true")

    def test_render_false(self):
        # TRIAGE: ai-specs sync — no verb prints the policy verdict for an
        # explicit render = false; observed at the policy script's own CLI.
        toml = self._write_toml("[brief]\nrender = false\n")
        result = self._run_policy(toml)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "false")

    def test_render_string_raises(self):
        # TRIAGE: ai-specs sync — validation of an invalid render TYPE is only
        # observable through the policy script's --validate exit contract.
        toml = self._write_toml('[brief]\nrender = "false"\n')
        result = self._run_policy(toml, "--validate")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("[brief].render", result.stderr)

    def test_render_int_raises(self):
        # TRIAGE: ai-specs sync — validation of a non-boolean int render value
        # is only observable through the policy script's --validate exit.
        toml = self._write_toml("[brief]\nrender = 1\n")
        result = self._run_policy(toml, "--validate")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("[brief].render", result.stderr)

    def test_load_from_toml_file(self):
        # TRIAGE: ai-specs sync — the TOML-file load path of the policy module
        # has no verb equivalent; observed at the policy script's own CLI.
        toml = self._write_toml("[brief]\nrender = false\n")
        result = self._run_policy(toml)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "false")

    def test_cli_prints_false(self):
        # TRIAGE: ai-specs sync — the exact stdout "false" verdict has no verb
        # equivalent; observed at the policy script's own CLI boundary.
        toml = self._write_toml("# managed by ai-specs\n[brief]\nrender = false\n")
        result = self._run_policy(toml)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout.strip(), "false")

    def test_render_uppercase_true_is_toml_error(self):
        # Characterization test: `render = True` is invalid TOML (TOML booleans
        # are lowercase-only). The parser must raise tomllib.TOMLDecodeError,
        # which surfaces as a non-zero exit code through the CLI path.
        # TRIAGE: ai-specs sync — the TOML-decode error exit of the policy
        # module is only observable at the policy script's own CLI boundary.
        toml = self._write_toml("[brief]\nrender = True\n")
        result = self._run_policy(toml)
        self.assertNotEqual(result.returncode, 0)
        # stderr must mention a TOML parse/decode error (not a value error)
        stderr_lower = result.stderr.lower()
        self.assertTrue(
            "invalid" in stderr_lower or "decode" in stderr_lower or "error" in stderr_lower,
            f"Expected a TOML error in stderr, got: {result.stderr!r}",
        )

    def test_cli_non_validate_invalid_render_defaults_to_true(self):
        # S1: In non-validate mode an invalid render value (e.g. a string) must
        # NOT exit 1. It must treat the value as the default (enabled) and print
        # "true" with exit code 0. This is the fail-safe contract.
        # TRIAGE: ai-specs sync — the fail-safe non-validate verdict (stdout
        # "true" on invalid input) has no verb equivalent; observed at the
        # policy script's own CLI boundary.
        toml = self._write_toml('[brief]\nrender = "false"\n')
        result = self._run_policy(toml)
        self.assertEqual(result.returncode, 0, f"stderr: {result.stderr!r}")
        self.assertEqual(result.stdout.strip(), "true")

    def test_cli_validate_rejects_string(self):
        # TRIAGE: ai-specs sync — the --validate rejection exit (rc 1 + needle
        # in stderr) has no verb equivalent; observed at the policy script's
        # own CLI boundary.
        toml = self._write_toml('[brief]\nrender = "false"\n')
        result = self._run_policy(toml, "--validate")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("[brief].render", result.stderr)

    # -- dead-recipe-fragment detection through the doctor verb -------------

    def _doctor_project(self, toml: str) -> Path:
        td = tempfile.TemporaryDirectory(prefix="ai-specs-doctor-")
        self.addCleanup(td.cleanup)
        project = Path(td.name) / "project"
        (project / "ai-specs" / "skills").mkdir(parents=True)
        (project / "ai-specs" / "commands").mkdir()
        (project / "ai-specs" / "ai-specs.toml").write_text(toml)
        # A pre-existing AGENTS.md keeps the unrelated agents-md check quiet.
        (project / "AGENTS.md").write_text("# manual brief\n")
        return project

    def test_has_dead_recipe_fragments_true(self):
        """An enabled recipe declaring brief fragments under render=false is dead.

        doctor surfaces has_dead_recipe_fragments as the brief-fragments-unused
        WARN — that is the CLI-observable form of this contract.
        """
        project = self._doctor_project(
            "[project]\nname = 'dead-fragments'\n\n"
            "[brief]\nrender = false\n\n"
            "[recipes.session-context]\nenabled = true\nversion = '2.0.0'\n"
        )
        result = invoke(project, "doctor", cli_home=_home_for(project))
        self.assertIn(
            "brief-fragments-unused",
            result.stdout + result.stderr,
            "doctor must WARN on enabled recipes whose brief fragments can never render",
        )

    def test_has_dead_recipe_fragments_false(self):
        """An enabled recipe WITHOUT brief fragments is not dead.

        Uses a fresh custom recipe (no [provides.brief]) inside the isolated
        home so no catalog recipe's fragments leak into the assertion.
        """
        project = self._doctor_project(
            "[project]\nname = 'live-fragments'\n\n"
            "[brief]\nrender = false\n\n"
            "[recipes.policy-plain-recipe]\nenabled = true\n"
        )
        home = _home_for(project)
        populate_catalog(home, "policy-plain-recipe")
        result = invoke(project, "doctor", cli_home=home)
        combined = result.stdout + result.stderr
        self.assertIn("brief-provenance", combined, "doctor must run")
        self.assertNotIn(
            "brief-fragments-unused",
            combined,
            "a recipe without brief fragments must not be reported dead",
        )


if __name__ == "__main__":
    unittest.main()
