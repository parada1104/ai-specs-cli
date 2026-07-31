"""Unit tests for util.py repo-topology helpers (worktree-flow-repo-topology).

Mirrors the temp-repo-via-subprocess fixture style used by
``tests/test_worktree_cleanup.py``.
"""

from __future__ import annotations

import hashlib
import importlib.util
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
UTIL_PATH = ROOT / "lib" / "_internal" / "util.py"


def git(repo: Path, *args: str) -> str:
    env = dict(os.environ)
    env.update(
        {
            "GIT_AUTHOR_NAME": "t",
            "GIT_AUTHOR_EMAIL": "t@t",
            "GIT_COMMITTER_NAME": "t",
            "GIT_COMMITTER_EMAIL": "t@t",
        }
    )
    return subprocess.run(
        ["git", *args],
        cwd=repo,
        env=env,
        check=True,
        capture_output=True,
        text=True,
    ).stdout


def _load_util():
    spec = importlib.util.spec_from_file_location("util_repo_topology", UTIL_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _make_bare_submodule_source(tmp: Path, *, label: str = "sub") -> Path:
    """Create a bare repo suitable as a ``git submodule add`` URL."""
    tmp.mkdir(parents=True, exist_ok=True)
    src = tmp / f"{label}-src"
    src.mkdir()
    git(src, "init", "-q", "-b", "main")
    (src / "README.md").write_text(f"{label}\n")
    git(src, "add", "-A")
    git(src, "commit", "-qm", f"init {label}")
    bare = tmp / f"{label}.git"
    subprocess.run(
        ["git", "clone", "-q", "--bare", str(src), str(bare)],
        check=True,
        capture_output=True,
        text=True,
    )
    return bare


def make_super_with_submodule(
    tmp: Path,
    *,
    initialized: bool = True,
    path: str = "apps/api",
    name: str | None = None,
    second: dict | None = None,
) -> Path:
    """Build a superproject with one (optionally two) submodule entries.

    Returns the superproject root. When ``initialized`` is False the primary
    submodule is deinit'd so ``git submodule status`` shows a ``-`` prefix.
    ``second`` may be ``{"path": "...", "name": "...", "initialized": True}``.
    """
    super_repo = tmp / "super"
    super_repo.mkdir(parents=True)
    git(super_repo, "init", "-q", "-b", "main")
    (super_repo / "README.md").write_text("super\n")
    (super_repo / ".gitignore").write_text(".worktrees/\n")
    git(super_repo, "add", "-A")
    git(super_repo, "commit", "-qm", "init super")

    bare = _make_bare_submodule_source(tmp, label="primary")
    add_args = ["-c", "protocol.file.allow=always", "submodule", "add"]
    if name is not None:
        add_args.extend(["--name", name])
    add_args.extend([str(bare), path])
    git(super_repo, *add_args)
    git(super_repo, "commit", "-qm", f"add submodule {path}")

    if second is not None:
        bare2 = _make_bare_submodule_source(tmp, label="second")
        add2 = ["-c", "protocol.file.allow=always", "submodule", "add"]
        if second.get("name"):
            add2.extend(["--name", second["name"]])
        add2.extend([str(bare2), second["path"]])
        git(super_repo, *add2)
        git(super_repo, "commit", "-qm", f"add submodule {second['path']}")
        if second.get("initialized", True) is False:
            git(super_repo, "submodule", "deinit", "-f", second["path"])

    if not initialized:
        git(super_repo, "submodule", "deinit", "-f", path)

    return super_repo


class SubmoduleFixtureTests(unittest.TestCase):
    """1.1 — shared fixture produces .gitmodules + expected status prefix."""

    def test_fixture_initialized_has_space_prefix(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), initialized=True)
        self.assertTrue((super_repo / ".gitmodules").is_file())
        status = git(super_repo, "submodule", "status")
        self.assertTrue(
            status.startswith(" "),
            f"expected leading space for initialized, got {status!r}",
        )
        self.assertIn("apps/api", status)

    def test_fixture_uninitialized_has_dash_prefix(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), initialized=False)
        self.assertTrue((super_repo / ".gitmodules").is_file())
        status = git(super_repo, "submodule", "status")
        self.assertTrue(
            status.startswith("-"),
            f"expected leading '-' for uninitialized, got {status!r}",
        )


