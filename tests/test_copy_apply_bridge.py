"""Contract tests for the Go copy-apply bridge in ``recipe-materialize.py``
(GO-08 WU2, strangler slice 5).

``worktree-gate --apply-copy`` owns the COPY DECISION + EXECUTION for the
three blind copiers — bundled skills, recipe commands, docs. Python keeps
hashing (set_recipe_skill_hashes + write_lock), the exact prints and
warnings, and the fail-open fallback. ``materialize_dep_skill`` stays Python
(true acquisition through vendor-skills.py) and ``materialize_template`` is
out of scope (slice 6).

The Python copy bodies survive as TEMPORARY fail-open fallbacks
(``GO_COPY_APPLY_BRIDGE_FALLBACK``) and these tests pin both seams:

* Go-path results equal the retained Python authority: tree identity
  (structure, content, modes, mtimes), exact prints/warnings, exact
  RuntimeError messages, identical lock bytes,
* the stdin/stdout envelope contract of ``--apply-copy``,
* the degraded path for every infrastructure failure: one warning, then the
  Python body executes identically (a partial Go bundled-skill copy is safe
  to redo: the fallback rmtree+copytree rewrites dest wholesale; copy2 is an
  idempotent overwrite).

The Go path needs a built binary (``dist/worktree-gate-current`` or
``$WORKTREE_GATE_BIN``); it skips loudly when none exists. Fallback tests run
with no usable binary, because failing open is the contract they pin.
"""
from __future__ import annotations

import contextlib
import importlib.util
import inspect
import io
import json
import os
import re
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]

RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
DIST_BINARY = ROOT / "dist" / "worktree-gate-current"

PAST = time.time() - 1_000_000


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def gate_binary() -> Path:
    """The built Go binary the bridge is proven against, or a loud skip."""
    pinned = os.environ.get("WORKTREE_GATE_BIN", "")
    candidate = Path(pinned) if pinned else DIST_BINARY
    if candidate.is_file() and os.access(candidate, os.X_OK):
        return candidate
    raise unittest.SkipTest(
        "no worktree-gate binary (run scripts/build-gate.sh or set "
        "WORKTREE_GATE_BIN); the Go copy-apply bridge cannot be proven "
        "without it"
    )


def make_skill_tree(src: Path) -> None:
    """A small bundled-skill source tree with pinned modes and mtimes."""
    (src / "sub").mkdir(parents=True)
    (src / "SKILL.md").write_text("skill body")
    (src / "sub" / "helper.py").write_text("helper")
    (src / "run.sh").write_text("#!/bin/sh\necho hi\n")
    os.chmod(src / "run.sh", 0o755)
    os.utime(src / "SKILL.md", (PAST, PAST))
    os.utime(src / "sub" / "helper.py", (PAST, PAST))
    os.utime(src / "run.sh", (PAST, PAST))
    os.utime(src / "sub", (PAST, PAST))


def assert_tree_identity(test: unittest.TestCase, left: Path, right: Path) -> None:
    """Two copied trees must be identical: structure, content, modes, mtimes."""
    left_entries = sorted(
        p for p in left.rglob("*")
    )
    right_entries = sorted(
        p for p in right.rglob("*")
    )
    left_rel = [p.relative_to(left) for p in left_entries]
    right_rel = [p.relative_to(right) for p in right_entries]
    test.assertEqual(left_rel, right_rel, "tree structure differs")
    for rel in left_rel:
        lp, rp = left / rel, right / rel
        test.assertEqual(
            lp.stat().st_mode & 0o7777,
            rp.stat().st_mode & 0o7777,
            f"mode differs for {rel}",
        )
        test.assertEqual(lp.is_dir(), rp.is_dir(), f"kind differs for {rel}")
        if lp.is_file():
            test.assertEqual(lp.read_bytes(), rp.read_bytes(), f"content differs for {rel}")
        test.assertEqual(
            lp.stat().st_mtime,
            rp.stat().st_mtime,
            f"mtime differs for {rel}",
        )


