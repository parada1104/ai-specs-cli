"""RED/GREEN tests for the tracker ledger mode configuration (A9).

Covers three things at once:

1. ``catalog/recipes/trello-mcp-workflow/recipe.toml`` declares the project
   ``ledger_mode`` (``always|ask|warn``, default ``warn``).
2. The A9 mapping the hosts apply: an explicit ``ledger_mode`` wins; otherwise
   the tracker ``gate_mode`` maps ``off``→skip, ``warn``→``warn``,
   ``always``→``always``. The worktree gate mode is never read.
3. The five checkpoint hosts all reach the one Go predicate. The four Tracker
   checkpoints are advisory: a ``block``/``ask``/``needs-item`` verdict is
   reported on stderr and the host still exits 0. The Plan Build ``work-start``
   host keeps its own blocking authority (spec "All five checkpoints reach one
   predicate").

The hosts are acquisition/JSON bridges, so every test drives them with a stub
``worktree-gate`` binary (``WORKTREE_GATE_BIN``) that records its argv. No host
may re-implement the ``## Tracker`` predicate (5.7).
"""
from __future__ import annotations

import json
import os
import subprocess
import tempfile
import tomllib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RECIPE_DIR = ROOT / "catalog" / "recipes" / "trello-mcp-workflow"
RECIPE_TOML = RECIPE_DIR / "recipe.toml"
PLAN_BUILD_GATE = ROOT / "catalog" / "recipes" / "plan-build-flow" / "hooks" / "plan-build-gate.sh"
TRACKER_GATE = RECIPE_DIR / "hooks" / "tracker-card-gate.sh"
GUARDIAN = ROOT / "lib" / "_internal" / "premerge_guardian.py"
DOCTOR = ROOT / "lib" / "_internal" / "doctor.py"
LEGACY_RECIPE = "trello-mcp-workflow"
LIB_INTERNAL = ROOT / "lib" / "_internal"
SCHEMA = ROOT / "lib" / "_internal" / "recipe_schema.py"

STUB_BINARY = """#!/usr/bin/env bash
printf '%s\\n' "$*" >> "${STUB_LOG}"
decision="${STUB_DECISION:-allow}"
reason="${STUB_REASON:-}"
checkpoint=""
mode=""
decide=""
while [ $# -gt 0 ]; do
  case "$1" in
    --checkpoint) checkpoint="$2"; shift 2 ;;
    --ledger-mode) mode="$2"; shift 2 ;;
    --decide) decide="$2"; shift 2 ;;
    *) shift ;;
  esac
done
if [ -n "$decide" ]; then
  printf 'DECIDE %s\\n' "$decide" >> "${STUB_LOG}"
fi
prompt='null'
if [ "$decision" = ask ]; then
  prompt='{"reason":"needs-item","evidence":{"local":"","remote":"","code":"","git":""},"choices":["continue","local"]}'
fi
printf '{"capability":"tracker","active":true,"checkpoint":"%s","mode":"%s","decision":"%s","reason":"%s","identity":{"common_dir":"","branch":"","change":null,"key":""},"item":null,"conflict":null,"prompt":%s,"doctor":{"severity":"OK","name":"tracker-ledger","message":""}}\\n' "$checkpoint" "$mode" "$decision" "$reason" "$prompt"
[ "$decision" = block ] && exit 2
exit 0
"""


def _git(cwd: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(cwd), *args], check=True, capture_output=True, text=True)


