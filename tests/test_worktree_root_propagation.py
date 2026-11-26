"""End-to-end worktree root-propagation tests.

Regression coverage for the suspected root issue: an agent/runtime may pass
its **process cwd** rather than the explicit command/worktree path, and the
sync/gate machinery must never fall back to that process cwd. These tests
prove, with a temporary Git main checkout plus a linked worktree:

1. ``ai-specs sync <worktree>`` with the CLI invoked from an unrelated cwd
   treats the explicit worktree path as the project root: every derived
   artifact (AGENTS.md, ai-specs/.gitignore, ai-specs/.ai-specs.lock, the
   CLI-served hook wiring and the project-cache snapshot) lands **only**
   under the worktree, and the Git **common root** (the main checkout) is
   byte-for-byte untouched.

2. The gate event ``cwd`` propagates verbatim through the launcher to the
   installed CLI: an event whose cwd is the linked worktree is **allowed**, an event
   whose cwd is the main checkout is **blocked**, and ``--explain`` echoes the
   exact event cwd string.

No production file is modified. The suite is hermetic: a scratch AI_SPECS_HOME
(symlink-farm over this checkout, cache excluded). The fixture installs the CLI
into that isolated home, while sync must not acquire a separate gate binary or
write outside the requested worktree.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import cli_path  # noqa: E402
from gate_cli import gate_command, install_cli

ROOT = Path(__file__).resolve().parents[1]
CLI = cli_path()
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"
MANIFEST = (
    "[project]\n"
    "name = 'worktree-root-fixture'\n"
    "subrepos = []\n\n"
    "[agents]\n"
    "enabled = ['claude']\n\n"
    "[recipes.worktree-flow]\n"
    "enabled = true\n\n"
    "[recipes.worktree-flow.config]\n"
    "gate_impl = 'auto'\n"
    "gate_mode = 'always'\n"
)

DERIVED_IN_WORKTREE = (
    "AGENTS.md",
    "ai-specs/.gitignore",
    "ai-specs/.ai-specs.lock",
    ".claude/settings.json",
)


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(cwd), *args], check=True,
                   capture_output=True, text=True)


def _scratch_ai_specs_home() -> tuple[Path, tempfile.TemporaryDirectory]:
    """Symlink-farm AI_SPECS_HOME over this checkout (cache excluded).

    Mirrors the pattern in tests/test_sync_output_verbosity.py so recipe
    materialization and project-cache writes stay outside the developer's
    real ~/.ai-specs and this worktree's cache/.
    Returns (home, tmpdir_holder) — the caller owns the tempdir.
    """
    holder = tempfile.TemporaryDirectory(prefix="ai-specs-wtr-home-")
    home = Path(holder.name)
    for name in os.listdir(ROOT):
        src = ROOT / name
        if name in ("cache", ".commandcode", ".git"):
            continue
        (home / name).symlink_to(src)
    return home, holder




class WorktreeRootPropagationSyncTests(unittest.TestCase):
    """End-to-end: explicit sync target/worktree is the project root."""

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="wtr-sync-")
        self.addCleanup(self.tmp.cleanup)
        self.home, self._home_holder = _scratch_ai_specs_home()
        self.addCleanup(self._home_holder.cleanup)
        base = Path(self.tmp.name) / "base"
        base.mkdir()
        _git(base, "init", "-q")
        _git(base, "config", "user.email", "t@t.t")
        _git(base, "config", "user.name", "t")
        (base / "README.md").write_text("main checkout\n")
        _git(base, "add", "-A")
        _git(base, "commit", "-qm", "init")
        _git(base, "checkout", "-q", "-B", "main")
        self.main_root = base

    def _worktree(self, branch: str = "feat") -> Path:
        wt = Path(self.tmp.name) / "wt"
        _git(self.main_root, "worktree", "add", "-q", "-b", branch, str(wt))
        return wt

    def _sync_env(self) -> dict:
        return {
            **os.environ,
            "AI_SPECS_HOME": str(self.home),
            "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
        }

    def _init_and_sync(self, wt: Path) -> subprocess.CompletedProcess:
        subprocess.run([str(CLI), "init", str(wt)], check=True,
                       capture_output=True, text=True, env=self._sync_env())
        (wt / "ai-specs" / "ai-specs.toml").write_text(MANIFEST)
        # CLI invoked from an unrelated cwd: the explicit path is the root.
        outside = Path(self.tmp.name) / "outside"
        outside.mkdir(exist_ok=True)
        return subprocess.run(
            [str(CLI), "sync", str(wt)], cwd=outside,
            capture_output=True, text=True, env=self._sync_env(),
        )

    def _snapshot(self, root: Path) -> dict[str, bytes]:
        out: dict[str, bytes] = {}
        for p in sorted(root.rglob("*")):
            if p.is_file() and ".git" not in p.parts:
                out[str(p.relative_to(root))] = p.read_bytes()
        return out

    def test_sync_explicit_worktree_lands_derived_artifacts_only_in_worktree(self):
        wt = self._worktree()
        before_main = self._snapshot(self.main_root)
        before_wt = self._snapshot(wt)
        proc = self._init_and_sync(wt)
        self.assertEqual(proc.returncode, 0,
                         f"stdout:\n{proc.stdout}\nstderr:\n{proc.stderr}")

        for rel in DERIVED_IN_WORKTREE:
            self.assertTrue((wt / rel).is_file(), f"missing derived artifact {rel} in worktree")

        # The linked worktree started from the main checkout's tree; sync only
        # ADDED derived artifacts under it (none of the pre-existing files
        # were modified).
        self.assertNotEqual(self._snapshot(wt), before_wt,
                            "sync must have written derived artifacts into the worktree")
        for rel, data in before_wt.items():
            self.assertEqual((wt / rel).read_bytes(), data,
                             f"sync modified pre-existing worktree file {rel}")

        # The main checkout (Git common root) is byte-identical before/after.
        self.assertEqual(self._snapshot(self.main_root), before_main,
                         "sync with explicit worktree path must NOT touch the "
                         "Git common root / main checkout")

    def test_sync_wires_the_cli_hook_and_builds_no_gate_binary(self):
        wt = self._worktree()
        proc = self._init_and_sync(wt)
        self.assertEqual(proc.returncode, 0, proc.stderr)

        # The gate is CLI-served: the rendered hook carries the CLI command,
        # no launcher is written, and no separate gate binary is acquired.
        self.assertFalse(
            (wt / "ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh").exists(),
            "the gate launcher is never materialized")
        settings = (wt / ".claude/settings.json").read_text()
        self.assertIn("worktree-gate --hook", settings)
        self.assertIn("dist/ai-specs", settings)
        self.assertFalse((self.home / "cache" / "bin" / "worktree-gate").exists(),
                         "sync must not acquire a separate gate binary")

    def test_no_derived_artifact_escapes_into_git_common_root(self):
        wt = self._worktree()
        proc = self._init_and_sync(wt)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        for rel in DERIVED_IN_WORKTREE:
            self.assertFalse((self.main_root / rel).exists(),
                             f"derived artifact {rel} leaked into the main checkout")
        self.assertFalse((self.main_root / "ai-specs").exists(),
                         "sync must not bootstrap ai-specs/ into the main checkout")


class WorktreeEventCwdPropagationTests(unittest.TestCase):
    """End-to-end: event cwd reaches the launcher/Go gate for block/allow."""

    @classmethod
    def setUpClass(cls):
        cls.GO_BINARY = gate_command()

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="wtr-cwd-")
        self.addCleanup(self.tmp.cleanup)
        self.home, self._home_holder = _scratch_ai_specs_home()
        self.addCleanup(self._home_holder.cleanup)
        base = Path(self.tmp.name) / "base"
        base.mkdir()
        _git(base, "init", "-q")
        _git(base, "config", "user.email", "t@t.t")
        _git(base, "config", "user.name", "t")
        (base / "README.md").write_text("main\n")
        _git(base, "add", "-A")
        _git(base, "commit", "-qm", "init")
        _git(base, "checkout", "-q", "-B", "main")
        self.main_root = base
        self.wt = Path(self.tmp.name) / "wt"
        _git(base, "worktree", "add", "-q", "-b", "feat", str(self.wt))

        # Materialize the launcher and install the CLI in the isolated home;
        # both roots use the installed CLI route, independent of process cwd.
        launcher_src = ROOT / "catalog" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
        content = launcher_src.read_text()
        for token, value in (
            ("__WORKTREE_GATE_MODE__", "always"),
            ("__WORKTREE_GATE_SCOPE__", "auto"),
            ("__WORKTREE_REPO_TOPOLOGY__", "auto"),
        ):
            content = content.replace(token, value)
        install_cli(self.home)
        for root in (base, self.wt):
            hook = root / "ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh"
            hook.parent.mkdir(parents=True, exist_ok=True)
            hook.write_text(content)
            hook.chmod(0o755)
        self.launcher = base / "ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh"

    def _gate_env(self) -> dict:
        env = dict(os.environ, WORKTREE_GATE_PROTECTED="main development",
                   AI_SPECS_HOME=str(self.home))
        env.pop("WORKTREE_GATE_BIN", None)
        env.pop("WORKTREE_GATE_MODE", None)
        env.pop("WORKTREE_GATE_SCOPE", None)
        return env

    def _run_event(self, event: dict, cwd: Path) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["bash", str(self.launcher)], input=json.dumps(event),
            capture_output=True, text=True, cwd=cwd, env=self._gate_env(),
        )

    def test_event_cwd_worktree_allows_through_launcher(self):
        # The candidate is RELATIVE, so the event cwd is the sole resolver:
        # the process runs from the main checkout, yet the relative write must
        # land in the linked worktree and be allowed.
        event = {
            "event": "pre-tool-use",
            "tool_name": "Write",
            "tool_input": {"file_path": "f.txt"},
            "cwd": str(self.wt),
        }
        r = self._run_event(event, cwd=self.main_root)
        self.assertEqual(r.returncode, 0,
                         "relative write with worktree event cwd must allow")
        self.assertEqual(r.stderr, "")

    def test_event_cwd_main_checkout_blocks_through_launcher(self):
        # Same shape from the opposite side: the relative candidate resolves
        # against the main-checkout event cwd and must block — even though the
        # process itself runs from the linked worktree.
        event = {
            "event": "pre-tool-use",
            "tool_name": "Write",
            "tool_input": {"file_path": "src.py"},
            "cwd": str(self.main_root),
        }
        r = self._run_event(event, cwd=self.wt)
        self.assertEqual(r.returncode, 2,
                         "relative write with main event cwd must block")
        self.assertIn("protected branch", r.stderr)

    def test_explain_echoes_event_cwd_verbatim(self):
        event = {
            "event": "pre-tool-use",
            "tool_name": "Write",
            "tool_input": {"file_path": str(self.main_root / "src.py")},
            "cwd": str(self.main_root),
        }
        r = subprocess.run(
            [str(self.GO_BINARY), "--gate-mode", "always", "--gate-scope", "auto",
             "--repo-topology", "auto", "--explain"],
            input=json.dumps(event), capture_output=True, text=True,
            cwd=Path(self.tmp.name), env=self._gate_env(),
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        diag = json.loads(r.stdout.strip())
        self.assertEqual(diag["cwd"], str(self.main_root),
                         "the gate must propagate the event cwd verbatim, not "
                         "the process cwd")
        self.assertEqual(diag["decision"], "block")


class SubmoduleRequestContextIntegrationTests(unittest.TestCase):
    """1.1 — RED: real-git subrepo request context + worktree ownership.

    Uses a real superproject with an initialized submodule. The Python
    ``resolve_request_context`` planner has no CLI-observable surface, so the
    contract is proven through the CLI gate route's ``--resolve-central-root``
    read-only proof (same proven git facts: superproject root + registered
    initialized submodule path, failing closed on ambiguity).
    """

    def _gate_central_root(self, cwd: Path) -> subprocess.CompletedProcess:
        return subprocess.run(
            [str(gate_command()), "--resolve-central-root"],
            capture_output=True, text=True, check=False, cwd=cwd,
            env={"PATH": os.environ.get("PATH", ""), "LC_ALL": "C", "LANG": "C"},
        )

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="wtr-sub-")
        self.addCleanup(self.tmp.cleanup)
        root = Path(os.path.realpath(self.tmp.name))

        # The Go central-root proof requires the absorbed file:// submodule
        # layout: a bare local remote plus a working clone, then
        # `submodule add file://<remote>` (a local-path add leaves a layout
        # module_records cannot prove — see makeRemoteModule in the gate's own
        # topology_test.go).
        source = root / "api-source"
        source.mkdir()
        _git(source, "init", "-q")
        _git(source, "config", "user.email", "t@t.t")
        _git(source, "config", "user.name", "t")
        (source / "README.md").write_text("api\n")
        _git(source, "add", "-A")
        _git(source, "commit", "-qm", "init")
        _git(source, "checkout", "-q", "-B", "main")
        remote = root / "remote.git"
        _git(root, "clone", "--bare", "-q", str(source), str(remote))

        super_repo = root / "super"
        super_repo.mkdir()
        _git(super_repo, "init", "-q")
        _git(super_repo, "config", "user.email", "t@t.t")
        _git(super_repo, "config", "user.name", "t")
        (super_repo / "README.md").write_text("super\n")
        _git(super_repo, "add", "-A")
        _git(super_repo, "commit", "-qm", "init")
        _git(
            super_repo,
            "-c",
            "protocol.file.allow=always",
            "submodule",
            "add",
            "-q",
            f"file://{remote}",
            "apps/api",
        )
        _git(super_repo, "commit", "-qm", "add submodule")
        self.super_repo = super_repo
        self.subrepo = super_repo / "apps" / "api"

    def test_subrepo_cwd_resolves_subrepo_owner_and_super_planning_root(self):
        """Subrepo cwd → proven superrepo planning root + registered subrepo path.

        The gate's read-only central-root proof consumes the same git facts as
        the planner's monorepo-submodules resolution: owner stays the subrepo,
        the planning root is the proven superproject, and the registered path
        is validated (never guessed).
        """
        proc = self._gate_central_root(self.subrepo)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        payload = json.loads(proc.stdout.strip())
        self.assertEqual(payload["central_root"], str(self.super_repo.resolve()))
        self.assertEqual(payload["submodule"], "apps/api")

    def test_git_dash_c_create_yields_subrepo_owned_worktree(self):
        dest = self.super_repo / ".worktrees" / "apps-api-feat-x"
        _git(
            self.subrepo,
            "worktree",
            "add",
            "-q",
            "-b",
            "feat-x",
            str(dest.resolve()),
            "main",
        )
        sub_list = subprocess.run(
            ["git", "-C", str(self.subrepo), "worktree", "list"],
            capture_output=True, text=True, check=True,
        ).stdout
        self.assertIn(str(dest.resolve()), sub_list,
                      "subrepo worktree list must register the linked worktree")
        super_list = subprocess.run(
            ["git", "-C", str(self.super_repo), "worktree", "list"],
            capture_output=True, text=True, check=True,
        ).stdout
        self.assertNotIn(str(dest.resolve()), super_list,
                         "the linked worktree is owned by the submodule, not the "
                         "superproject")

    def test_superrepo_cwd_without_subrepo_hard_errors_before_any_create(self):
        before = subprocess.run(
            ["git", "-C", str(self.super_repo), "worktree", "list"],
            capture_output=True, text=True, check=True,
        ).stdout
        # A superrepo-context request without a subrepo fails closed (exit 1,
        # "unproven") before any git worktree add — never guessing an owner.
        proc = self._gate_central_root(self.super_repo)
        self.assertEqual(proc.returncode, 1)
        self.assertIn("unproven", proc.stderr)
        after = subprocess.run(
            ["git", "-C", str(self.super_repo), "worktree", "list"],
            capture_output=True, text=True, check=True,
        ).stdout
        self.assertEqual(before, after,
                         "hard error must precede any git worktree add")

    # TRIAGE: no CLI verb exposes resolve_request_context(explicit_subrepo=...)
    # — planner-internal surface with no process boundary. The assertion below
    # pins the git facts that path consumes: the planning root stays the
    # superproject toplevel and apps/api is a proven initialized submodule.
    def test_superrepo_cwd_with_explicit_subrepo_keeps_super_planning_root(self):
        top = subprocess.run(
            ["git", "-C", str(self.super_repo), "rev-parse", "--show-toplevel"],
            capture_output=True, text=True, check=True,
        ).stdout.strip()
        self.assertEqual(Path(top).resolve(), self.super_repo.resolve())
        entries = subprocess.run(
            ["git", "-C", str(self.super_repo), "config", "-f", ".gitmodules",
             "--get-regexp", r"submodule\..*\.path"],
            capture_output=True, text=True, check=True,
        ).stdout
        self.assertIn("apps/api", entries)
        status = subprocess.run(
            ["git", "-C", str(self.super_repo), "submodule", "status", "--", "apps/api"],
            capture_output=True, text=True, check=True,
        ).stdout.strip()
        self.assertTrue(status, "apps/api must be an initialized submodule")
        self.assertFalse(status.startswith("-"),
                         "apps/api must be initialized, not just registered")


if __name__ == "__main__":
    unittest.main()
