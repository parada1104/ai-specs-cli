"""Phase 3 materialization tests for worktree-flow gate distribution.

Tasks 3.7-3.9, 3.17-3.19: `gate_impl` config validation and stamping, legacy
Bash reference materialization, sentinel-upgrade of pre-Go materialized gates,
invalid gate_impl rejection, and the rollback rehearsal (gate_impl=bash
answers the parity corpus through the materialized legacy copy).
"""
from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RECIPE_DIR = ROOT / "catalog" / "recipes" / "worktree-flow"
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"

sys.path.insert(0, str(ROOT / "tests"))
from _fixture_catalog import populate_catalog  # noqa: E402


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def recipe_version() -> str:
    with open(RECIPE_DIR / "recipe.toml", "rb") as fh:
        return tomllib.load(fh)["recipe"]["version"]


def _cache_platform() -> str:
    """Host `<goos>-<goarch>` segment for the launcher's version-keyed cache,
    computed the same way gate acquisition does (Rosetta-aware), so these
    launcher-resolution tests stay portable off darwin-arm64."""
    gb = load_module(
        ROOT / "lib" / "_internal" / "gate_binary.py",
        "gate_binary_dist_config_under_test",
    )
    goos, goarch = gb.detect_platform()
    if not goos or not goarch:
        raise unittest.SkipTest(f"unsupported platform {goos}/{goarch}")
    return f"{goos}-{goarch}"


class WorktreeGatePhase3MaterializeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_p3_internal")
        cls.materialize = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_p3_internal"
        )

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.home = Path(self.tmp.name) / "home"
        populate_catalog(self.home / "catalog" / "recipes")
        (self.home / "VERSION").write_text("0.21.0\n")

    def _project(self, config_block: str = "") -> Path:
        proj = Path(self.tmp.name) / "proj"
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

    def _hook(self, proj: Path) -> Path:
        return proj / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate.sh"

    def _legacy_hook(self, proj: Path) -> Path:
        return proj / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate-legacy.sh"

    def test_recipe_declares_gate_impl_enum_with_default_auto(self):
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        field = recipe.config_schema.fields["gate_impl"]
        self.assertEqual(field.default, "auto")
        self.assertEqual(field.enum, ["auto", "go", "bash"])

    def test_gate_impl_defaults_to_auto_and_stamps(self):
        proj = self._project()
        self.assertEqual(self.materialize.materialize_recipes(proj, self.home), 0)
        content = self._hook(proj).read_text()
        self.assertIn('stamped_gate_impl="auto"', content)
        self.assertIn('stamped_gate_version="0.21.0"', content)
        self.assertNotIn("__WORKTREE_GATE_IMPL__", content)
        self.assertNotIn("__WORKTREE_GATE_VERSION__", content)

    def test_gate_impl_bash_stamps_and_materializes_legacy(self):
        proj = self._project(
            '[recipes.worktree-flow.config]\ngate_impl = "bash"'
        )
        self.assertEqual(self.materialize.materialize_recipes(proj, self.home), 0)
        content = self._hook(proj).read_text()
        self.assertIn('stamped_gate_impl="bash"', content)
        legacy = self._legacy_hook(proj)
        self.assertTrue(legacy.is_file(), "legacy reference must materialize")
        src = RECIPE_DIR / "hooks" / "worktree-gate-legacy.sh"
        self.assertEqual(legacy.read_bytes(), src.read_bytes())
        self.assertTrue(os.access(legacy, os.X_OK))

    def test_gate_impl_go_stamps(self):
        proj = self._project(
            '[recipes.worktree-flow.config]\ngate_impl = "go"'
        )
        self.assertEqual(self.materialize.materialize_recipes(proj, self.home), 0)
        self.assertIn(
            'stamped_gate_impl="go"', self._hook(proj).read_text()
        )

    def test_invalid_gate_impl_rejected_at_sync_with_enum(self):
        proj = self._project(
            '[recipes.worktree-flow.config]\ngate_impl = "rust"'
        )
        proc = subprocess.run(
            ["python3", str(RECIPE_MATERIALIZE_PATH), str(proj), str(self.home)],
            capture_output=True, text=True,
        )
        self.assertEqual(proc.returncode, 1)
        combined = proc.stderr + proc.stdout
        self.assertIn("rust", combined)
        self.assertIn("auto", combined)
        self.assertIn("go", combined)
        self.assertIn("bash", combined)

    def test_merge_config_rejects_invalid_gate_impl(self):
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        with self.assertRaisesRegex(RuntimeError, r"auto.*go.*bash"):
            self.materialize.merge_config(recipe, {"gate_impl": "rust"})

    def test_sentinel_upgrade_replaces_pre_go_gate(self):
        # Task 3.18: a project with a pre-Go materialized gate (the e080483
        # gate, which already carries the stamped_gate_scope sentinel) must be
        # UPGRADED to the launcher, not skipped as stale. The launcher keeps
        # the literal sentinel so the staleness probe passes.
        proj = self._project()
        hook = self._hook(proj)
        hook.parent.mkdir(parents=True, exist_ok=True)
        hook.write_text(
            '#!/usr/bin/env bash\n'
            'stamped_gate_mode="always"\n'
            'stamped_gate_scope="auto"\n'
            'stamped_repo_topology="auto"\n'
            'exit 0\n'
        )
        self.assertEqual(self.materialize.materialize_recipes(proj, self.home), 0)
        content = hook.read_text()
        self.assertIn('stamped_gate_scope="', content, "launcher must keep the sentinel")
        self.assertIn("_resolve_gate_mode", content, "must be replaced by the launcher")
        self.assertIn("stamped_gate_impl=", content, "launcher must carry gate_impl")

    def test_launcher_keeps_literal_staleness_sentinel(self):
        # Task 3.2: recipe-materialize.py:494-508 upgrades existing projects
        # when the literal `stamped_gate_scope="` token is present.
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

