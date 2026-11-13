"""Phase 3 materialization tests for worktree-flow gate distribution.

`gate_impl` config validation and stamping (`auto | go` only), sentinel-upgrade
of pre-Go materialized gates, and invalid-value rejection. `gate_impl=bash` is
retired: sync must fail closed naming bash as removed.

Every materialize assertion goes through the native `ai-specs sync` verb (the
only producer of the stamped launcher); the retired `python3
lib/_internal/recipe-materialize.py` transport is gone. The digest-recheck half
of this suite moved to `internal/gatebinary` with the port.
"""
from __future__ import annotations

import json
import os
import platform
import subprocess
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
RECIPE_DIR = ROOT / "catalog" / "recipes" / "worktree-flow"


def recipe_version() -> str:
    with open(RECIPE_DIR / "recipe.toml", "rb") as fh:
        return tomllib.load(fh)["recipe"]["version"]


def _cache_platform() -> str:
    """Host `<goos>-<goarch>` segment for the launcher's version-keyed cache.

    Mirrors the launcher's own `uname` mapping (catalog/recipes/worktree-flow/
    hooks/worktree-gate.sh) so these launcher-resolution tests stay portable off
    darwin-arm64 without importing any lib/ module.
    """
    goos = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system(), "")
    goarch = {
        "arm64": "arm64", "aarch64": "arm64",
        "x86_64": "amd64", "amd64": "amd64",
    }.get(platform.machine().lower(), "")
    if not goos or not goarch:
        raise unittest.SkipTest(f"unsupported platform {goos}/{goarch}")
    return f"{goos}-{goarch}"


