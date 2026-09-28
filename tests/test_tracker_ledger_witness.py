"""Black-box tests for the durable tracker binding witness written by sync.

The witness lands at ``<git-common-dir>/ai-specs/ledger/witness.json`` with
exactly one of the four design states, records ambiguous candidates without
ever guessing a provider, is written atomically with no temp residue, survives
the ``RESOLVED_CONFIG_TEMP`` EXIT trap of a real ``ai-specs sync``, and is
shared between a main checkout and its linked worktrees. Every sync drives
``bin/ai-specs sync`` through the process boundary; the git common dir is
resolved with ``git rev-parse --git-common-dir``.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import isolated_home  # noqa: E402
from _fixture_catalog import (  # noqa: E402
    allow_internal_test_recipes_env,
    populate_catalog,
)

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
SYNC_SH = ROOT / "lib" / "sync.sh"
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"

TRACKER_RECIPE = '[recipes.test-tracker-ledger]\nenabled = true\nversion = "1.0.0"\n'
TRACKER_CONFLICT_RECIPE = (
    '[recipes.test-tracker-ledger-conflict]\nenabled = true\nversion = "1.0.0"\n'
)
NON_TRACKER_RECIPE = '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
TRACKING_DECLARATION = (
    "schema: spec-driven\n"
    "\n"
    "tracking:\n"
    "  tracker: trello\n"
    '  board_id: "69ec097f13e2d38ecd89a557"\n'
)

# Module-level memo for the CLI-acquired Go gate binary (built once offline).
_ACQUIRED_GATE: list[Path] = []
_GATE_HOLDER: list[tempfile.TemporaryDirectory] = []


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(
        ["git", "-C", str(cwd), *args], check=True, capture_output=True, text=True
    )


def _make_home(base: Path) -> Path:
    """Isolated CLI home: real lib copy + a REAL catalog populated with the
    public recipes plus the internal test-* fixture recipes.

    The repo catalog is never touched: the temp catalog holds symlinks to the
    public + fixture recipe directories, and every cache write lands in the
    temp home's own cache (the lib copy keeps realpath-derived cache roots
    out of the repository).
    """
    home = isolated_home(base, catalog=False)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    populate_catalog(home / "catalog" / "recipes")
    return home


def git_common_dir(root: Path) -> str:
    """Process-boundary replacement for the internal git_common_dir helper."""
    proc = subprocess.run(
        ["git", "-C", str(root), "rev-parse", "--path-format=absolute",
         "--git-common-dir"],
        capture_output=True, text=True, check=False,
    )
    return proc.stdout.strip() if proc.returncode == 0 else ""


def _platform() -> tuple[str, str]:
    """Mirror the gate acquisition's platform mapping for cache paths."""
    import platform as _platform

    goos = {"Darwin": "darwin", "Linux": "linux"}.get(_platform.system(), "")
    machine = _platform.machine()
    goarch = "arm64" if machine in ("arm64", "aarch64") else (
        "amd64" if machine in ("x86_64", "amd64") else "")
    return goos, goarch


