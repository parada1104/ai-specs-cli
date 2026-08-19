"""Black-box CLI tests for runtime brief ownership (provenance) decisions.

Converted from the coupled agents-render.py/lock.py unit tests: every test
drives `bin/ai-specs <verb>` as a subprocess via `_blackbox.invoke` against a
hermetic project and an isolated CLI home. The original render-level state
machine ("preserved" / "adopted" / "current" / "undetermined") is observed
through its CLI effects: exit codes, stderr messages, byte preservation, and
the `ai-specs/.ai-specs.lock` managed baseline.
"""
from __future__ import annotations

import hashlib
import importlib.util
import io
import json
import re
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path
from unittest.mock import patch

from _blackbox import invoke, isolated_home

ROOT = Path(__file__).resolve().parents[1]
AGENTS_RENDER = ROOT / "lib/_internal/agents-render.py"


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _sha(data: bytes) -> str:
    """sha256 over LF-normalized bytes — the baseline recorded by the lock."""
    return hashlib.sha256(data.replace(b"\r\n", b"\n")).hexdigest()


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


_RENDER_MODULE = None


def _coupled_renderer():
    global _RENDER_MODULE
    if _RENDER_MODULE is None:
        _RENDER_MODULE = load_module(AGENTS_RENDER, "agents_render_ownership_triage")
    return _RENDER_MODULE


def _render_coupled(toml: Path, output: Path, *, adopt: bool = False) -> str:
    resolved = toml.parent / "resolved.json"
    resolved.write_text(json.dumps({"enabled": [], "recipes": {}, "bindings": {}}))
    return _coupled_renderer().render(
        toml,
        output,
        preserve_if_marker=False,
        resolved_config_path=resolved,
        adopt_brief=adopt,
    )


