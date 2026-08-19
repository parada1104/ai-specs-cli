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

from _blackbox import invoke, isolated_home


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



def _init_with_worktree_flow(repo: Path, cli_home: Path, *, topology: str | None = None) -> None:
    """Bootstrap ai-specs with worktree-flow in *repo* so hub/doctor emit topology."""
    invoke(repo, "init", "--name", "test", cli_home=cli_home)
    toml_path = repo / "ai-specs" / "ai-specs.toml"
    extra = "\n[recipes.worktree-flow]\nenabled = true\n"
    if topology is not None:
        extra += f'\n[recipes.worktree-flow.config]\nrepo_topology = "{topology}"\n'
    toml_path.write_text(toml_path.read_text() + extra)


def _topology_from_hub(repo: Path, cli_home: Path) -> str:
    """Return the raw 'topology: ...' line from hub output."""
    r = invoke(repo, "hub", cli_home=cli_home)
    for line in r.stdout.splitlines():
        if "topology:" in line:
            return line.strip()
    return ""


def _topology_from_doctor(repo: Path, cli_home: Path) -> str:
    """Return the raw repo-topology line from doctor output."""
    r = invoke(repo, "doctor", cli_home=cli_home)
    for line in r.stdout.splitlines():
        if "repo-topology" in line:
            return line.strip()
    return ""


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

    # TRIAGE: test_space_plus_u_count_as_initialized — asserts detect_submodules()
    # returns present=True, paths=("apps/api",) for "+" and "U" git-status prefixes.
    # Ran: bin/ai-specs hub and bin/ai-specs doctor — both show "topology: monorepo-
    # submodules (auto)" but do not expose the specific (present, paths) tuple or
    # the internal "+"/U" prefix parsing. The mock for "U" prefix requires
    # mock.patch on _run_submodule_status, not reachable via CLI.
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

    # TRIAGE: test_dash_prefix_skipped — asserts detect_submodules() returns
    # present=True, paths=() for uninitialized ("-" prefix) submodules.
    # Ran: bin/ai-specs doctor — shows "0 initialized submodule(s)" but the
    # present=True (vs False) distinction and empty paths tuple are internal.
    # doctor only reports the final topology, not the intermediate present flag.
    def test_dash_prefix_skipped(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), initialized=False)
        present, paths = self.util.detect_submodules(super_repo)
        self.assertTrue(present)
        self.assertEqual(paths, ())

    # TRIAGE: test_no_gitmodules_returns_false_empty — asserts detect_submodules()
    # returns (False, ()) when .gitmodules is absent. Ran: bin/ai-specs hub — shows
    # "topology: standalone (auto)" but does not expose present=False vs present=True
    # with empty paths. The distinction matters for error reporting in resolve_subrepo.
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

    # TRIAGE: test_path_in_gitmodules_missing_from_status_ignored — asserts
    # detect_submodules() excludes phantom .gitmodules entries not in git-status.
    # Ran: bin/ai-specs doctor — shows "1 initialized submodule(s)" but does not
    # list which paths were included or excluded. The assertion that "apps/ghost"
    # is NOT in the paths tuple requires internal access.
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

    # TRIAGE: test_name_ne_path_returns_path — asserts detect_submodules() returns
    # the filesystem path ("apps/api"), not the gitmodule name ("api-service").
    # Ran: bin/ai-specs doctor — shows submodule count but not individual paths.
    # The path-vs-name distinction requires inspecting the returned tuple.
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
    """1.3 — resolve_repo_topology black-box tests via ai-specs hub / doctor."""

    def _cli_home(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return isolated_home(Path(tmp.name))

    def test_auto_with_initialized_submodules(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = self._cli_home()
        super_repo = make_super_with_submodule(Path(tmp.name))
        _init_with_worktree_flow(super_repo, home)
        hub = _topology_from_hub(super_repo, home)
        self.assertIn("monorepo-submodules", hub)
        self.assertIn("auto", hub)
        doc = _topology_from_doctor(super_repo, home)
        self.assertIn("monorepo-submodules", doc)
        self.assertIn("via auto", doc)
        self.assertIn("1 initialized submodule", doc)

    def test_auto_only_uninitialized_or_no_gitmodules_is_standalone(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = self._cli_home()
        # Uninitialized submodule
        super_u = make_super_with_submodule(Path(tmp.name) / "u", initialized=False)
        _init_with_worktree_flow(super_u, home)
        hub_u = _topology_from_hub(super_u, home)
        self.assertIn("standalone", hub_u)
        self.assertIn("auto", hub_u)
        doc_u = _topology_from_doctor(super_u, home)
        self.assertIn("0 initialized submodule", doc_u)
        # No gitmodules at all
        bare = Path(tmp.name) / "bare-standalone"
        bare.mkdir()
        git(bare, "init", "-q", "-b", "main")
        (bare / "README.md").write_text("x\n")
        git(bare, "add", "-A")
        git(bare, "commit", "-qm", "init")
        _init_with_worktree_flow(bare, home)
        hub_s = _topology_from_hub(bare, home)
        self.assertIn("standalone", hub_s)
        self.assertIn("auto", hub_s)
        doc_s = _topology_from_doctor(bare, home)
        self.assertIn("0 initialized submodule", doc_s)

    def test_auto_never_returns_monorepo_apps(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = self._cli_home()
        with_sub = make_super_with_submodule(Path(tmp.name) / "a")
        without = Path(tmp.name) / "b"
        without.mkdir()
        git(without, "init", "-q", "-b", "main")
        (without / "README.md").write_text("x\n")
        git(without, "add", "-A")
        git(without, "commit", "-qm", "init")
        for root in (with_sub, without):
            _init_with_worktree_flow(root, home)
            hub = _topology_from_hub(root, home)
            self.assertNotIn("monorepo-apps", hub)

    def test_explicit_bypass_detection(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = self._cli_home()
        super_repo = make_super_with_submodule(Path(tmp.name))
        for value in ("standalone", "monorepo-apps"):
            _init_with_worktree_flow(super_repo, home, topology=value)
            hub = _topology_from_hub(super_repo, home)
            self.assertIn(value, hub)
            self.assertIn("config", hub)
            doc = _topology_from_doctor(super_repo, home)
            self.assertIn(value, doc)
            self.assertIn("via config", doc)
            # Remove ai-specs dir to re-init with next topology
            import shutil
            shutil.rmtree(super_repo / "ai-specs", ignore_errors=True)
        # monorepo-submodules explicit
        _init_with_worktree_flow(super_repo, home, topology="monorepo-submodules")
        hub = _topology_from_hub(super_repo, home)
        self.assertIn("monorepo-submodules", hub)
        self.assertIn("config", hub)
        doc = _topology_from_doctor(super_repo, home)
        self.assertIn("via config", doc)
        self.assertIn("1 initialized submodule", doc)

    def test_absent_or_empty_config_treated_as_auto(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = self._cli_home()
        super_repo = make_super_with_submodule(Path(tmp.name))
        # Default config (no topology key) = auto
        _init_with_worktree_flow(super_repo, home)
        hub = _topology_from_hub(super_repo, home)
        self.assertIn("auto", hub)
        self.assertIn("monorepo-submodules", hub)
        doc = _topology_from_doctor(super_repo, home)
        self.assertIn("via auto", doc)

    def test_git_missing_or_not_a_repo_degrades_to_standalone(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = self._cli_home()
        not_repo = Path(tmp.name) / "plain"
        not_repo.mkdir()
        _init_with_worktree_flow(not_repo, home)
        hub = _topology_from_hub(not_repo, home)
        self.assertIn("standalone", hub)
        doc = _topology_from_doctor(not_repo, home)
        self.assertIn("standalone", doc)
        self.assertIn("0 initialized submodule", doc)


class RequestContextTests(unittest.TestCase):
    """1.1 — RED: resolve_request_context owner/planning-root separation.

    Covers the worktree-flow "Request context owner and planning root
    separation" requirement: subrepo cwd owns the subrepo with the proven
    superrepo as planning root; a superrepo-context request cannot infer a
    subrepo and hard-errors before any create; detached/uninitialized/
    ambiguous topology fails safe with no planning-root exception.
    """

    @classmethod
    def setUpClass(cls):
        cls.util = _load_util()

    # TRIAGE: test_subrepo_cwd_owns_subrepo_with_super_planning_root — asserts
    # resolve_request_context(cwd=subrepo) returns owner_root=cwd, subrepo_path=
    # "apps/api", planning_root=super. Ran: bin/ai-specs hub <subrepo> — hub runs
    # from project root where ai-specs.toml lives, not subrepo cwd. The owner_root,
    # subrepo_path, planning_root, worktrees_dir fields are internal RequestContext
    # attributes not exposed in any CLI output.
    def test_subrepo_cwd_owns_subrepo_with_super_planning_root(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(
            Path(tmp.name), path="apps/api", name="api"
        )
        cwd = super_repo / "apps" / "api"
        ctx = self.util.resolve_request_context(cwd)
        self.assertEqual(ctx.owner_root, cwd.resolve())
        self.assertEqual(ctx.subrepo_path, "apps/api")
        self.assertEqual(ctx.planning_root, super_repo.resolve())
        self.assertEqual(ctx.topology.resolved, "monorepo-submodules")
        self.assertEqual(ctx.worktrees_dir, ".worktrees")

    # TRIAGE: test_superrepo_cwd_without_explicit_subrepo_hard_errors — asserts
    # resolve_request_context(cwd=super) raises SubrepoResolutionError with
    # "subrepo" in the message. Ran: bin/ai-specs hub <super> — hub does not call
    # resolve_request_context with the same parameters; it uses its own flow.
    # The SubrepoResolutionError type and message are internal Python exceptions.
    def test_superrepo_cwd_without_explicit_subrepo_hard_errors(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), path="apps/api")
        with self.assertRaises(self.util.SubrepoResolutionError) as ctx:
            self.util.resolve_request_context(super_repo)
        self.assertIn("subrepo", str(ctx.exception).lower())

    # TRIAGE: test_superrepo_cwd_with_explicit_subrepo_uses_super_planning_root —
    # asserts resolve_request_context(cwd=super, explicit_subrepo="apps/api")
    # returns owner_root=super, subrepo_path="apps/api", planning_root=super.
    # No CLI verb accepts an explicit_subrepo argument. The RequestContext fields
    # are internal.
    def test_superrepo_cwd_with_explicit_subrepo_uses_super_planning_root(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(
            Path(tmp.name), path="apps/api", name="api"
        )
        ctx = self.util.resolve_request_context(
            super_repo, explicit_subrepo="apps/api"
        )
        self.assertEqual(ctx.owner_root, super_repo.resolve())
        self.assertEqual(ctx.subrepo_path, "apps/api")
        self.assertEqual(ctx.planning_root, super_repo.resolve())

    # TRIAGE: test_standalone_request_planning_root_is_owner_root — asserts
    # resolve_request_context(standalone) returns planning_root == owner_root,
    # subrepo_path=None. Ran: bin/ai-specs hub — shows "topology: standalone (auto)"
    # but owner_root, planning_root, subrepo_path are internal RequestContext fields.
    def test_standalone_request_planning_root_is_owner_root(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        repo = Path(tmp.name) / "repo"
        repo.mkdir()
        git(repo, "init", "-q", "-b", "main")
        (repo / "README.md").write_text("x\n")
        git(repo, "add", "-A")
        git(repo, "commit", "-qm", "init")
        ctx = self.util.resolve_request_context(repo)
        self.assertEqual(ctx.owner_root, repo.resolve())
        self.assertEqual(ctx.planning_root, repo.resolve())
        self.assertIsNone(ctx.subrepo_path)
        self.assertEqual(ctx.topology.resolved, "standalone")

    # TRIAGE: test_uninitialized_submodule_fails_safe_without_planning_exception —
    # asserts resolve_request_context(super_with_uninit) returns subrepo_path=None,
    # planning_root==owner_root. The fail-safe behavior is internal to RequestContext;
    # hub shows "topology: standalone (auto)" but the exception-absence assertion
    # requires calling the Python function directly.
    def test_uninitialized_submodule_fails_safe_without_planning_exception(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(
            Path(tmp.name), path="apps/api", initialized=False
        )
        ctx = self.util.resolve_request_context(super_repo)
        self.assertIsNone(ctx.subrepo_path)
        self.assertEqual(ctx.planning_root, ctx.owner_root)

    # TRIAGE: test_non_git_cwd_fails_safe_to_owner_root — asserts
    # resolve_request_context(non_git_dir) returns owner_root=cwd=planning_root,
    # subrepo_path=None, topology="standalone". The internal fail-safe fields are
    # not exposed by any CLI verb. hub from a non-git dir shows topology standalone
    # but the specific RequestContext field values are internal.
    def test_non_git_cwd_fails_safe_to_owner_root(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        plain = Path(tmp.name) / "plain"
        plain.mkdir()
        ctx = self.util.resolve_request_context(plain)
        self.assertEqual(ctx.owner_root, plain.resolve())
        self.assertEqual(ctx.planning_root, plain.resolve())
        self.assertIsNone(ctx.subrepo_path)
        self.assertEqual(ctx.topology.resolved, "standalone")

    # TRIAGE: test_explicit_inferred_mismatch_names_both_values — asserts
    # resolve_request_context(cwd=api, explicit_subrepo="apps/web") raises
    # SubrepoResolutionError mentioning both "apps/api" and "apps/web". No CLI
    # verb accepts explicit_subrepo; the error message content is internal.
    def test_explicit_inferred_mismatch_names_both_values(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(
            Path(tmp.name),
            path="apps/api",
            name="api",
            second={"path": "apps/web", "name": "web"},
        )
        cwd = super_repo / "apps" / "api"
        with self.assertRaises(self.util.SubrepoResolutionError) as ctx:
            self.util.resolve_request_context(cwd, explicit_subrepo="apps/web")
        msg = str(ctx.exception)
        self.assertIn("apps/api", msg)
        self.assertIn("apps/web", msg)

    # TRIAGE: test_linked_submodule_worktree_longest_prefix_inference — asserts
    # resolve_request_context(cwd=linked_worktree) infers subrepo_path=
    # "alquimia-front-web" via longest-prefix matching and returns the super as
    # planning_root. The subrepo_path inference is internal to RequestContext;
    # no CLI output exposes the inferred subrepo path.
    def test_linked_submodule_worktree_longest_prefix_inference(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(
            Path(tmp.name),
            path="alquimia-front",
            name="alquimia-front",
            second={"path": "alquimia-front-web", "name": "alquimia-front-web"},
        )
        wt = super_repo / ".worktrees" / "alquimia-front-web-feat-x"
        wt.parent.mkdir(parents=True, exist_ok=True)
        git(
            super_repo / "alquimia-front-web",
            "worktree",
            "add",
            "-q",
            "-b",
            "feat-x",
            str(wt.resolve()),
            "main",
        )
        ctx = self.util.resolve_request_context(wt)
        self.assertEqual(ctx.subrepo_path, "alquimia-front-web")
        self.assertEqual(ctx.planning_root, super_repo.resolve())
        self.assertNotEqual(ctx.subrepo_path, "alquimia-front")


class OverrideIsStaleTests(unittest.TestCase):
    """1.4 — override_is_stale unit tests."""

    @classmethod
    def setUpClass(cls):
        cls.util = _load_util()

    # TRIAGE: test_missing_dest_false — asserts override_is_stale(src, missing_dest)
    # returns False. override_is_stale is a pure file-hash comparison utility used
    # internally by worktree-flow recipe override sync. Ran: bin/ai-specs sync and
    # bin/ai-specs doctor — neither emits per-file staleness booleans. The function
    # return value is internal.
    def test_missing_dest_false(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        src = Path(tmp.name) / "src.sh"
        src.write_text("hello\n")
        dest = Path(tmp.name) / "missing.sh"
        self.assertFalse(self.util.override_is_stale(src, dest))

    # TRIAGE: test_missing_catalog_src_false — asserts override_is_stale(missing_src,
    # dest) returns False. Same internal utility; no CLI verb exposes the result.
    def test_missing_catalog_src_false(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        dest = Path(tmp.name) / "dest.sh"
        dest.write_text("hello\n")
        src = Path(tmp.name) / "missing.sh"
        self.assertFalse(self.util.override_is_stale(src, dest))

    # TRIAGE: test_identical_bytes_false — asserts override_is_stale returns False
    # for identical files. Same internal utility; no CLI verb exposes per-file
    # staleness. doctor may report override staleness warnings but not for the
    # exact (src, dest) pair constructed here.
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

    # TRIAGE: test_divergent_bytes_true — asserts override_is_stale returns True
    # for divergent files. Same internal utility; no CLI verb directly exposes the
    # boolean return value.
    def test_divergent_bytes_true(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        src = Path(tmp.name) / "src.sh"
        dest = Path(tmp.name) / "dest.sh"
        src.write_text("catalog\n")
        dest.write_text("customized\n")
        self.assertTrue(self.util.override_is_stale(src, dest))



class ResolveSubrepoTests(unittest.TestCase):
    """Behavioral tests for util.resolve_subrepo (design §2 / 8 delta scenarios)."""

    @classmethod
    def setUpClass(cls):
        cls.util = _load_util()

    def _entries(self, super_repo: Path):
        return self.util.parse_gitmodules_entries(super_repo)

    def _add_linked_worktree(self, super_repo: Path, module: str, slug: str) -> Path:
        wt = super_repo / ".worktrees" / f"{module}-{slug}"
        wt.parent.mkdir(parents=True, exist_ok=True)
        git(
            super_repo / module,
            "worktree",
            "add",
            "-q",
            "-b",
            slug,
            str(wt.resolve()),
            "main",
        )
        return wt

    # TRIAGE: test_cwd_inference_from_primary_checkout — asserts resolve_subrepo()
    # returns "apps/api" when cwd is inside the primary submodule checkout.
    # resolve_subrepo is an internal Python function called by worktree-flow; no
    # CLI verb accepts (super_repo, worktrees_dir, initialized, cwd, explicit,
    # entries) arguments or exposes the returned subrepo path string.
    def test_cwd_inference_from_primary_checkout(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), path="apps/api", name="api")
        present, initialized = self.util.detect_submodules(super_repo)
        self.assertTrue(present)
        cwd = super_repo / "apps" / "api"
        got = self.util.resolve_subrepo(
            super_repo,
            ".worktrees",
            initialized,
            cwd,
            None,
            self._entries(super_repo),
        )
        self.assertEqual(got, "apps/api")

    # TRIAGE: test_cwd_inference_from_linked_worktree_longest_prefix — asserts
    # resolve_subrepo() returns "alquimia-front-web" (not "alquimia-front") via
    # longest-prefix matching from a linked worktree cwd. Same internal function;
    # no CLI verb exposes the resolved subrepo path.
    def test_cwd_inference_from_linked_worktree_longest_prefix(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(
            Path(tmp.name),
            path="alquimia-front",
            name="alquimia-front",
            second={"path": "alquimia-front-web", "name": "alquimia-front-web"},
        )
        _present, initialized = self.util.detect_submodules(super_repo)
        wt = self._add_linked_worktree(super_repo, "alquimia-front-web", "feat-x")
        got = self.util.resolve_subrepo(
            super_repo,
            ".worktrees",
            initialized,
            wt,
            None,
            self._entries(super_repo),
        )
        self.assertEqual(got, "alquimia-front-web")

    # TRIAGE: test_explicit_path_validated — asserts resolve_subrepo() validates
    # an explicit "apps/api" against initialized submodules and returns it. Same
    # internal function; no CLI verb passes explicit_subrepo to resolve_subrepo.
    def test_explicit_path_validated(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), path="apps/api", name="api")
        _p, initialized = self.util.detect_submodules(super_repo)
        got = self.util.resolve_subrepo(
            super_repo,
            ".worktrees",
            initialized,
            super_repo,  # cwd not inside a submodule
            "apps/api",
            self._entries(super_repo),
        )
        self.assertEqual(got, "apps/api")

    # TRIAGE: test_explicit_unique_name_resolves_to_path — asserts resolve_subrepo()
    # resolves a unique gitmodule name "api" to its path "apps/api". Same internal
    # function; no CLI verb exposes name-to-path resolution.
    def test_explicit_unique_name_resolves_to_path(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), path="apps/api", name="api")
        _p, initialized = self.util.detect_submodules(super_repo)
        got = self.util.resolve_subrepo(
            super_repo,
            ".worktrees",
            initialized,
            super_repo,
            "api",
            self._entries(super_repo),
        )
        self.assertEqual(got, "apps/api")

    # TRIAGE: test_explicit_inferred_mismatch_raises — asserts resolve_subrepo()
    # raises SubrepoResolutionError when explicit="apps/web" but cwd infers
    # "apps/api". The error type and message content are internal.
    def test_explicit_inferred_mismatch_raises(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(
            Path(tmp.name),
            path="apps/api",
            name="api",
            second={"path": "apps/web", "name": "web"},
        )
        _p, initialized = self.util.detect_submodules(super_repo)
        cwd = super_repo / "apps" / "api"
        with self.assertRaises(self.util.SubrepoResolutionError) as ctx:
            self.util.resolve_subrepo(
                super_repo,
                ".worktrees",
                initialized,
                cwd,
                "apps/web",
                self._entries(super_repo),
            )
        msg = str(ctx.exception)
        self.assertIn("apps/api", msg)
        self.assertIn("apps/web", msg)

    # TRIAGE: test_uninitialized_submodule_rejected — asserts resolve_subrepo()
    # raises SubrepoResolutionError with "not initialized" and "git submodule
    # update --init" when requesting an uninitialized submodule. The error message
    # content is internal to the Python exception.
    def test_uninitialized_submodule_rejected(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(
            Path(tmp.name), path="apps/api", name="api", initialized=False
        )
        _p, initialized = self.util.detect_submodules(super_repo)
        self.assertEqual(initialized, ())
        entries = self._entries(super_repo)
        with self.assertRaises(self.util.SubrepoResolutionError) as ctx:
            self.util.resolve_subrepo(
                super_repo,
                ".worktrees",
                initialized,
                super_repo,
                "apps/api",
                entries,
            )
        msg = str(ctx.exception)
        self.assertIn("not initialized", msg.lower())
        self.assertIn("git submodule update --init", msg)

    # TRIAGE: test_unknown_submodule_rejected — asserts resolve_subrepo() raises
    # SubrepoResolutionError with "unknown submodule" and the bogus name. Same
    # internal function; the error message is not exposed by any CLI verb.
    def test_unknown_submodule_rejected(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        super_repo = make_super_with_submodule(Path(tmp.name), path="apps/api", name="api")
        _p, initialized = self.util.detect_submodules(super_repo)
        with self.assertRaises(self.util.SubrepoResolutionError) as ctx:
            self.util.resolve_subrepo(
                super_repo,
                ".worktrees",
                initialized,
                super_repo,
                "does-not-exist",
                self._entries(super_repo),
            )
        self.assertIn("unknown submodule", str(ctx.exception).lower())
        self.assertIn("does-not-exist", str(ctx.exception))

    # TRIAGE: test_ambiguous_name_requires_path — asserts resolve_subrepo() raises
    # SubrepoResolutionError with "ambiguous" and "path" when a synthetic duplicate
    # name is passed. Same internal function with synthetic entries; no CLI path.
    def test_ambiguous_name_requires_path(self):
        # gitmodules names are unique keys in real git; pass synthetic duplicate
        # entries to exercise the design §2 ambiguous-name rejection path.
        entries = (("shared", "apps/a"), ("shared", "apps/b"))
        initialized = ("apps/a", "apps/b")
        with self.assertRaises(self.util.SubrepoResolutionError) as ctx:
            self.util.resolve_subrepo(
                Path("/tmp"),
                ".worktrees",
                initialized,
                Path("/tmp"),
                "shared",
                entries,
            )
        msg = str(ctx.exception).lower()
        self.assertIn("ambiguous", msg)
        self.assertIn("path", msg)



if __name__ == "__main__":
    unittest.main()
