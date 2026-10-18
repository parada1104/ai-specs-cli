"""Contract tests for the Go merge-config bridge in ``recipe-materialize.py``.

The Go gate binary owns the merge_config DECISION (schema defaults + manifest
overrides + structured-config validation); Python keeps schema ACQUISITION of
the already-loaded Recipe as a thin bridge. The Python implementation survives
as a TEMPORARY fail-open fallback (``GO_MERGE_CONFIG_BRIDGE_FALLBACK``) and
these tests pin both seams:

* bridge results are identical to the Python authority they replaced
  (config values, key order, warning order, error messages),
* a valid Go response never reaches the Python fallback,
* every transport failure (unavailable binary, nonzero exit, timeout,
  non-JSON, malformed envelope, serialization error) falls back exactly once
  with one greppable warning,
* a semantic validation error raises RuntimeError without falling back.

The Go path needs a built binary (``dist/worktree-gate-current`` or
``$WORKTREE_GATE_BIN``); it skips loudly when none exists. Fallback tests run
without any binary, because failing open is the contract they pin.
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
from datetime import date, datetime, time
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
sys.path.insert(0, str(ROOT / "tests"))

from lib._internal import gate_binary as gb  # noqa: E402
from lib._internal.recipe_schema import (  # noqa: E402
    ConfigField,
    ConfigSchema,
    ConfigTable,
    STRUCTURED_CONFIG_SHAPES,
)
from lib._internal.recipe_schema import Recipe  # noqa: E402

RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
WORKTREE_FLOW_RECIPE_DIR = ROOT / "catalog" / "recipes" / "worktree-flow"
PLAN_BUILD_RECIPE_DIR = ROOT / "catalog" / "recipes" / "plan-build-flow"
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
        "WORKTREE_GATE_BIN); the Go merge-config bridge cannot be proven "
        "without it"
    )


def make_recipe(
    fields: dict[str, ConfigField] | None = None,
    tables: dict[str, ConfigTable] | None = None,
    name: str = "R",
) -> Recipe:
    return Recipe(
        id="r",
        name=name,
        description="D",
        version="1.0",
        config_schema=ConfigSchema(fields=fields or {}, tables=tables or {}),
    )


def warn_lines(stderr: str) -> list[str]:
    """The ``  ! <msg>`` warning lines Python's warn() wrote to stderr."""
    return [
        line[4:] for line in stderr.splitlines() if line.startswith("  ! ")
    ]


def outcome(call) -> tuple[dict | None, list[str], str | None]:
    """Run one merge call, projecting it to (config, warnings, error)."""
    captured = io.StringIO()
    try:
        with contextlib.redirect_stderr(captured):
            config = call()
    except RuntimeError as exc:
        return None, warn_lines(captured.getvalue()), str(exc)
    return config, warn_lines(captured.getvalue()), None


class _GoBridgeTestCase(unittest.TestCase):
    """Shared fixture: the module under test plus a pinned Go binary."""

    @classmethod
    def setUpClass(cls):
        cls.binary = gate_binary()
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_merge_bridge"
        )

    def setUp(self):
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(self.binary)})
        pin.start()
        self.addCleanup(pin.stop)

    @contextlib.contextmanager
    def _forbid_python_authority(self):
        """Fail loudly if a bridged call reaches the temporary fallback."""
        with mock.patch.object(
            self.mod,
            "_python_merge_config",
            side_effect=AssertionError(
                "_python_merge_config ran: the bridge fell back"
            ),
        ):
            yield

    def _go_outcome(self, recipe, manifest_config):
        """The bridge outcome, with the Python fallback forbidden."""
        with self._forbid_python_authority():
            return outcome(lambda: self.mod.merge_config(recipe, manifest_config))

    def _python_outcome(self, recipe, manifest_config):
        """The reference outcome from the temporary Python authority."""
        return outcome(lambda: self.mod._python_merge_config(recipe, manifest_config))

    def _assert_parity(self, recipe, manifest_config):
        """Bridge results equal the Python authority that used to compute them."""
        go_config, go_warnings, go_error = self._go_outcome(recipe, manifest_config)
        py_config, py_warnings, py_error = self._python_outcome(
            recipe, manifest_config
        )
        self.assertEqual(go_error, py_error, "error message parity")
        self.assertEqual(go_warnings, py_warnings, "warning parity")
        self.assertEqual(go_config, py_config, "config value parity")
        self.assertEqual(
            list(go_config or {}),
            list(py_config or {}),
            "config key-order parity",
        )
        return go_config, go_warnings, go_error