class DetectSubmodulesTests(unittest.TestCase):
    """1.2 — detect_submodules unit tests."""

    @classmethod
    def setUpClass(cls):
        cls.util = _load_util()

    def test_space_plus_u_count_as_initialized(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), path="apps/api")
        # '+' prefix: commit inside the submodule without updating the super index
        git(super_repo / "apps/api", "commit", "--allow-empty", "-qm", "ahead")
        status = git(super_repo, "submodule", "status")
        self.assertTrue(status.startswith("+"), status)

        present, paths = self.util.detect_submodules(super_repo)
        self.assertTrue(present)
        self.assertEqual(paths, ("apps/api",))

        # 'U' prefix via mocked status (hard to force a real merge conflict)
        with mock.patch.object(
            self.util,
            "_run_submodule_status",
            return_value=["Udeadbeef apps/api (heads/main)"],
        ):
            present_u, paths_u = self.util.detect_submodules(super_repo)
        self.assertTrue(present_u)
        self.assertEqual(paths_u, ("apps/api",))

    def test_dash_prefix_skipped(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), initialized=False)
        present, paths = self.util.detect_submodules(super_repo)
        self.assertTrue(present)
        self.assertEqual(paths, ())

    def test_no_gitmodules_returns_false_empty(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        repo = Path(tmp.name) / "repo"
        repo.mkdir()
        git(repo, "init", "-q", "-b", "main")
        (repo / "README.md").write_text("x\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-qm", "init")
        present, paths = self.util.detect_submodules(repo)
        self.assertEqual((present, paths), (False, ()))

    def test_path_in_gitmodules_missing_from_status_ignored(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), path="apps/api")
        # Inject a phantom .gitmodules path that status will never list.
        gm = (super_repo / ".gitmodules").read_text()
        gm += '\n[submodule "ghost"]\n\tpath = apps/ghost\n\turl = ../ghost.git\n'
        (super_repo / ".gitmodules").write_text(gm)
        present, paths = self.util.detect_submodules(super_repo)
        self.assertTrue(present)
        self.assertEqual(paths, ("apps/api",))
        self.assertNotIn("apps/ghost", paths)

    def test_name_ne_path_returns_path(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(
            Path(tmp.name), path="apps/api", name="api-service"
        )
        present, paths = self.util.detect_submodules(super_repo)
        self.assertTrue(present)
        self.assertEqual(paths, ("apps/api",))
        self.assertNotIn("api-service", paths)


class ResolveRepoTopologyTests(unittest.TestCase):
    """1.3 — resolve_repo_topology unit tests."""

    @classmethod
    def setUpClass(cls):
        cls.util = _load_util()

    def test_auto_with_initialized_submodules(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name))
        res = self.util.resolve_repo_topology(super_repo, "auto")
        self.assertEqual(res.resolved, "monorepo-submodules")
        self.assertEqual(res.configured, "auto")
        self.assertEqual(res.via, "auto")
        self.assertEqual(res.submodules, ("apps/api",))
        self.assertTrue(res.gitmodules_present)

    def test_auto_only_uninitialized_or_no_gitmodules_is_standalone(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_u = make_super_with_submodule(Path(tmp.name) / "u", initialized=False)
        res_u = self.util.resolve_repo_topology(super_u, "auto")
        self.assertEqual(res_u.resolved, "standalone")
        self.assertEqual(res_u.via, "auto")
        self.assertEqual(res_u.submodules, ())

        bare = Path(tmp.name) / "bare-standalone"
        bare.mkdir()
        git(bare, "init", "-q", "-b", "main")
        (bare / "README.md").write_text("x\n")
        git(bare, "add", "-A")
        git(bare, "commit", "-qm", "init")
        res_s = self.util.resolve_repo_topology(bare, "auto")
        self.assertEqual(res_s.resolved, "standalone")
        self.assertEqual(res_s.via, "auto")
        self.assertFalse(res_s.gitmodules_present)

    def test_auto_never_returns_monorepo_apps(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        with_sub = make_super_with_submodule(Path(tmp.name) / "a")
        without = Path(tmp.name) / "b"
        without.mkdir()
        git(without, "init", "-q", "-b", "main")
        (without / "README.md").write_text("x\n")
        git(without, "add", "-A")
        git(without, "commit", "-qm", "init")
        for root in (with_sub, without):
            res = self.util.resolve_repo_topology(root, "auto")
            self.assertNotEqual(res.resolved, "monorepo-apps")

    def test_explicit_bypass_detection(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name))

        for value in ("standalone", "monorepo-apps"):
            res = self.util.resolve_repo_topology(super_repo, value)
            self.assertEqual(res.resolved, value)
            self.assertEqual(res.configured, value)
            self.assertEqual(res.via, "config")
            self.assertEqual(res.submodules, ())
            self.assertFalse(res.gitmodules_present)

        res_sub = self.util.resolve_repo_topology(super_repo, "monorepo-submodules")
        self.assertEqual(res_sub.resolved, "monorepo-submodules")
        self.assertEqual(res_sub.via, "config")
        self.assertEqual(res_sub.submodules, ("apps/api",))

    def test_absent_or_empty_config_treated_as_auto(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name))
        for value in ("", None, "   "):
            res = self.util.resolve_repo_topology(super_repo, value)  # type: ignore[arg-type]
            self.assertEqual(res.configured, "auto")
            self.assertEqual(res.via, "auto")
            self.assertEqual(res.resolved, "monorepo-submodules")

    def test_git_missing_or_not_a_repo_degrades_to_standalone(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        not_repo = Path(tmp.name) / "plain"
        not_repo.mkdir()
        (not_repo / ".gitmodules").write_text(
            '[submodule "x"]\n\tpath = x\n\turl = ./x\n'
        )
        res = self.util.resolve_repo_topology(not_repo, "auto")
        self.assertEqual(res.resolved, "standalone")
        self.assertEqual(res.submodules, ())

        with mock.patch.object(
            self.util.subprocess,
            "run",
            side_effect=FileNotFoundError("git"),
        ):
            # .gitmodules present so detect_submodules enters git calls
            res2 = self.util.resolve_repo_topology(not_repo, "auto")
        self.assertEqual(res2.resolved, "standalone")


class OverrideIsStaleTests(unittest.TestCase):
    """1.4 — override_is_stale unit tests."""

    @classmethod
    def setUpClass(cls):
        cls.util = _load_util()

    def test_missing_dest_false(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        src = Path(tmp.name) / "src.sh"
        src.write_text("hello\n")
        dest = Path(tmp.name) / "missing.sh"
        self.assertFalse(self.util.override_is_stale(src, dest))

    def test_missing_catalog_src_false(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        dest = Path(tmp.name) / "dest.sh"
        dest.write_text("hello\n")
        src = Path(tmp.name) / "missing.sh"
        self.assertFalse(self.util.override_is_stale(src, dest))

    def test_identical_bytes_false(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        src = Path(tmp.name) / "src.sh"
        dest = Path(tmp.name) / "dest.sh"
        payload = b"identical-bytes\n"
        src.write_bytes(payload)
        dest.write_bytes(payload)
        self.assertEqual(
            hashlib.sha256(src.read_bytes()).digest(),
            hashlib.sha256(dest.read_bytes()).digest(),
        )
        self.assertFalse(self.util.override_is_stale(src, dest))

    def test_divergent_bytes_true(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        src = Path(tmp.name) / "src.sh"
        dest = Path(tmp.name) / "dest.sh"
        src.write_text("catalog\n")
        dest.write_text("customized\n")
        self.assertTrue(self.util.override_is_stale(src, dest))


if __name__ == "__main__":
    unittest.main()
