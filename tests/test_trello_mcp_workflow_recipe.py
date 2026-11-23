"""Black-box trello-mcp-workflow recipe tests: every behavioral test drives ``bin/ai-specs``.

No test may import ``lib/_internal`` modules. Assertions preserve the original
contract intents (schema validation, declared hooks/config/reconcile tables,
sync stamping of hook scripts and manifest config, per-agent hook rendering,
golden skill/command/README content) through the CLI process boundary:

- Recipe schema validity is observable via ``recipe add`` (validation + exact
  id + "The next sync will materialize:" plan); declared primitives are read
  from the recipe's own catalog declaration (recipe.toml), mirroring the
  golden-content pattern.
- Sync stamping is observable through the materialized project manifest
  (reconcile table, lifecycle defaults) and the per-project runtime snapshot
  (``cache/projects/<key>/runtime.toml``). The gate hook itself is CLI-served,
  so no ``ai-specs/recipes/trello-mcp-workflow/hooks/`` script may appear.
- Per-agent hook rendering is observable via ``sync`` outputs: Claude's
  settings.json, Cursor's hooks.json + wrapper scripts, OMP's extensions.
"""
from __future__ import annotations

import json
import re
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import cache_project_dir, invoke, isolated_home  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
RECIPE_DIR = ROOT / "catalog" / "recipes" / "trello-mcp-workflow"
RECIPE_ID = "trello-mcp-workflow"


def norm(text: str) -> str:
    """Collapse markdown emphasis and whitespace so phrase assertions survive
    line wrapping and `inline code` styling."""
    return re.sub(r"[`*\s]+", " ", text).strip().lower()


def _make_home(base: Path) -> Path:
    """Isolated CLI install root.

    ``lib/`` is not installed: ``recipe add``/``sync`` are the native binary,
    which derives its cache and catalog roots from this home.
    """
    home = isolated_home(base)
    (home / "lib").unlink(missing_ok=True)
    return home