class MergeConfigParityTests(_GoBridgeTestCase):
    """Bridge results equal the Python authority that used to compute them."""

    def test_defaults_false_zero_and_empty_still_apply(self):
        recipe = make_recipe(
            {
                "flag": ConfigField(required=False, type="bool", default=False),
                "count": ConfigField(required=False, type="integer", default=0),
                "label": ConfigField(required=False, type="string", default=""),
                "size": ConfigField(required=False, type="integer", default=30),
                "board_id": ConfigField(required=True, type="string"),
            }
        )
        config, _, _ = self._assert_parity(
            recipe, {"board_id": "abc", "flag": True, "count": 7, "label": "x"}
        )
        self.assertEqual(
            config,
            {"flag": True, "count": 7, "label": "x", "size": 30, "board_id": "abc"},
        )

    def test_defaults_apply_when_manifest_omits_them(self):
        recipe = make_recipe(
            {
                "flag": ConfigField(required=False, type="bool", default=False),
                "count": ConfigField(required=False, type="integer", default=0),
            }
        )
        config, _, _ = self._assert_parity(recipe, {})
        self.assertEqual(config, {"flag": False, "count": 0})

    def test_missing_required_field_error(self):
        recipe = make_recipe({"board_id": ConfigField(required=True, type="string")})
        _, _, error = self._assert_parity(recipe, {})
        self.assertIn("missing required config field 'board_id'", error)

    def test_required_error_precedes_enum_error(self):
        recipe = make_recipe(
            {
                "board_id": ConfigField(required=True, type="string"),
                "gate_mode": ConfigField(
                    required=False, type="string", enum=["always", "ask", "off"]
                ),
            }
        )
        _, _, error = self._assert_parity(recipe, {"gate_mode": "rust"})
        self.assertIn("missing required config field 'board_id'", error)

    def test_enum_error_lists_allowed_values(self):
        recipe = make_recipe(
            {
                "gate_mode": ConfigField(
                    required=False, type="string", enum=["always", "ask", "off"]
                ),
                "board_id": ConfigField(required=True, type="string"),
            }
        )
        _, _, error = self._assert_parity(
            recipe, {"board_id": "abc", "gate_mode": "rust"}
        )
        self.assertEqual(
            error,
            "recipe 'R': config field 'gate_mode' value 'rust' is invalid; "
            "allowed: always | ask | off",
        )

    def test_gate_impl_enum_error_is_special(self):
        recipe = make_recipe(
            {
                "gate_impl": ConfigField(
                    required=False, type="string", default="auto", enum=["auto", "go"]
                ),
            }
        )
        _, _, error = self._assert_parity(recipe, {"gate_impl": "bash"})
        self.assertEqual(
            error, "invalid gate_impl 'bash'; bash has been removed; allowed: auto | go"
        )

    def test_gate_scope_falsy_and_whitespace_resolves_to_auto(self):
        recipe = make_recipe(
            {"gate_scope": ConfigField(required=False, type="string", default="auto")}
        )
        for value in ("", "   ", False, 0):
            config, _, _ = self._assert_parity(recipe, {"gate_scope": value})
            self.assertEqual(config["gate_scope"], "auto")

    def test_gate_scope_truthy_value_wins(self):
        recipe = make_recipe(
            {"gate_scope": ConfigField(required=False, type="string", default="auto")}
        )
        config, _, _ = self._assert_parity(recipe, {"gate_scope": "manifest"})
        self.assertEqual(config["gate_scope"], "manifest")

    def test_gate_scope_not_declared_is_left_alone(self):
        """No gate_scope field: the manifest key is just an unknown key."""
        recipe = make_recipe({"other": ConfigField(required=False, type="string")})
        config, warnings, _ = self._assert_parity(recipe, {"gate_scope": ""})
        self.assertEqual(config, {})
        self.assertEqual(
            warnings,
            ["recipe 'R': unknown config key 'gate_scope' in manifest (ignored)"],
        )

    def test_unknown_keys_warn_in_manifest_order(self):
        recipe = make_recipe(
            {"board_id": ConfigField(required=True, type="string")}
        )
        config, warnings, error = self._assert_parity(
            recipe, {"zz_last": 1, "board_id": "abc", "aa_first": 2}
        )
        self.assertEqual(
            warnings,
            [
                "recipe 'R': unknown config key 'zz_last' in manifest (ignored)",
                "recipe 'R': unknown config key 'aa_first' in manifest (ignored)",
            ],
        )
        self.assertEqual(config, {"board_id": "abc"})

    def test_warnings_are_emitted_before_a_semantic_error(self):
        recipe = make_recipe(
            {"board_id": ConfigField(required=True, type="string")}
        )
        _, warnings, error = self._assert_parity(recipe, {"unknown": 1})
        self.assertEqual(
            warnings, ["recipe 'R': unknown config key 'unknown' in manifest (ignored)"]
        )
        self.assertIn("missing required config field 'board_id'", error)

    def test_config_key_order_is_defaults_then_manifest(self):
        recipe = make_recipe(
            {
                "zeta": ConfigField(required=False, type="string", default="z"),
                "alpha": ConfigField(required=False, type="string", default="a"),
                "board_id": ConfigField(required=True, type="string"),
            }
        )
        config, _, _ = self._assert_parity(
            recipe, {"board_id": "abc", "alpha": "override"}
        )
        self.assertEqual(list(config), ["zeta", "alpha", "board_id"])

    def test_structured_reconcile_table_is_carried_through(self):
        recipe = make_recipe(
            {"board_id": ConfigField(required=True, type="string")},
            tables={
                "reconcile": ConfigTable(shape=STRUCTURED_CONFIG_SHAPES["reconcile"])
            },
        )
        declared = {
            "scope_field": "board_id",
            "max_age_seconds": 900,
            "expectations": [
                {
                    "event": "delivery",
                    "property": "list",
                    "config_field": "default_list",
                }
            ],
        }
        config, _, _ = self._assert_parity(
            recipe, {"board_id": "abc", "reconcile": declared}
        )
        self.assertEqual(config["reconcile"], declared)

    def test_structured_unknown_sub_key_error(self):
        recipe = make_recipe(
            tables={"reconcile": ConfigTable(shape=STRUCTURED_CONFIG_SHAPES["reconcile"])}
        )
        _, _, error = self._assert_parity(
            recipe, {"reconcile": {"scope_field": "a", "bogus": 1}}
        )
        self.assertEqual(
            error,
            "recipe 'R': invalid config field 'reconcile': [config.reconcile]: "
            "unknown key 'bogus'",
        )

    def test_structured_nested_type_error(self):
        recipe = make_recipe(
            tables={"reconcile": ConfigTable(shape=STRUCTURED_CONFIG_SHAPES["reconcile"])}
        )
        _, _, error = self._assert_parity(
            recipe, {"reconcile": {"expectations": [{"event": 5}]}}
        )
        self.assertEqual(
            error,
            "recipe 'R': invalid config field 'reconcile': "
            "[config.reconcile].expectations[0].event: expected string, got int",
        )

    def test_structured_bool_is_not_an_integer(self):
        recipe = make_recipe(
            tables={"reconcile": ConfigTable(shape=STRUCTURED_CONFIG_SHAPES["reconcile"])}
        )
        _, _, error = self._assert_parity(
            recipe, {"reconcile": {"max_age_seconds": True}}
        )
        self.assertIn("expected integer, got bool", error)

    def test_structured_list_over_the_cap(self):
        recipe = make_recipe(
            tables={"reconcile": ConfigTable(shape=STRUCTURED_CONFIG_SHAPES["reconcile"])}
        )
        expectations = [
            {"event": "e", "property": "p", "config_field": "c"} for _ in range(33)
        ]
        _, _, error = self._assert_parity(
            recipe, {"reconcile": {"expectations": expectations}}
        )
        self.assertIn("expected at most 32 entries, got 33", error)

    def test_structured_non_table_value(self):
        recipe = make_recipe(
            tables={"reconcile": ConfigTable(shape=STRUCTURED_CONFIG_SHAPES["reconcile"])}
        )
        _, _, error = self._assert_parity(recipe, {"reconcile": "nope"})
        self.assertIn("expected table, got str", error)

    def test_canonical_float_and_negative_zero_render_identically(self):
        recipe = make_recipe(
            {"ratio": ConfigField(required=False, type="float", enum=["0.5"])}
        )
        for value in (0.1, -0.0, 1e30):
            _, _, error = self._assert_parity(recipe, {"ratio": value})
            self.assertEqual(
                error,
                f"recipe 'R': config field 'ratio' value '{value}' is invalid; "
                "allowed: 0.5",
            )

    def test_nested_repr_in_enum_error(self):
        recipe = make_recipe(
            {"shape": ConfigField(required=False, type="string", enum=["ok"])}
        )
        _, _, error = self._assert_parity(
            recipe, {"shape": {"x": [1, "two"]}}
        )
        self.assertIn("value '{'x': [1, 'two']}'", error)

    def test_repr_parity_for_apostrophes_and_control_chars(self):
        """pyRepr matches CPython repr() for apostrophe and control strings."""
        recipe = make_recipe(
            {"shape": ConfigField(required=False, type="string", enum=["ok"])}
        )
        for value in (["a'b"], {"k": "o'clock"}, ["a\u0001b"]):
            _, _, error = self._assert_parity(recipe, {"shape": value})
            self.assertIn("is invalid", error)

    def test_real_worktree_flow_schema_defaults_and_overrides(self):
        schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_merge_bridge_real")
        recipe = schema.load_recipe_toml(WORKTREE_FLOW_RECIPE_DIR / "recipe.toml")
        config, _, _ = self._assert_parity(recipe, {})
        self.assertEqual(config["worktrees_dir"], ".worktrees")
        self.assertEqual(config["gate_impl"], "auto")
        self.assertEqual(config["auto_remove_merged"], True)
        self.assertEqual(list(config), list(self.mod._python_merge_config(recipe, {})))

    def test_real_worktree_flow_schema_overrides_and_unknown_key(self):
        schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_merge_bridge_real")
        recipe = schema.load_recipe_toml(WORKTREE_FLOW_RECIPE_DIR / "recipe.toml")
        config, warnings, _ = self._assert_parity(
            recipe,
            {
                "gate_mode": "ask",
                "worktrees_dir": "custom",
                "gate_scope": "   ",
                "mystery": 1,
            },
        )
        self.assertEqual(config["gate_mode"], "ask")
        self.assertEqual(config["worktrees_dir"], "custom")
        self.assertEqual(config["gate_scope"], "auto")
        self.assertEqual(len(warnings), 1)
        self.assertIn("mystery", warnings[0])

    def test_real_worktree_flow_schema_rejects_invalid_gate_impl(self):
        schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_merge_bridge_real")
        recipe = schema.load_recipe_toml(WORKTREE_FLOW_RECIPE_DIR / "recipe.toml")
        _, _, error = self._assert_parity(recipe, {"gate_impl": "bash"})
        self.assertEqual(
            error, "invalid gate_impl 'bash'; bash has been removed; allowed: auto | go"
        )

    def test_real_plan_build_schema_enum(self):
        schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_merge_bridge_real")
        recipe = schema.load_recipe_toml(PLAN_BUILD_RECIPE_DIR / "recipe.toml")
        config, _, _ = self._assert_parity(recipe, {})
        self.assertEqual(config, {"artifact_store_default": "openspec"})
        config, _, _ = self._assert_parity(
            recipe, {"artifact_store_default": "both"}
        )
        self.assertEqual(config, {"artifact_store_default": "both"})
        _, _, error = self._assert_parity(
            recipe, {"artifact_store_default": "vault"}
        )
        self.assertIn("allowed:", error)