class RuntimeBriefOwnershipTests(unittest.TestCase):
    """Every test drives the real CLI subprocess against a hermetic project."""

    def _home(self) -> Path:
        if not hasattr(self, "_cli_home"):
            td = tempfile.TemporaryDirectory(prefix="bb-own-home-")
            self.addCleanup(td.cleanup)
            self._cli_home = isolated_home(Path(td.name))
        return self._cli_home

    def _cli(self, project: Path, verb: str, *args: str):
        """Single shared wrapper: every test in this class invokes through here."""
        return invoke(project, verb, *args, cli_home=self._home())

    def _project(self, root: Path, name: str = "demo") -> tuple[Path, Path, Path]:
        project = root / "project"
        ai_specs = project / "ai-specs"
        ai_specs.mkdir(parents=True)
        toml = ai_specs / "ai-specs.toml"
        toml.write_text(
            f"[project]\nname = '{name}'\n\n[agents]\nenabled = ['claude']\n"
        )
        return project, toml, project / "AGENTS.md"

    def _sync(self, project: Path, *, adopt: bool = False):
        args = ("--adopt-brief",) if adopt else ()
        return self._cli(project, "sync", *args)

    def _lock_path(self, project: Path) -> Path:
        return project / "ai-specs" / ".ai-specs.lock"

    def _read_lock(self, project: Path) -> dict:
        lock_path = self._lock_path(project)
        if not lock_path.exists():
            return {}
        return tomllib.loads(lock_path.read_text())

    def _clear_lock_managed(self, project: Path) -> None:
        """Rewrite the lock with an empty [managed] table (no baseline)."""
        self._lock_path(project).write_text("[managed]\n")

    def _corrupt_lock_sha(self, project: Path, new_sha: str) -> None:
        """Replace the single sha256 baseline line (interrupted-write fixture)."""
        lock_path = self._lock_path(project)
        text = lock_path.read_text()
        text, n = re.subn(r'sha256 = "[0-9a-f]{64}"', f'sha256 = "{new_sha}"', text)
        assert n == 1, "expected exactly one sha256 baseline line"
        lock_path.write_text(text)

    def test_untracked_brief_is_preserved_with_both_remedies(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# hand-written instructions\n"
            output.write_bytes(original)
            result = self._sync(project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(output.read_bytes(), original)
            self.assertIn("untracked", result.stderr)
            self.assertIn("--adopt-brief", result.stderr)
            self.assertIn("ai-specs:runtime-brief", result.stderr)

    def test_missing_brief_is_written_and_baseline_recorded(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            result = self._sync(project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertTrue(output.is_file())
            entry = self._read_lock(project)["managed"]["AGENTS.md"]
            self.assertEqual(entry["kind"], "runtime-brief")
            self.assertEqual(entry["policy"], "never-force")
            self.assertEqual(entry["sha256"], _sha(output.read_bytes()))

    def test_user_modified_brief_is_preserved(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            self._sync(project)
            original = output.read_bytes()
            output.write_bytes(original + b"\n# local context\n")
            edited = output.read_bytes()
            toml.write_text("[project]\nname = 'changed'\n\n[agents]\nenabled = ['claude']\n")
            result = self._sync(project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(output.read_bytes(), edited)
            self.assertIn("user_modified", result.stderr)

    def test_managed_stale_brief_updates_and_records_new_baseline(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            self._sync(project)
            old = output.read_bytes()
            toml.write_text("[project]\nname = 'changed'\n\n[agents]\nenabled = ['claude']\n")
            result = self._sync(project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertNotEqual(output.read_bytes(), old)
            self.assertIn("# changed Runtime Brief", output.read_text())
            self.assertEqual(result.stderr, "")
            self.assertEqual(
                self._read_lock(project)["managed"]["AGENTS.md"]["sha256"],
                _sha(output.read_bytes()),
            )

    def test_exact_match_without_baseline_adopts_silently(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            self._sync(project)
            self._clear_lock_managed(project)
            result = self._sync(project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stderr, "")
            self.assertEqual(
                self._read_lock(project)["managed"]["AGENTS.md"]["sha256"],
                _sha(output.read_bytes()),
            )

    def test_divergent_brief_without_baseline_is_preserved_without_adoption(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# stale generated-looking text\n"
            output.write_bytes(original)
            result = self._sync(project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(output.read_bytes(), original)
            self.assertNotIn("AGENTS.md", self._read_lock(project).get("managed", {}))
            self.assertIn("untracked", result.stderr)

    def test_explicit_adoption_records_current_bytes(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# intentionally adopted brief\n"
            output.write_bytes(original)
            result = self._sync(project, adopt=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(output.read_bytes(), original)
            self.assertEqual(
                self._read_lock(project)["managed"]["AGENTS.md"]["sha256"],
                _sha(original),
            )

    def test_marker_is_unconditional_even_without_legacy_flag(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# mine\n<!-- ai-specs:runtime-brief -->\n"
            output.write_bytes(original)
            result = self._sync(project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(output.read_bytes(), original)

    def test_managed_current_is_a_silent_noop(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            self._sync(project)
            before_lock = self._lock_path(project).read_bytes()
            result = self._sync(project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stderr, "")
            self.assertEqual(self._lock_path(project).read_bytes(), before_lock)

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
            result = self._cli(project, "sync-agent", "--all")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(child.read_bytes(), b"# child instructions\n")
            combined = result.stdout + result.stderr
            self.assertIn("--adopt-brief", combined)
            self.assertIn("ai-specs:runtime-brief", combined)

    def test_doctor_reports_untracked_brief_with_both_remedies(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            output.write_bytes(b"# hand-written instructions\n")
            result = self._cli(project, "doctor")
            combined = result.stdout + result.stderr
            self.assertIn("untracked", combined)
            self.assertIn("--adopt-brief", combined)
            self.assertIn("runtime-brief", combined)

    def test_init_preserves_pre_existing_brief_without_marker(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            original = b"# written before ai-specs\n"
            (project / "AGENTS.md").write_bytes(original)
            result = self._cli(project, "init")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((project / "AGENTS.md").read_bytes(), original)
            self.assertIn("--adopt-brief", result.stdout + result.stderr)

    def test_explicit_adoption_via_sync_cli_records_untracked_brief(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            original = b"# written before ai-specs\n"
            (project / "AGENTS.md").write_bytes(original)
            init = self._cli(project, "init")
            self.assertEqual(init.returncode, 0, init.stderr)
            self.assertNotIn("AGENTS.md", self._read_lock(project).get("managed", {}))
            result = self._sync(project, adopt=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual((project / "AGENTS.md").read_bytes(), original)
            self.assertIn("AGENTS.md", self._read_lock(project)["managed"])

    def test_unreadable_lock_preserves_without_traceback(self):
        # TRIAGE: coupled to lib/_internal/agents-render.py.
        # (1) Specific assertion: render() returns "preserved", leaves the brief
        #     bytes untouched, and reports a "preserv*" message when the lock
        #     file is unparseable.
        # (2) Exact command run: `bin/ai-specs sync <project>` with an invalid
        #     ai-specs/.ai-specs.lock exits 1 with a Traceback
        #     (refresh-bundled.py → load_lock) — the CLI treats a corrupt lock
        #     as a hard error, so no render-level preserve path is reachable.
        #     That actual behavior is frozen by the companion test
        #     test_invalid_lock_fails_sync_loudly_and_preserves_brief.
        # (3) What it did not expose: the renderer's tolerant
        #     "preserved + preserve-message" fallback for an unreadable lock.
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# keep this\n"
            output.write_bytes(original)
            self._lock_path(project).write_text("not = [valid\n")
            stderr = io.StringIO()
            with patch("sys.stderr", stderr):
                state = _render_coupled(toml, output)
            self.assertEqual(state, "preserved")
            self.assertEqual(output.read_bytes(), original)
            self.assertIn("preserv", stderr.getvalue().lower())

    def test_unreadable_target_preserves_without_traceback(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# keep this\n"
            output.write_bytes(original)
            self._sync(project)
            output.chmod(0o000)
            result = self._sync(project)
            self.assertEqual(result.returncode, 0, result.stderr)
            output.chmod(0o644)
            self.assertEqual(output.read_bytes(), original)
            self.assertIn("undetermined", result.stderr)

    def test_invalid_lock_fails_sync_loudly_and_preserves_brief(self):
        # CLI-observable behavior behind the TRIAGE case above: a corrupt lock
        # is a hard sync failure (Traceback), never a silent preserve.
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            original = b"# keep this\n"
            output.write_bytes(original)
            self._lock_path(project).write_text("not = [valid\n")
            result = self._sync(project)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("Traceback", result.stderr)
            self.assertEqual(output.read_bytes(), original)


class BriefRecoveryTests(RuntimeBriefOwnershipTests):
    """Judgment-day round one: the documented remedy must actually work."""

    def test_adopt_brief_works_for_user_modified(self):
        """JD C1 (both judges): the preserve message, doctor guidance and the
        troubleshooting doc all tell the user to run `ai-specs sync
        --adopt-brief`. The user_modified branch never inspected the flag, so
        the documented remedy silently did nothing — forever.

        Design D3 forbids AUTOMATIC updates of user_modified. It does not
        forbid an explicit user-issued handoff; D6 says --adopt-brief is safe
        precisely because the user issues it.
        """
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            self._sync(project)  # now managed
            edited = output.read_bytes() + b"\nmy own section\n"
            output.write_bytes(edited)

            result = self._sync(project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                output.read_bytes(), edited, "fixture precondition: preserved"
            )
            self.assertIn("user_modified", result.stderr)

            adopt = self._sync(project, adopt=True)
            self.assertEqual(adopt.returncode, 0, adopt.stderr)
            self.assertEqual(
                output.read_bytes(), edited,
                "adoption must keep the user's bytes, never overwrite them",
            )
            self.assertEqual(
                self._read_lock(project)["managed"]["AGENTS.md"]["sha256"],
                _sha(edited),
            )

    def test_interrupted_write_self_heals(self):
        """JD (judge B): write_bytes runs before set_brief_baseline. A crash
        between them leaves disk ahead of the lock, so the next sync classifies
        an ordinary never-edited brief as user_modified and preserves it
        forever — with no working recovery, since --adopt-brief was dead for
        that state.

        Content byte-identical to what we would write is provably ours, so
        re-recording the baseline is safe: it can never adopt foreign content.
        """
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            self._sync(project)
            self._corrupt_lock_sha(project, "0" * 64)  # crash before record

            result = self._sync(project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                self._read_lock(project)["managed"]["AGENTS.md"]["sha256"],
                _sha(output.read_bytes()),
                "an unmodified brief stayed stuck after an interrupted write",
            )

    def test_adopt_gate_agrees_with_the_classifier_on_line_endings(self):
        """JD (judge A): the classifier normalizes CRLF before hashing, the
        adopt gate compared raw bytes. A CRLF checkout of our own output was
        therefore called divergent and preserved — hitting exactly the
        no-regression cohort the migration rule exists to protect.
        """
        with tempfile.TemporaryDirectory() as tmp:
            project, toml, output = self._project(Path(tmp))
            self._sync(project)
            rendered = output.read_bytes()
            self._clear_lock_managed(project)  # no baseline: first sight
            output.write_bytes(rendered.replace(b"\n", b"\r\n"))

            result = self._sync(project)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(result.stderr, "")
            self.assertEqual(
                self._read_lock(project)["managed"]["AGENTS.md"]["sha256"],
                _sha(output.read_bytes()),
                "a CRLF checkout of our own output was not adopted: "
                + result.stderr,
            )


if __name__ == "__main__":
    unittest.main()
