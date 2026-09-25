"""Contract tests for the Go lock-write bridge in ``lock.py`` (GO-08 WU2).

``worktree-gate --write-lock`` owns the full-state lock write: the Go port of
``write_lock`` performs the byte-exact TOML emission and the atomic replace.
Python keeps the retained writer as a TEMPORARY fail-open fallback
(``GO_LOCK_WRITE_BRIDGE_FALLBACK``) and these tests pin both seams:

* the bridge-written lock is byte-identical to the retained Python writer's
  lock (including legacy-section dropping and no temp files left behind),
* the stdout envelope contract (``{"written": true}`` on exit 0),
* fail open on ALL failures — including a Go refusal (exit 2 with a stdout
  ``{"error": "<string>"}`` envelope). This is a deliberate divergence from
  the recipe-config bridge, which fails closed on refusals: the lock write is
  a full-state idempotent atomic replace, so the Python fallback can only
  rewrite the same correct state and there is no destructive ambiguity to
  protect against.

The Go path needs a built binary (``dist/worktree-gate-current`` or
``$WORKTREE_GATE_BIN``); it skips loudly when none exists. Every fallback test
runs with no usable binary at all, because failing open is the contract they
pin.
"""
from __future__ import annotations

import contextlib
import copy
import importlib.util
import io
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]

LOCK_PATH = ROOT / "lib" / "_internal" / "lock.py"
DIST_BINARY = ROOT / "dist" / "worktree-gate-current"

# Realistic lock dict: meta scalars, managed overrides, per-harness agent
# files, plus legacy skills/recipes/deps groups that neither authority may
# re-emit.
LOCK = {
    "skills": {"trello-mcp-workflow": {"SKILL.md": {"SKILL.md": "legacyhash"}}},
    "meta": {"cli_version": "0.42.0", "synced_at": "2026-01-15T10:00:00Z"},
    "recipes": {"worktree-flow": {"SKILL.md": {"SKILL.md": "legacyhash"}}},
    "deps": {},
    "agents": {
        "claude": {"SKILL.md": "agenthash-a", "sub/deep.md": "agenthash-b"},
        "pi": {"AGENTS.md": "agenthash-c"},
    },
    "managed": {
        "pi/AGENTS.md": {
            "sha256": "managedhash-a",
            "recipe": "worktree-flow",
            "kind": "gate",
            "policy": "auto",
        },
        "ai-specs/AGENTS.md": {
            "sha256": "managedhash-b",
            "kind": "runtime-brief",
            "policy": "never-force",
        },
    },
}


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
        "WORKTREE_GATE_BIN); the Go lock-write bridge cannot be proven "
        "without it"
    )


class _BridgeTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(LOCK_PATH, "lock_bridge")

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)

    def lock_copy(self) -> dict:
        return copy.deepcopy(LOCK)

    def stub(self, body: str) -> Path:
        path = self.tmp / f"worktree-gate-stub-{abs(hash(body))}"
        path.write_text(f"#!/bin/sh\n{body}\n")
        path.chmod(0o755)
        return path

    def pin_binary(self, path: Path) -> None:
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(path)})
        pin.start()
        self.addCleanup(pin.stop)

    def run_write(self, lock_path: Path, lock: dict | None = None) -> str:
        """Run write_lock capturing stderr; return stderr."""
        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            self.mod.write_lock(lock_path, lock if lock is not None else self.lock_copy())
        return stderr.getvalue()

    def forbid_python_writer(self):
        """Fails loudly if a bridged call reaches the temporary fallback."""
        return mock.patch.object(
            self.mod,
            "_write_lock_python",
            side_effect=AssertionError(
                "_write_lock_python ran: the bridge fell back"
            ),
        )

    def reference_bytes(self) -> bytes:
        """What the retained pure Python writer alone produces for LOCK."""
        path = self.tmp / "reference.ai-specs.lock"
        self.mod._write_lock_python(path, self.lock_copy())
        return path.read_bytes()

    def legacy_lock_file(self, name: str) -> Path:
        """An on-disk lock carrying legacy [skills]/[recipes] sections."""
        path = self.tmp / name
        path.write_text(
            "# Managed by ai-specs. Do not edit by hand.\n"
            "[skills.\"trello-mcp-workflow\"]\n"
            "[skills.\"trello-mcp-workflow\".SKILL.md]\n"
            '"SKILL.md" = "legacyhash"\n'
            "\n"
            "[recipes.worktree-flow]\n"
            "[recipes.worktree-flow.SKILL.md]\n"
            '"SKILL.md" = "legacyhash"\n'
            "\n"
            "[meta]\n"
            'cli_version = "0.41.0"\n'
            'synced_at = "2025-12-01T09:00:00Z"\n',
            encoding="utf-8",
        )
        return path