class MergeConfigBridgeTransportTests(_GoBridgeTestCase):
    """The bridge runs exactly one Go invocation with the exact contract."""

    def test_bridge_runs_the_pinned_binary_with_the_exact_flag_and_envelope(self):
        recipe = make_recipe(
            {
                "timeout": ConfigField(required=False, type="integer", default=30),
                "flag": ConfigField(required=False, type="bool", default=False),
                "board_id": ConfigField(required=True, type="string"),
                "gate_mode": ConfigField(
                    required=False, type="string", enum=["always", "ask"]
                ),
            }
        )
        manifest = {"board_id": "abc", "gate_mode": "ask", "unknown": 1}
        observed = {}

        def fake_run(argv, **kwargs):
            observed["argv"] = argv
            observed["input"] = kwargs.get("input")
            return subprocess.CompletedProcess(
                argv,
                0,
                stdout=json.dumps(
                    {
                        "config": {"timeout": 30, "board_id": "abc"},
                        "warnings": ["recipe 'R': unknown config key 'unknown' "
                                     "in manifest (ignored)"],
                        "error": "",
                    }
                ),
                stderr="",
            )

        with mock.patch.object(self.mod.subprocess, "run", side_effect=fake_run):
            captured = io.StringIO()
            with contextlib.redirect_stderr(captured):
                with self._forbid_python_authority():
                    config = self.mod.merge_config(recipe, manifest)

        self.assertEqual(
            observed["argv"], [str(self.binary), "--plan-merge-config"]
        )
        request = json.loads(observed["input"])
        self.assertEqual(request["recipe_name"], "R")
        self.assertEqual(
            [(f["key"], f["required"], f["has_default"], f["default"], f["enum"])
             for f in request["fields"]],
            [
                ("timeout", False, True, 30, []),
                ("flag", False, True, False, []),
                ("board_id", True, False, None, []),
                ("gate_mode", False, False, None, ["always", "ask"]),
            ],
        )
        self.assertEqual(
            [(p["key"], p["value"]) for p in request["manifest"]],
            [("board_id", "abc"), ("gate_mode", "ask"), ("unknown", 1)],
        )
        # A valid Go response is returned verbatim (order preserved) and its
        # warnings are emitted exactly once; the Python fallback never ran.
        self.assertEqual(config, {"timeout": 30, "board_id": "abc"})
        self.assertEqual(
            warn_lines(captured.getvalue()),
            ["recipe 'R': unknown config key 'unknown' in manifest (ignored)"],
        )

    def test_valid_response_never_executes_the_python_fallback(self):
        recipe = make_recipe(
            {"board_id": ConfigField(required=True, type="string")}
        )
        config = self._go_outcome(recipe, {"board_id": "abc"})[0]
        self.assertEqual(config, {"board_id": "abc"})

    def test_semantic_error_raises_without_falling_back(self):
        recipe = make_recipe(
            {"board_id": ConfigField(required=True, type="string")}
        )
        captured = io.StringIO()
        with contextlib.redirect_stderr(captured):
            with self.assertRaises(RuntimeError) as ctx:
                with self._forbid_python_authority():
                    self.mod.merge_config(recipe, {})
        self.assertIn("missing required config field 'board_id'", str(ctx.exception))
        self.assertNotIn(
            self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK, captured.getvalue()
        )