def _load_schema():
    import importlib.util
    import sys

    spec = importlib.util.spec_from_file_location("recipe_schema_ledger_mode", SCHEMA)
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class LedgerModeConfigTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        base = Path(self.tmp.name)
        self.repo = base / "repo"
        self.repo.mkdir()
        _git(self.repo, "init", "-q")
        _git(self.repo, "config", "user.email", "t@t.t")
        _git(self.repo, "config", "user.name", "t")
        (self.repo / "README.md").write_text("x\n")
        _git(self.repo, "add", "-A")
        _git(self.repo, "commit", "-qm", "init")
        self.stub_log = base / "stub.log"
        self.stub = base / "worktree-gate"
        self.stub.write_text(STUB_BINARY)
        self.stub.chmod(0o755)

    # --- witness-derived recipe lookup (task 3.1) ---

    def _witness(self, recipe_id: str) -> None:
        common = subprocess.run(
            ["git", "-C", str(self.repo), "rev-parse", "--path-format=absolute", "--git-common-dir"],
            check=True, capture_output=True, text=True,
        ).stdout.strip()
        ledger_dir = Path(common) / "ai-specs" / "ledger"
        ledger_dir.mkdir(parents=True, exist_ok=True)
        (ledger_dir / "witness.json").write_text(json.dumps({
            "v": 1, "capability": "tracker", "state": "bound", "recipe_id": recipe_id,
            "candidates": [], "written_at": "2026-01-01T00:00:00Z",
        }))

    def _manifest_two_recipes(self, *, legacy_gate: str | None, fixture_mode: str | None) -> None:
        text = (
            "[project]\nname = 'mode'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
        )
        if legacy_gate is not None:
            text += f'gate_mode = "{legacy_gate}"\n'
        text += "[recipes.fixture-tracker]\nenabled = true\n[recipes.fixture-tracker.config]\n"
        if fixture_mode is not None:
            text += f'ledger_mode = "{fixture_mode}"\n'
        ai_specs = self.repo / "ai-specs"
        ai_specs.mkdir(exist_ok=True)
        (ai_specs / "ai-specs.toml").write_text(text)

    def test_witness_recipe_id_drives_the_tracker_card_gate_mode_lookup(self):
        self._manifest_two_recipes(legacy_gate="off", fixture_mode="always")
        self._witness("fixture-tracker")
        r = self._tracker_path("warn", env=self._env())
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(
            self._ledger_modes(), ["always"],
            "the mode must come from the witness-bound recipe's config, not the literal",
        )

    def test_witness_recipe_id_drives_the_tracker_host_mode_lookup(self):
        self._manifest_two_recipes(legacy_gate="off", fixture_mode="always")
        self._witness("fixture-tracker")
        active = self.repo / "openspec" / "changes" / "demo-change"
        active.mkdir(parents=True, exist_ok=True)
        (active / "tasks.md").write_text("Depth: light\n")
        (active / "proposal.md").write_text("# proposal\n")
        r = self._tracker_host("archive-close", env=self._env())
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._ledger_modes(), ["always"],
                         "the tracker host must read the witness recipe's config too")

    def test_witness_recipe_gate_mode_maps_forward(self):
        self._manifest_two_recipes(legacy_gate="off", fixture_mode=None)
        ai = self.repo / "ai-specs" / "ai-specs.toml"
        ai.write_text(ai.read_text() + 'gate_mode = "always"\n')
        self._witness("fixture-tracker")
        self._tracker_path("warn", env=self._env())
        self.assertEqual(self._ledger_modes(), ["always"])

    def test_env_override_still_beats_the_witness_recipe(self):
        self._manifest_two_recipes(legacy_gate="off", fixture_mode="always")
        self._witness("fixture-tracker")
        self._tracker_path("warn", env=self._env(TRACKER_LEDGER_MODE="warn"))
        self.assertEqual(self._ledger_modes(), ["warn"])

    def test_missing_witness_keeps_the_legacy_lookup(self):
        self._manifest(ledger_mode="always", gate_mode="off")
        r = self._tracker_path("off", env=self._env())
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), ["apply-start"], r.stderr)
        self.assertEqual(self._ledger_modes(), ["always"], r.stderr)

    def test_witness_recipe_id_drives_the_plan_build_work_start_mode(self):
        """W6: the last provider literal is gone; work-start resolves the witness."""
        self._manifest_two_recipes(legacy_gate="warn", fixture_mode="always")
        self._witness("fixture-tracker")
        r = self._plan_build(env=self._env())
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(
            self._ledger_modes(), ["always"],
            "work-start must read the witness-bound recipe's config, not a literal",
        )

    # --- 3.4 / W6: no host resolves its config from a hardcoded recipe id ---

    def test_no_hardcoded_recipe_lookup_remains_at_any_host_site(self):
        sites = (
            ("tracker-gate", TRACKER_GATE, "_ledger_recipe_id"),
            ("plan-build-gate", PLAN_BUILD_GATE, "_ledger_recipe_id"),
            ("doctor", DOCTOR, "recipe_id"),
        )
        for label, path, resolver in sites:
            with self.subTest(site=label):
                text = path.read_text(encoding="utf-8")
                self.assertNotIn(
                    'get("trello-mcp-workflow")', text,
                    f"{path.name} still looks config up by the hardcoded recipe id; the "
                    "fallback belongs in ledger_bridge.LEGACY_RECIPE_ID",
                )
                self.assertIn(resolver, text, f"{path.name} must resolve the witness recipe id")

    def test_plan_build_gate_uses_the_stamped_bridge_seam(self):
        plan_build = PLAN_BUILD_GATE.read_text(encoding="utf-8")
        self.assertNotIn(LEGACY_RECIPE, plan_build,
                         "the final provider literal must be gone from plan-build-gate.sh")
        self.assertIn("__TRACKER_LIB_INTERNAL__", plan_build,
                      "the gate must resolve the recipe through the stamped ledger_bridge seam")
        self.assertIn("ledger_bridge", plan_build)
        docs = (ROOT / "docs" / "capabilities.md").read_text(encoding="utf-8")
        self.assertIn("plan-build", docs)
        self.assertIn("work-start", docs)
        self.assertNotIn(
            "resolves its config through the legacy literal", docs,
            "the named residue is closed; docs must not still claim it",
        )

    # --- 5.2: recipe declares the field ---

    def test_recipe_declares_ledger_mode_enum_and_default(self):
        schema = _load_schema()
        recipe = schema.load_recipe_toml(RECIPE_TOML)
        fields = recipe.config_schema.fields
        self.assertIn("ledger_mode", fields)
        self.assertEqual(fields["ledger_mode"].default, "warn")
        self.assertEqual(set(fields["ledger_mode"].enum or []), {"always", "ask", "warn"})

    # --- W2: the ledger core is a provider-neutral Tracker domain port ---

    def test_production_go_ledger_has_no_provider_vocabulary(self):
        gate = ROOT / "catalog" / "recipes" / "worktree-flow" / "gate"
        production = sorted(p for p in gate.rglob("*.go") if not p.name.endswith("_test.go"))
        self.assertTrue(production, "expected the production Go gate sources")
        for path in production:
            text = path.read_text(encoding="utf-8").lower()
            for token in ("trello", "jira", "linear", "bitbucket", "gitlab", "github"):
                with self.subTest(file=path.name, token=token):
                    self.assertNotIn(
                        token, text,
                        f"{path.name} promotes provider vocabulary into the Tracker-domain core",
                    )

    def test_reconcile_mapping_is_adapter_data_not_policy(self):
        mapping = tomllib.loads(RECIPE_TOML.read_text())["config"]["reconcile"]
        self.assertEqual(
            set(mapping), {"scope_field", "max_age_seconds", "expectations"},
            "the adapter mapping surface is exactly the declarative mapping keys",
        )
        for policy in ("ledger_mode", "gate_mode"):
            with self.subTest(policy=policy):
                self.assertNotIn(
                    policy, mapping,
                    "Tracker-domain policy must stay out of the adapter mapping",
                )
        self.assertEqual(
            {entry["event"] for entry in mapping["expectations"]},
            {"delivery", "review", "merge"},
        )

    def test_adapter_boundary_is_documented(self):
        readme = (RECIPE_DIR / "README.md").read_text(encoding="utf-8")
        self.assertIn("Tracker-domain", readme)
        self.assertIn("adapter", readme)
        self.assertIn("adapter", RECIPE_TOML.read_text(encoding="utf-8"))
        capabilities = (ROOT / "docs" / "capabilities.md").read_text(encoding="utf-8")
        self.assertIn("Tracker-domain port", capabilities)
        self.assertIn("adapter", capabilities)

    # --- helpers ---

    def _manifest(self, *, ledger_mode: str | None = None, gate_mode: str | None = None,
                  worktree_gate_mode: str | None = None) -> None:
        text = (
            "[project]\nname = 'mode'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            'board_id = "69ec097f13e2d38ecd89a557"\n'
        )
        if ledger_mode is not None:
            text += f'ledger_mode = "{ledger_mode}"\n'
        if gate_mode is not None:
            text += f'gate_mode = "{gate_mode}"\n'
        if worktree_gate_mode is not None:
            text += (
                "\n[recipes.worktree-flow]\nenabled = true\n"
                f'[recipes.worktree-flow.config]\ngate_mode = "{worktree_gate_mode}"\n'
            )
        ai_specs = self.repo / "ai-specs"
        ai_specs.mkdir(exist_ok=True)
        (ai_specs / "ai-specs.toml").write_text(text)

    def _seed_change(self, slug: str = "demo-change") -> None:
        d = self.repo / "openspec" / "changes" / slug
        d.mkdir(parents=True, exist_ok=True)
        (d / "tasks.md").write_text("# tasks\n")

    def _stamped_tracker_gate(self, gate_mode: str) -> Path:
        path = Path(self.tmp.name) / f"tracker-gate-{gate_mode}.sh"
        text = (
            TRACKER_GATE.read_text()
            .replace("__TRACKER_CARD_GATE_MODE__", gate_mode)
            .replace("__TRACKER_CLI_HOME__", "")
            .replace("__TRACKER_LIB_INTERNAL__", str(LIB_INTERNAL))
        )
        path.write_text(text)
        path.chmod(0o755)
        return path

    def _stamped_plan_build_gate(self) -> Path:
        """The materialized hook: sync stamps the CLI's lib/_internal seam."""
        path = Path(self.tmp.name) / "plan-build-gate.sh"
        path.write_text(
            PLAN_BUILD_GATE.read_text().replace(
                "__TRACKER_LIB_INTERNAL__", str(LIB_INTERNAL)
            )
        )
        path.chmod(0o755)
        return path

    def _env(self, **extra: str) -> dict:
        env = dict(os.environ)
        env.pop("TRACKER_CARD_GATE_MODE", None)
        env.pop("TRACKER_LEDGER_MODE", None)
        env.pop("AI_SPECS_HOME", None)
        env["WORKTREE_GATE_BIN"] = str(self.stub)
        env["STUB_LOG"] = str(self.stub_log)
        env.update(extra)
        return env

    def _plan_build(self, *, env: dict) -> subprocess.CompletedProcess:
        self._seed_change()
        event = {
            "event": "pre-tool-use",
            "tool_name": "Write",
            "tool_input": {"file_path": str(self.repo / "src" / "app.py")},
            "cwd": str(self.repo),
        }
        return subprocess.run(
            ["bash", str(self._stamped_plan_build_gate())],
            input=json.dumps(event), capture_output=True, text=True, env=env,
        )

    def _tracker_path(self, gate_mode: str, *, env: dict) -> subprocess.CompletedProcess:
        event = {
            "event": "pre-tool-use",
            "tool_name": "Edit",
            "tool_input": {"file_path": str(self.repo / "lib" / "foo.py")},
            "cwd": str(self.repo),
        }
        return subprocess.run(
            ["bash", str(self._stamped_tracker_gate(gate_mode))],
            input=json.dumps(event), capture_output=True, text=True, env=env,
        )

    def _logged_checkpoints(self) -> list[str]:
        if not self.stub_log.exists():
            return []
        out = []
        for line in self.stub_log.read_text().splitlines():
            if not line.startswith("--ledger "):
                continue
            parts = line.split()
            for i, token in enumerate(parts):
                if token == "--checkpoint" and i + 1 < len(parts):
                    out.append(parts[i + 1])
        return out

    def _ledger_modes(self) -> list[str]:
        if not self.stub_log.exists():
            return []
        out = []
        for line in self.stub_log.read_text().splitlines():
            parts = line.split()
            for i, token in enumerate(parts):
                if token == "--ledger-mode" and i + 1 < len(parts):
                    out.append(parts[i + 1])
        return out

    # --- 5.1: A9 mapping ---

    def test_ledger_mode_wins_over_gate_mode(self):
        self._manifest(ledger_mode="always", gate_mode="warn")
        r = self._plan_build(env=self._env())
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._ledger_modes(), ["always"], r.stderr)

    def test_gate_mode_off_skips_the_checkpoint(self):
        self._manifest(gate_mode="off")
        r = self._plan_build(env=self._env())
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), [], "off must skip the ledger entirely")

    def test_gate_mode_warn_maps_to_warn(self):
        self._manifest(gate_mode="warn")
        self._plan_build(env=self._env())
        self.assertEqual(self._ledger_modes(), ["warn"])

    def test_gate_mode_always_maps_to_always(self):
        self._manifest(gate_mode="always")
        self._plan_build(env=self._env())
        self.assertEqual(self._ledger_modes(), ["always"])

    def test_env_override_beats_manifest(self):
        self._manifest(ledger_mode="always")
        self._plan_build(env=self._env(TRACKER_LEDGER_MODE="warn"))
        self.assertEqual(self._ledger_modes(), ["warn"])

    def test_worktree_gate_mode_is_never_read(self):
        self._manifest(gate_mode="warn", worktree_gate_mode="always")
        self._plan_build(env=self._env())
        self.assertEqual(self._ledger_modes(), ["warn"])

    def test_tracker_host_applies_same_mapping(self):
        self._manifest(ledger_mode="always", gate_mode="off")
        r = self._tracker_host("archive-close", env=self._env())
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(self._logged_checkpoints(), ["archive-close"], r.stderr)
        self.assertEqual(self._ledger_modes(), ["always"])

    def _tracker_host(self, checkpoint: str, *, env: dict,
                      slug: str | None = None) -> subprocess.CompletedProcess:
        """The direct shell-host surface the merge skills invoke."""
        args = ["--root", str(self.repo), "--checkpoint", checkpoint]
        if slug is not None:
            args.append(slug)
        return subprocess.run(
            ["bash", str(self._stamped_tracker_gate("warn")), *args],
            capture_output=True, text=True, env=env,
        )

    # --- 5.6: all five checkpoints reach one predicate ---

    def _five_host_commands(self) -> list[tuple[str, list[str], str, Path]]:
        active = self.repo / "openspec" / "changes" / "demo-change"
        (active / "specs").mkdir(parents=True, exist_ok=True)
        (active / "proposal.md").write_text("# proposal\n")
        (active / "tasks.md").write_text("Depth: light\n")
        # pre-merge needs a conforming archive; the active folder stays for the
        # path/plan hosts, so pre-merge grades a different, already-archived slug.
        archived = self.repo / "openspec" / "changes" / "archive" / "2026-09-13-done-change"
        archived.mkdir(parents=True, exist_ok=True)
        (archived / "proposal.md").write_text("# proposal\n")
        (archived / "tasks.md").write_text("Depth: light\n")
        gate = self._stamped_tracker_gate("warn")
        plan_event = json.dumps({
            "event": "pre-tool-use",
            "tool_name": "Write",
            "tool_input": {"file_path": str(self.repo / "src" / "app.py")},
            "cwd": str(self.repo),
        })
        path_event = json.dumps({
            "event": "pre-tool-use",
            "tool_name": "Edit",
            "tool_input": {"file_path": str(self.repo / "lib" / "foo.py")},
            "cwd": str(self.repo),
        })
        shell_event = json.dumps({
            "event": "pre-tool-use",
            "tool_name": "Bash",
            "tool_input": {"command": "gh pr create --fill"},
            "cwd": str(self.repo),
        })
        return [
            ("work-start", ["bash", str(self._stamped_plan_build_gate())], plan_event, active),
            ("apply-start", ["bash", str(gate)], path_event, active),
            ("pr-review", ["bash", str(gate)], shell_event, active),
            (
                "archive-close",
                ["bash", str(gate), "--root", str(self.repo),
                 "--checkpoint", "archive-close", "demo-change"],
                "",
                active,
            ),
            (
                "pre-merge",
                ["bash", str(gate), "--root", str(self.repo),
                 "--checkpoint", "pre-merge", "done-change"],
                "",
                archived,
            ),
        ]

    def test_work_start_blocks_while_tracker_hosts_report_advisory(self):
        self._manifest(gate_mode="warn")
        env = self._env(STUB_DECISION="block", STUB_REASON="needs-item")
        for checkpoint, cmd, payload, _ in self._five_host_commands():
            with self.subTest(checkpoint=checkpoint):
                r = subprocess.run(cmd, input=payload, capture_output=True, text=True, env=env)
                if checkpoint == "work-start":
                    self.assertNotEqual(r.returncode, 0, f"{checkpoint}: {r.stderr}")
                else:
                    self.assertEqual(
                        r.returncode, 0,
                        f"{checkpoint} is a Tracker checkpoint and must stay advisory: {r.stderr}",
                    )
                    self.assertIn(checkpoint, r.stderr,
                                  "the advisory host still reports the verdict")
                    self.assertIn("advisory", r.stderr.lower())
        self.assertEqual(
            sorted(self._logged_checkpoints()),
            ["apply-start", "archive-close", "pr-review", "pre-merge", "work-start"],
            self.stub_log.read_text(),
        )

    def test_all_five_hosts_allow_on_the_same_verdict(self):
        self._manifest(gate_mode="warn")
        env = self._env(STUB_DECISION="allow")
        for checkpoint, cmd, payload, _ in self._five_host_commands():
            with self.subTest(checkpoint=checkpoint):
                r = subprocess.run(cmd, input=payload, capture_output=True, text=True, env=env)
                self.assertEqual(r.returncode, 0, f"{checkpoint}: {r.stderr}")

    # --- 5.7: acquisition/JSON bridge only ---

    def test_hosts_add_no_tracker_predicate(self):
        forbidden = ("is_valid_link", "RECOGNIZED", "card_id", "## Tracker")
        for path in (PLAN_BUILD_GATE, TRACKER_GATE, GUARDIAN):
            text = path.read_text()
            for token in forbidden:
                with self.subTest(host=path.name, token=token):
                    self.assertNotIn(token, text, f"{path} re-introduced a predicate token")

    def test_recipe_registers_ledger_mode_help_text(self):
        data = tomllib.loads(RECIPE_TOML.read_text())
        self.assertIn("ledger_mode", data["config"])


if __name__ == "__main__":
    unittest.main()