class GoLockWriteAuthorityTests(_BridgeTestCase):
    """The Go path is authoritative whenever a verified binary runs."""

    @classmethod
    def setUpClass(cls):
        super().setUpClass()
        cls.binary = gate_binary()

    def setUp(self):
        super().setUp()
        self.pin_binary(self.binary)

    def test_go_bridge_writes_byte_identical_lock_without_the_python_writer(self):
        path = self.tmp / "ai-specs.lock"
        with self.forbid_python_writer():
            stderr = self.run_write(path)
        self.assertEqual(stderr, "")
        self.assertEqual(path.read_bytes(), self.reference_bytes())
        self.assertEqual(self.mod.GO_LOCK_WRITE_BRIDGE_TIMEOUT_SECONDS, 60)

    def test_go_bridge_drops_legacy_sections_through_the_dispatcher(self):
        legacy = self.legacy_lock_file("legacy.ai-specs.lock")
        lock = self.mod.load_lock(legacy)
        stderr = self.run_write(legacy, lock)
        self.assertEqual(stderr, "")
        text = legacy.read_text(encoding="utf-8")
        self.assertNotIn("[skills.", text)
        self.assertNotIn("[recipes.", text)
        self.assertEqual(legacy.read_bytes(), self.reference_bytes_for(lock))

    def reference_bytes_for(self, lock: dict) -> bytes:
        path = self.tmp / "reference-legacy.ai-specs.lock"
        self.mod._write_lock_python(path, lock)
        return path.read_bytes()

    def test_go_bridge_leaves_no_temp_files_behind(self):
        path = self.tmp / "ai-specs.lock"
        with self.forbid_python_writer():
            self.run_write(path)
        self.assertEqual(sorted(p.name for p in self.tmp.iterdir()), [path.name])

    def test_go_bridge_rewrites_a_missing_parent_directory(self):
        path = self.tmp / "deep" / "nested" / "ai-specs.lock"
        with self.forbid_python_writer():
            stderr = self.run_write(path)
        self.assertEqual(stderr, "")
        self.assertEqual(path.read_bytes(), self.reference_bytes())