class MergeConfigHomeResolutionTests(unittest.TestCase):
    """go_merge_config resolves the gate binary from the active CLI home.

    Sibling bridges (orphan plan, resolved config) pass the active home so the
    acquired binary under ``<home>/cache`` is found; the merge bridge must do
    the same. Without ``home`` the historical package-root resolution is
    preserved so existing direct callers keep their behavior.
    """

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_merge_bridge_home"
        )

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.home = Path(tmp.name)
        self.cache_bin = gb.cache_bin_path(self.home.resolve())
        self.cache_bin.parent.mkdir(parents=True, exist_ok=True)
        self.cache_bin.write_bytes(b"#!/bin/sh\nexit 0\n")
        self.cache_bin.chmod(0o755)
        gb.verification_record_path(self.cache_bin).touch()
        # Resolution authority one is the override; these tests prove the
        # cache step under the active home, so it must stay unset.
        pin = mock.patch.dict(os.environ)
        pin.start()
        self.addCleanup(pin.stop)
        os.environ.pop("WORKTREE_GATE_BIN", None)

    def _recipe(self):
        return make_recipe({"board_id": ConfigField(required=True, type="string")})

    def _run_bridge(self, recipe, manifest, *, forbid_python=True, **kwargs):
        observed = {}

        def fake_run(argv, **run_kwargs):
            observed["argv"] = argv
            return subprocess.CompletedProcess(
                argv,
                0,
                stdout=json.dumps(
                    {"config": {"board_id": "abc"}, "warnings": [], "error": ""}
                ),
                stderr="",
            )

        captured = io.StringIO()
        forbid = (
            mock.patch.object(
                self.mod,
                "_python_merge_config",
                side_effect=AssertionError("the bridge fell back"),
            )
            if forbid_python
            else contextlib.nullcontext()
        )
        with mock.patch.object(self.mod.subprocess, "run", side_effect=fake_run):
            with contextlib.redirect_stderr(captured):
                with forbid:
                    config = self.mod.merge_config(recipe, manifest, **kwargs)
        return config, observed, captured.getvalue()

    def test_binary_at_active_home_is_found_when_home_is_passed(self):
        config, observed, stderr = self._run_bridge(
            self._recipe(), {"board_id": "abc"}, home=self.home
        )
        self.assertEqual(observed["argv"], [str(self.cache_bin), "--plan-merge-config"])
        self.assertEqual(config, {"board_id": "abc"})
        self.assertNotIn(self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK, stderr)

    def test_without_home_the_active_home_binary_is_not_consulted(self):
        """No ``home``: package-root resolution, exactly as before.

        Whatever the package-root resolution yields (a binary, or the
        documented fallback when no verified binary exists there), the binary
        staged under the active home must never be consulted.
        """
        _, observed, stderr = self._run_bridge(
            self._recipe(), {"board_id": "abc"}, forbid_python=False
        )
        argv = observed.get("argv")
        if argv is None:
            self.assertIn(self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK, stderr)
        else:
            self.assertNotEqual(argv[0], str(self.cache_bin))