class WorktreeGatePhase3MaterializeTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        # A real VERSION file: the test pins stamped_gate_version to it.
        self.home = isolated_home(self.base)
        (self.home / "VERSION").unlink()
        (self.home / "VERSION").write_text("0.21.0\n")

    def _project(self, config_block: str = "") -> Path:
        proj = self.base / "proj"
        (proj / "ai-specs").mkdir(parents=True)
        (proj / "ai-specs" / "skills").mkdir()
        (proj / "ai-specs" / "commands").mkdir()
        ver = recipe_version()
        text = (
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f"[recipes.worktree-flow]\nenabled = true\nversion = \"{ver}\"\n"
        )
        if config_block:
            text = text.rstrip() + "\n" + config_block + "\n"
        (proj / "ai-specs" / "ai-specs.toml").write_text(text)
        return proj

    def _sync(self, proj: Path, *args: str):
        result = invoke(proj, "sync", *args, cli_home=self.home)
        return result

    def _hook(self, proj: Path) -> Path:
        return proj / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"

    def _legacy_hook(self, proj: Path) -> Path:
        return proj / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate-legacy.sh"

    def test_gate_impl_defaults_to_auto_and_stamps(self):
        proj = self._project()
        result = self._sync(proj)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        content = self._hook(proj).read_text()
        self.assertIn('stamped_gate_impl="auto"', content)
        self.assertIn('stamped_gate_version="0.21.0"', content)
        self.assertNotIn("__WORKTREE_GATE_IMPL__", content)
        self.assertNotIn("__WORKTREE_GATE_VERSION__", content)

    def test_gate_impl_bash_rejected_at_sync(self):
        """gate_impl=bash is removed; sync must fail closed with auto | go."""
        proj = self._project(
            '[recipes.worktree-flow.config]\ngate_impl = "bash"'
        )
        result = self._sync(proj)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        combined = result.stderr + result.stdout
        self.assertIn("bash", combined)
        self.assertIn("removed", combined)
        self.assertIn("auto | go", combined)
        self.assertFalse(
            self._legacy_hook(proj).exists(),
            "rejected bash must not materialize a legacy gate",
        )

    def test_gate_impl_go_stamps(self):
        proj = self._project(
            '[recipes.worktree-flow.config]\ngate_impl = "go"'
        )
        result = self._sync(proj)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(
            'stamped_gate_impl="go"', self._hook(proj).read_text()
        )
        self.assertFalse(
            self._legacy_hook(proj).exists(),
            "ordinary sync must not write LEGACY_HOOK_REL",
        )

    def test_ordinary_sync_never_creates_legacy_hook(self):
        proj = self._project()
        result = self._sync(proj)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(self._hook(proj).is_file())
        cleanup = (
            proj / "ai-specs" / "recipes" / "worktree-flow"
            / "overrides" / "bin" / "worktree-cleanup.sh"
        )
        self.assertTrue(cleanup.is_file(), "cleanup override must still materialize")
        self.assertFalse(self._legacy_hook(proj).exists())

    def test_leftover_legacy_hook_is_not_rewritten(self):
        proj = self._project()
        leftover = self._legacy_hook(proj)
        leftover.parent.mkdir(parents=True, exist_ok=True)
        marker = b"LEFTOVER-INERT-BYTES-MUST-STAY\n"
        leftover.write_bytes(marker)
        leftover.chmod(0o644)
        result = self._sync(proj)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(leftover.read_bytes(), marker)
        self.assertFalse(os.access(leftover, os.X_OK),
                         "leftover must not be classified/refreshed to executable catalog bytes")

    def test_invalid_gate_impl_rejected_at_sync_with_enum(self):
        proj = self._project(
            '[recipes.worktree-flow.config]\ngate_impl = "rust"'
        )
        result = self._sync(proj)
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        combined = result.stderr + result.stdout
        self.assertIn("rust", combined)
        self.assertIn("auto | go", combined)
        self.assertNotIn(" | bash", combined)

    def test_sentinel_upgrade_replaces_pre_go_gate(self):
        # A pre-Go materialized gate (the e080483 gate, which already carries
        # the stamped_gate_scope sentinel) has NO lock baseline under the gate
        # provenance model: ordinary sync preserves it with a warning (unknown
        # provenance, no seeding), and an explicit refresh upgrades it to the
        # launcher after backing up the pre-refresh bytes.
        proj = self._project()
        hook = self._hook(proj)
        hook.parent.mkdir(parents=True, exist_ok=True)
        pre_go = (
            '#!/usr/bin/env bash\n'
            'stamped_gate_mode="always"\n'
            'stamped_gate_scope="auto"\n'
            'stamped_repo_topology="auto"\n'
            'exit 0\n'
        )
        hook.write_text(pre_go)

        result = self._sync(proj)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(hook.read_text(), pre_go,
                         "unknown provenance must preserve the pre-Go gate")
        self.assertIn("no recorded provenance", result.stderr)

        result = self._sync(proj, "--refresh-gates")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        content = hook.read_text()
        self.assertIn('stamped_gate_scope="', content, "launcher must keep the sentinel")
        self.assertIn("--gate-mode", content, "must be replaced by the launcher")
        self.assertNotIn("_resolve_gate_mode", content,
                         "mode resolution is Go-owned; the launcher must not resolve")
        self.assertIn("stamped_gate_impl=", content, "launcher must carry gate_impl")

    def test_launcher_forwards_the_raw_stamped_mode_to_go(self):
        # The launcher is transport only: env/stamp precedence and fallback are
        # Go's ResolveGateMode (config.go / config_test.go). The shell must
        # forward the raw stamped value and do no resolution of its own.
        src = (RECIPE_DIR / "hooks" / "worktree-gate.sh").read_text()
        self.assertIn('--gate-mode "$stamped_gate_mode"', src)
        for token in ("_resolve_gate_mode", 'gate_mode="$('):
            with self.subTest(token=token):
                self.assertNotIn(token, src,
                                 "shell mode resolution belongs in Go's ResolveGateMode")

    def test_launcher_keeps_literal_staleness_sentinel(self):
        # Task 3.2: sync upgrades existing projects when the literal
        # `stamped_gate_scope="` token is present.
        src = (RECIPE_DIR / "hooks" / "worktree-gate.sh").read_text()
        self.assertIn('stamped_gate_scope="', src)
        # And the launcher source itself is bash-3.2 clean (no mapfile, no
        # associative arrays, no ${v,,}) — scanning only non-comment lines.
        code_lines = [
            line for line in src.splitlines()
            if line.strip() and not line.lstrip().startswith("#")
        ]
        for banned in ("mapfile", "readarray", "declare -A", ",,"):
            self.assertNotIn(banned, "\n".join(code_lines))
        proc = subprocess.run(
            ["bash", "-n", str(RECIPE_DIR / "hooks" / "worktree-gate.sh")],
            capture_output=True, text=True,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)


