"""Contract tests for the Go hook/gate-actuator bridge in ``recipe-materialize.py``
(GO-10, strangler slice 7).

``worktree-gate --materialize-hook`` owns the runtime-hook actuation DECISION
+ EXECUTION: rendering (the 8 hook placeholders), the ownership classification
(the shared classifyManagedOverride core behind --plan-classify), the write +
chmod 0755, and the refresh backup/rollback. Python keeps the lock load/write
(set_gate_baseline + write_lock), ALL printing, the refresh backup-path
precomputation (project-cache ownership), the gate-version resolution, and the
fail-open fallback. The historical Python body survives as
``_python_materialize_hook_script`` (``GO_HOOK_GATE_BRIDGE_FALLBACK``).

These tests pin both seams:

* Go-path results equal the retained Python authority: dest bytes, mode 0755,
  exact prints, identical lock bytes, identical record shas,
* the stdin/stdout envelope contract of ``--materialize-hook``,
* FAIL CLOSED on a Go refusal (nonzero exit with a stdout error envelope):
  the bridge raises ``RuntimeError`` naming the Go error instead of falling
  back, so the Python body can never bypass the Go authority's decision, and
* the degraded path for every infrastructure failure AND every record
  mismatch (target/source/recipe/kind/sha): exactly one warning, then the
  Python body executes identically; a ``wrote: true`` envelope without a
  record is a mismatch (it would leave the gate on disk with no baseline).

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

REL = "ai-specs/recipes/worktree-flow/hooks/gate.sh"
HOOK_BODY = "#!/bin/sh\n# gate_mode=__WORKTREE_GATE_MODE__\n"
MERGED_CFG = {
    "gate_mode": "always",
    "gate_scope": "auto",
    "repo_topology": "auto",
    "gate_impl": "auto",
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
        "WORKTREE_GATE_BIN); the Go hook-gate bridge cannot be proven "
        "without it"
    )


class _HookBridgeTestCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_hook_bridge"
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

    def fixture(self, name: str, body: str = HOOK_BODY) -> dict:
        """A project root + recipe dir with one runtime hook source."""
        root = self.tmp / name / "project"
        recipe_dir = self.tmp / name / "catalog" / "recipes" / "worktree-flow"
        (recipe_dir / "hooks").mkdir(parents=True)
        (recipe_dir / "hooks" / "gate.sh").write_text(body)
        root.mkdir(parents=True)
        return {"root": root, "recipe_dir": recipe_dir}

    def hook(self) -> SimpleNamespace:
        return SimpleNamespace(script="hooks/gate.sh", id="gate")

    def dest_of(self, fixture: dict) -> Path:
        return fixture["root"] / REL

    def seed_managed(self, fixture: dict, content: bytes) -> None:
        """Pre-seed the destination AND its managed lock entry."""
        dest = self.dest_of(fixture)
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_bytes(content)
        lock_path = fixture["root"] / "ai-specs" / ".ai-specs.lock"
        lock = self.mod.load_lock(lock_path)
        self.mod.set_gate_baseline(
            lock, REL, self.mod._load_util().sha256_bytes(content),
            recipe="worktree-flow", source="hooks/gate.sh",
        )
        self.mod.write_lock(lock_path, lock)

    def run_materialize(self, fn, *args, **kwargs) -> tuple[str, str]:
        out, err = io.StringIO(), io.StringIO()
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            fn(*args, **kwargs)
        return out.getvalue(), err.getvalue()

    def stub_envelope(self, fixture: dict, **overrides) -> str:
        """A valid materialize-hook envelope payload for a stub binary."""
        payload = {
            "rel": REL,
            "dest": str(self.dest_of(fixture)),
            "wrote": True,
            "record": {
                "target": REL, "sha256": "a" * 64, "recipe": "worktree-flow",
                "source": "hooks/gate.sh", "kind": "gate", "policy": "auto",
            },
            "message": f"✓ hook script {REL}",
            "warnings": [],
            "error": None,
        }
        payload.update(overrides)
        return json.dumps(payload)


class HookGateGoAuthorityTests(_HookBridgeTestCase):
    """The Go path is authoritative whenever a verified binary runs."""

    @classmethod
    def setUpClass(cls):
        super().setUpClass()
        cls.binary = gate_binary()

    def setUp(self):
        super().setUp()
        self.pin_binary(self.binary)

    def test_fresh_write_go_path_matches_the_python_reference(self):
        go = self.fixture("hook-go-fresh")
        fb = self.fixture("hook-fb-fresh")
        go_out, go_err = self.run_materialize(
            self.mod.materialize_hook_script,
            go["recipe_dir"], self.hook(), go["root"], "worktree-flow",
            MERGED_CFG, cli_home=None,
        )
        # The fallback reference run must not see the real binary.
        self.pin_binary(self.tmp / "no-such-gate")
        fb_out, fb_err = self.run_materialize(
            self.mod.materialize_hook_script,
            fb["recipe_dir"], self.hook(), fb["root"], "worktree-flow",
            MERGED_CFG, cli_home=None,
        )
        self.assertEqual(go_out, fb_out)
        self.assertIn(f"    ✓ hook script {REL}", go_out)
        self.assertEqual(go_err, "", "the Go path must not warn")
        self.assertEqual(fb_err.count(self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK), 1)
        self.assertEqual(
            self.dest_of(go).read_bytes(), self.dest_of(fb).read_bytes()
        )
        self.assertEqual(
            self.dest_of(go).stat().st_mode & 0o777, 0o755,
            "gate files always carry mode 0755",
        )
        self.assertEqual(
            (go["root"] / "ai-specs" / ".ai-specs.lock").read_bytes(),
            (fb["root"] / "ai-specs" / ".ai-specs.lock").read_bytes(),
            "lock bytes must be identical (Python owns the lock write)",
        )

    def test_user_modified_go_path_preserves_and_warns(self):
        go = self.fixture("hook-go-user")
        self.seed_managed(go, b"#!/bin/sh\n# gate_mode=warn\n")
        self.dest_of(go).write_bytes(b"user edited\n")
        out, err = self.run_materialize(
            self.mod.materialize_hook_script,
            go["recipe_dir"], self.hook(), go["root"], "worktree-flow",
            MERGED_CFG, cli_home=None,
        )
        self.assertIn("hook " + REL + " is user-modified", err)
        self.assertNotIn(self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK, err)
        self.assertIn(f"    · hook skipped (user-modified) {REL}", out)
        self.assertEqual(self.dest_of(go).read_bytes(), b"user edited\n")

    def test_refresh_go_path_backs_up_and_rewrites(self):
        go = self.fixture("hook-go-refresh")
        prior = b"#!/bin/sh\n# user customization\n"
        self.seed_managed(go, prior)
        self.dest_of(go).write_bytes(prior)
        out, err = self.run_materialize(
            self.mod.materialize_hook_script,
            go["recipe_dir"], self.hook(), go["root"], "worktree-flow",
            MERGED_CFG, cli_home=None, refresh=True,
        )
        self.assertEqual(err, "")
        self.assertIn(f"    ✓ hook refreshed {REL}", out)
        self.assertEqual(
            self.dest_of(go).read_bytes(),
            b"#!/bin/sh\n# gate_mode=always\n",
        )
        lock = self.mod.load_lock(go["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertEqual(
            lock["managed"][REL]["sha256"],
            self.mod._load_util().sha256_bytes(b"#!/bin/sh\n# gate_mode=always\n"),
        )

    def test_envelope_contract_one_call_per_hook(self):
        fixture = self.fixture("hook-envelope")
        argv_capture = self.tmp / "argv"
        stdin_capture = self.tmp / "stdin"
        record = {
            "target": REL,
            "sha256": self.mod._load_util().sha256_bytes(
                b"#!/bin/sh\n# gate_mode=always\n"
            ),
            "recipe": "worktree-flow", "source": "hooks/gate.sh",
            "kind": "gate", "policy": "auto",
        }
        payload = self.stub_envelope(fixture, record=record)
        self.pin_binary(self.stub(
            # A record is only applied to the lock when its sha256 matches the
            # destination actually on disk: the fake writes those bytes first.
            f"echo '#!/bin/sh' > '{self.dest_of(fixture)}'; "
            f"echo '# gate_mode=always' >> '{self.dest_of(fixture)}'\n"
            f"printf '%s\\n' \"$1\" >> '{argv_capture}'\n"
            f"cat > \"{stdin_capture}.$1\"\n"
            f"printf '%s' '{payload}'\n"
        ))
        out, _ = self.run_materialize(
            self.mod.materialize_hook_script,
            fixture["recipe_dir"], self.hook(), fixture["root"],
            "worktree-flow", MERGED_CFG, cli_home=None,
        )
        self.assertEqual(argv_capture.read_text().splitlines()[0], "--materialize-hook")
        sent = json.loads((self.tmp / "stdin.--materialize-hook").read_text())
        self.assertEqual(
            sent,
            {
                "project_root": str(fixture["root"]),
                "recipe_dir": str(fixture["recipe_dir"]),
                "recipe_id": "worktree-flow",
                "script": "hooks/gate.sh",
                "config": MERGED_CFG,
                "cli_home": "",
                "gate_version": "dev",
                "refresh": False,
                "managed_entry": None,
                "backup_path": "",
            },
        )
        self.assertIn(f"    ✓ hook script {REL}", out)
        lock = self.mod.load_lock(fixture["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertEqual(lock["managed"][REL]["sha256"], record["sha256"])

    def test_stub_refusal_envelope_fails_closed(self):
        """A valid Go refusal (exit 2, stdout error envelope) is the
        authority's decision: RuntimeError, no fallback warning, dest
        untouched."""
        fixture = self.fixture("hook-go-refusal")
        self.dest_of(fixture).parent.mkdir(parents=True, exist_ok=True)
        self.dest_of(fixture).write_bytes(b"user stuff\n")
        self.pin_binary(self.stub(
            "printf '%s' '{\"error\": \"boom refusal\"}'; exit 2"
        ))
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            with self.assertRaises(RuntimeError) as ctx:
                self.mod.materialize_hook_script(
                    fixture["recipe_dir"], self.hook(), fixture["root"],
                    "worktree-flow", MERGED_CFG, cli_home=None,
                )
        self.assertEqual(
            str(ctx.exception), "worktree-gate refused the hook: boom refusal"
        )
        self.assertNotIn(self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK, err.getvalue())
        self.assertEqual(self.dest_of(fixture).read_bytes(), b"user stuff\n")

    def test_timeout_matches_the_bridge_family(self):
        self.assertEqual(self.mod.GO_HOOK_GATE_BRIDGE_TIMEOUT_SECONDS, 60)


class HookGateFallbackTests(_HookBridgeTestCase):
    """No usable Go authority: the Python hook body runs, with one warning."""

    def setUp(self):
        super().setUp()
        # Fallback tests run with no usable binary even when the outer
        # environment pins one (the verification harness sets WORKTREE_GATE_BIN).
        self.pin_binary(self.tmp / "no-such-gate")

    def test_missing_binary_falls_back_with_one_warning(self):
        fixture = self.fixture("hook-fb-missing")
        out, err = self.run_materialize(
            self.mod.materialize_hook_script,
            fixture["recipe_dir"], self.hook(), fixture["root"],
            "worktree-flow", MERGED_CFG, cli_home=None,
        )
        self.assertEqual(err.count(self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK), 1, err)
        self.assertIn(f"    ✓ hook script {REL}", out)
        self.assertEqual(
            self.dest_of(fixture).read_bytes(), b"#!/bin/sh\n# gate_mode=always\n"
        )
        lock = self.mod.load_lock(fixture["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertIn(REL, lock["managed"])

    def test_garbage_stdout_falls_back_with_one_warning(self):
        fixture = self.fixture("hook-fb-garbage")
        self.pin_binary(self.stub("printf '%s' 'garbage'"))
        out, err = self.run_materialize(
            self.mod.materialize_hook_script,
            fixture["recipe_dir"], self.hook(), fixture["root"],
            "worktree-flow", MERGED_CFG, cli_home=None,
        )
        self.assertEqual(err.count(self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK), 1, err)
        self.assertIn(f"    ✓ hook script {REL}", out)
        self.assertTrue(self.dest_of(fixture).is_file())

    def test_exit_2_without_error_envelope_falls_back(self):
        """Exit 2 WITHOUT a valid stdout error envelope is an infrastructure
        failure, not a refusal: the bridge fails open with one warning."""
        fixture = self.fixture("hook-fb-exit2")
        self.pin_binary(self.stub("echo 'crashed' >&2; exit 2"))
        out, err = self.run_materialize(
            self.mod.materialize_hook_script,
            fixture["recipe_dir"], self.hook(), fixture["root"],
            "worktree-flow", MERGED_CFG, cli_home=None,
        )
        self.assertEqual(err.count(self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK), 1, err)
        self.assertIn("exited 2 without a hook envelope", err)
        self.assertIn(f"    ✓ hook script {REL}", out)

    def test_envelope_mismatch_falls_back(self):
        """A structurally invalid exit-0 envelope (rel not a string) is an
        envelope mismatch: one warning, then the Python body."""
        fixture = self.fixture("hook-fb-mismatch")
        self.pin_binary(self.stub("printf '%s' '{\"rel\": 3}'"))
        out, err = self.run_materialize(
            self.mod.materialize_hook_script,
            fixture["recipe_dir"], self.hook(), fixture["root"],
            "worktree-flow", MERGED_CFG, cli_home=None,
        )
        self.assertEqual(err.count(self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK), 1, err)
        self.assertIn("did not match the materialize-hook envelope", err)
        self.assertTrue(self.dest_of(fixture).is_file())

    def _record_mismatch_case(self, name: str, record_overrides: dict, needle: str):
        fixture = self.fixture(f"hook-fb-{name}")
        record = {
            "target": REL, "sha256": "a" * 64, "recipe": "worktree-flow",
            "source": "hooks/gate.sh", "kind": "gate", "policy": "auto",
        }
        record.update(record_overrides)
        payload = self.stub_envelope(fixture, record=record)
        self.pin_binary(self.stub(f"printf '%s' '{payload}'"))
        out, err = self.run_materialize(
            self.mod.materialize_hook_script,
            fixture["recipe_dir"], self.hook(), fixture["root"],
            "worktree-flow", MERGED_CFG, cli_home=None,
        )
        self.assertEqual(err.count(self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK), 1, err)
        self.assertIn(needle, err)
        self.assertIn(f"    ✓ hook script {REL}", out)
        self.assertEqual(
            self.dest_of(fixture).read_bytes(),
            b"#!/bin/sh\n# gate_mode=always\n",
            "the Python body rewrites the destination with its own bytes",
        )
        lock = self.mod.load_lock(fixture["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertNotIn("evil", lock.get("managed", {}).get(REL, {}).get("source", ""))
        self.assertEqual(
            lock["managed"][REL]["sha256"],
            self.mod._load_util().sha256_bytes(b"#!/bin/sh\n# gate_mode=always\n"),
            "the Python body's own record is what lands in the lock",
        )

    def test_record_target_mismatch_falls_back_without_lock_write(self):
        self._record_mismatch_case(
            "rec-target", {"target": "ai-specs/recipes/evil/hooks/gate.sh"},
            "does not match the sent target",
        )

    def test_record_source_mismatch_falls_back_without_lock_write(self):
        self._record_mismatch_case(
            "rec-source", {"source": "hooks/evil.sh"},
            "does not match the sent source",
        )

    def test_record_recipe_mismatch_falls_back_without_lock_write(self):
        self._record_mismatch_case(
            "rec-recipe", {"recipe": "evil-recipe"},
            "does not match the sent recipe",
        )

    def test_record_kind_mismatch_falls_back_without_lock_write(self):
        self._record_mismatch_case(
            "rec-kind", {"kind": "template"}, "does not match the sent kind",
        )

    def test_record_sha_not_lowercase_hex_falls_back(self):
        self._record_mismatch_case(
            "rec-sha", {"sha256": "A" * 64}, "64 lowercase hex characters",
        )

    def test_wrote_true_without_record_falls_back(self):
        """``wrote: true`` with ``record: null`` would leave the gate on disk
        with no baseline — the envelope is unusable: one warning naming it,
        then the Python body whose own record ends up in the lock."""
        fixture = self.fixture("hook-fb-wrote-null")
        payload = self.stub_envelope(fixture, record=None)
        self.pin_binary(self.stub(f"printf '%s' '{payload}'"))
        out, err = self.run_materialize(
            self.mod.materialize_hook_script,
            fixture["recipe_dir"], self.hook(), fixture["root"],
            "worktree-flow", MERGED_CFG, cli_home=None,
        )
        self.assertEqual(err.count(self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK), 1, err)
        self.assertIn("wrote without a record", err)
        self.assertIn(f"    ✓ hook script {REL}", out)
        self.assertEqual(
            self.dest_of(fixture).read_bytes(), b"#!/bin/sh\n# gate_mode=always\n"
        )
        lock = self.mod.load_lock(fixture["root"] / "ai-specs" / ".ai-specs.lock")
        self.assertEqual(
            lock["managed"][REL]["sha256"],
            self.mod._load_util().sha256_bytes(b"#!/bin/sh\n# gate_mode=always\n"),
        )

    def test_fallback_warning_matches_the_bridge_family_format(self):
        fixture = self.fixture("hook-fb-format")
        _, err = self.run_materialize(
            self.mod.materialize_hook_script,
            fixture["recipe_dir"], self.hook(), fixture["root"],
            "worktree-flow", MERGED_CFG, cli_home=None,
        )
        (line,) = [
            ln for ln in err.splitlines()
            if self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK in ln
        ]
        self.assertTrue(
            line.startswith(f"  ! {self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK}: "), line
        )
        self.assertTrue(
            line.endswith("; using the temporary Python hook authority"), line
        )

    def test_fallback_source_not_found_raises_the_exact_runtime_error(self):
        fixture = self.fixture("hook-fb-srcmissing")
        with self.assertRaises(RuntimeError) as ctx:
            self.run_materialize(
                self.mod.materialize_hook_script,
                fixture["recipe_dir"],
                SimpleNamespace(script="hooks/missing.sh", id="gate"),
                fixture["root"], "worktree-flow", MERGED_CFG, cli_home=None,
            )
        self.assertEqual(
            str(ctx.exception),
            f"hook script not found: {fixture['recipe_dir'] / 'hooks' / 'missing.sh'}",
        )

    def test_fallback_invalid_scope_raises_the_exact_runtime_error(self):
        fixture = self.fixture("hook-fb-scope")
        (fixture["recipe_dir"] / "hooks" / "gate.sh").write_text(
            "scope=__WORKTREE_GATE_SCOPE__\n"
        )
        with self.assertRaises(RuntimeError) as ctx:
            self.run_materialize(
                self.mod.materialize_hook_script,
                fixture["recipe_dir"], self.hook(), fixture["root"],
                "worktree-flow", {"gate_scope": "bogus"}, cli_home=None,
            )
        self.assertEqual(
            str(ctx.exception),
            "invalid gate_scope 'bogus'; allowed: auto | superrepo | subrepo",
        )

    def test_fallback_refuses_symlinked_destination(self):
        """Both authorities refuse a symlinked destination with the same
        actionable refusal and never write through the link (two-layer
        guard: is_symlink pre-check + os.O_NOFOLLOW + errno.ELOOP)."""
        fixture = self.fixture("hook-fb-symlink")
        dest = self.dest_of(fixture)
        victim = fixture["root"] / "victim.sh"
        dest.parent.mkdir(parents=True, exist_ok=True)
        os.symlink(victim, dest)
        with self.assertRaises(RuntimeError) as ctx:
            self.run_materialize(
                self.mod._python_materialize_hook_script,
                fixture["recipe_dir"], self.hook(), fixture["root"],
                "worktree-flow", MERGED_CFG, cli_home=None,
            )
        self.assertEqual(
            str(ctx.exception),
            f"destination {REL} is a symlink; refusing to write through it. "
            "Replace it with a regular file and run sync again:\n"
            f"  rm {REL} && ai-specs sync",
        )
        self.assertFalse(victim.exists(), "dangling link target was created")

    def test_fallback_never_raises_on_bridge_infrastructure_failures(self):
        """The bridge itself must never raise for infra failures: a raising
        stub binary still degrades to the Python body."""
        fixture = self.fixture("hook-fb-raising")
        self.pin_binary(self.stub("exit 70"))
        out, err = self.run_materialize(
            self.mod.materialize_hook_script,
            fixture["recipe_dir"], self.hook(), fixture["root"],
            "worktree-flow", MERGED_CFG, cli_home=None,
        )
        self.assertEqual(err.count(self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK), 1, err)
        self.assertTrue(self.dest_of(fixture).is_file())


class HookGateRepairTests(_HookBridgeTestCase):
    """An unusable envelope may still follow a Go write of the destination.

    The bridge must not leave a gate on disk with no lock entry: when the
    on-disk bytes are exactly what the CLI renders for the hook, the lock is
    reconciled (repair); bytes the CLI cannot prove it rendered are never
    overwritten and never recorded. Stubs that claim ``wrote: true`` really
    write the destination first, so the repair is proven, not assumed.
    """

    RENDERED = b"#!/bin/sh\n# gate_mode=always\n"

    def write_rendered_stub_body(self, fixture) -> str:
        dest = self.dest_of(fixture)
        return (
            f"echo '#!/bin/sh' > '{dest}'; "
            f"echo '# gate_mode=always' >> '{dest}'"
        )

    def run_with_stub(self, fixture, body, **envelope_overrides):
        payload = self.stub_envelope(fixture, **envelope_overrides)
        self.pin_binary(self.stub(f"{body}\nprintf '%s' '{payload}'"))
        return self.run_materialize(
            self.mod.materialize_hook_script,
            fixture["recipe_dir"], self.hook(), fixture["root"],
            "worktree-flow", MERGED_CFG, cli_home=None,
        )

    def lock_of(self, fixture):
        return self.mod.load_lock(fixture["root"] / "ai-specs" / ".ai-specs.lock")

    def test_wrote_true_null_record_with_written_dest_reconciles_the_lock(self):
        """R4-001 / R3-wrote-no-record-fallback-gap: Go really wrote the gate
        and returned ``record: null`` — the lock must record the digest of
        the on-disk bytes (repair), never leave an unrecorded gate."""
        fixture = self.fixture("hook-repair-wrote-null")
        dest = self.dest_of(fixture)
        out, err = self.run_with_stub(
            fixture, self.write_rendered_stub_body(fixture), record=None,
        )
        self.assertEqual(err.count(self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK), 1, err)
        self.assertIn("wrote without a record", err)
        self.assertEqual(dest.read_bytes(), self.RENDERED)
        lock = self.lock_of(fixture)
        self.assertEqual(
            lock["managed"][REL]["sha256"],
            self.mod._load_util().sha256_bytes(self.RENDERED),
            "the lock records the digest of the on-disk bytes (repair)",
        )

    def test_record_sha_mismatching_disk_preserves_user_bytes_and_lock(self):
        """R3-record-without-wrote-guard / R1-lock-baseline-unverified-disk:
        a record whose digest does not match the destination on disk is never
        applied; user bytes are untouched and the lock gains no baseline."""
        fixture = self.fixture("hook-repair-sha-mismatch")
        dest = self.dest_of(fixture)
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_bytes(b"user bytes\n")
        out, err = self.run_with_stub(
            fixture,
            "true",
            wrote=False,
            record={
                "target": REL, "sha256": "b" * 64, "recipe": "worktree-flow",
                "source": "hooks/gate.sh", "kind": "gate", "policy": "auto",
            },
        )
        self.assertEqual(err.count(self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK), 1, err)
        self.assertIn("does not match the destination on disk", err)
        self.assertEqual(dest.read_bytes(), b"user bytes\n")
        lock = self.lock_of(fixture)
        self.assertNotIn(REL, lock.get("managed", {}))

    def test_record_matching_disk_is_applied_as_before(self):
        """Regression guard: a record whose digest matches the destination on
        disk is applied to the lock exactly as before, with no fallback."""
        fixture = self.fixture("hook-repair-match")
        sha = self.mod._load_util().sha256_bytes(self.RENDERED)
        out, err = self.run_with_stub(
            fixture,
            self.write_rendered_stub_body(fixture),
            record={
                "target": REL, "sha256": sha, "recipe": "worktree-flow",
                "source": "hooks/gate.sh", "kind": "gate", "policy": "auto",
            },
        )
        self.assertNotIn(
            self.mod.GO_HOOK_GATE_BRIDGE_FALLBACK, err,
            "a matching record must be applied directly, with no fallback",
        )
        self.assertIn(f"    ✓ hook script {REL}", out)
        lock = self.lock_of(fixture)
        self.assertEqual(lock["managed"][REL]["sha256"], sha)

    def test_fallback_refresh_refuses_dangling_symlink_destination(self):
        """D2: with no verified binary and refresh=True, a dangling symlink
        destination is refused before any read or write — the victim file is
        never created."""
        fixture = self.fixture("hook-repair-refresh-dangling")
        dest = self.dest_of(fixture)
        victim = fixture["root"] / "victim.sh"
        dest.parent.mkdir(parents=True, exist_ok=True)
        os.symlink(victim, dest)
        self.pin_binary(self.tmp / "no-such-gate")
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            with self.assertRaises(RuntimeError) as ctx:
                self.mod.materialize_hook_script(
                    fixture["recipe_dir"], self.hook(), fixture["root"],
                    "worktree-flow", MERGED_CFG, cli_home=None, refresh=True,
                )
        self.assertEqual(
            str(ctx.exception),
            f"destination {REL} is a symlink; refusing to write through it. "
            "Replace it with a regular file and run sync again:\n"
            f"  rm {REL} && ai-specs sync",
        )
        self.assertTrue(dest.is_symlink(), "the symlink itself was replaced")
        self.assertFalse(victim.exists(), "the symlink target was created")

    def test_fallback_refresh_refuses_symlink_to_existing_file(self):
        """D2: a symlink pointing at an existing file is refused the same way
        and the victim's bytes are unchanged."""
        fixture = self.fixture("hook-repair-refresh-victim")
        dest = self.dest_of(fixture)
        victim = fixture["root"] / "victim.sh"
        victim.write_bytes(b"victim bytes\n")
        dest.parent.mkdir(parents=True, exist_ok=True)
        os.symlink(victim, dest)
        self.pin_binary(self.tmp / "no-such-gate")
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            with self.assertRaises(RuntimeError) as ctx:
                self.mod.materialize_hook_script(
                    fixture["recipe_dir"], self.hook(), fixture["root"],
                    "worktree-flow", MERGED_CFG, cli_home=None, refresh=True,
                )
        self.assertEqual(
            str(ctx.exception),
            f"destination {REL} is a symlink; refusing to write through it. "
            "Replace it with a regular file and run sync again:\n"
            f"  rm {REL} && ai-specs sync",
        )
        self.assertEqual(victim.read_bytes(), b"victim bytes\n")

    def test_exactly_one_materialize_hook_script_definition_remains(self):
        """R2-shadowed-original-body: the shadowed historical body stays
        deleted — exactly one dispatcher and one Python fallback body."""
        source = RECIPE_MATERIALIZE_PATH.read_text()
        self.assertEqual(
            source.count("def materialize_hook_script("), 1,
            "materialize_hook_script must be defined exactly once",
        )
        self.assertEqual(
            source.count("def _python_materialize_hook_script("), 1,
            "_python_materialize_hook_script must be defined exactly once",
        )
        self.assertNotIn(
            "hook script with gate provenance", source,
            "the shadowed body's docstring is gone",
        )


if __name__ == "__main__":
    unittest.main()
