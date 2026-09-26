"""Contract tests for the Go template-actuator bridge in ``recipe-materialize.py``
(GO-09, strangler slice 6).

``worktree-gate --materialize-template`` owns the template actuation DECISION
+ EXECUTION: git-path destination resolution, rendering, the ownership
classification (the shared classifyManagedOverride core behind
--plan-classify), the write + chmod, and the managed-override record payload.
Python keeps the lock load/write (set_managed_override + write_lock), ALL
printing (indent + print_step_output compact filtering), and the fail-open
fallback. The historical Python body survives as
``_python_materialize_template`` (``GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK``).

These tests pin both seams:

* Go-path results equal the retained Python authority: dest bytes, modes,
  exact prints, identical lock bytes, identical record shas,
* the stdin/stdout envelope contract of ``--materialize-template``,
* FAIL CLOSED on a Go refusal (nonzero exit with a stdout error envelope):
  the bridge raises ``RuntimeError`` naming the Go error instead of falling
  back, so the Python body can never bypass the Go authority's decision
  (GO-08 findings fix, mirrored for templates), and
* the degraded path for every infrastructure failure: exactly one warning,
  then the Python body executes identically.

The Go path needs a built binary (``dist/worktree-gate-current`` or
``$WORKTREE_GATE_BIN``); it skips loudly when none exists. Fallback tests run
with no usable binary, because failing open is the contract they pin.
"""
from __future__ import annotations

import contextlib
import importlib.util
import io
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]

RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
DIST_BINARY = ROOT / "dist" / "worktree-gate-current"

TARGET = "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh"
MERGED_CFG = {
    "repo_topology": "auto",
    "worktrees_dir": ".worktrees",
    "integration_branch": "main",
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
        "WORKTREE_GATE_BIN); the Go template-actuator bridge cannot be "
        "proven without it"
    )


class _TemplateBridgeTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_template_bridge"
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

    def fixture(self, name: str, body: str = "#!/bin/sh\necho hi\n", mode: int = 0o755) -> dict:
        """A project root + recipe dir with one executable template source."""
        root = self.tmp / name / "project"
        recipe_dir = self.tmp / name / "catalog" / "worktree-flow"
        (recipe_dir / "templates").mkdir(parents=True)
        (recipe_dir / "templates" / "post-merge.sh").write_text(body)
        os.chmod(recipe_dir / "templates" / "post-merge.sh", mode)
        root.mkdir(parents=True)
        return {"root": root, "recipe_dir": recipe_dir}

    def tpl(self, source: str = "templates/post-merge.sh", target: str = TARGET,
            condition: str = "not_exists", policy: str = "auto") -> SimpleNamespace:
        return SimpleNamespace(
            source=source, target=target, condition=condition, update_policy=policy
        )

    def dest_of(self, fixture: dict) -> Path:
        return fixture["root"] / TARGET

    def seed_managed(self, fixture: dict, content: bytes) -> None:
        """Pre-seed the destination AND its managed lock entry."""
        dest = self.dest_of(fixture)
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_bytes(content)
        lock_path = fixture["root"] / "ai-specs" / ".ai-specs.lock"
        lock = self.mod.load_lock(lock_path)
        self.mod.set_managed_override(
            lock, TARGET, self.mod._load_util().sha256_bytes(content),
            recipe="worktree-flow", source="templates/post-merge.sh",
            kind="template", policy="auto",
        )
        self.mod.write_lock(lock_path, lock)

    def run_materialize(self, fn, *args, **kwargs) -> tuple[str, str]:
        out, err = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            fn(*args, **kwargs)
        return out.getvalue(), err.getvalue()