class DatetimeTransportBoundaryTests(_GoBridgeTestCase):
    """JSON cannot carry datetime/date/time: the bridge falls back exactly once
    and the Python authority keeps the typed values and its own validation.
    Stringifying them would NOT be parity: merge_config returns typed objects,
    and a datetime supplied to a structured string field must stay a Python
    type error, never become accepted text.
    """

    def _bridge_outcome(self, recipe, manifest_config):
        captured = io.StringIO()
        with contextlib.redirect_stderr(captured):
            try:
                config = self.mod.merge_config(recipe, manifest_config)
            except RuntimeError as exc:
                return None, captured.getvalue(), str(exc)
        return config, captured.getvalue(), None

    def _assert_single_serialization_fallback(self, stderr):
        self.assertEqual(
            stderr.count(self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("serialized", stderr)

    def test_datetime_manifest_value_falls_back_and_keeps_type(self):
        recipe = make_recipe({"starts": ConfigField(required=False, type="string")})
        stamp = datetime(2024, 1, 2, 3, 4, 5)
        config, stderr, error = self._bridge_outcome(recipe, {"starts": stamp})
        self.assertIsNone(error)
        self.assertIs(config["starts"], stamp)
        self._assert_single_serialization_fallback(stderr)

    def test_date_and_time_manifest_values_fall_back_and_keep_type(self):
        recipe = make_recipe(
            {
                "day": ConfigField(required=False, type="string"),
                "moment": ConfigField(required=False, type="string"),
            }
        )
        day = date(2024, 1, 2)
        moment = time(3, 4, 5)
        config, stderr, error = self._bridge_outcome(
            recipe, {"day": day, "moment": moment}
        )
        self.assertIsNone(error)
        self.assertIs(config["day"], day)
        self.assertIs(config["moment"], moment)
        self._assert_single_serialization_fallback(stderr)

    def test_nested_datetime_falls_back_and_keeps_type(self):
        recipe = make_recipe({"blob": ConfigField(required=False, type="string")})
        stamp = datetime(2024, 1, 2, 3, 4, 5)
        config, stderr, error = self._bridge_outcome(
            recipe, {"blob": {"when": [stamp]}}
        )
        self.assertIsNone(error)
        self.assertIs(config["blob"]["when"][0], stamp)
        self._assert_single_serialization_fallback(stderr)

    def test_structured_string_field_supplied_datetime_stays_a_type_error(self):
        recipe = make_recipe(
            tables={
                "reconcile": ConfigTable(shape=STRUCTURED_CONFIG_SHAPES["reconcile"])
            }
        )
        config, stderr, error = self._bridge_outcome(
            recipe, {"reconcile": {"scope_field": datetime(2024, 1, 2)}}
        )
        self.assertIsNone(config)
        self.assertIn("expected string, got datetime", error)
        self._assert_single_serialization_fallback(stderr)


class FailOpenFallbackTests(unittest.TestCase):
    """Transport failure: the temporary Python authority runs, with one warning."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_merge_bridge_fallback"
        )

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.tmp = Path(tmp.name)
        self.missing = self.tmp / "no-such-worktree-gate"
        self._pin(self.missing)

    def _pin(self, path: Path) -> None:
        pin = mock.patch.dict(os.environ, {"WORKTREE_GATE_BIN": str(path)})
        pin.start()
        self.addCleanup(pin.stop)

    def _stub(self, body: str) -> Path:
        path = self.tmp / "worktree-gate-stub"
        path.write_text(f"#!/bin/sh\n{body}\n")
        path.chmod(0o755)
        return path

    def _simple_recipe(self) -> Recipe:
        return make_recipe(
            {"board_id": ConfigField(required=True, type="string")}, name="F"
        )

    def _fallback_outcome(self, recipe, manifest_config):
        captured = io.StringIO()
        with contextlib.redirect_stderr(captured):
            try:
                config = self.mod.merge_config(recipe, manifest_config)
            except RuntimeError as exc:
                return None, captured.getvalue(), exc
        return config, captured.getvalue(), None

    def test_missing_binary_falls_back_with_one_warning(self):
        config, stderr, error = self._fallback_outcome(
            self._simple_recipe(), {"board_id": "abc"}
        )
        self.assertIsNone(error)
        self.assertEqual(config, {"board_id": "abc"})
        self.assertEqual(
            stderr.count(self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("no verified worktree-gate binary", stderr)

    def test_nonzero_exit_falls_back_with_one_warning(self):
        self._pin(self._stub("exit 2"))
        config, stderr, error = self._fallback_outcome(
            self._simple_recipe(), {"board_id": "abc"}
        )
        self.assertIsNone(error)
        self.assertEqual(config, {"board_id": "abc"})
        self.assertEqual(
            stderr.count(self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("exited 2", stderr)

    def test_non_json_output_falls_back_with_one_warning(self):
        self._pin(self._stub("echo not-json"))
        config, stderr, error = self._fallback_outcome(
            self._simple_recipe(), {"board_id": "abc"}
        )
        self.assertIsNone(error)
        self.assertEqual(config, {"board_id": "abc"})
        self.assertEqual(
            stderr.count(self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("not JSON", stderr)

    def test_malformed_envelope_falls_back_with_one_warning(self):
        self._pin(self._stub('echo \'{"config": null, "warnings": []}\''))
        config, stderr, error = self._fallback_outcome(
            self._simple_recipe(), {"board_id": "abc"}
        )
        self.assertIsNone(error)
        self.assertEqual(config, {"board_id": "abc"})
        self.assertEqual(
            stderr.count(self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("merge-config envelope", stderr)

    def test_timeout_falls_back_with_one_warning(self):
        self._pin(self._stub("sleep 5"))
        with mock.patch.object(
            self.mod, "GO_MERGE_CONFIG_BRIDGE_TIMEOUT_SECONDS", 1
        ):
            config, stderr, error = self._fallback_outcome(
                self._simple_recipe(), {"board_id": "abc"}
            )
        self.assertIsNone(error)
        self.assertEqual(config, {"board_id": "abc"})
        self.assertEqual(
            stderr.count(self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK), 1, stderr
        )

    def test_nonfinite_float_serialization_falls_back_with_one_warning(self):
        """inf has no JSON form Go accepts: Python owns the value instead."""
        # The stub exits 9 so the test can only pass through the serialization
        # fallback, never through a (wrongly successful) binary run.
        self._pin(self._stub("exit 9"))
        recipe = make_recipe(
            {"ratio": ConfigField(required=False, type="float")}, name="F"
        )
        config, stderr, error = self._fallback_outcome(
            recipe, {"ratio": float("inf")}
        )
        self.assertIsNone(error)
        self.assertEqual(config, {"ratio": float("inf")})
        self.assertEqual(
            stderr.count(self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK), 1, stderr
        )
        self.assertIn("serialized", stderr)
        self.assertNotIn("exited 9", stderr)

    def test_fallback_still_raises_semantic_errors(self):
        recipe = self._simple_recipe()
        captured = io.StringIO()
        with contextlib.redirect_stderr(captured):
            with self.assertRaises(RuntimeError) as ctx:
                self.mod.merge_config(recipe, {})
        self.assertIn("missing required config field 'board_id'", str(ctx.exception))
        # The transport already degraded, so exactly one fallback line precedes
        # the semantic failure raised by the Python authority itself.
        self.assertEqual(
            captured.getvalue().count(self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK),
            1,
        )

    def test_unicode_decode_error_falls_back_with_one_warning(self):
        """A Go reply that cannot be decoded is a transport failure, not a crash.

        subprocess.run(text=True) without an explicit encoding decodes with the
        locale codec; under a non-UTF-8 locale a non-ASCII Go reply raised
        UnicodeDecodeError inside run(), escaping transport handling and
        crashing merge_config instead of degrading. The mock makes the failure
        deterministic regardless of the host locale.
        """
        self._pin(self._stub("exit 0"))

        def decode_failure(argv, **kwargs):
            raise UnicodeDecodeError("utf-8", b"\xff", 0, 1, "invalid start byte")

        with mock.patch.object(
            self.mod.subprocess, "run", side_effect=decode_failure
        ):
            config, stderr, error = self._fallback_outcome(
                self._simple_recipe(), {"board_id": "abc"}
            )
        self.assertIsNone(error)
        self.assertEqual(config, {"board_id": "abc"})
        self.assertEqual(
            stderr.count(self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK), 1, stderr
        )

    def test_undecodable_go_output_falls_back_with_one_warning(self):
        """Invalid bytes on stdout degrade to exactly one fallback warning."""
        stub = self.tmp / "worktree-gate-bad-utf8"
        stub.write_bytes(b"#!/bin/sh\nprintf '\xff\xfe not json'\n")
        stub.chmod(0o755)
        self._pin(stub)
        config, stderr, error = self._fallback_outcome(
            self._simple_recipe(), {"board_id": "abc"}
        )
        self.assertIsNone(error)
        self.assertEqual(config, {"board_id": "abc"})
        self.assertEqual(
            stderr.count(self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK), 1, stderr
        )

    def test_fallback_path_is_marked_temporary_in_the_source(self):
        self.assertEqual(
            self.mod.GO_MERGE_CONFIG_BRIDGE_FALLBACK,
            "GO_MERGE_CONFIG_BRIDGE_FALLBACK",
        )
        doc = self.mod._python_merge_config.__doc__ or ""
        self.assertIn("GO_MERGE_CONFIG_BRIDGE_FALLBACK", doc)
        self.assertIn("TEMPORARY", doc)


if __name__ == "__main__":
    unittest.main()