def acquired_gate_binary() -> Path:
    """Acquire the REAL Go gate binary offline through the CLI's own
    local-build path (AI_SPECS_GATE_OFFLINE=1 AI_SPECS_GATE_BUILD=1 sync into
    a scratch home). Memoized module-wide; no network, no dist/ dependency.
    """
    if _ACQUIRED_GATE:
        return _ACQUIRED_GATE[0]
    holder = tempfile.TemporaryDirectory(prefix="witness-gate-acquire-")
    base = Path(holder.name)
    home = _make_home(base / "home")
    project = base / "proj"
    (project / "ai-specs").mkdir(parents=True)
    _git(project, "init", "-q")
    _git(project, "config", "user.email", "t@t.t")
    _git(project, "config", "user.name", "t")
    (project / "README.md").write_text("gate acquisition fixture\n")
    _git(project, "add", "-A")
    _git(project, "commit", "-qm", "init")
    (project / "ai-specs" / "ai-specs.toml").write_text(
        "[project]\nname = 'gate-acquire'\n\n[agents]\nenabled = ['claude']\n\n"
        "[recipes.worktree-flow]\nenabled = true\n"
    )
    env = {
        "PATH": os.environ.get("PATH", ""),
        "HOME": str(base / "subhome"),
        "TMPDIR": str(base),
        "AI_SPECS_HOME": str(home),
        "AI_SPECS_NO_NETWORK": "1",
        "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
        "AI_SPECS_GATE_OFFLINE": "1",
        "AI_SPECS_GATE_BUILD": "1",
        "LC_ALL": "C",
        "LANG": "C",
    }
    (base / "subhome").mkdir()
    for verb in ("init", "sync"):
        proc = subprocess.run(
            [str(CLI), verb, str(project)], cwd=ROOT, env=env,
            capture_output=True, text=True, check=False, input="",
        )
        assert proc.returncode == 0, (
            f"gate acquisition `{verb}` failed:\n{proc.stdout}\n{proc.stderr}"
        )
    version = (home / "VERSION").read_text().strip()
    goos, goarch = _platform()
    binary = (home / "cache" / "bin" / "worktree-gate" / version /
              f"{goos}-{goarch}" / "worktree-gate")
    assert binary.is_file(), f"gate binary was not built into the scratch cache: {binary}"
    _GATE_HOLDER.append(holder)
    _ACQUIRED_GATE.append(binary)
    return binary


class TrackerLedgerWitnessTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls._home_tmp = tempfile.TemporaryDirectory(prefix="witness-home-")
        cls.home = _make_home(Path(cls._home_tmp.name))

    @classmethod
    def tearDownClass(cls):
        cls._home_tmp.cleanup()

    # --- helpers ---------------------------------------------------------------

    def _tmp(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return Path(tmp.name)

    def _project(
        self,
        recipes: str,
        *,
        declaration: str | None = None,
        git: bool = True,
        base: Path | None = None,
    ) -> Path:
        root = (base or self._tmp()) / "repo"
        (root / "ai-specs").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'witness'\n\n[agents]\nenabled = ['claude']\n\n"
            + recipes
        )
        if declaration is not None:
            (root / "openspec").mkdir()
            (root / "openspec" / "config.yaml").write_text(declaration)
        if git:
            _git(root, "init", "-q")
        return root

    def _sync(self, root: Path) -> Path:
        """Real `bin/ai-specs sync` against the isolated home; returns the
        resolved-config path that the sync EXIT trap removes."""
        out = root / "ai-specs" / ".resolved-config.json"
        proc = self._run_cli(root, "sync")
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        return out

    def _run_cli(self, root: Path, *args: str) -> subprocess.CompletedProcess:
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(Path(self._home_tmp.name) / "subhome"),
            "TMPDIR": str(self._home_tmp.name),
            "AI_SPECS_HOME": str(self.home),
            "AI_SPECS_NO_NETWORK": "1",
            "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
            "LC_ALL": "C",
            "LANG": "C",
        }
        env.update(allow_internal_test_recipes_env())
        (Path(self._home_tmp.name) / "subhome").mkdir(exist_ok=True)
        # stdin is a closed pipe so the CLI can never block on a prompt.
        return subprocess.run(
            [str(CLI), *args, str(root)], cwd=ROOT, env=env,
            capture_output=True, text=True, check=False, input="",
        )

    def _witness_path(self, root: Path) -> Path:
        common = git_common_dir(root)
        self.assertTrue(common, "expected a git common dir for a git repo")
        return Path(common) / "ai-specs" / "ledger" / "witness.json"

    def _witness(self, root: Path) -> dict:
        return json.loads(self._witness_path(root).read_text())

    # --- 4.1 RED: four states --------------------------------------------------

    def test_bound_witness_names_the_resolved_recipe(self):
        root = self._project(TRACKER_RECIPE)
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["v"], 1)
        self.assertEqual(witness["capability"], "tracker")
        self.assertEqual(witness["state"], "bound")
        self.assertEqual(witness["recipe_id"], "test-tracker-ledger")
        self.assertEqual(witness["candidates"], [])
        self.assertTrue(witness["written_at"].endswith("Z"))

    def test_ambiguous_witness_records_candidates_without_guessing(self):
        root = self._project(TRACKER_RECIPE + TRACKER_CONFLICT_RECIPE)
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "ambiguous")
        self.assertEqual(
            sorted(witness["candidates"]),
            ["test-tracker-ledger", "test-tracker-ledger-conflict"],
        )
        # No provider is guessed or selected (D6): no bound recipe id at all.
        self.assertEqual(witness["recipe_id"], "")

    def test_unbound_witness_when_no_recipe_declares_tracker(self):
        root = self._project("")
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "unbound")
        self.assertEqual(witness["recipe_id"], "")
        self.assertEqual(witness["candidates"], [])

    def test_declared_not_bound_witness_when_only_config_declares_tracking(self):
        root = self._project(NON_TRACKER_RECIPE, declaration=TRACKING_DECLARATION)
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "declared-not-bound")
        self.assertEqual(witness["recipe_id"], "")
        self.assertEqual(witness["candidates"], [])

    def test_witness_has_no_provider_vocabulary(self):
        root = self._project(TRACKER_RECIPE)
        self._sync(root)
        witness = self._witness(root)
        for banned in ("board_id", "board", "list", "trello", "provider"):
            self.assertNotIn(banned, witness)

    # --- 4.1 RED: trap survival + atomic write ---------------------------------

    def test_witness_survives_resolved_config_temp_deletion(self):
        root = self._project(TRACKER_RECIPE)
        resolved_config = self._sync(root)
        # A real sync's EXIT trap already removed RESOLVED_CONFIG_TEMP; the
        # witness must have outlived it.
        self.assertFalse(resolved_config.exists())
        witness = self._witness(root)
        self.assertEqual(witness["state"], "bound")
        self.assertEqual(witness["recipe_id"], "test-tracker-ledger")

    def test_atomic_write_leaves_no_temp_residue(self):
        root = self._project(TRACKER_RECIPE)
        self._sync(root)
        ledger_dir = self._witness_path(root).parent
        residue = list(ledger_dir.glob("witness.json.tmp.*"))
        self.assertEqual(residue, [])

    def test_no_witness_outside_a_git_repo(self):
        root = self._project(TRACKER_RECIPE, git=False)
        self._sync(root)
        self.assertEqual(git_common_dir(root), "")

    def test_sync_sh_trap_never_names_the_witness(self):
        text = SYNC_SH.read_text()
        trap_line = next(
            line for line in text.splitlines() if "RESOLVED_CONFIG_TEMP:-" in line
        )
        self.assertIn("rm -f", trap_line)
        self.assertNotIn("witness", trap_line.lower())
        self.assertEqual(text.count("RESOLVED_CONFIG_TEMP="), 1)

    # --- 4.5 TRIANGULATE: linked worktree shares the common-dir witness --------

    def test_linked_worktree_reads_main_checkout_witness(self):
        base = self._tmp()
        root = self._project(TRACKER_RECIPE, base=base)
        _git(root, "config", "user.email", "t@t.t")
        _git(root, "config", "user.name", "t")
        (root / "README.md").write_text("x\n")
        _git(root, "add", "-A")
        _git(root, "commit", "-qm", "init")
        linked = base / "linked"
        _git(root, "worktree", "add", "-q", str(linked), "-b", "linked-branch")

        self._sync(root)

        # The linked worktree has its own .git file but one shared common dir.
        self.assertTrue((linked / ".git").is_file())
        self.assertEqual(git_common_dir(linked), git_common_dir(root))
        witness = json.loads(self._witness_path(linked).read_text())
        self.assertEqual(witness["state"], "bound")
        self.assertFalse((linked / ".git" / "ai-specs").exists())

    # --- 4.5 TRIANGULATE: explicit binding + deactivation -----------------------

    def test_explicit_binding_wins_over_two_declarers(self):
        root = self._project(
            TRACKER_RECIPE
            + TRACKER_CONFLICT_RECIPE
            + '[[bindings]]\ncapability = "tracker"\nrecipe = "test-tracker-ledger-conflict"\n'
        )
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "bound")
        self.assertEqual(witness["recipe_id"], "test-tracker-ledger-conflict")

    def test_disabling_the_bound_recipe_overwrites_the_witness(self):
        root = self._project(TRACKER_RECIPE)
        self._sync(root)
        self.assertEqual(self._witness(root)["state"], "bound")
        # Recipe list shrinks to none: the next sync must deactivate the witness,
        # never leave a disabled provider active.
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'witness'\n\n[agents]\nenabled = ['claude']\n"
        )
        self._sync(root)
        witness = self._witness(root)
        self.assertEqual(witness["state"], "unbound")
        self.assertEqual(witness["recipe_id"], "")

    def test_go_ledger_reads_the_python_witness_from_a_linked_worktree(self):
        base = self._tmp()
        root = self._project(TRACKER_RECIPE, base=base)
        _git(root, "config", "user.email", "t@t.t")
        _git(root, "config", "user.name", "t")
        (root / "README.md").write_text("x\n")
        _git(root, "add", "-A")
        _git(root, "commit", "-qm", "init")
        linked = base / "linked"
        _git(root, "worktree", "add", "-q", str(linked), "-b", "linked-branch")

        self._sync(root)

        # The Go gate binary is acquired offline through the CLI's own
        # local-build path (no dist/ dependency, no network).
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(base / "subhome"),
            "TMPDIR": str(base),
            "AI_SPECS_NO_NETWORK": "1",
            "LC_ALL": "C",
            "LANG": "C",
        }
        (base / "subhome").mkdir()
        proc = subprocess.run(
            [
                str(acquired_gate_binary()),
                "--ledger",
                "--checkpoint",
                "apply-start",
                "--ledger-mode",
                "warn",
                "--project-root",
                str(linked),
            ],
            capture_output=True,
            text=True,
            check=False,
            env=env,
            input="",
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        verdict = json.loads(proc.stdout)
        self.assertTrue(verdict["active"])
        self.assertEqual(verdict["decision"], "allow")
        self.assertEqual(verdict["identity"]["branch"], "linked-branch")


class SyncTrapIntegrationTests(unittest.TestCase):
    """End-to-end: a real ``ai-specs sync`` leaves the witness after its EXIT trap."""

    @classmethod
    def setUpClass(cls):
        cls._home_tmp = tempfile.TemporaryDirectory(prefix="witness-e2e-home-")
        cls.home = _make_home(Path(cls._home_tmp.name))

    @classmethod
    def tearDownClass(cls):
        cls._home_tmp.cleanup()

    def _run_cli(self, root: Path, *args: str) -> subprocess.CompletedProcess:
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(Path(self._home_tmp.name) / "subhome"),
            "TMPDIR": str(self._home_tmp.name),
            "AI_SPECS_HOME": str(self.home),
            "AI_SPECS_NO_NETWORK": "1",
            "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
            "LC_ALL": "C",
            "LANG": "C",
        }
        (Path(self._home_tmp.name) / "subhome").mkdir(exist_ok=True)
        return subprocess.run(
            [str(CLI), *args, str(root)], cwd=ROOT, env=env,
            capture_output=True, text=True, check=False, input="",
        )

    def test_real_sync_leaves_witness_after_trap(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        workspace = Path(tmp.name) / "workspace"
        workspace.mkdir()
        proc = self._run_cli(workspace, "init")
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        (workspace / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'witness-e2e'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n"
        )
        _git(workspace, "init", "-q")
        proc = self._run_cli(workspace, "sync")
        self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
        common = git_common_dir(workspace)
        self.assertTrue(common, "expected a git common dir after sync")
        witness_path = Path(common) / "ai-specs" / "ledger" / "witness.json"
        self.assertTrue(witness_path.is_file(), "witness must survive the sync EXIT trap")
        witness = json.loads(witness_path.read_text())
        self.assertEqual(witness["state"], "bound")
        self.assertEqual(witness["recipe_id"], "trello-mcp-workflow")


if __name__ == "__main__":
    unittest.main()
