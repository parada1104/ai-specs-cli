"""Tests for runtime-brief-baseline change.

Covers:
  - Unit: template default enables session-context (TemplateDefaultTests)
  - E2E: fresh init produces behavioral brief (InitBriefE2ETests)
  - E2E: render failure → placeholder fallback, init exits 0
  - E2E: init→sync byte-stability
  - E2E: --preserve-if-runtime-brief marker preserved under --force
  - E2E: no this-repo tokens in baseline AGENTS.md

All offline: catalog read from AI_SPECS_HOME; session-context skills are bundled.
"""
import importlib.util
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
TEMPLATE_PATH = ROOT / "templates" / "ai-specs.toml.tmpl"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class TemplateDefaultTests(unittest.TestCase):
    """Unit tests: the default TOML template pre-enables session-context."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_baseline_unit")

    def _make_project_from_template(self) -> Path:
        """Render ai-specs.toml.tmpl into a fresh temp project directory."""
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()

        # Mimic what init.sh does: sed replace {{PROJECT_NAME}} and write toml
        template_text = TEMPLATE_PATH.read_text()
        toml_text = template_text.replace("{{PROJECT_NAME}}", "test-proj")
        (ai_specs / "ai-specs.toml").write_text(toml_text)

        return root

    def test_template_default_enables_session_context(self):
        """build_resolved_config on the default template yields session-context in enabled."""
        root = self._make_project_from_template()
        result = self.mod.build_resolved_config(root)
        self.assertIn(
            "session-context",
            result["enabled"],
            f"Expected 'session-context' in enabled list. Got: {result['enabled']!r}",
        )

    def test_template_default_no_project_specific_tokens(self):
        """Resolved config from the default template must not contain this-repo tokens."""
        root = self._make_project_from_template()
        result = self.mod.build_resolved_config(root)
        serialized = json.dumps(result)

        # These are tokens from the ai-specs-cli dogfood project; they must not
        # appear in a generic project's baseline config.
        forbidden_tokens = [
            "69ec097f13e2d38ecd89a557",   # board id
            "nnodes/proyectos",             # vault scope
            "ai-specs-cli",                 # project name
        ]
        for token in forbidden_tokens:
            self.assertNotIn(
                token,
                serialized,
                f"Found project-specific token {token!r} in resolved config output.",
            )


# ---------------------------------------------------------------------------
# E2E: fresh init produces behavioral brief
# ---------------------------------------------------------------------------

class InitBriefE2ETests(unittest.TestCase):
    """E2E tests for the init → AGENTS.md rendering pipeline."""

    def _make_target(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        target = Path(tmp.name) / "project"
        target.mkdir()
        return target

    def test_fresh_init_produces_behavioral_brief(self):
        """After init, AGENTS.md must contain the session-context behavioral sections."""
        target = self._make_target()
        result = subprocess.run(
            [str(CLI), "init", str(target)],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, f"init failed:\n{result.stderr}")

        agents_md = target / "AGENTS.md"
        self.assertTrue(agents_md.exists(), "AGENTS.md was not created")
        content = agents_md.read_text()

        # Must contain Workflow Rules section
        self.assertIn(
            "## Workflow Rules",
            content,
            "AGENTS.md must contain '## Workflow Rules' section",
        )
        # Must have at least one session-context workflow_rules bullet
        self.assertIn(
            "A session works on one explicit user request",
            content,
            "AGENTS.md must contain session-context workflow_rules fragment",
        )
        # Must contain Conflict Policy section
        self.assertIn(
            "## Conflict Policy",
            content,
            "AGENTS.md must contain '## Conflict Policy' section",
        )
        # Must have at least two conflict_policy bullets
        conflict_count = content.count("- ")
        # Count bullets specifically in the Conflict Policy section
        cp_start = content.find("## Conflict Policy")
        self.assertGreater(cp_start, -1, "## Conflict Policy section must exist")
        # Find the next ## heading after Conflict Policy
        tail = content[cp_start:]
        next_heading = tail.find("\n## ", 4)  # skip past the ## Conflict Policy line itself
        if next_heading > 0:
            cp_section = tail[:next_heading]
        else:
            cp_section = tail
        bullet_count = cp_section.count("\n- ")
        self.assertGreaterEqual(
            bullet_count, 2,
            f"## Conflict Policy must have at least 2 bullets, found {bullet_count}:\n{cp_section}",
        )

    def test_init_render_failure_falls_back_to_placeholder(self):
        """When the render scripts fail, init must still create AGENTS.md and exit 0.

        Uses a fake python3 that delegates to the real python3 for all scripts
        EXCEPT recipe-materialize.py and agents-render.py, which it makes exit 1.
        This simulates a render-pipeline failure without breaking the rest of init
        (gitignore-render.py, refresh-bundled.py, etc. still run via real python3).
        """
        target = self._make_target()

        # Find the real python3
        import shutil as _shutil
        real_python3 = _shutil.which("python3")
        if not real_python3:
            self.skipTest("python3 not found on PATH")

        # Create a selective fake python3 that fails only for render scripts
        fake_bin = Path(target.parent) / "fake-bin"
        fake_bin.mkdir()
        fake_python = fake_bin / "python3"
        fake_python.write_text(
            "#!/bin/sh\n"
            "# Fail only for the render pipeline scripts; pass through for others.\n"
            "case \"$*\" in\n"
            "  *recipe-materialize*|*agents-render*) exit 1 ;;\n"
            f"  *) exec \"{real_python3}\" \"$@\" ;;\n"
            "esac\n"
        )
        fake_python.chmod(0o755)

        # Build a PATH that puts fake-bin FIRST
        original_path = os.environ.get("PATH", "")
        patched_path = f"{fake_bin}:{original_path}"

        result = subprocess.run(
            [str(CLI), "init", str(target)],
            env={**os.environ, "PATH": patched_path},
            text=True,
            capture_output=True,
            check=False,
        )

        # init MUST exit 0 even if the render pipeline fails
        self.assertEqual(
            result.returncode, 0,
            f"init must exit 0 on render failure; got {result.returncode}\nstderr: {result.stderr}",
        )

        # AGENTS.md must still exist (fallback placeholder)
        agents_md = target / "AGENTS.md"
        self.assertTrue(agents_md.exists(), "AGENTS.md must still be created on render failure")
        content = agents_md.read_text()
        self.assertTrue(len(content) > 0, "AGENTS.md must be non-empty (placeholder)")

        # stderr must mention the skip/fallback
        self.assertIn(
            "render skipped",
            result.stderr,
            f"stderr must mention render skip; got:\n{result.stderr}",
        )

    def test_init_then_sync_is_byte_stable(self):
        """Running sync after init must produce byte-identical AGENTS.md."""
        target = self._make_target()

        subprocess.run(
            [str(CLI), "init", str(target)],
            text=True,
            check=True,
        )

        agents_md = target / "AGENTS.md"
        after_init = agents_md.read_bytes()

        subprocess.run(
            [str(CLI), "sync", str(target)],
            text=True,
            check=True,
        )

        after_sync = agents_md.read_bytes()
        self.assertEqual(
            after_init,
            after_sync,
            "AGENTS.md must be byte-identical after init and after sync",
        )

    def test_force_init_preserves_runtime_brief_marker(self):
        """If AGENTS.md contains <!-- ai-specs:runtime-brief -->, --force must not overwrite it."""
        target = self._make_target()

        # First init to bootstrap the directory
        subprocess.run(
            [str(CLI), "init", str(target)],
            text=True,
            check=True,
        )

        # Write the user-managed marker into AGENTS.md
        agents_md = target / "AGENTS.md"
        original_content = "# Manual Brief\n<!-- ai-specs:runtime-brief -->\n\nCustom content.\n"
        agents_md.write_text(original_content)

        # --force init: must preserve the file because the marker is present
        result = subprocess.run(
            [str(CLI), "init", str(target), "--force"],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(
            result.returncode, 0,
            f"--force init must exit 0; stderr:\n{result.stderr}",
        )

        final_content = agents_md.read_text()
        self.assertIn(
            "<!-- ai-specs:runtime-brief -->",
            final_content,
            "The runtime-brief marker must be preserved after --force init",
        )
        # The file must not have been overwritten (custom content preserved)
        self.assertIn(
            "Custom content.",
            final_content,
            "User custom content must be preserved when marker is present",
        )

    def test_no_project_specific_tokens_in_baseline_agents_md(self):
        """A fresh default init must not leak any this-repo tokens into AGENTS.md."""
        target = self._make_target()

        result = subprocess.run(
            [str(CLI), "init", str(target)],
            text=True,
            capture_output=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, f"init failed:\n{result.stderr}")

        agents_md = target / "AGENTS.md"
        content = agents_md.read_text()

        forbidden_tokens = [
            "69ec097f13e2d38ecd89a557",   # dogfood board id
            "nnodes/proyectos",             # dogfood vault scope
            "ai-specs-cli",                 # dogfood project name
        ]
        for token in forbidden_tokens:
            self.assertNotIn(
                token,
                content,
                f"Found project-specific token {token!r} in baseline AGENTS.md",
            )


if __name__ == "__main__":
    unittest.main()