class _CopyBridgeTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_copy_bridge"
        )

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        self.home = self.tmp / "home"
        self.home.mkdir()

    def stub(self, body: str) -> Path:
        path = self.tmp / f"worktree-gate-stub-{abs(hash(body))}"
        path.write_text(f"#!/bin/sh\n{body}\n")
        path.chmod(0o755)
        return path

    def pin_binary(self, path: Path) -> None:
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(path)})
        pin.start()
        self.addCleanup(pin.stop)

    def forbid_python_copies(self):
        """Fails loudly if a bridged call reaches the temporary fallbacks."""
        stack = contextlib.ExitStack()
        for name in (
            "_python_bundled_skill_copy",
            "_python_command_copy",
            "_python_doc_copy",
        ):
            stack.enter_context(
                mock.patch.object(
                    self.mod,
                    name,
                    side_effect=AssertionError(f"{name} ran: the bridge fell back"),
                )
            )
        return stack

    def run_materialize(self, fn, *args, **kwargs) -> tuple[str, str]:
        out, err = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            fn(*args, **kwargs)
        return out.getvalue(), err.getvalue()

    def recipe_root(self, name: str) -> dict:
        """A project root + a recipe dir with a bundled skill, command, doc."""
        root = self.tmp / name / "repo"
        (root / "ai-specs").mkdir(parents=True)
        recipe_dir = self.tmp / name / "catalog" / "worktree-flow"
        make_skill_tree(recipe_dir / "skills" / "my-skill")
        (recipe_dir / "commands").mkdir(parents=True)
        (recipe_dir / "commands" / "deploy.md").write_text("deploy steps v2")
        (recipe_dir / "docs").mkdir(parents=True)
        (recipe_dir / "docs" / "guide.md").write_text("guide v2")
        return {"root": root, "recipe_dir": recipe_dir}