class TemplateActuatorGoAuthorityTests(_TemplateBridgeTestCase):
    """The Go path is authoritative whenever a verified binary runs."""

    @classmethod
    def setUpClass(cls):
        super().setUpClass()
        cls.binary = gate_binary()

    def setUp(self):
        super().setUp()
        self.pin_binary(self.binary)

    def test_fresh_write_go_path_matches_the_python_reference(self):
        go = self.fixture("tpl-go-fresh")
        fb = self.fixture("tpl-fb-fresh")
        go_out, go_err = self.run_materialize(
            self.mod.materialize_template,
            go["recipe_dir"], self.tpl(), go["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        # The fallback reference run must not see the real binary.
        self.pin_binary(self.tmp / "no-such-gate")
        fb_out, fb_err = self.run_materialize(
            self.mod.materialize_template,
            fb["recipe_dir"], self.tpl(), fb["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        self.assertEqual(go_out, fb_out)
        self.assertIn(f"    ✓ template {TARGET}", go_out)
        self.assertEqual(go_err, "", "the Go path must not warn")
        self.assertEqual(fb_err.count(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK), 1)
        self.assertEqual(
            self.dest_of(go).read_bytes(), self.dest_of(fb).read_bytes()
        )
        self.assertEqual(
            self.dest_of(go).stat().st_mode & 0o777,
            self.dest_of(fb).stat().st_mode & 0o777,
            "source permission bits must be copied on both paths",
        )
        self.assertEqual(
            (go["root"] / "ai-specs" / ".ai-specs.lock").read_bytes(),
            (fb["root"] / "ai-specs" / ".ai-specs.lock").read_bytes(),
            "lock bytes must be identical (Python owns the lock write)",
        )

    def test_seed_rendered_copy_go_path_records_without_rewrite(self):
        body = "#!/bin/sh\necho hi\n"
        go = self.fixture("tpl-go-seed", body=body, mode=0o644)
        self.dest_of(go).parent.mkdir(parents=True)
        self.dest_of(go).write_bytes(body.encode())
        go_out, go_err = self.run_materialize(
            self.mod.materialize_template,
            go["recipe_dir"], self.tpl(), go["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        self.assertIn(f"    · template skipped (exists) {TARGET}", go_out)
        self.assertEqual(go_err, "")
        lock = self.mod.load_lock(go["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertEqual(
            lock["managed"][TARGET]["sha256"],
            self.mod._load_util().sha256_bytes(body.encode()),
            "the seeded copy's own sha is recorded",
        )

    def test_managed_stale_auto_refresh_go_path(self):
        go = self.fixture("tpl-go-stale")
        self.seed_managed(go, b"echo old\n")
        (go["recipe_dir"] / "templates" / "post-merge.sh").write_text("echo new\n")
        out, err = self.run_materialize(
            self.mod.materialize_template,
            go["recipe_dir"], self.tpl(), go["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        self.assertEqual(err, "")
        self.assertIn("  ℹ refreshed managed template " + TARGET, out)
        self.assertIn(f"    · template skipped (exists) {TARGET}", out)
        self.assertEqual(self.dest_of(go).read_bytes(), b"echo new\n")
        lock = self.mod.load_lock(go["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertEqual(lock["managed"][TARGET]["sha256"],
                         self.mod._load_util().sha256_bytes(b"echo new\n"))

    def test_user_modified_refusal_go_path_preserves_and_skips(self):
        go = self.fixture("tpl-go-user")
        self.seed_managed(go, b"echo old\n")
        self.dest_of(go).write_bytes(b"user edited\n")
        out, err = self.run_materialize(
            self.mod.materialize_template,
            go["recipe_dir"], self.tpl(), go["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        self.assertIn("override user-modified", err)
        self.assertNotIn(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK, err)
        self.assertIn(f"    · template skipped (exists) {TARGET}", out)
        self.assertEqual(self.dest_of(go).read_bytes(), b"user edited\n")

    def test_envelope_contract_one_call_per_template(self):
        fixture = self.fixture("tpl-envelope")
        argv_capture = self.tmp / "argv"
        stdin_capture = self.tmp / "stdin"
        payload = json.dumps({
            "dest": str(self.dest_of(fixture)),
            "wrote": True,
            "record": {
                "target": TARGET, "sha256": "a" * 64, "recipe": "worktree-flow",
                "source": "templates/post-merge.sh", "kind": "template",
                "policy": "auto",
            },
            "message": f"✓ template {TARGET}",
            "warnings": [],
            "error": None,
        })
        self.pin_binary(self.stub(
            f"printf '%s\\n' \"$1\" >> '{argv_capture}'\n"
            f"cat > \"{stdin_capture}.$1\"\n"
            f"printf '%s' '{payload}'\n"
        ))
        out, _ = self.run_materialize(
            self.mod.materialize_template,
            fixture["recipe_dir"], self.tpl(), fixture["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        # The stub is invoked again by the lock-write bridge (--write-lock);
        # capture the template actuator's own stdin per argv.
        self.assertEqual(argv_capture.read_text().splitlines()[0], "--materialize-template")
        sent = json.loads((self.tmp / "stdin.--materialize-template").read_text())
        self.assertEqual(
            sent,
            {
                "project_root": str(fixture["root"]),
                "recipe_dir": str(fixture["recipe_dir"]),
                "recipe_id": "worktree-flow",
                "source": "templates/post-merge.sh",
                "target": TARGET,
                "condition": "not_exists",
                "update_policy": "auto",
                "config": MERGED_CFG,
                "managed_entry": None,
            },
        )
        self.assertIn(f"    ✓ template {TARGET}", out)
        lock = self.mod.load_lock(fixture["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertEqual(lock["managed"][TARGET]["sha256"], "a" * 64)

    def test_invalid_policy_refusal_envelope_fails_closed(self):
        """A valid Go refusal (exit 2, stdout error envelope) is the
        authority's decision: RuntimeError, no fallback warning, dest
        untouched (GO-08 findings fix, mirrored for templates)."""
        fixture = self.fixture("tpl-go-refusal")
        dest = self.dest_of(fixture)
        dest.parent.mkdir(parents=True)
        dest.write_bytes(b"user stuff\n")
        with self.assertRaises(RuntimeError) as ctx:
            self.run_materialize(
                self.mod.materialize_template,
                fixture["recipe_dir"],
                self.tpl(policy="force"), fixture["root"],
                MERGED_CFG, recipe_id="worktree-flow",
            )
        self.assertIn("worktree-gate refused the template:", str(ctx.exception))
        self.assertIn("invalid update policy 'force'", str(ctx.exception))
        self.assertEqual(dest.read_bytes(), b"user stuff\n")
        lock = self.mod.load_lock(fixture["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertNotIn(TARGET, lock.get("managed", {}))

    def test_stub_refusal_envelope_fails_closed_without_fallback(self):
        fixture = self.fixture("tpl-stub-refusal")
        self.pin_binary(self.stub(
            "printf '%s' '{\"error\": \"boom refusal\"}'; exit 2"
        ))
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            with self.assertRaises(RuntimeError) as ctx:
                self.mod.materialize_template(
                    fixture["recipe_dir"], self.tpl(), fixture["root"],
                    MERGED_CFG, recipe_id="worktree-flow",
                )
        self.assertEqual(str(ctx.exception), "worktree-gate refused the template: boom refusal")
        self.assertNotIn(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK, err.getvalue())
        self.assertFalse(self.dest_of(fixture).exists())

    def test_timeout_matches_the_bridge_family(self):
        self.assertEqual(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_TIMEOUT_SECONDS, 60)


class TemplateActuatorFallbackTests(_TemplateBridgeTestCase):
    """No usable Go authority: the Python template body runs, with one warning."""

    def setUp(self):
        super().setUp()
        # Fallback tests run with no usable binary even when the outer
        # environment pins one (the verification harness sets WORKTREE_GATE_BIN).
        self.pin_binary(self.tmp / "no-such-gate")

    def test_missing_binary_falls_back_with_one_warning(self):
        fixture = self.fixture("tpl-fb-missing")
        out, err = self.run_materialize(
                self.mod.materialize_template,
                fixture["recipe_dir"], self.tpl(), fixture["root"],
                MERGED_CFG, recipe_id="worktree-flow",
            )
        self.assertEqual(err.count(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK), 1, err)
        self.assertIn(f"    ✓ template {TARGET}", out)
        self.assertEqual(self.dest_of(fixture).read_bytes(), b"#!/bin/sh\necho hi\n")
        lock = self.mod.load_lock(fixture["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertIn(TARGET, lock["managed"])

    def test_garbage_stdout_falls_back_with_one_warning(self):
        fixture = self.fixture("tpl-fb-garbage")
        self.pin_binary(self.stub("printf '%s' 'garbage'"))
        out, err = self.run_materialize(
            self.mod.materialize_template,
            fixture["recipe_dir"], self.tpl(), fixture["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        self.assertEqual(err.count(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK), 1, err)
        self.assertIn(f"    ✓ template {TARGET}", out)
        self.assertTrue(self.dest_of(fixture).is_file())

    def test_exit_2_without_error_envelope_falls_back(self):
        """Exit 2 WITHOUT a valid stdout error envelope is an infrastructure
        failure, not a refusal: the bridge fails open with one warning."""
        fixture = self.fixture("tpl-fb-exit2")
        self.pin_binary(self.stub("echo 'crashed' >&2; exit 2"))
        out, err = self.run_materialize(
            self.mod.materialize_template,
            fixture["recipe_dir"], self.tpl(), fixture["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        self.assertEqual(err.count(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK), 1, err)
        self.assertIn("exited 2 without a template envelope", err)
        self.assertIn(f"    ✓ template {TARGET}", out)

    def test_envelope_mismatch_falls_back(self):
        """A structurally invalid exit-0 envelope (record missing keys) is an
        envelope mismatch: one warning, then the Python body."""
        fixture = self.fixture("tpl-fb-mismatch")
        self.pin_binary(self.stub("printf '%s' '{\"dest\": 3}'"))
        out, err = self.run_materialize(
            self.mod.materialize_template,
            fixture["recipe_dir"], self.tpl(), fixture["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        self.assertEqual(err.count(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK), 1, err)
        self.assertIn("did not match the materialize-template envelope", err)
        self.assertTrue(self.dest_of(fixture).is_file())

    def test_record_target_mismatch_falls_back_without_lock_write(self):
        """R1-lock-record-trust: a structurally valid envelope whose record
        target does not match the sent plan is unusable — one warning, the
        Python body runs, and the mismatched record never reaches the lock."""
        fixture = self.fixture("tpl-fb-rec-target")
        payload = json.dumps({
            "dest": str(self.dest_of(fixture)),
            "wrote": True,
            "record": {
                "target": "other/evil-target.sh", "sha256": "a" * 64,
                "recipe": "worktree-flow", "source": "templates/post-merge.sh",
                "kind": "template", "policy": "auto",
            },
            "message": f"✓ template {TARGET}",
            "warnings": [],
            "error": None,
        })
        self.pin_binary(self.stub(f"printf '%s' '{payload}'"))
        out, err = self.run_materialize(
            self.mod.materialize_template,
            fixture["recipe_dir"], self.tpl(), fixture["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        self.assertEqual(err.count(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK), 1, err)
        self.assertIn("does not match the sent target", err)
        self.assertIn("other/evil-target.sh", err)
        self.assertIn(f"    ✓ template {TARGET}", out)
        self.assertEqual(self.dest_of(fixture).read_bytes(), b"#!/bin/sh\necho hi\n")
        lock = self.mod.load_lock(fixture["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertNotIn("other/evil-target.sh", lock.get("managed", {}))
        self.assertEqual(
            lock["managed"][TARGET]["sha256"],
            self.mod._load_util().sha256_bytes(b"#!/bin/sh\necho hi\n"),
            "the Python body's own record is what lands in the lock",
        )

    def test_record_source_mismatch_falls_back_without_lock_write(self):
        """R1-lock-record-trust: a record source that does not match the sent
        plan is unusable — one warning, Python body, no lock write from it."""
        fixture = self.fixture("tpl-fb-rec-source")
        payload = json.dumps({
            "dest": str(self.dest_of(fixture)),
            "wrote": True,
            "record": {
                "target": TARGET, "sha256": "a" * 64,
                "recipe": "worktree-flow", "source": "templates/evil.sh",
                "kind": "template", "policy": "auto",
            },
            "message": f"✓ template {TARGET}",
            "warnings": [],
            "error": None,
        })
        self.pin_binary(self.stub(f"printf '%s' '{payload}'"))
        out, err = self.run_materialize(
            self.mod.materialize_template,
            fixture["recipe_dir"], self.tpl(), fixture["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        self.assertEqual(err.count(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK), 1, err)
        self.assertIn("does not match the sent source", err)
        self.assertIn("templates/evil.sh", err)
        self.assertIn(f"    ✓ template {TARGET}", out)
        lock = self.mod.load_lock(fixture["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertEqual(lock["managed"][TARGET]["source"], "templates/post-merge.sh")

    def test_wrote_true_without_record_falls_back(self):
        """R3-record-null-lock-drift: ``wrote: true`` with ``record: null``
        would leave the target on disk with no ownership record — the
        envelope is unusable: one warning naming it, then the Python body
        whose own record ends up in the lock."""
        fixture = self.fixture("tpl-fb-wrote-null")
        payload = json.dumps({
            "dest": str(self.dest_of(fixture)),
            "wrote": True,
            "record": None,
            "message": f"✓ template {TARGET}",
            "warnings": [],
            "error": None,
        })
        self.pin_binary(self.stub(f"printf '%s' '{payload}'"))
        out, err = self.run_materialize(
            self.mod.materialize_template,
            fixture["recipe_dir"], self.tpl(), fixture["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        self.assertEqual(err.count(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK), 1, err)
        self.assertIn("wrote without a record", err)
        self.assertIn(f"    ✓ template {TARGET}", out)
        self.assertEqual(self.dest_of(fixture).read_bytes(), b"#!/bin/sh\necho hi\n")
        lock = self.mod.load_lock(fixture["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertEqual(
            lock["managed"][TARGET]["sha256"],
            self.mod._load_util().sha256_bytes(b"#!/bin/sh\necho hi\n"),
        )

    def test_record_sha_not_lowercase_hex_falls_back(self):
        """R1-lock-record-trust: a record sha256 that is not 64 lowercase
        hex characters is unusable — fallback, no lock write from it."""
        fixture = self.fixture("tpl-fb-rec-sha")
        payload = json.dumps({
            "dest": str(self.dest_of(fixture)),
            "wrote": True,
            "record": {
                "target": TARGET, "sha256": "A" * 64,
                "recipe": "worktree-flow", "source": "templates/post-merge.sh",
                "kind": "template", "policy": "auto",
            },
            "message": f"✓ template {TARGET}",
            "warnings": [],
            "error": None,
        })
        self.pin_binary(self.stub(f"printf '%s' '{payload}'"))
        out, err = self.run_materialize(
            self.mod.materialize_template,
            fixture["recipe_dir"], self.tpl(), fixture["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        self.assertEqual(err.count(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK), 1, err)
        self.assertIn("64 lowercase hex characters", err)
        lock = self.mod.load_lock(fixture["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertEqual(
            lock["managed"][TARGET]["sha256"],
            self.mod._load_util().sha256_bytes(b"#!/bin/sh\necho hi\n"),
        )

    def test_fallback_warning_matches_the_bridge_family_format(self):
        fixture = self.fixture("tpl-fb-format")
        _, err = self.run_materialize(
            self.mod.materialize_template,
            fixture["recipe_dir"], self.tpl(), fixture["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        (line,) = [
            ln for ln in err.splitlines()
            if self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK in ln
        ]
        self.assertTrue(
            line.startswith(f"  ! {self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK}: "), line
        )
        self.assertTrue(
            line.endswith("; using the temporary Python template authority"), line
        )

    def test_fallback_source_not_found_raises_the_exact_runtime_error(self):
        fixture = self.fixture("tpl-fb-srcmissing")
        with self.assertRaises(RuntimeError) as ctx:
            self.run_materialize(
                self.mod.materialize_template,
                fixture["recipe_dir"],
                self.tpl(source="templates/missing.sh"), fixture["root"],
                MERGED_CFG, recipe_id="worktree-flow",
            )
        self.assertEqual(
            str(ctx.exception),
            f"template source not found: {fixture['recipe_dir'] / 'templates' / 'missing.sh'}",
        )

    def test_fallback_invalid_policy_raises_the_exact_runtime_error(self):
        fixture = self.fixture("tpl-fb-policy")
        with self.assertRaises(RuntimeError) as ctx:
            self.run_materialize(
                self.mod.materialize_template,
                fixture["recipe_dir"],
                self.tpl(policy="force"), fixture["root"],
                MERGED_CFG, recipe_id="worktree-flow",
            )
        self.assertEqual(
            str(ctx.exception),
            f"invalid update policy 'force' for template '{TARGET}'; "
            "expected auto | confirm | never-force",
        )

    def test_fallback_refuses_symlinked_destination(self):
        """R1 parity: the Python fallback authority refuses a symlinked
        destination with the same actionable refusal (Go:
        templateSymlinkRefusal) and never writes through the link."""
        cases = [
            # A link to a regular file with the overwrite path (condition
            # "always"): the write must be refused, target bytes untouched.
            ("always", True),
            # A dangling link with condition not_exists: os.Stat-style exists
            # reads it as missing, so the write falls through — and must be
            # refused without creating the link target.
            ("not_exists", False),
        ]
        for i, (condition, regular) in enumerate(cases):
            with self.subTest(condition=condition, regular=regular):
                fixture = self.fixture(f"tpl-fb-symlink-{i}")
                dest = self.dest_of(fixture)
                target = fixture["root"] / "victim.sh"
                if regular:
                    target.write_bytes(b"echo victim\n")
                dest.parent.mkdir(parents=True, exist_ok=True)
                os.symlink(target, dest)
                with self.assertRaises(RuntimeError) as ctx:
                    self.run_materialize(
                        self.mod._python_materialize_template,
                        fixture["recipe_dir"],
                        self.tpl(condition=condition), fixture["root"],
                        MERGED_CFG, recipe_id="worktree-flow",
                    )
                self.assertIn(self.mod._symlink_refusal(TARGET), str(ctx.exception))
                if regular:
                    self.assertEqual(target.read_bytes(), b"echo victim\n")
                else:
                    self.assertFalse(target.exists(), "dangling link target was created")
                    # R3-toctou-py-dest-guard: the dangling case must carry the
                    # EXACT refusal string (the Go/Python parity contract),
                    # not a wrapped or partial diagnostic.
                    self.assertEqual(str(ctx.exception), self.mod._symlink_refusal(TARGET))

    def test_fallback_never_raises_on_bridge_infrastructure_failures(self):
        """The bridge itself must never raise for infra failures: a raising
        stub binary still degrades to the Python body."""
        fixture = self.fixture("tpl-fb-raising")
        self.pin_binary(self.stub("exit 70"))
        out, err = self.run_materialize(
            self.mod.materialize_template,
            fixture["recipe_dir"], self.tpl(), fixture["root"],
            MERGED_CFG, recipe_id="worktree-flow",
        )
        self.assertEqual(err.count(self.mod.GO_TEMPLATE_ACTUATOR_BRIDGE_FALLBACK), 1, err)
        self.assertTrue(self.dest_of(fixture).is_file())


if __name__ == "__main__":
    unittest.main()