def _make_manifest(root: Path, name: str = "fixture") -> None:
    """Minimal initialized project (manifest + harness dirs) in temp."""
    (root / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (root / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
    (root / "ai-specs" / "ai-specs.toml").write_text(
        f"[project]\nname = {name!r}\n\n[agents]\nenabled = ['claude']\n"
    )


class _CliFixtureMixin:
    """One shared isolated cli_home and temp project per test command sequence."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-trello-")
        self.addCleanup(tmp.cleanup)
        self.base = Path(tmp.name)
        self.home = _make_home(self.base)
        self.root = self.base / "proj"
        _make_manifest(self.root)

    def recipe_add(self, recipe_id: str):
        result = invoke(self.root, "recipe", "add", recipe_id, cli_home=self.home)
        self.assertEqual(
            result.returncode, 0,
            f"recipe add {recipe_id} failed: {result.stdout}{result.stderr}",
        )
        return result

    def sync(self):
        result = invoke(self.root, "sync", cli_home=self.home)
        return result

    def configure_board_id(self) -> None:
        """Satisfy the recipe's required board_id in the project manifest."""
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(
            manifest.read_text().replace(
                'board_id = ""  # REQUIRED',
                'board_id = "69ec097f13e2d38ecd89a557"',
            )
        )

    def tracker_gate_script(self) -> Path:
        return (
            self.root / "ai-specs" / "recipes" / RECIPE_ID
            / "hooks" / "tracker-card-gate.sh"
        )

    def runtime_snapshot(self) -> dict:
        path = cache_project_dir(self.root, self.home) / "runtime.toml"
        return tomllib.loads(path.read_text())


class TrelloMcpWorkflowRecipeTests(_CliFixtureMixin, unittest.TestCase):
    def test_recipe_validates_with_dual_hooks_and_gate_mode(self):
        """Recipe validates through the CLI and declares dual pre-tool-use
        hooks, the deprecated gate_mode field, and tracker brief rules.

        ``recipe add`` runs schema validation and adds by the exact id; the
        declared primitives are read from the recipe's catalog declaration.
        """
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            f"Recipe '{RECIPE_ID}' added to the manifest.", result.stdout,
            "trello-mcp-workflow must pass schema validation and add by its exact id",
        )
        raw = tomllib.loads((RECIPE_DIR / "recipe.toml").read_text())
        self.assertEqual(raw["recipe"]["id"], RECIPE_ID)
        self.assertEqual(raw["recipe"]["version"], "1.4.0")
        gate_mode = raw["config"]["gate_mode"]
        self.assertEqual(gate_mode["default"], "warn")
        self.assertEqual(set(gate_mode["enum"]), {"off", "warn", "always"})
        hooks = {h["id"]: h for h in raw["provides"]["hooks"]}
        self.assertEqual(set(hooks), {"tracker-card-gate", "tracker-card-gate-shell"})
        self.assertEqual(hooks["tracker-card-gate"]["matcher"], "Edit|Write|MultiEdit|NotebookEdit")
        self.assertEqual(
            hooks["tracker-card-gate-shell"]["matcher"], "Bash|Shell|Execute|Terminal"
        )
        self.assertEqual(hooks["tracker-card-gate"]["script"], "hooks/tracker-card-gate.sh")
        self.assertEqual(hooks["tracker-card-gate-shell"]["script"], "hooks/tracker-card-gate.sh")
        # Tracker hooks are advisory metadata: they never claim blocking = true.
        self.assertFalse(hooks["tracker-card-gate"]["blocking"])
        self.assertFalse(hooks["tracker-card-gate-shell"]["blocking"])
        rules = " ".join(raw["provides"]["brief"]["workflow_rules"])
        self.assertIn("## Tracker", rules)
        self.assertIn("tracker.none", rules)
        self.assertIn("never bypass", rules.lower())
        self.assertIn("phase", rules.lower())

    def test_tracker_hooks_and_gate_mode_are_metadata_advisory(self):
        """T3: Tracker hook metadata and the legacy gate_mode field are advisory,
        never declared as a blocking gate, while ledger_mode stays canonical."""
        raw = tomllib.loads((RECIPE_DIR / "recipe.toml").read_text())
        hooks = {h["id"]: h for h in raw["provides"]["hooks"]}
        for hook_id in ("tracker-card-gate", "tracker-card-gate-shell"):
            with self.subTest(hook=hook_id):
                hook = hooks[hook_id]
                self.assertFalse(hook["blocking"], "Tracker hooks must not claim blocking")
                self.assertIn("advisory", (hook.get("description") or "").lower())
        self.assertIn("ledger_mode", raw["config"])
        self.assertNotIn(
            "off", set(raw["config"]["ledger_mode"]["enum"]),
            "ledger_mode is always|ask|warn; off stays a gate_mode compatibility value",
        )
        self.assertIn("deprecated", (raw["config"]["gate_mode"].get("help_text") or "").lower())

    def test_tracking_declaration_matches_recipe_config(self):
        config_text = (ROOT / "openspec" / "config.yaml").read_text()
        board_match = re.search(r"^  board_id:\s*\"([^\"]+)\"", config_text, re.MULTILINE)
        mode_match = re.search(r"^  gate_mode:\s*(\w+)", config_text, re.MULTILINE)
        manifest = tomllib.loads((ROOT / "ai-specs" / "ai-specs.toml").read_text())
        configured = manifest["recipes"]["trello-mcp-workflow"]["config"]
        self.assertIsNotNone(board_match)
        self.assertIsNotNone(mode_match)
        self.assertEqual(board_match.group(1), configured["board_id"])
        self.assertEqual(mode_match.group(1), configured["gate_mode"])

    def test_recipe_declares_first_class_reconcile_table(self):
        """The declared reconcile block is a first-class table whose exact shape
        is the authority the Go gate decodes; ``recipe add`` validates it."""
        self.recipe_add(RECIPE_ID)
        raw = tomllib.loads((RECIPE_DIR / "recipe.toml").read_text())
        reconcile = raw["config"]["reconcile"]
        self.assertEqual(
            set(reconcile), {"scope_field", "max_age_seconds", "expectations"}
        )
        self.assertIsInstance(reconcile["scope_field"], str)
        self.assertIsInstance(reconcile["max_age_seconds"], int)
        # The declared expectation shape is exactly these four fields (every
        # entry is a subset, and the conditional merge entry uses all of them).
        declared_keys = set().union(*(set(e) for e in reconcile["expectations"]))
        self.assertEqual(
            declared_keys,
            {"event", "property", "config_field", "config_field_when_set"},
        )

    def test_recipe_declares_lifecycle_event_defaults(self):
        """Recipe-owned mapping: review/merge events resolve list names from
        defaulted config fields, so a synced project reconciles with zero
        per-project configuration."""
        raw = tomllib.loads((RECIPE_DIR / "recipe.toml").read_text())
        self.assertEqual(raw["config"]["review_list"]["default"], "Review")
        self.assertEqual(raw["config"]["done_list"]["default"], "Done")
        expectations = raw["config"]["reconcile"]["expectations"]
        by_event = {e["event"]: e for e in expectations}
        self.assertEqual(
            by_event["review"], {"event": "review", "property": "list", "config_field": "review_list"}
        )
        self.assertEqual(
            by_event["merge"],
            {
                "event": "merge",
                "property": "list",
                "config_field": "done_list",
                "config_field_when_set": "published_list",
            },
        )

    def test_recipe_declares_optional_published_list_without_invented_default(self):
        """The Published lifecycle list is optional and has no fabricated default:
        a project that never configured it keeps the merge target on Done."""
        raw = tomllib.loads((RECIPE_DIR / "recipe.toml").read_text())
        self.assertIn("published_list", raw["config"])
        published = raw["config"]["published_list"]
        self.assertNotIn("default", published)
        self.assertFalse(published.get("required", False))

    def test_sync_stamps_conditional_merge_mapping_without_published_list(self):
        self.recipe_add(RECIPE_ID)
        self.configure_board_id()
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        cfg = tomllib.loads(manifest.read_text())["recipes"][RECIPE_ID]["config"]
        merge = next(e for e in cfg["reconcile"]["expectations"] if e["event"] == "merge")
        self.assertEqual(merge["config_field"], "done_list")
        self.assertEqual(merge["config_field_when_set"], "published_list")
        # No fabricated Published list: the project never configured one.
        self.assertNotIn("published_list", cfg)

    def test_sync_preserves_project_published_list_override(self):
        self.recipe_add(RECIPE_ID)
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(
            manifest.read_text().replace(
                'board_id = ""  # REQUIRED',
                'board_id = "69ec097f13e2d38ecd89a557"\npublished_list = "Published"',
            )
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        cfg = tomllib.loads(manifest.read_text())["recipes"][RECIPE_ID]["config"]
        self.assertEqual(cfg["published_list"], "Published")
        merge = next(e for e in cfg["reconcile"]["expectations"] if e["event"] == "merge")
        self.assertEqual(merge["config_field_when_set"], "published_list")

    def test_sync_stamps_declared_reconcile_and_lifecycle_defaults(self):
        """Recipe-declared reconcile table and lifecycle list defaults propagate
        into the project manifest during sync, so reconciliation works out of
        the box and per-project config remains an override."""
        self.recipe_add(RECIPE_ID)
        self.configure_board_id()
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        cfg = tomllib.loads(manifest.read_text())["recipes"][RECIPE_ID]["config"]
        self.assertEqual(cfg["review_list"], "Review")
        self.assertEqual(cfg["done_list"], "Done")
        self.assertIn("reconcile", cfg)
        events = {e["event"] for e in cfg["reconcile"]["expectations"]}
        self.assertEqual(events, {"delivery", "review", "merge"})

    def test_sync_accepts_declared_reconcile_block_without_warning(self):
        self.recipe_add(RECIPE_ID)
        self.configure_board_id()
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        block = (
            f"[recipes.{RECIPE_ID}.config.reconcile]\n"
            'scope_field = "board_id"\n'
            "max_age_seconds = 900\n\n"
            f"[[recipes.{RECIPE_ID}.config.reconcile.expectations]]\n"
            'event = "delivery"\nproperty = "list"\nconfig_field = "default_list"\n'
        )
        manifest.write_text(manifest.read_text() + "\n" + block)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertNotIn("unknown config key", result.stderr)

    def test_sync_stamps_tracker_gate_mode_default_warn(self):
        self.recipe_add(RECIPE_ID)
        self.configure_board_id()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.tracker_gate_script().exists(),
                         "tracker-card-gate is CLI-served, never materialized")
        # The verb reads the ledger hint from the sync snapshot, not the toml.
        self.assertEqual(self.runtime_snapshot()["tracker_gate"]["gate_mode"], "warn")

    def test_sync_stamps_tracker_gate_mode_override(self):
        self.recipe_add(RECIPE_ID)
        self.configure_board_id()
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(
            manifest.read_text().replace('gate_mode = "warn"', 'gate_mode = "always"')
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.runtime_snapshot()["tracker_gate"]["gate_mode"], "always")

    def test_each_cli_served_recipe_keeps_its_own_gate_mode(self):
        """Both gates are CLI-served and each reads its own snapshot table: a
        worktree-flow project never sees the tracker hint and vice versa."""
        self.recipe_add("worktree-flow")
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(
            manifest.read_text().replace('gate_mode = "always"', 'gate_mode = "ask"')
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        wt_hook = (
            self.root / "ai-specs" / "recipes" / "worktree-flow"
            / "hooks" / "worktree-gate.sh"
        )
        self.assertFalse(wt_hook.exists(), "the worktree gate is CLI-served too")
        snap = self.runtime_snapshot()
        self.assertEqual(snap["worktree_gate"]["gate_mode"], "ask")
        # trello-mcp-workflow is not enabled here, so the tracker hint is empty
        # (the retired launcher's default-warn stamp only existed for the
        # recipe's own hook; the verb falls back to warn at run time).
        self.assertEqual(snap["tracker_gate"]["gate_mode"], "")

    def test_claude_dual_pretooluse_same_verb(self):
        self.recipe_add(RECIPE_ID)
        self.configure_board_id()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        settings = json.loads((self.root / ".claude" / "settings.json").read_text())
        pre = settings["hooks"]["PreToolUse"]
        managed = [e for e in pre if "tracker-card-gate" in json.dumps(e)]
        self.assertEqual(len(managed), 2)
        for entry in managed:
            self.assertIn("exec", entry["hooks"][0]["command"])
            self.assertIn("tracker-card-gate", entry["hooks"][0]["command"])
            self.assertNotIn("tracker-card-gate.sh", entry["hooks"][0]["command"])
        matchers = {e.get("matcher") for e in managed}
        self.assertIn("Edit|Write|MultiEdit|NotebookEdit", matchers)
        self.assertIn("Bash|Shell|Execute|Terminal", matchers)

    def test_cursor_shell_registers_filewrite_skipped(self):
        (self.root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['cursor']\n"
        )
        self.recipe_add(RECIPE_ID)
        self.configure_board_id()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        file_wrapper = self.root / ".cursor" / "hooks" / "trello-mcp-workflow-tracker-card-gate.sh"
        shell_wrapper = (
            self.root / ".cursor" / "hooks" / "trello-mcp-workflow-tracker-card-gate-shell.sh"
        )
        self.assertFalse(file_wrapper.exists(), "cursor skips file-write matcher")
        self.assertTrue(shell_wrapper.is_file(), "cursor registers shell id")
        hooks_json = json.loads((self.root / ".cursor" / "hooks.json").read_text())
        self.assertIn("beforeShellExecution", json.dumps(hooks_json))
        # The skipped file-write hook is reported as a warning naming the hook id.
        self.assertIn("tracker-card-gate", result.stderr.lower())

    def test_omp_both_matchers_case_insensitive(self):
        (self.root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['omp']\n"
        )
        self.recipe_add(RECIPE_ID)
        self.configure_board_id()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        ext_path = self.root / ".omp" / "extensions"
        files = list(ext_path.glob("*.ts")) if ext_path.is_dir() else []
        self.assertTrue(files, "omp extensions should be generated")
        blob = "\n".join(f.read_text() for f in files).lower()
        # Case-insensitive matcher coverage for file + shell tool names
        self.assertTrue("write" in blob or "edit" in blob)
        self.assertTrue("bash" in blob or "shell" in blob)

    def test_skill_documents_recipe_default_lifecycle_events(self):
        """The observation producer must state the recipe-supported events and
        their defaulted list mapping, so the agent observes against the mapping
        the gate will compare with."""
        skill = (RECIPE_DIR / "skills" / "trello-mcp-workflow" / "SKILL.md").read_text()
        self.assertIn("review_list", skill)
        self.assertIn("done_list", skill)
        self.assertIn("review", skill)
        self.assertIn("merge", skill)
        self.assertIn("overrides it", skill)  # config is override-only

    def test_skill_and_capabilities_document_published_and_closed_item_binding(self):
        """The optional conditional merge target and the corroborated closed-item
        comparison are documented, so the recipe-owned boundary is discoverable
        without the agent hand-authoring an event mapping."""
        skill = (RECIPE_DIR / "skills" / "trello-mcp-workflow" / "SKILL.md").read_text()
        capabilities = (ROOT / "docs" / "capabilities.md").read_text()
        self.assertIn("published_list", skill)
        self.assertIn("config_field_when_set", skill)
        self.assertIn("corroborat", skill.lower())
        self.assertIn("config_field_when_set", capabilities)
        self.assertIn("closed row", capabilities)
        self.assertIn("unbound-identity", capabilities)

    def test_tracker_lifecycle_host_is_documented_as_plan_build_independent(self):
        """The generic Tracker recipe surfaces the lifecycle host as a reusable
        command, separate from Plan Build/OpenSpec and from provider mapping.

        The host is the CLI verb's direct mode
        (``ai-specs tracker-card-gate --root <root> --checkpoint <name>``) — the
        retired Python host and the materialized shell bridge are both gone.
        """
        skill = (RECIPE_DIR / "skills" / "trello-mcp-workflow" / "SKILL.md").read_text()
        quick = (RECIPE_DIR / "commands" / "trello-workflow.md").read_text()
        readme = (RECIPE_DIR / "README.md").read_text()
        brief = (RECIPE_DIR / "recipe.toml").read_text()
        for name, text in (("skill", skill), ("quick-reference", quick),
                           ("README", readme), ("brief", brief)):
            with self.subTest(surface=name):
                self.assertIn("ai-specs tracker-card-gate", text)
                self.assertIn("--checkpoint", text)
                if name != "brief":
                    # The recipe declaration keeps its historical script path as
                    # the CLI-served marker; the prose surfaces name the verb.
                    self.assertNotIn("tracker-card-gate.sh", text)
                self.assertNotIn("tracker_ledger_host.py", text)
        for name, text in (("skill", skill), ("README", readme)):
            with self.subTest(surface=name):
                lowered = text.lower()
                self.assertIn("archive-close", lowered)
                self.assertIn("tracker item closure", lowered)
                self.assertIn("openspec", lowered)
        # Provider recipes only map native state; they never own the ledger.
        self.assertIn("[config.reconcile]", readme)
        self.assertIn("do not enable", readme)

    def _tracker_surfaces(self):
        return {
            "skill": (RECIPE_DIR / "skills" / "trello-mcp-workflow" / "SKILL.md").read_text(),
            "command": (RECIPE_DIR / "commands" / "trello-workflow.md").read_text(),
            "README": (RECIPE_DIR / "README.md").read_text(),
            "brief": (RECIPE_DIR / "recipe.toml").read_text(),
        }

    def test_skill_and_command_document_provider_backed_bind_payload(self):
        """T2 seam: after a provider card is created or linked, the recipe
        documents the exact local `bind` payload (kind/item_id/url/native_type/
        state plus the opaque provider snapshot) and states the `## Tracker`
        section is artifact sugar that never opens or binds a ledger row."""
        surfaces = self._tracker_surfaces()
        for name in ("skill", "command"):
            with self.subTest(surface=name):
                text = norm(surfaces[name])
                self.assertIn('"kind":"bind"', text)
                self.assertIn("--ledger-mode", text)
                self.assertIn("--project-root", text)
                self.assertIn("native_type", text)
                self.assertIn('"card"', text)
                self.assertIn("provider", text)
                self.assertIn("artifact sugar", text)
                self.assertIn("never opens or binds", text)

    def test_skill_and_command_document_reconcile_observation_seam(self):
        """T2 seam: lifecycle transitions read the card through MCP, emit the
        closed observation payload, and call the gate's explicit comparison;
        only `agree` is provider-backed compliance, while the Go ledger stays
        provider-neutral and provider writes stay outside it."""
        surfaces = self._tracker_surfaces()
        for name in ("skill", "command"):
            with self.subTest(surface=name):
                text = norm(surfaces[name])
                self.assertIn("trello_get_card", text)
                self.assertIn("--reconcile", text)
                self.assertIn("--reconcile-event", text)
                for key in ("provider_id", "scope", "item_id", "observed_at", "properties"):
                    self.assertIn(key, text)
                self.assertIn("only agree is provider-backed compliance", text)
                self.assertIn("provider-neutral", text)
                self.assertIn("no provider write", text)

    def test_skill_and_command_document_needs_item_ask_path(self):
        """T2 seam: an `ask` `needs-item` verdict is answered by creating or
        linking the provider item and then binding it locally; an explicit
        decline is lifecycle-scoped and is not repeated at every checkpoint."""
        surfaces = self._tracker_surfaces()
        for name in ("skill", "command"):
            with self.subTest(surface=name):
                text = norm(surfaces[name])
                self.assertIn("needs-item", text)
                self.assertIn("create or link", text)
                self.assertIn("then bind", text)
                self.assertIn("lifecycle-scoped", text)
                self.assertIn("not repeated", text)
                self.assertIn("opt-out", text)

    def test_readme_and_brief_keep_provider_neutral_boundary(self):
        """The provider-neutral core boundary stays stated on the README and
        recipe-brief surfaces: the recipe maps native state and never grades,
        enables, or implements the ledger."""
        surfaces = self._tracker_surfaces()
        readme = norm(surfaces["README"])
        brief = norm(surfaces["brief"])
        self.assertIn("provider-neutral", readme)
        self.assertIn("by mapping", readme)
        self.assertIn("never by grading", readme)
        self.assertIn("--ledger-mode", readme)
        self.assertIn('"kind":"bind"', readme)
        self.assertIn("provider recipes do not enable", readme)
        self.assertIn("map native state", brief)
        self.assertIn("never enable", brief)
        self.assertIn("never grade", brief)

    def test_bind_and_reconcile_surfaces_reject_local_only_compliance(self):
        """Triangulation: every documented bind is mode-aware, and the skill and
        command state that a non-agreeing comparison is a pending decision, never
        compliance — local-only success is never claimed on any surface."""
        surfaces = self._tracker_surfaces()
        for name in ("skill", "command", "README"):
            with self.subTest(surface=name, part="mode-aware bind"):
                self.assertIn(
                    "--ledger --checkpoint apply-start --ledger-mode",
                    norm(surfaces[name]),
                )
        for name in ("skill", "command"):
            with self.subTest(surface=name, part="pending decision"):
                self.assertIn("pending decision", norm(surfaces[name]))

    def test_recipe_version_and_migration_surface(self):
        """W6: the recipe version bump is recorded in the migration surface."""
        with open(RECIPE_DIR / "recipe.toml", "rb") as fh:
            self.assertEqual(tomllib.load(fh)["recipe"]["version"], "1.4.0")
        readme = (RECIPE_DIR / "README.md").read_text()
        catalog = (ROOT / "docs" / "recipes-catalog.md").read_text()
        self.assertIn('version = "1.4.0"', readme)
        self.assertIn('version = "1.4.0"', catalog)
        unreleased = (ROOT / "CHANGELOG.md").read_text().split("## [0.21.0]", 1)[0]
        self.assertIn("1.3.0` → `1.4.0", unreleased)

    def test_skill_doc_content_contract(self):
        skill = (RECIPE_DIR / "skills" / "trello-mcp-workflow" / "SKILL.md").read_text()
        self.assertNotIn("Allow the agent to skip card creation", skill)
        self.assertIn("## Tracker", skill)
        self.assertIn("tracker.none", skill)
        self.assertIn("missing link", skill.lower())
        self.assertIn("cache/projects/", skill)
        # auto_invoke triggers present
        self.assertIn("New structured change or feature request", skill)
        self.assertIn("missing a linked Trello card", skill)
        # unavailable excuse forbidden for missing artifact
        self.assertIn("availability failure", skill.lower())
        self.assertIn("do not claim", skill.lower())
        cmd = (RECIPE_DIR / "commands" / "trello-workflow.md").read_text()
        self.assertIn("## Tracker", cmd)
        boot = (
            ROOT / "catalog" / "recipes" / "session-context" / "skills"
            / "session-bootstrap" / "SKILL.md"
        ).read_text()
        self.assertIn("mandatory", boot.lower())
        self.assertIn("## Tracker", boot)
        self.assertNotIn("only if needed", boot)


if __name__ == "__main__":
    unittest.main()