class LockWriteFallbackTests(_BridgeTestCase):
    """No usable Go authority: the retained Python writer runs, with one warning."""

    INFRASTRUCTURE_CASES = {
        # A pinned-but-missing binary: resolution step 1 yields None.
        "missing-binary": lambda self: self.pin_binary(self.tmp / "no-such-gate"),
        # An unloadable gate_binary helper: resolution raises.
        "resolution-raises": lambda self: mock.patch.object(
            self.mod, "_load_gate_binary", side_effect=RuntimeError("unloadable")
        ),
        # The process never starts. Resolution must yield a binary first, so
        # the case pins one; its behavior is irrelevant because subprocess.run
        # is patched.
        "oserror": lambda self: (
            self.pin_binary(self.stub("exit 0")),
            mock.patch.object(
                self.mod.subprocess, "run", side_effect=OSError("no exec")
            ),
        ),
        # The process outlives the bridge timeout (same pin rationale).
        "subprocess-timeout": lambda self: (
            self.pin_binary(self.stub("exit 0")),
            mock.patch.object(
                self.mod.subprocess,
                "run",
                side_effect=subprocess.TimeoutExpired(cmd="gate", timeout=60),
            ),
        ),
        # text=True decodes stdout strictly; invalid bytes must not escape.
        "invalid-utf8": lambda self: self.pin_binary(
            self.stub("printf '\\377\\376not utf8'")
        ),
        # Non-JSON stdout on a successful exit.
        "non-json": lambda self: self.pin_binary(self.stub("echo 'not json'")),
        # Exit-2 error envelope: a Go refusal must ALSO fail open here (the
        # pinned divergence from the recipe-config bridge).
        "exit-two-error-envelope": lambda self: self.pin_binary(
            self.stub("printf '%s' '{\"error\": \"boom refusal\"}'; exit 2")
        ),
        # Exit 0 but the documented written envelope shape, violated.
        "envelope-mismatch": lambda self: self.pin_binary(
            self.stub("printf '%s' '{\"written\": \"yes\"}'")
        ),
    }

    # Injected-fault cases: the fallback warning's <reason> must name the
    # injected fault, not the unrelated missing-binary path.
    INJECTED_REASON_MARKERS = {
        "resolution-raises": "unloadable",
        "oserror": "no exec",
        "subprocess-timeout": "timed out",
        "exit-two-error-envelope": "boom refusal",
    }

    def test_all_failures_fall_back_with_one_warning_and_identical_bytes(self):
        for case, prepare in self.INFRASTRUCTURE_CASES.items():
            with self.subTest(case=case), contextlib.ExitStack() as stack:
                # pin_binary cases return None (already active); the injected
                # fault cases return patchers, or a (pin, patcher) tuple, that
                # must be entered to fire.
                prepared = prepare(self)
                effects = prepared if isinstance(prepared, tuple) else (prepared,)
                for effect in effects:
                    if hasattr(effect, "__enter__"):
                        stack.enter_context(effect)
                path = self.tmp / f"{case}.ai-specs.lock"
                stderr = self.run_write(path)
                self.assertEqual(
                    stderr.count(self.mod.GO_LOCK_WRITE_BRIDGE_FALLBACK),
                    1,
                    stderr,
                )
                marker = self.INJECTED_REASON_MARKERS.get(case)
                if marker is not None:
                    self.assertIn(marker, stderr, stderr)
                self.assertEqual(path.read_bytes(), self.reference_bytes())

    def test_fallback_warning_matches_the_bridge_family_format(self):
        self.pin_binary(self.tmp / "no-such-gate")
        path = self.tmp / "ai-specs.lock"
        stderr = self.run_write(path)
        (line,) = [ln for ln in stderr.splitlines() if self.mod.GO_LOCK_WRITE_BRIDGE_FALLBACK in ln]
        self.assertTrue(
            line.startswith(f"  ! {self.mod.GO_LOCK_WRITE_BRIDGE_FALLBACK}: "),
            line,
        )
        self.assertTrue(
            line.endswith("; using the temporary Python lock-write authority"),
            line,
        )

    def test_fallback_on_refusal_keeps_the_lock_untouched_until_the_fallback_writes(self):
        # The fallback rewrites the same full state, so the end state is the
        # same correct lock either way — never a partial or missing one.
        self.pin_binary(self.stub("printf '%s' '{\"error\": \"refused\"}'; exit 2"))
        path = self.tmp / "ai-specs.lock"
        stderr = self.run_write(path)
        self.assertEqual(stderr.count(self.mod.GO_LOCK_WRITE_BRIDGE_FALLBACK), 1)
        self.assertEqual(path.read_bytes(), self.reference_bytes())


if __name__ == "__main__":
    unittest.main()