class CopyApplyGoAuthorityTests(_CopyBridgeTestCase):
    """The Go path is authoritative whenever a verified binary runs."""

    @classmethod
    def setUpClass(cls):
        super().setUpClass()
        cls.binary = gate_binary()

    def setUp(self):
        super().setUp()
        self.pin_binary(self.binary)
        self.mod._project_cache_module = None  # re-resolve per fixture

    def _dest_skill(self, fixture: dict) -> Path:
        pc = self.mod._load_project_cache()
        return (
            pc.recipe_skills_root(fixture["root"], cli_home=self.home)
            / "worktree-flow"
            / "skills"
            / "my-skill"
        )

    def _dest_command(self, fixture: dict) -> Path:
        pc = self.mod._load_project_cache()
        return pc.commands_dir(fixture["root"], cli_home=self.home) / "deploy.md"

    def test_bundled_skill_go_path_matches_the_python_reference(self):
        go = self.recipe_root("go-skill")
        py = self.recipe_root("py-skill")
        with self.forbid_python_copies():
            go_out, go_err = self.run_materialize(
                self.mod.materialize_bundled_skill,
                go["recipe_dir"], "my-skill", go["root"], "worktree-flow",
                cli_home=self.home,
            )
        py_out, py_err = self.run_materialize(
            self.mod.materialize_bundled_skill,
            py["recipe_dir"], "my-skill", py["root"], "worktree-flow",
            cli_home=self.home,
        )
        self.assertEqual(go_out, py_out)
        self.assertIn("    ✓ bundled skill my-skill", go_out)
        self.assertEqual(go_err, py_err, "stderr parity (no fallback warning)")
        assert_tree_identity(
            self, self._dest_skill(py), self._dest_skill(go)
        )
        # Hashing + lock writes stay Python-owned and must agree byte for byte.
        # (The current authoritative writer drops legacy [recipes] sections —
        # pinned WU1b behavior — so byte equality is the parity that matters.)
        go_lock = go["root"] / "ai-specs" / ".ai-specs.lock"
        py_lock = py["root"] / "ai-specs" / ".ai-specs.lock"
        self.assertEqual(go_lock.read_bytes(), py_lock.read_bytes())

    def test_command_go_path_warns_and_overwrites_like_the_reference(self):
        go = self.recipe_root("go-cmd")
        py = self.recipe_root("py-cmd")
        for fixture in (go, py):
            dest = self._dest_command(fixture)
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_text("deploy steps v1")
        with self.forbid_python_copies():
            go_out, go_err = self.run_materialize(
                self.mod.materialize_command,
                go["recipe_dir"],
                SimpleNamespace(id="deploy", path="commands/deploy.md"),
                go["root"],
                cli_home=self.home,
            )
        py_out, py_err = self.run_materialize(
            self.mod.materialize_command,
            py["recipe_dir"],
            SimpleNamespace(id="deploy", path="commands/deploy.md"),
            py["root"],
            cli_home=self.home,
        )
        self.assertEqual(go_out, py_out)
        self.assertIn("    ✓ command deploy", go_out)
        # The warn embeds each fixture's dest path (the cache dir is keyed by
        # a hash of the project root); normalize that hash before comparing.
        self.assertEqual(
            re.sub(r"[0-9a-f]{12}-repo", "REPO", go_err),
            re.sub(r"[0-9a-f]{12}-repo", "REPO", py_err),
        )
        self.assertIn(
            "recipe command 'deploy' overwrites existing managed command",
            go_err,
        )
        self.assertEqual(
            self._dest_command(go).read_bytes(),
            self._dest_command(py).read_bytes(),
        )

    def test_command_go_path_identical_dest_does_not_warn(self):
        fixture = self.recipe_root("go-cmd-quiet")
        dest = self._dest_command(fixture)
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text("deploy steps v2")
        with self.forbid_python_copies():
            out, err = self.run_materialize(
                self.mod.materialize_command,
                fixture["recipe_dir"],
                SimpleNamespace(id="deploy", path="commands/deploy.md"),
                fixture["root"],
                cli_home=self.home,
            )
        self.assertIn("    ✓ command deploy", out)
        self.assertEqual(err, "")

    def test_doc_go_path_matches_the_python_reference(self):
        go = self.recipe_root("go-doc")
        py = self.recipe_root("py-doc")
        with self.forbid_python_copies():
            go_out, go_err = self.run_materialize(
                self.mod.materialize_doc,
                go["recipe_dir"],
                SimpleNamespace(source="docs/guide.md", target="docs/guide.md"),
                go["root"],
            )
        py_out, py_err = self.run_materialize(
            self.mod.materialize_doc,
            py["recipe_dir"],
            SimpleNamespace(source="docs/guide.md", target="docs/guide.md"),
            py["root"],
        )
        self.assertEqual(go_out, py_out)
        self.assertIn("    ✓ doc docs/guide.md", go_out)
        self.assertEqual(go_err, py_err)
        self.assertEqual(
            (go["root"] / "docs" / "guide.md").read_bytes(),
            (py["root"] / "docs" / "guide.md").read_bytes(),
        )

    def test_source_missing_raises_the_exact_runtime_error_on_the_go_path(self):
        fixture = self.recipe_root("go-missing")
        cases = [
            (
                lambda f: self.run_materialize(
                    self.mod.materialize_bundled_skill,
                    f["recipe_dir"], "no-such-skill", f["root"], "worktree-flow",
                    cli_home=self.home,
                ),
                lambda f: f["recipe_dir"] / "skills" / "no-such-skill",
                "bundled skill not found: ",
            ),
            (
                lambda f: self.run_materialize(
                    self.mod.materialize_command,
                    f["recipe_dir"],
                    SimpleNamespace(id="deploy", path="commands/missing.md"),
                    f["root"],
                    cli_home=self.home,
                ),
                lambda f: f["recipe_dir"] / "commands" / "missing.md",
                "command source not found: ",
            ),
            (
                lambda f: self.run_materialize(
                    self.mod.materialize_doc,
                    f["recipe_dir"],
                    SimpleNamespace(source="docs/missing.md", target="docs/missing.md"),
                    f["root"],
                ),
                lambda f: f["recipe_dir"] / "docs" / "missing.md",
                "doc source not found: ",
            ),
        ]
        for run, src_of, prefix in cases:
            with self.subTest(prefix=prefix), self.forbid_python_copies():
                with self.assertRaises(RuntimeError) as ctx:
                    run(fixture)
                self.assertEqual(str(ctx.exception), f"{prefix}{src_of(fixture)}")

    def test_command_dest_directory_fails_open_and_copies_into_it(self):
        """A dest directory: Go's decision sees overwrite, its execution fails,
        the bridge fails open, and the reference copy2 copies INTO the dir."""
        fixture = self.recipe_root("go-cmd-dir")
        dest = self._dest_command(fixture)
        dest.mkdir(parents=True)
        out, err = self.run_materialize(
            self.mod.materialize_command,
            fixture["recipe_dir"],
            SimpleNamespace(id="deploy", path="commands/deploy.md"),
            fixture["root"],
            cli_home=self.home,
        )
        self.assertEqual(err.count(self.mod.GO_COPY_APPLY_BRIDGE_FALLBACK), 1, err)
        self.assertIn("    ✓ command deploy", out)
        self.assertEqual(
            (dest / "deploy.md").read_bytes(), b"deploy steps v2"
        )

    def test_envelope_contract_one_item_per_call_with_commands_dir(self):
        fixture = self.recipe_root("go-envelope")
        argv_capture = self.tmp / "argv"
        stdin_capture = self.tmp / "stdin"
        payload = json.dumps({"results": [{"id": "deploy", "status": "ok"}]})
        stub = self.stub(
            f"printf '%s' \"$1\" > '{argv_capture}'\n"
            f"cat > '{stdin_capture}'\n"
            f"printf '%s' '{payload}'\n"
        )
        self.pin_binary(stub)
        pc = self.mod._load_project_cache()
        dest = self._dest_command(fixture)
        out, _ = self.run_materialize(
            self.mod.materialize_command,
            fixture["recipe_dir"],
            SimpleNamespace(id="deploy", path="commands/deploy.md"),
            fixture["root"],
            cli_home=self.home,
        )
        self.assertEqual(argv_capture.read_text(), "--apply-copy")
        self.assertEqual(
            json.loads(stdin_capture.read_text()),
            {
                "items": [
                    {
                        "kind": "command",
                        "id": "deploy",
                        "src": str(fixture["recipe_dir"] / "commands" / "deploy.md"),
                        "dest": str(dest),
                        "commands_dir": str(pc.commands_dir(fixture["root"], cli_home=self.home)),
                    }
                ]
            },
        )
        self.assertEqual(out.count("    ✓ command deploy"), 1)

    def test_timeout_matches_the_bridge_family(self):
        self.assertEqual(self.mod.GO_COPY_APPLY_BRIDGE_TIMEOUT_SECONDS, 60)