class WorktreeGateRollbackTests(unittest.TestCase):
    """Task 3.17: rollback rehearsal — gate_impl=bash answers the corpus."""

    @classmethod
    def setUpClass(cls):
        cls.materialize = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_rollback_internal"
        )

    def _git(self, cwd: Path, *args: str) -> None:
        subprocess.run(["git", "-C", str(cwd), *args], check=True,
                       capture_output=True, text=True)

    def test_rollback_gate_impl_bash_answers_full_corpus(self):
        corpus = ROOT / "tests" / "fixtures" / "worktree-gate-corpus"
        cases = sorted(corpus.glob("*.json"))
        self.assertGreaterEqual(len(cases), 10, "corpus must be populated")

        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = Path(tmp.name) / "home"
        populate_catalog(home / "catalog" / "recipes")
        (home / "VERSION").write_text("0.21.0\n")
        proj = Path(tmp.name) / "proj"
        (proj / "ai-specs").mkdir(parents=True)
        (proj / "ai-specs" / "skills").mkdir()
        (proj / "ai-specs" / "commands").mkdir()
        ver = recipe_version()
        (proj / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'rb'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f"[recipes.worktree-flow]\nenabled = true\nversion = \"{ver}\"\n"
            "[recipes.worktree-flow.config]\ngate_impl = 'bash'\n"
        )
        self.assertEqual(self.materialize.materialize_recipes(proj, home), 0)
        legacy = proj / "ai-specs" / "recipes" / "worktree-flow" / "hooks" / "worktree-gate-legacy.sh"
        self.assertTrue(legacy.is_file())
        # The materialized legacy copy carries unstamped sentinels (the
        # catalog source's placeholders). The rollback rehearsal pins exact
        # stderr, so stamp the copy exactly like the parity runner does
        # (always / auto / auto), then assert the file is byte-identical to
        # the frozen catalog reference apart from those three stamps.
        content = legacy.read_text()
        content = content.replace(
            'stamped_gate_mode="__WORKTREE_GATE_MODE__"',
            'stamped_gate_mode="always"',
        )
        content = content.replace(
            'stamped_gate_scope="__WORKTREE_GATE_SCOPE__"',
            'stamped_gate_scope="auto"',
        )
        content = content.replace(
            'stamped_repo_topology="__WORKTREE_REPO_TOPOLOGY__"',
            'stamped_repo_topology="auto"',
        )
        legacy.write_text(content)
        legacy.chmod(0o755)
        self.assertTrue(legacy.is_file())

        def substitute(text: str, locations: dict[str, Path]) -> str:
            for name, path in locations.items():
                marker = "{" + name + "}"
                while marker in text:
                    head, _, tail = text.partition(marker)
                    if not tail:
                        text = head + str(path)
                    else:
                        text = head + str(path) + "/" + tail.lstrip("/")
            return text

        for case_file in cases:
            case = json.loads(case_file.read_text())
            with self.subTest(case=case_file.name):
                root = Path(tmp.name) / f"fixture-{case_file.stem}"
                if root.exists():
                    import shutil as _sh
                    _sh.rmtree(root)
                root.mkdir()
                repo = root / "repo"
                repo.mkdir()
                self._git(repo, "init", "-q")
                self._git(repo, "config", "user.email", "t@t.t")
                self._git(repo, "config", "user.name", "t")
                (repo / "README.md").write_text("x\n")
                self._git(repo, "add", "-A")
                self._git(repo, "commit", "-qm", "init")
                self._git(repo, "checkout", "-q", "-B", "main")
                locations = {"repo": repo}
                fixture = case.get("fixture", "none")
                if fixture in (None, "none", "protected-main"):
                    pass  # repo already on main
                elif fixture == "feature-branch":
                    self._git(repo, "checkout", "-q", "-B", "feature-x")
                elif fixture == "development-branch":
                    self._git(repo, "checkout", "-q", "-B", "development")
                elif fixture == "external-path":
                    locations["external"] = root / "external"
                    locations["external"].mkdir()
                elif fixture == "linked-worktree":
                    locations["worktree"] = root / "wt"
                    self._git(repo, "worktree", "add", "-q", "-b", "feat", str(locations["worktree"]))
                else:
                    raise AssertionError(f"unhandled fixture {fixture}")

                if case.get("stdin") is not None:
                    payload = case["stdin"]
                else:
                    event = json.loads(json.dumps(case["event"]))
                    if "cwd" in event:
                        event["cwd"] = substitute(event["cwd"], locations)
                    ti = event.get("tool_input") or {}
                    for key in ("file_path", "notebook_path", "command"):
                        if key in ti:
                            ti[key] = substitute(ti[key], locations)
                    for key in ("command", "script"):
                        if key in event:
                            event[key] = substitute(event[key], locations)
                    payload = json.dumps(event)
                event_cwd = locations.get("repo", root)
                env = dict(os.environ, WORKTREE_GATE_PROTECTED="main development")
                result = subprocess.run(
                    ["bash", str(legacy)], input=payload,
                    capture_output=True, text=True, cwd=str(event_cwd), env=env,
                )
                self.assertEqual(result.returncode, case["expected_exit"], result.stderr)
                expected = case.get("expected_stderr")
                if expected is not None:
                    self.assertEqual(
                        result.stderr,
                        substitute(expected, locations) + "\n")
                else:
                    self.assertEqual(result.stderr, "")


if __name__ == "__main__":
    unittest.main()
