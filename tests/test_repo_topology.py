"""Shared git fixtures for submodule topology tests.

The behavior tests that used to live here (detect_submodules,
resolve_repo_topology, resolve_request_context, resolve_subrepo,
override_is_stale, project_repo_topology) are retired with GO-20 U2e T5: every
surviving one is now asserted natively (internal/target, the worktree gate's
proof-based owner classification, internal/sync ProjectOwnedRecipeConfig), and
util.py is deleted with lib/.

What remains is the real-git fixture three kept suites import —
``make_super_with_submodule`` in ``test_worktree_cleanup``, ``test_hub`` and
``test_agents_render_brief_fragments`` — plus the two fixture self-tests that
keep its ``git submodule status`` contract honest.
"""

from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


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


if __name__ == "__main__":
    unittest.main()