class CopyApplyFallbackTests(_CopyBridgeTestCase):
    """No usable Go authority: the Python copy bodies run, with one warning."""

    INFRASTRUCTURE_CASES = {
        "missing-binary": lambda self: self.pin_binary(self.tmp / "no-such-gate"),
        "resolution-raises": lambda self: mock.patch.object(
            self.mod, "_load_gate_binary", side_effect=RuntimeError("unloadable")
        ),
        "oserror": lambda self: (
            self.pin_binary(self.stub("exit 0")),
            mock.patch.object(
                self.mod.subprocess, "run", side_effect=OSError("no exec")
            ),
        ),
        "subprocess-timeout": lambda self: (
            self.pin_binary(self.stub("exit 0")),
            mock.patch.object(
                self.mod.subprocess,
                "run",
                side_effect=subprocess.TimeoutExpired(cmd="gate", timeout=60),
            ),
        ),
        "invalid-utf8": lambda self: self.pin_binary(
            self.stub("printf '\\377\\376not utf8'")
        ),
        "non-json": lambda self: self.pin_binary(self.stub("echo 'not json'")),
        "exit-two-error-envelope": lambda self: self.pin_binary(
            self.stub("printf '%s' '{\"error\": \"boom refusal\"}'; exit 2")
        ),
        "envelope-mismatch": lambda self: self.pin_binary(
            self.stub("printf '%s' '{\"results\": \"yes\"}'")
        ),
    }

    INJECTED_REASON_MARKERS = {
        "resolution-raises": "unloadable",
        "oserror": "no exec",
        "subprocess-timeout": "timed out",
        "exit-two-error-envelope": "boom refusal",
    }

    def test_all_failures_fall_back_with_one_warning_and_python_executes(self):
        for case, prepare in self.INFRASTRUCTURE_CASES.items():
            with self.subTest(case=case):
                fixture = self.recipe_root(f"fb-{case}")
                with contextlib.ExitStack() as stack:
                    prepared = prepare(self)
                    effects = prepared if isinstance(prepared, tuple) else (prepared,)
                    for effect in effects:
                        if hasattr(effect, "__enter__"):
                            stack.enter_context(effect)
                    out, err = self.run_materialize(
                        self.mod.materialize_command,
                        fixture["recipe_dir"],
                        SimpleNamespace(id="deploy", path="commands/deploy.md"),
                        fixture["root"],
                        cli_home=self.home,
                    )
                self.assertEqual(
                    err.count(self.mod.GO_COPY_APPLY_BRIDGE_FALLBACK), 1, err
                )
                marker = self.INJECTED_REASON_MARKERS.get(case)
                if marker is not None:
                    self.assertIn(marker, err, err)
                self.assertIn("    ✓ command deploy", out)
                self.assertEqual(
                    self._dest_command(fixture).read_bytes(), b"deploy steps v2"
                )

    def _dest_command(self, fixture: dict) -> Path:
        pc = self.mod._load_project_cache()
        return pc.commands_dir(fixture["root"], cli_home=self.home) / "deploy.md"

    def test_fallback_bundled_skill_rewrites_dest_wholesale(self):
        """A partial Go copy (here: none at all) is safe to redo: the fallback
        rmtree+copytree replaces dest wholesale."""
        fixture = self.recipe_root("fb-skill")
        self.pin_binary(self.stub("printf '%s' '{\"error\": \"partial\"}'; exit 2"))
        dest = fixture["root"] / "cache" / "stale"
        # The pinned binary always fails, so run against the real dest via the
        # normal call; pre-seed a stale dest tree.
        pc = self.mod._load_project_cache()
        dest = pc.recipe_skills_root(fixture["root"], cli_home=self.home) / "worktree-flow" / "skills" / "my-skill"
        dest.mkdir(parents=True)
        (dest / "stale.txt").write_text("old")
        out, err = self.run_materialize(
            self.mod.materialize_bundled_skill,
            fixture["recipe_dir"], "my-skill", fixture["root"], "worktree-flow",
            cli_home=self.home,
        )
        self.assertEqual(err.count(self.mod.GO_COPY_APPLY_BRIDGE_FALLBACK), 1, err)
        self.assertIn("    ✓ bundled skill my-skill", out)
        self.assertFalse((dest / "stale.txt").exists())
        assert_tree_identity(
            self, fixture["recipe_dir"] / "skills" / "my-skill", dest
        )

    def test_fallback_source_missing_raises_the_exact_runtime_error(self):
        fixture = self.recipe_root("fb-missing")
        self.pin_binary(self.tmp / "no-such-gate")
        with self.assertRaises(RuntimeError) as ctx:
            self.run_materialize(
                self.mod.materialize_doc,
                fixture["recipe_dir"],
                SimpleNamespace(source="docs/missing.md", target="docs/missing.md"),
                fixture["root"],
            )
        self.assertEqual(
            str(ctx.exception),
            f"doc source not found: {fixture['recipe_dir'] / 'docs' / 'missing.md'}",
        )

    def test_fallback_warning_matches_the_bridge_family_format(self):
        self.pin_binary(self.tmp / "no-such-gate")
        fixture = self.recipe_root("fb-format")
        _, err = self.run_materialize(
            self.mod.materialize_command,
            fixture["recipe_dir"],
            SimpleNamespace(id="deploy", path="commands/deploy.md"),
            fixture["root"],
            cli_home=self.home,
        )
        (line,) = [
            ln for ln in err.splitlines()
            if self.mod.GO_COPY_APPLY_BRIDGE_FALLBACK in ln
        ]
        self.assertTrue(
            line.startswith(f"  ! {self.mod.GO_COPY_APPLY_BRIDGE_FALLBACK}: "), line
        )
        self.assertTrue(
            line.endswith("; using the temporary Python copy authority"), line
        )

    def test_dep_skill_boundary_stays_python(self):
        """materialize_dep_skill is true acquisition (vendor-skills.py), not a
        blind copy: the bridge must not have swallowed it (rank-3 precedent)."""
        source = inspect.getsource(self.mod.materialize_dep_skill)
        self.assertIn("vendor-skills.py", source)
        self.assertIn("sync_dep_target", source)
        self.assertNotIn("go_apply_copy", source)


if __name__ == "__main__":
    unittest.main()
