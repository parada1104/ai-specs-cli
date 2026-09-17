"""RED/GREEN tests for repository-native canonical spec promotion (W3).

The promoter composes file-backed delta specs
(``openspec/changes/<slug>/specs/<domain>/spec.md``) into canonical specs
(``openspec/specs/<domain>/spec.md``) using the existing SDD archive contract:
ADDED appends, MODIFIED replaces by exact requirement name, unrelated canonical
requirements and sections survive, REMOVED is destructive and needs the explicit
safe path, and RENAMED is unsupported.
"""

from __future__ import annotations

import importlib.util
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "lib" / "_internal" / "spec_promotion.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _requirement(name: str, body: str, scenario: str = "behaviour") -> str:
    return (
        f"### Requirement: {name}\n\n"
        f"{body}\n\n"
        f"#### Scenario: {scenario}\n\n"
        f"- **WHEN** {scenario}\n"
        f"- **THEN** {scenario}\n"
    )


def _canonical(*blocks: str, purpose: str = "Existing purpose.") -> str:
    body = "\n".join(blocks)
    return (
        "# capability Specification\n\n"
        f"## Purpose\n\n{purpose}\n\n"
        f"## Requirements\n\n{body}"
    )


EXISTING = _requirement("Existing requirement", "Existing body.")
UNTOUCHED = _requirement("Untouched requirement", "Untouched body.")
ADDED = _requirement("Added requirement", "Added body.")


def _delta(*sections: str) -> str:
    return "# Delta for capability\n\n" + "\n".join(sections)


def added_section(*blocks: str) -> str:
    return "## ADDED Requirements\n\n" + "\n".join(blocks)


def modified_section(*blocks: str) -> str:
    return "## MODIFIED Requirements\n\n" + "\n".join(blocks)


def removed_section(*blocks: str) -> str:
    return "## REMOVED Requirements\n\n" + "\n".join(blocks)


class SpecPromotionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(MODULE_PATH, "spec_promotion_test")

    def _repo(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "openspec" / "changes" / "archive").mkdir(parents=True)
        (root / "openspec" / "specs").mkdir(parents=True)
        return root

    def _change(self, root: Path, slug: str, domain: str, delta_text: str) -> Path:
        folder = root / "openspec" / "changes" / slug
        specs = folder / "specs" / domain
        specs.mkdir(parents=True, exist_ok=True)
        (specs / "spec.md").write_text(delta_text, encoding="utf-8")
        return folder

    def _canonical_path(self, root: Path, domain: str = "capability") -> Path:
        return root / "openspec" / "specs" / domain / "spec.md"

    def _write_canonical(self, root: Path, text: str, domain: str = "capability") -> Path:
        path = self._canonical_path(root, domain)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
        return path

    def _promote(self, root: Path, slug: str, **kwargs):
        report = self.mod.promote_change(root, slug, **kwargs)
        self.assertEqual(report.blockers, [], report.blockers)
        return report

    # --- composition -----------------------------------------------------

    def test_added_requirement_is_appended_into_existing_canonical(self):
        root = self._repo()
        self._write_canonical(root, _canonical(EXISTING))
        self._change(root, "add-spec", "capability", _delta(added_section(ADDED)))

        report = self._promote(root, "add-spec")

        text = self._canonical_path(root).read_text(encoding="utf-8")
        self.assertIn("### Requirement: Added requirement", text)
        self.assertIn("Added body.", text)
        self.assertIn("Existing body.", text)
        self.assertTrue(report.changed)
        self.assertEqual(self.mod.check_folder_parity(root, report.folder), [])

    def test_modified_requirement_replaces_canonical_block(self):
        root = self._repo()
        self._write_canonical(root, _canonical(EXISTING, UNTOUCHED))
        replacement = _requirement("Existing requirement", "Rewritten body.", "rewritten")
        self._change(root, "mod-spec", "capability", _delta(modified_section(replacement)))

        self._promote(root, "mod-spec")

        text = self._canonical_path(root).read_text(encoding="utf-8")
        self.assertIn("Rewritten body.", text)
        self.assertNotIn("Existing body.", text)
        self.assertIn("Untouched body.", text)
        self.assertEqual(text.count("### Requirement: Existing requirement"), 1)

    def test_new_domain_creates_a_canonical_spec(self):
        root = self._repo()
        self._change(root, "new-domain", "fresh-domain", _delta(added_section(ADDED)))

        self._promote(root, "new-domain")

        path = self._canonical_path(root, "fresh-domain")
        self.assertTrue(path.is_file())
        text = path.read_text(encoding="utf-8")
        self.assertIn("### Requirement: Added requirement", text)
        self.assertIn("## Requirements", text)
        # The delta scaffolding is never copied verbatim into canonical form.
        self.assertNotIn("## ADDED Requirements", text)
        self.assertNotIn("# Delta for", text)
        self.assertEqual(text.count("### Requirement: Added requirement"), 1)

    def test_rerun_is_idempotent_and_resume_safe(self):
        root = self._repo()
        canonical = self._write_canonical(root, _canonical(EXISTING))
        self._change(root, "rerun", "capability", _delta(added_section(ADDED)))

        self._promote(root, "rerun")
        after_first = canonical.read_text(encoding="utf-8")

        second = self.mod.promote_change(root, "rerun")

        self.assertEqual(second.blockers, [], second.blockers)
        self.assertFalse(second.changed)
        self.assertEqual(canonical.read_text(encoding="utf-8"), after_first)
        self.assertEqual(self.mod.check_folder_parity(root, second.folder), [])
        leftovers = [p.name for p in canonical.parent.iterdir() if p.name != "spec.md"]
        self.assertEqual(leftovers, [])

    def test_unrelated_canonical_requirements_and_sections_are_preserved(self):
        root = self._repo()
        canonical = self._write_canonical(
            root, _canonical(EXISTING, UNTOUCHED) + "\n\n## Notes\n\nCanonical note.\n"
        )
        self._change(root, "preserve", "capability", _delta(added_section(ADDED)))

        self._promote(root, "preserve")

        text = canonical.read_text(encoding="utf-8")
        self.assertIn("Untouched body.", text)
        self.assertIn("## Notes", text)
        self.assertIn("Canonical note.", text)
        self.assertIn("# capability Specification", text)

    # --- refusal paths ---------------------------------------------------

    def test_added_collision_with_different_content_is_rejected(self):
        root = self._repo()
        canonical = self._write_canonical(root, _canonical(EXISTING))
        before = canonical.read_text(encoding="utf-8")
        colliding = _requirement("Existing requirement", "Conflicting body.")
        self._change(root, "collision", "capability", _delta(added_section(colliding)))

        report = self.mod.promote_change(root, "collision")

        self.assertTrue(report.blockers)
        self.assertTrue(any("collides" in b for b in report.blockers), report.blockers)
        self.assertEqual(canonical.read_text(encoding="utf-8"), before)

    def test_missing_modified_target_is_rejected(self):
        root = self._repo()
        canonical = self._write_canonical(root, _canonical(EXISTING))
        before = canonical.read_text(encoding="utf-8")
        orphan = _requirement("Absent requirement", "New body.")
        self._change(root, "orphan-mod", "capability", _delta(modified_section(orphan)))

        report = self.mod.promote_change(root, "orphan-mod")

        self.assertTrue(report.blockers)
        self.assertTrue(
            any("MODIFIED" in b or "modified" in b.lower() for b in report.blockers),
            report.blockers,
        )
        self.assertEqual(canonical.read_text(encoding="utf-8"), before)

    def test_renamed_operation_is_unsupported(self):
        root = self._repo()
        canonical = self._write_canonical(root, _canonical(EXISTING))
        before = canonical.read_text(encoding="utf-8")
        renamed = "## RENAMED Requirements\n\n- FROM: `### Requirement: Existing requirement`\n  TO: `### Requirement: Renamed requirement`\n"
        self._change(root, "renamed", "capability", _delta(renamed))

        report = self.mod.promote_change(root, "renamed")

        self.assertTrue(report.blockers)
        self.assertTrue(any("RENAMED" in b for b in report.blockers), report.blockers)
        self.assertEqual(canonical.read_text(encoding="utf-8"), before)

    def test_removed_is_refused_without_the_explicit_safe_path(self):
        root = self._repo()
        canonical = self._write_canonical(root, _canonical(EXISTING, UNTOUCHED))
        before = canonical.read_text(encoding="utf-8")
        self._change(
            root, "destructive", "capability", _delta(removed_section(EXISTING))
        )

        report = self.mod.promote_change(root, "destructive")

        self.assertTrue(report.blockers)
        self.assertTrue(
            any("REMOVED" in b or "removed" in b.lower() for b in report.blockers),
            report.blockers,
        )
        self.assertEqual(canonical.read_text(encoding="utf-8"), before)

        allowed = self.mod.promote_change(root, "destructive", allow_removed=True)
        self.assertEqual(allowed.blockers, [], allowed.blockers)
        text = canonical.read_text(encoding="utf-8")
        self.assertNotIn("### Requirement: Existing requirement", text)
        self.assertIn("Untouched body.", text)

    def test_one_blocked_domain_aborts_every_write(self):
        root = self._repo()
        canonical = self._write_canonical(root, _canonical(EXISTING))
        before = canonical.read_text(encoding="utf-8")
        self._change(root, "atomic", "capability", _delta(added_section(ADDED)))
        self._change(
            root,
            "atomic",
            "other-domain",
            _delta(modified_section(_requirement("Absent requirement", "body"))),
        )

        report = self.mod.promote_change(root, "atomic")

        self.assertTrue(report.blockers)
        self.assertEqual(canonical.read_text(encoding="utf-8"), before)
        self.assertFalse(self._canonical_path(root, "other-domain").exists())

    # --- active same-domain collisions -----------------------------------

    def test_active_same_domain_collision_blocks_before_any_write(self):
        """Another active change on the same domain stops promotion, unwritten."""
        root = self._repo()
        canonical = self._write_canonical(root, _canonical(EXISTING))
        before = canonical.read_text(encoding="utf-8")
        self._change(root, "selected", "capability", _delta(added_section(ADDED)))
        self._change(root, "sibling", "capability", _delta(added_section(ADDED)))

        report = self.mod.promote_change(root, "selected")

        self.assertTrue(report.blockers)
        self.assertTrue(any("sibling" in b for b in report.blockers), report.blockers)
        self.assertTrue(any("capability" in b for b in report.blockers), report.blockers)
        self.assertEqual(canonical.read_text(encoding="utf-8"), before)

        blocked = subprocess.run(
            [sys.executable, str(MODULE_PATH), "selected", "--root", str(root)],
            capture_output=True,
            text=True,
        )
        self.assertEqual(blocked.returncode, 1)
        self.assertIn("sibling", blocked.stderr)
        self.assertEqual(canonical.read_text(encoding="utf-8"), before)

    def test_active_collision_on_one_domain_blocks_every_domain(self):
        root = self._repo()
        canonical = self._write_canonical(root, _canonical(EXISTING))
        before = canonical.read_text(encoding="utf-8")
        self._change(root, "selected", "capability", _delta(added_section(ADDED)))
        self._change(root, "selected", "clean-domain", _delta(added_section(ADDED)))
        self._change(root, "sibling", "capability", _delta(added_section(ADDED)))

        report = self.mod.promote_change(root, "selected")

        self.assertTrue(report.blockers)
        self.assertEqual(canonical.read_text(encoding="utf-8"), before)
        self.assertFalse(self._canonical_path(root, "clean-domain").exists())

    def test_archived_unrelated_and_spec_less_changes_do_not_collide(self):
        root = self._repo()
        canonical = self._write_canonical(root, _canonical(EXISTING))
        self._change(root, "selected", "capability", _delta(added_section(ADDED)))
        # A closed change on the same domain is history, not a competing order.
        archived = (
            root / "openspec" / "changes" / "archive" / "closed" / "specs" / "capability"
        )
        archived.mkdir(parents=True)
        (archived / "spec.md").write_text(_delta(added_section(ADDED)), encoding="utf-8")
        # A sibling on another domain and a sibling with no deltas are unrelated.
        self._change(root, "other-domain", "elsewhere", _delta(added_section(ADDED)))
        task_only = root / "openspec" / "changes" / "task-only"
        task_only.mkdir(parents=True)
        (task_only / "tasks.md").write_text("Depth: light\n", encoding="utf-8")

        report = self.mod.promote_change(root, "selected")

        self.assertEqual(report.blockers, [], report.blockers)
        self.assertIn("Added body.", canonical.read_text(encoding="utf-8"))

    def test_symlinked_active_change_is_not_followed_by_the_collision_scan(self):
        root = self._repo()
        outside_tmp = tempfile.TemporaryDirectory()
        self.addCleanup(outside_tmp.cleanup)
        outside_change = Path(outside_tmp.name) / "outside-change"
        outside_delta = outside_change / "specs" / "capability" / "spec.md"
        outside_delta.parent.mkdir(parents=True)
        outside_delta.write_text(_delta(added_section(ADDED)), encoding="utf-8")
        before = outside_delta.read_text(encoding="utf-8")
        (root / "openspec" / "changes" / "linked").symlink_to(
            outside_change, target_is_directory=True
        )
        self._change(root, "selected", "capability", _delta(added_section(ADDED)))

        report = self.mod.promote_change(root, "selected")

        self.assertEqual(report.blockers, [], report.blockers)
        self.assertTrue(self._canonical_path(root).is_file())
        self.assertEqual(outside_delta.read_text(encoding="utf-8"), before)

    # --- boundaries ------------------------------------------------------

    def test_slug_traversal_and_root_escape_are_rejected(self):
        root = self._repo()
        outside_tmp = tempfile.TemporaryDirectory()
        self.addCleanup(outside_tmp.cleanup)
        outside = Path(outside_tmp.name) / "outside-canonical"
        for slug in ("../escape", "escape/../..", "/abs"):
            report = self.mod.promote_change(root, slug)
            self.assertTrue(report.blockers, slug)
        self.assertFalse(outside.exists())
        self.assertTrue((Path(outside_tmp.name)).is_dir())

    def test_non_planning_root_is_rejected(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        report = self.mod.promote_change(Path(tmp.name), "any-slug")
        self.assertTrue(report.blockers)

    def test_symlinked_canonical_domain_is_rejected(self):
        root = self._repo()
        outside_tmp = tempfile.TemporaryDirectory()
        self.addCleanup(outside_tmp.cleanup)
        outside = Path(outside_tmp.name) / "outside-specs"
        outside.mkdir()
        specs = root / "openspec" / "specs"
        (specs / "capability").symlink_to(outside, target_is_directory=True)
        self._change(root, "symlink", "capability", _delta(added_section(ADDED)))

        report = self.mod.promote_change(root, "symlink")

        self.assertTrue(report.blockers)
        self.assertEqual(list(outside.iterdir()), [])

    def test_symlinked_openspec_ancestor_is_rejected(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name) / "repo"
        root.mkdir()
        outside_tmp = tempfile.TemporaryDirectory()
        self.addCleanup(outside_tmp.cleanup)
        outside = Path(outside_tmp.name) / "outside-openspec"
        (outside / "specs").mkdir(parents=True)
        (root / "openspec").symlink_to(outside, target_is_directory=True)
        self._change(root, "symlink-ancestor", "capability", _delta(added_section(ADDED)))

        report = self.mod.promote_change(root, "symlink-ancestor")

        self.assertTrue(report.blockers, report.blockers)
        self.assertEqual(list((outside / "specs").iterdir()), [])

    def test_missing_specs_directory_is_a_no_op(self):
        root = self._repo()
        folder = root / "openspec" / "changes" / "odd-task-only"
        folder.mkdir(parents=True)
        (folder / "tasks.md").write_text("Depth: light\n", encoding="utf-8")

        report = self.mod.promote_change(root, "odd-task-only")

        self.assertEqual(report.blockers, [])
        self.assertFalse(report.changed)
        self.assertEqual(self.mod.check_folder_parity(root, folder), [])
        self.assertEqual(list((root / "openspec" / "specs").iterdir()), [])

    def test_promoter_has_no_network_surface(self):
        source = MODULE_PATH.read_text(encoding="utf-8")
        for forbidden in ("urllib", "socket", "requests", "http.client", "subprocess"):
            self.assertNotIn(forbidden, source)


class SpecPromotionCliTests(unittest.TestCase):
    def _repo(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "openspec" / "changes" / "archive").mkdir(parents=True)
        (root / "openspec" / "specs").mkdir(parents=True)
        return root

    def test_cli_promotes_with_a_required_root_and_slug(self):
        root = self._repo()
        folder = root / "openspec" / "changes" / "cli-change" / "specs" / "capability"
        folder.mkdir(parents=True)
        (folder / "spec.md").write_text(_delta(added_section(ADDED)), encoding="utf-8")

        run = subprocess.run(
            [sys.executable, str(MODULE_PATH), "cli-change", "--root", str(root)],
            capture_output=True,
            text=True,
        )

        self.assertEqual(run.returncode, 0, run.stderr)
        self.assertTrue((root / "openspec" / "specs" / "capability" / "spec.md").is_file())

        rerun = subprocess.run(
            [sys.executable, str(MODULE_PATH), "cli-change", "--root", str(root)],
            capture_output=True,
            text=True,
        )
        self.assertEqual(rerun.returncode, 0, rerun.stderr)
        self.assertIn("already promoted", rerun.stdout)

    def test_cli_requires_the_root_flag(self):
        run = subprocess.run(
            [sys.executable, str(MODULE_PATH), "cli-change"],
            capture_output=True,
            text=True,
        )
        self.assertEqual(run.returncode, 2)
        self.assertIn("--root", run.stderr)

    def test_cli_exits_nonzero_when_promotion_is_blocked(self):
        root = self._repo()
        canonical = root / "openspec" / "specs" / "capability" / "spec.md"
        canonical.parent.mkdir(parents=True)
        canonical.write_text(_canonical(EXISTING), encoding="utf-8")
        folder = root / "openspec" / "changes" / "cli-block" / "specs" / "capability"
        folder.mkdir(parents=True)
        (folder / "spec.md").write_text(
            _delta(added_section(_requirement("Existing requirement", "Conflicting body."))),
            encoding="utf-8",
        )

        run = subprocess.run(
            [sys.executable, str(MODULE_PATH), "cli-block", "--root", str(root)],
            capture_output=True,
            text=True,
        )

        self.assertEqual(run.returncode, 1)
        self.assertIn("BLOCKED", run.stderr)


if __name__ == "__main__":
    unittest.main()
