"""Contract tests for the Go managed-override classification bridge in ``util.py``.

``worktree-gate --plan-classify`` owns the ownership DECISION: the
missing/untracked/user_modified/managed_current/managed_stale state derived from
the destination bytes, the lock metadata, and the would-write bytes.
``util.classify_managed_override`` keeps the argument adaptation it already
performed (resolving the would-write bytes from ``catalog_src``) and is
otherwise a thin bridge over the JSON envelope.

The Python decision survives as a TEMPORARY fail-open fallback
(``GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK``) and these tests pin both seams:

* bridge results equal the retained Python authority case by case,
* the stdin/stdout envelope contract of ``--plan-classify``,
* the degraded path when no verified binary can run.

The Go path needs a built binary (``dist/worktree-gate-current`` or
``$WORKTREE_GATE_BIN``); it skips loudly when none exists. Every fallback test
runs with no usable binary at all, because failing open is the contract they pin.
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]

UTIL_PATH = ROOT / "lib" / "_internal" / "util.py"
DIST_BINARY = ROOT / "dist" / "worktree-gate-current"


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
        "WORKTREE_GATE_BIN); the Go classify bridge cannot be proven without it"
    )


# One case per ownership state. ``dest`` is None for the missing state; every
# other field feeds the stdin envelope and the Python authority identically.
STATES = (
    ("missing", None, {"sha256": "a" * 64}, b"catalog", "missing"),
    ("untracked", b"catalog", None, b"catalog", "untracked"),
    (
        "user_modified",
        b"user edit",
        {"sha256": None},  # replaced below with sha256("catalog")
        b"catalog",
        "user_modified",
    ),
    ("managed_current", b"catalog", {"sha256": None}, b"catalog", "managed_current"),
    ("managed_stale", b"catalog", {"sha256": None}, b"evolved", "managed_stale"),
)


class _ClassifyBridgeTestCase(unittest.TestCase):
    """Shared fixture: the module under test and a pinned verified binary."""

    @classmethod
    def setUpClass(cls):
        cls.binary = gate_binary()
        cls.mod = load_module(UTIL_PATH, "util_classify_bridge")

    def setUp(self):
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(self.binary)})
        pin.start()
        self.addCleanup(pin.stop)
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)

    def _forbid_python_authority(self) -> None:
        """Fail loudly if a bridged call reaches the temporary fallback."""
        patcher = mock.patch.object(
            self.mod,
            "_python_classify_managed_override",
            side_effect=AssertionError(
                "_python_classify_managed_override ran: the bridge fell back"
            ),
        )
        patcher.start()
        self.addCleanup(patcher.stop)

    def _case(self, name: str, disk, managed_sha, would_write: bytes, expected: str):
        dest = self.tmp / f"{name}.md"
        if disk is not None:
            dest.write_bytes(disk)
        catalog_sha = self.mod.sha256_bytes(b"catalog")
        managed_entry = None if managed_sha is None else {"sha256": catalog_sha}
        self._forbid_python_authority()
        state = self.mod.classify_managed_override(
            dest, managed_entry, would_write=would_write
        )
        self.assertEqual(state, expected)


class ClassifyParityTests(_ClassifyBridgeTestCase):
    """Bridge results equal the Python authority that used to compute them."""

    def test_each_ownership_state_round_trips_through_the_bridge(self):
        for name, disk, managed_sha, would_write, expected in STATES:
            with self.subTest(case=name):
                self._case(name, disk, managed_sha, would_write, expected)

    def test_managed_state_uses_catalog_src_rendering_via_the_bridge(self):
        # The bridge adapts a ``Path`` catalog_src into the exact would-write
        # bytes, so the Python fallback contract is preserved without running it.
        dest = self.tmp / "rendered.md"
        dest.write_text("topology auto")
        rendered = self.tmp / "template.md"
        rendered.write_text("topology __WORKTREE_REPO_TOPOLOGY__")
        managed = {"sha256": self.mod.sha256_bytes(b"topology auto")}
        self._forbid_python_authority()
        state = self.mod.classify_managed_override(
            dest, managed, catalog_src=rendered
        )
        self.assertEqual(state, "managed_current")

    def test_missing_destination_wins_over_a_managed_entry(self):
        dest = self.tmp / "absent.md"
        self._forbid_python_authority()
        self.assertEqual(
            self.mod.classify_managed_override(
                dest, {"sha256": "b" * 64}, would_write=b"catalog"
            ),
            "missing",
        )


class ClassifyEnvelopeContractTests(_ClassifyBridgeTestCase):
    """The stdin/stdout contract of ``worktree-gate --plan-classify``."""

    def _stub(self, body: str) -> Path:
        path = self.tmp / "worktree-gate-stub"
        path.write_text(f"#!/bin/sh\n{body}\n")
        path.chmod(0o755)
        return path

    def test_bridge_invokes_plan_classify_with_json_stdin(self):
        argv_capture = self.tmp / "argv"
        stdin_capture = self.tmp / "stdin"
        stub = self._stub(
            f"printf '%s' \"$1\" > '{argv_capture}'\n"
            f"cat > '{stdin_capture}'\n"
            "printf '%s' '{\"state\": \"managed_current\", \"dest\": \"ignored\"}'"
        )
        dest = self.tmp / "card.md"
        dest.write_text("catalog")
        managed = {"sha256": self.mod.sha256_bytes(b"catalog")}
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            state = self.mod.classify_managed_override(
                dest, managed, would_write=b"catalog"
            )
        self.assertEqual(state, "managed_current")
        self.assertEqual(argv_capture.read_text(), "--plan-classify")
        self.assertEqual(
            json.loads(stdin_capture.read_text()),
            {
                "dest": str(dest),
                "managed_entry": managed,
                "would_write": "catalog",
            },
        )

    def test_absent_would_write_is_sent_as_null(self):
        stdin_capture = self.tmp / "stdin"
        stub = self._stub(
            f"cat > '{stdin_capture}'\n"
            "printf '%s' '{\"state\": \"managed_current\", \"dest\": \"ignored\"}'"
        )
        dest = self.tmp / "card.md"
        dest.write_text("catalog")
        managed = {"sha256": self.mod.sha256_bytes(b"catalog")}
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            state = self.mod.classify_managed_override(dest, managed)
        self.assertEqual(state, "managed_current")
        self.assertIsNone(json.loads(stdin_capture.read_text())["would_write"])

    def test_bad_shape_envelope_falls_back_to_python(self):
        # A syntactically valid JSON object that is not the classify envelope
        # must never be treated as authority.
        stub = self._stub("printf '%s' '{\"state\": \"not_a_state\"}'")
        dest = self.tmp / "card.md"
        dest.write_text("catalog")
        managed = {"sha256": self.mod.sha256_bytes(b"catalog")}
        captured = io.StringIO()
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            with contextlib.redirect_stderr(captured):
                state = self.mod.classify_managed_override(dest, managed)
        self.assertEqual(state, "managed_current")
        self.assertEqual(
            captured.getvalue().count(self.mod.GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK), 1
        )


class ClassifyFailOpenFallbackTests(unittest.TestCase):
    """No usable binary: the legacy Python decision runs, with one warning."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(UTIL_PATH, "util_classify_bridge_fallback")

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        home = self.tmp / "home"
        home.mkdir()
        # No binary anywhere: an empty override short-circuits the cache lookup
        # and AI_SPECS_HOME points at an empty cache root.
        env = {"WORKTREE_GATE_BIN": "", "AI_SPECS_HOME": str(home)}
        pin = mock.patch.dict(os.environ, env)
        pin.start()
        self.addCleanup(pin.stop)
        self.dest = self.tmp / "card.md"
        self.dest.write_text("user edit")
        self.managed = {"sha256": self.mod.sha256_bytes(b"catalog")}

    def _stub(self, body: str) -> Path:
        path = self.tmp / "worktree-gate-stub"
        path.write_text(f"#!/bin/sh\n{body}\n")
        path.chmod(0o755)
        return path

    def _run(self) -> str:
        captured = io.StringIO()
        with contextlib.redirect_stderr(captured):
            state = self.mod.classify_managed_override(
                self.dest, self.managed, would_write=b"catalog"
            )
        self.assertEqual(state, "user_modified", "the Python authority result")
        return captured.getvalue()

    def test_missing_binary_falls_back_to_python_with_one_warning(self):
        stderr = self._run()
        self.assertEqual(
            stderr.count(self.mod.GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("temporary", stderr.lower())
        self.assertIn("no verified worktree-gate binary", stderr)

    def test_binary_that_fails_falls_back_with_one_warning(self):
        stub = self._stub("exit 2")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            stderr = self._run()
        self.assertEqual(
            stderr.count(self.mod.GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("exited 2", stderr)

    def test_unreadable_binary_output_falls_back_with_one_warning(self):
        stub = self._stub("echo not-json")
        with mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            stderr = self._run()
        self.assertEqual(
            stderr.count(self.mod.GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK), 1, stderr
        )

    def test_subprocess_failure_falls_back_with_one_warning(self):
        stub = self._stub("exit 0")
        with mock.patch.object(
            self.mod.subprocess,
            "run",
            side_effect=subprocess.TimeoutExpired("worktree-gate", 60),
        ), mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(stub)}):
            stderr = self._run()
        self.assertEqual(
            stderr.count(self.mod.GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("did not run", stderr)

    def test_fallback_path_is_marked_temporary_in_the_source(self):
        self.assertEqual(
            self.mod.GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK,
            "GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK",
        )
        doc = self.mod._python_classify_managed_override.__doc__ or ""
        self.assertIn("GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK", doc)
        self.assertIn("TEMPORARY", doc)


if __name__ == "__main__":
    unittest.main()