class WorktreeGateLauncherResolutionTests(unittest.TestCase):
    """Task 3.4: launcher implementation resolution order.

    Order: $WORKTREE_GATE_BIN -> project-local pin -> version-keyed cache ->
    legacy Bash. Each step is exercised independently so a precedence
    regression (e.g. the cache silently winning over the explicit override)
    fails loudly.
    """

    GO_BINARY = ROOT / "dist" / "worktree-gate-current"

    def _stamp(self, dest: Path, *, impl: str = "auto",
               version: str = "0.21.0") -> Path:
        content = (RECIPE_DIR / "hooks" / "worktree-gate.sh").read_text()
        content = content.replace("__WORKTREE_GATE_MODE__", "always")
        content = content.replace("__WORKTREE_GATE_SCOPE__", "auto")
        content = content.replace("__WORKTREE_REPO_TOPOLOGY__", "auto")
        content = content.replace("__WORKTREE_GATE_IMPL__", impl)
        content = content.replace("__WORKTREE_GATE_VERSION__", version)
        dest.write_text(content, encoding="utf-8")
        dest.chmod(0o755)
        return dest

    def _launcher(self, root: Path, *, impl: str = "auto",
                  version: str = "0.21.0") -> Path:
        """Materialize the launcher at its standard synced location.

        The launcher derives its installation root from BASH_SOURCE[0], so the
        fixture must place it exactly where sync materializes it — under
        ``<root>/ai-specs/recipes/worktree-flow/hooks/`` — for the derived
        recipe root ``hooks/../`` to contain the project-local pin.
        """
        hook = root / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"
        hook.parent.mkdir(parents=True, exist_ok=True)
        return self._stamp(hook, impl=impl, version=version)

    def _fixture(self, root: Path) -> tuple[Path, str]:
        repo = root / "repo"
        repo.mkdir()
        for args in (("init", "-q"), ("config", "user.email", "t@t.t"),
                     ("config", "user.name", "t")):
            subprocess.run(["git", "-C", str(repo), *args], check=True,
                           capture_output=True, text=True)
        (repo / "README.md").write_text("x\n")
        subprocess.run(["git", "-C", str(repo), "add", "-A"], check=True,
                       capture_output=True, text=True)
        subprocess.run(["git", "-C", str(repo), "commit", "-qm", "init"],
                       check=True, capture_output=True, text=True)
        subprocess.run(["git", "-C", str(repo), "checkout", "-q", "-B", "main"],
                       check=True, capture_output=True, text=True)
        event = json.dumps({
            "event": "pre-tool-use",
            "tool_name": "Write",
            "tool_input": {"file_path": str(repo / "src.py")},
            "cwd": str(repo),
        })
        return repo, event

    def _env(self, root: Path, **extra: str) -> dict:
        env = dict(os.environ)
        env["WORKTREE_GATE_PROTECTED"] = "main development"
        env["AI_SPECS_HOME"] = str(root / "home")
        env.pop("WORKTREE_GATE_BIN", None)
        env.pop("WORKTREE_GATE_MODE", None)
        env.pop("WORKTREE_GATE_SCOPE", None)
        env.update(extra)
        return env

    def _run(self, launcher: Path, event: str, root: Path,
             env: dict) -> subprocess.CompletedProcess:
        return subprocess.run(
            ["bash", str(launcher)], input=event, capture_output=True,
            text=True, cwd=str(root), env=env)

    def test_bin_override_wins_over_project_pin(self):
        """Spec 'Explicit binary override wins' (F5)."""
        if not self.GO_BINARY.exists():
            self.skipTest("no Go gate binary in dist/")
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            launcher = self._launcher(root)
            repo, event = self._fixture(root)
            # A project-local pin that would ALLOW the write if the launcher
            # wrongly consulted it before the override.
            pin = root / "ai-specs" / "recipes" / "worktree-flow" / "bin" / "worktree-gate"
            pin.parent.mkdir(parents=True, exist_ok=True)
            pin.write_bytes(b"#!/usr/bin/env bash\nexit 0\n")
            pin.chmod(0o755)
            override = root / "override-gate"
            override.write_bytes(self.GO_BINARY.read_bytes())
            override.chmod(0o755)
            env = self._env(root, WORKTREE_GATE_BIN=str(override))
            proc = self._run(launcher, event, root, env)
            self.assertEqual(proc.returncode, 2,
                             "WORKTREE_GATE_BIN must win over the project pin")
            self.assertIn("refusing", proc.stderr)

    def test_project_pin_wins_over_cache(self):
        """Project-local pin outranks the version-keyed cache."""
        if not self.GO_BINARY.exists():
            self.skipTest("no Go gate binary in dist/")
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            launcher = self._launcher(root)
            repo, event = self._fixture(root)
            # Cache binary that would ALLOW (exit 0) if wrongly consulted.
            cache_bin = (root / "home" / "cache" / "bin" / "worktree-gate" / "0.21.0" /
                         _cache_platform() / "worktree-gate")
            cache_bin.parent.mkdir(parents=True, exist_ok=True)
            cache_bin.write_bytes(b"#!/usr/bin/env bash\nexit 0\n")
            cache_bin.chmod(0o755)
            # Project pin = the real gate (blocks).
            pin = root / "ai-specs" / "recipes" / "worktree-flow" / "bin" / "worktree-gate"
            pin.parent.mkdir(parents=True, exist_ok=True)
            pin.write_bytes(self.GO_BINARY.read_bytes())
            pin.chmod(0o755)
            proc = self._run(launcher, event, root, self._env(root))
            self.assertEqual(proc.returncode, 2,
                             "project pin must win over the cache")
            self.assertIn("refusing", proc.stderr)

    def test_cache_binary_used_when_no_pin_or_override(self):
        """Version-keyed cache serves when neither override nor pin exists."""
        if not self.GO_BINARY.exists():
            self.skipTest("no Go gate binary in dist/")
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            launcher = self._launcher(root)
            repo, event = self._fixture(root)
            cache_bin = (root / "home" / "cache" / "bin" / "worktree-gate" / "0.21.0" /
                         _cache_platform() / "worktree-gate")
            cache_bin.parent.mkdir(parents=True, exist_ok=True)
            cache_bin.write_bytes(self.GO_BINARY.read_bytes())
            cache_bin.chmod(0o755)
            proc = self._run(launcher, event, root, self._env(root))
            self.assertEqual(proc.returncode, 2,
                             "cache binary must serve when no pin/override")
            self.assertIn("refusing", proc.stderr)

    def test_non_executable_override_ignored(self):
        """A WORKTREE_GATE_BIN that is not executable is ignored with a warning."""
        if not self.GO_BINARY.exists():
            self.skipTest("no Go gate binary in dist/")
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            launcher = self._launcher(root)
            repo, event = self._fixture(root)
            # Project pin = real gate so the launcher still enforces after
            # ignoring the broken override.
            pin = root / "ai-specs" / "recipes" / "worktree-flow" / "bin" / "worktree-gate"
            pin.parent.mkdir(parents=True, exist_ok=True)
            pin.write_bytes(self.GO_BINARY.read_bytes())
            pin.chmod(0o755)
            broken = root / "broken"
            broken.write_text("#!/usr/bin/env bash\nexit 0\n")
            env = self._env(root, WORKTREE_GATE_BIN=str(broken))
            proc = self._run(launcher, event, root, env)
            self.assertEqual(proc.returncode, 2)
            self.assertIn("is not executable", proc.stderr)
            self.assertIn("refusing", proc.stderr)


if __name__ == "__main__":
    unittest.main()
