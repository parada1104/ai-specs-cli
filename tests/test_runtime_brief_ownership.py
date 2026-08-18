"""Provenance ownership tests for the generated runtime brief."""
from __future__ import annotations

import importlib.util
import io
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
AGENTS_RENDER = ROOT / "lib/_internal/agents-render.py"
LOCK = ROOT / "lib/_internal/lock.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class RuntimeBriefOwnershipTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.render = load_module(AGENTS_RENDER, "agents_render_runtime_brief_ownership")
        cls.lock = load_module(LOCK, "lock_runtime_brief_ownership")
        cls.doctor = load_module(ROOT / "lib/_internal/doctor.py", "doctor_runtime_brief_ownership")

    def _project(self, root: Path, name: str = "demo") -> tuple[Path, Path, Path]:
        project = root / "project"
        ai_specs = project / "ai-specs"
        ai_specs.mkdir(parents=True)
        toml = ai_specs / "ai-specs.toml"
        toml.write_text(f"[project]\nname = '{name}'\n")
        return project, toml, project / "AGENTS.md"

    def _render(self, toml: Path, output: Path, *, adopt: bool = False) -> str:
        resolved = toml.parent / "resolved.json"
        resolved.write_text(json.dumps({"enabled": [], "recipes": {}, "bindings": {}}))
        return self.render.render(
            toml,
            output,
            preserve_if_marker=False,
            resolved_config_path=resolved,
            adopt_brief=adopt,
        )

    def test_untracked_brief_is_preserved_with_both_remedies(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# hand-written instructions\n"
            output.write_bytes(original)
            stderr = io.StringIO()
            with patch("sys.stderr", stderr):
                state = self._render(toml, output)
            self.assertEqual(state, "preserved")
            self.assertEqual(output.read_bytes(), original)
            message = stderr.getvalue()
            self.assertIn("untracked", message)
            self.assertIn("--adopt-brief", message)
            self.assertIn("ai-specs:runtime-brief", message)

    def test_missing_brief_is_written_and_baseline_recorded(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            self._render(toml, output)
            self.assertTrue(output.is_file())
            lock = self.lock.load_lock(project / "ai-specs/.ai-specs.lock")
            entry = lock["managed"]["AGENTS.md"]
            self.assertEqual(entry["kind"], "runtime-brief")
            self.assertEqual(entry["policy"], "never-force")
            self.assertEqual(entry["sha256"], self.lock.sha256_of(output))

    def test_user_modified_brief_is_preserved(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            self._render(toml, output)
            original = output.read_bytes()
            output.write_bytes(original + b"\n# local context\n")
            edited = output.read_bytes()
            toml.write_text("[project]\nname = 'changed'\n")
            stderr = io.StringIO()
            with patch("sys.stderr", stderr):
                self._render(toml, output)
            self.assertEqual(output.read_bytes(), edited)
            self.assertIn("user_modified", stderr.getvalue())

    def test_managed_stale_brief_updates_and_records_new_baseline(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            self._render(toml, output)
            old = output.read_bytes()
            toml.write_text("[project]\nname = 'changed'\n")
            stderr = io.StringIO()
            with patch("sys.stderr", stderr):
                self._render(toml, output)
            self.assertNotEqual(output.read_bytes(), old)
            self.assertIn("# changed Runtime Brief", output.read_text())
            self.assertEqual(stderr.getvalue(), "")
            lock = self.lock.load_lock(project / "ai-specs/.ai-specs.lock")
            self.assertEqual(lock["managed"]["AGENTS.md"]["sha256"], self.lock.sha256_of(output))

    def test_exact_match_without_baseline_adopts_silently(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            expected = "\n".join(self.render._render_lines({"project": {"name": "demo"}}, {"enabled": [], "recipes": {}, "bindings": {}})).encode()
            output.write_bytes(expected)
            stderr = io.StringIO()
            with patch("sys.stderr", stderr):
                state = self._render(toml, output)
            self.assertEqual(state, "adopted")
            self.assertEqual(stderr.getvalue(), "")
            self.assertEqual(
                self.lock.load_lock(project / "ai-specs/.ai-specs.lock")["managed"]["AGENTS.md"]["sha256"],
                self.lock.sha256_of(output),
            )

    def test_divergent_brief_without_baseline_is_preserved_without_adoption(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# stale generated-looking text\n"
            output.write_bytes(original)
            stderr = io.StringIO()
            with patch("sys.stderr", stderr):
                self._render(toml, output)
            self.assertEqual(output.read_bytes(), original)
            self.assertNotIn("AGENTS.md", self.lock.load_lock(project / "ai-specs/.ai-specs.lock").get("managed", {}))
            self.assertIn("untracked", stderr.getvalue())

    def test_explicit_adoption_records_current_bytes(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# intentionally adopted brief\n"
            output.write_bytes(original)
            self._render(toml, output, adopt=True)
            self.assertEqual(output.read_bytes(), original)
            self.assertEqual(
                self.lock.load_lock(project / "ai-specs/.ai-specs.lock")["managed"]["AGENTS.md"]["sha256"],
                self.lock.sha256_bytes(original),
            )

    def test_marker_is_unconditional_even_without_legacy_flag(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# mine\n<!-- ai-specs:runtime-brief -->\n"
            output.write_bytes(original)
            state = self._render(toml, output)
            self.assertEqual(state, "preserved")
            self.assertEqual(output.read_bytes(), original)

    def test_managed_current_is_a_silent_noop(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            self._render(toml, output)
            before_lock = (project / "ai-specs/.ai-specs.lock").read_bytes()
            stderr = io.StringIO()
            with patch("sys.stderr", stderr):
                state = self._render(toml, output)
            self.assertEqual(state, "current")
            self.assertEqual(stderr.getvalue(), "")
            self.assertEqual((project / "ai-specs/.ai-specs.lock").read_bytes(), before_lock)

    def test_sync_agent_fanout_uses_same_preservation_decision(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            (project / "packages" / "a" / "ai-specs").mkdir(parents=True)
            output.write_bytes(b"# root instructions\n")
            child = project / "packages" / "a" / "AGENTS.md"
            child.write_bytes(b"# child instructions\n")
            toml.write_text(
                "[project]\nname = 'demo'\nsubrepos = ['packages/a']\n\n"
                "[agents]\nenabled = ['claude']\n"
            )
            env = {**os.environ, "AI_SPECS_HOME": str(ROOT)}
            result = subprocess.run(
                [str(ROOT / "bin/ai-specs"), "sync-agent", str(project), "--all"],
                env=env,
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(child.read_bytes(), b"# child instructions\n")
            combined = result.stdout + result.stderr
            self.assertIn("--adopt-brief", combined)
            self.assertIn("ai-specs:runtime-brief", combined)

    def test_doctor_reports_untracked_brief_with_both_remedies(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            output.write_bytes(b"# hand-written instructions\n")
            doctor = self.doctor.Doctor(project)
            doctor._check_brief_provenance()
            checks = [c for c in doctor.checks if c.name == "brief-provenance"]
            self.assertEqual(len(checks), 1)
            self.assertIn("untracked", checks[0].message)
            self.assertIn("--adopt-brief", checks[0].guidance)
            self.assertIn("runtime-brief", checks[0].guidance)

    def test_init_preserves_pre_existing_brief_without_marker(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            original = b"# written before ai-specs\n"
            (project / "AGENTS.md").write_bytes(original)
            result = subprocess.run(
                [str(ROOT / "bin/ai-specs"), "init", str(project)],
                env={**os.environ, "AI_SPECS_HOME": str(ROOT)},
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((project / "AGENTS.md").read_bytes(), original)
            self.assertIn("--adopt-brief", result.stdout + result.stderr)

    def test_explicit_adoption_via_sync_cli_records_untracked_brief(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            original = b"# written before ai-specs\n"
            (project / "AGENTS.md").write_bytes(original)
            env = {**os.environ, "AI_SPECS_HOME": str(ROOT)}
            subprocess.run([str(ROOT / "bin/ai-specs"), "init", str(project)], env=env, check=True)
            lock_path = project / "ai-specs/.ai-specs.lock"
            self.assertNotIn("AGENTS.md", self.lock.load_lock(lock_path).get("managed", {}))
            result = subprocess.run(
                [str(ROOT / "bin/ai-specs"), "sync", str(project), "--adopt-brief"],
                env=env,
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((project / "AGENTS.md").read_bytes(), original)
            self.assertIn("AGENTS.md", self.lock.load_lock(lock_path)["managed"])

    def test_unreadable_lock_preserves_without_traceback(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# keep this\n"
            output.write_bytes(original)
            (project / "ai-specs/.ai-specs.lock").write_text("not = [valid\n")
            stderr = io.StringIO()
            with patch("sys.stderr", stderr):
                state = self._render(toml, output)
            self.assertEqual(state, "preserved")
            self.assertEqual(output.read_bytes(), original)
            self.assertIn("preserv", stderr.getvalue().lower())

    def test_unreadable_target_preserves_without_traceback(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# keep this\n"
            output.write_bytes(original)
            real_read_text = Path.read_text

            def unreadable_target(path, *args, **kwargs):
                if path == output:
                    raise OSError("permission denied")
                return real_read_text(path, *args, **kwargs)

            stderr = io.StringIO()
            with patch.object(Path, "read_text", unreadable_target), patch("sys.stderr", stderr):
                state = self._render(toml, output)
            self.assertEqual(state, "preserved")
            self.assertEqual(output.read_bytes(), original)
            self.assertIn("undetermined", stderr.getvalue())


if __name__ == "__main__":
    unittest.main()
