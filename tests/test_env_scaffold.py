"""Tests for env_scaffold.py (harness env layout)."""
from __future__ import annotations

import importlib.util
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

ROOT = Path(__file__).resolve().parents[1]
MOD_PATH = ROOT / "lib" / "_internal" / "env_scaffold.py"
VENDOR = ROOT / "lib" / "_vendor"

sys.path.insert(0, str(ROOT / "tests"))
from _blackbox import invoke, isolated_home  # noqa: E402
from _fixture_catalog import populate_catalog  # noqa: E402


def _ensure_vendor_path() -> None:
    if VENDOR.is_dir() and str(VENDOR) not in sys.path:
        sys.path.insert(0, str(VENDOR))


def load_module(path: Path, name: str):
    _ensure_vendor_path()
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class EnvScaffoldTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        _ensure_vendor_path()
        cls.mod = load_module(MOD_PATH, "env_scaffold_internal")

    def _project_with_recipe(
        self,
        root: Path,
        *,
        recipe_id: str,
        recipe_toml: str,
        enabled: bool = True,
    ) -> Path:
        catalog = root / "catalog" / "recipes" / recipe_id
        catalog.mkdir(parents=True)
        (catalog / "recipe.toml").write_text(recipe_toml, encoding="utf-8")
        project = root / "project"
        (project / "ai-specs").mkdir(parents=True)
        flag = "true" if enabled else "false"
        (project / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "p"\n\n'
            f"[recipes.{recipe_id}]\nenabled = {flag}\nversion = \"1.0\"\n",
            encoding="utf-8",
        )
        return project

    def _trello_toml(self) -> str:
        return (
            "[recipe]\n"
            'id = "trello-mcp-workflow"\n'
            'name = "Trello"\n'
            'description = "D"\n'
            'version = "1.0"\n\n'
            "[[provides.mcp]]\n"
            'id = "trello"\n'
            'command = "npx"\n'
            "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\", TRELLO_TOKEN = \"$TRELLO_TOKEN\" }\n"
        )

    def _cli_home_with_trello(self, base: Path) -> Path:
        """isolated_home whose catalog carries the custom trello recipe (custom wins)."""
        home = isolated_home(base)
        catalog = home / "catalog"
        catalog.unlink()
        recipes_dir = catalog / "recipes"
        recipes_dir.mkdir(parents=True)
        (recipes_dir / "trello-mcp-workflow").mkdir()
        (recipes_dir / "trello-mcp-workflow" / "recipe.toml").write_text(
            self._trello_toml(), encoding="utf-8"
        )
        populate_catalog(recipes_dir)
        return home

    # TRIAGE: asserts write_env renders dotenv ('TRELLO_API_KEY=secret', no 'export ') and leaves
    # the app .env untouched; witness `ai-specs doctor <proj>` is read-only (stdout is only OK/WARN
    # check lines — never the ai-specs.env bytes) and `ai-specs configure-recipes <proj>` exits 3
    # non-TTY (stderr 'configure-recipes requires an interactive TTY', snapshot diff {}); write_env
    # runs only inside the TTY config/init wizard, so no non-TTY surface reproduces the emit.
    def test_write_env_uses_dotenv_not_export(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            app_env = project / ".env"
            app_env.write_text("APP=keep\n", encoding="utf-8")
            path = self.mod.write_env(project, {"TRELLO_API_KEY": "secret"})
            text = path.read_text(encoding="utf-8")
            self.assertIn("TRELLO_API_KEY=secret", text)
            self.assertNotIn("export ", text)
            self.assertEqual(app_env.read_text(encoding="utf-8"), "APP=keep\n")

    # TRIAGE: asserts write_env merge keeps an unrelated CUSTOM=1 line while replacing the existing
    # TRELLO_API_KEY value; the only reachable surfaces are `ai-specs doctor <proj>` (read-only,
    # WARN/OK check lines, never per-key survival) and `ai-specs configure-recipes <proj>` (rc 3
    # non-TTY, tree unchanged) — no non-TTY verb runs the merge or reports which key survived.
    def test_write_env_merges_preserves_extras(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            env = project / "ai-specs.env"
            env.write_text("CUSTOM=1\nTRELLO_API_KEY=old\n", encoding="utf-8")
            self.mod.write_env(project, {"TRELLO_API_KEY": "new"})
            text = env.read_text(encoding="utf-8")
            self.assertIn("CUSTOM=1", text)
            self.assertIn("TRELLO_API_KEY=new", text)

    # TRIAGE: asserts write_env quotes a value containing spaces; the quoting is only exercised by
    # the TTY offer flow, and neither `ai-specs doctor <proj>` (read-only check lines) nor
    # `ai-specs configure-recipes <proj>` (rc 3 non-TTY) exposes the ai-specs.env emission.
    def test_write_env_quotes_spaces(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            self.mod.write_env(project, {"CANONICAL_VAULT_PATH": "/path with spaces/x"})
            text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn('CANONICAL_VAULT_PATH="/path with spaces/x"', text)

    # TRIAGE: asserts blank/whitespace updates do not wipe prior non-empty harness secrets; doctor
    # only reports which keys are missing (\"missing/empty in ai-specs.env: ...\"), never that a
    # blank update preserved the old value, and configure-recipes is rc 3 non-TTY — so the
    # preserve-on-blank outcome has no CLI surface.
    def test_write_env_blank_preserves_existing_secret(self):
        """JD-1: blank/whitespace updates must not wipe non-empty harness secrets."""
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            self.mod.write_env(project, {"TRELLO_API_KEY": "secret", "TRELLO_TOKEN": "tok"})
            self.mod.write_env(
                project,
                {"TRELLO_API_KEY": "", "TRELLO_TOKEN": "   ", "CUSTOM": "keep-me"},
            )
            text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn("TRELLO_API_KEY=secret", text)
            self.assertIn("TRELLO_TOKEN=tok", text)
            self.assertIn("CUSTOM=keep-me", text)

    # TRIAGE: offer_harness_env with blank prompt values is a TTY wizard step; asserts the prior
    # ai-specs.env keys survive. `ai-specs configure-recipes <proj>` exits 3 non-TTY and
    # `ai-specs doctor <proj>` stays read-only, so key preservation after a rejected prompt is not
    # observable on any non-TTY surface.
    def test_offer_harness_env_blank_prompt_preserves_existing(self):
        """JD-1: offer path with blank prompt values preserves prior ai-specs.env."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            (project / "ai-specs.env").write_text(
                "TRELLO_API_KEY=keep-key\nTRELLO_TOKEN=keep-tok\n",
                encoding="utf-8",
            )
            blanks = {"TRELLO_API_KEY": "", "TRELLO_TOKEN": ""}
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.object(
                self.mod, "prompt_env_vars", return_value=blanks
            ), patch.object(self.mod, "direnv_allow", return_value=True):
                self.mod.offer_harness_env(project, offer_direnv_install=False)
            text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn("TRELLO_API_KEY=keep-key", text)
            self.assertIn("TRELLO_TOKEN=keep-tok", text)

    # TRIAGE: asserts generate_env_example writes ai-specs.env.example (plus deprecated stubs) with
    # the trello keys and no `export`; only the TTY configure-recipes/init wizard emits the file —
    # `ai-specs configure-recipes <proj>` exits 3 non-TTY (stderr 'configure-recipes requires an
    # interactive TTY', snapshot {}: no example/stub appears), `ai-specs doctor <proj>` only reads
    # ai-specs.env (never the .example), and `ai-specs recipe configure trello-mcp-workflow
    # --inspect --json` returns rc 0 but surfaces just the collected env-var NAMES (TRELLO_API_KEY)
    # — never the example body, the trello.com/power-ups/admin URL, or the DEPRECATED stubs.
    def test_generate_env_example(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                path = self.mod.generate_env_example(project)
            text = path.read_text(encoding="utf-8")
            self.assertTrue(str(path).endswith("ai-specs.env.example"))
            self.assertIn("TRELLO_API_KEY=", text)
            self.assertIn("trello.com/power-ups/admin", text)
            self.assertNotIn("export ", text)
            stub = (project / "ai-specs" / ".envrc.example").read_text(encoding="utf-8")
            self.assertIn("DEPRECATED", stub)
            nested_stub = (project / "ai-specs" / ".env.example").read_text(encoding="utf-8")
            self.assertIn("DEPRECATED", nested_stub)

    # TRIAGE: asserts an existing ai-specs.env.example is first backed up to .bak; the backup is a
    # byproduct of the TTY-only generator and shows on no surface — `ai-specs configure-recipes
    # <proj>` returns 3 without running it and `ai-specs doctor <proj>` prints only check lines.
    def test_generate_env_example_backup(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            example = project / "ai-specs.env.example"
            example.write_text("OLD\n", encoding="utf-8")
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                self.mod.generate_env_example(project)
            self.assertEqual(
                (project / "ai-specs.env.example.bak").read_text(encoding="utf-8"),
                "OLD\n",
            )

    # TRIAGE: asserts ensure_root_envrc creates the root .envrc with the managed markers and
    # dotenv_if_exists lines; creating .envrc is a TTY wizard action — `configure-recipes <proj>`
    # exits 3 non-TTY (snapshot {}: no .envrc appears) and `doctor` only flags a missing/stale
    # block instead of writing one, so the emitted marker body is not observable.
    def test_ensure_root_envrc_creates(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            path = self.mod.ensure_root_envrc(project)
            text = path.read_text(encoding="utf-8")
            self.assertIn(self.mod.MANAGED_START, text)
            self.assertIn("dotenv_if_exists .env", text)
            self.assertIn("dotenv_if_exists ai-specs.env", text)

    # TRIAGE: asserts ensure_root_envrc preserves a leading custom `use nix\\n` line while appending
    # the managed block; the preserve-then-append text surgery is unreachable outside the TTY wizard
    # — `ai-specs configure-recipes` rc 3 non-TTY, `ai-specs doctor` read-only (never touches the
    # custom prefix) — so the literal leading line cannot be asserted through the CLI.
    def test_ensure_root_envrc_appends_preserving_custom(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            envrc = project / ".envrc"
            envrc.write_text("use nix\n", encoding="utf-8")
            self.mod.ensure_root_envrc(project)
            text = envrc.read_text(encoding="utf-8")
            self.assertTrue(text.startswith("use nix\n"))
            self.assertIn(self.mod.MANAGED_START, text)

    # TRIAGE: asserts a double ensure_root_envrc keeps exactly one managed block; the second call
    # is a TTY write with no CLI equivalent — `configure-recipes` exits 3 non-TTY and `doctor` is
    # read-only, so the idempotence count cannot be reproduced.
    def test_ensure_root_envrc_idempotent(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            self.mod.ensure_root_envrc(project)
            self.mod.ensure_root_envrc(project)
            text = (project / ".envrc").read_text(encoding="utf-8")
            self.assertEqual(text.count(self.mod.MANAGED_START), 1)

    # TRIAGE: asserts migrate_legacy_envrc folds export lines into ai-specs.env, renames
    # ai-specs/.envrc to .bak, and creates the root .envrc; the migration runs on no non-TTY verb —
    # `ai-specs configure-recipes <proj>` exits 3 and `ai-specs doctor <proj>` is read-only (never
    # renames a file or folds an export) — so neither the merged text nor the .envrc.bak appears.
    def test_migrate_legacy_envrc(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            legacy = project / "ai-specs" / ".envrc"
            legacy.write_text(
                'export TRELLO_TOKEN="abc"\nexport EMPTY=""\n',
                encoding="utf-8",
            )
            (project / "ai-specs.env").write_text(
                "TRELLO_API_KEY=keep\nTRELLO_TOKEN=existing\n",
                encoding="utf-8",
            )
            self.assertTrue(self.mod.migrate_legacy_envrc(project))
            env_text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn("TRELLO_API_KEY=keep", env_text)
            self.assertIn("TRELLO_TOKEN=existing", env_text)
            self.assertFalse(legacy.exists())
            self.assertTrue((project / "ai-specs" / ".envrc.bak").is_file())
            self.assertTrue((project / ".envrc").is_file())

    # TRIAGE: asserts migrate_legacy_envrc fills an absent harness key from the legacy export value
    # (TRELLO_TOKEN=abc); the gap-fill is migration-internal — `doctor` reports only that the key is
    # missing (never the legacy source value) and `configure-recipes` is rc 3 non-TTY, so the
    # migrated source cannot be asserted.
    def test_migrate_legacy_envrc_fills_absent_key(self):
        """Absent harness key receives migrated export value from legacy .envrc."""
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            legacy = project / "ai-specs" / ".envrc"
            legacy.write_text(
                'export TRELLO_TOKEN="abc"\n',
                encoding="utf-8",
            )
            (project / "ai-specs.env").write_text(
                "TRELLO_API_KEY=keep\n",
                encoding="utf-8",
            )
            self.assertTrue(self.mod.migrate_legacy_envrc(project))
            env_text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn("TRELLO_API_KEY=keep", env_text)
            self.assertIn("TRELLO_TOKEN=abc", env_text)
            self.assertFalse(legacy.exists())
            self.assertTrue((project / "ai-specs" / ".envrc.bak").is_file())

    # TRIAGE: asserts migrate_nested_harness_env copies ai-specs/.env keys up to root ai-specs.env,
    # renames the nested file to .bak, and adds `dotenv_if_exists ai-specs.env` to .envrc; neither
    # effect appears in `ai-specs doctor <proj>` (read-only check lines) and `configure-recipes`
    # exits 3 non-TTY, leaving the nested-to-root migration unobservable.
    def test_migrate_nested_harness_env(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            nested = project / "ai-specs" / ".env"
            nested.write_text("TRELLO_API_KEY=legacy\n", encoding="utf-8")
            self.assertTrue(self.mod.migrate_nested_harness_env(project))
            env_text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn("TRELLO_API_KEY=legacy", env_text)
            self.assertFalse(nested.exists())
            self.assertTrue((project / "ai-specs" / ".env.bak").is_file())
            self.assertIn("dotenv_if_exists ai-specs.env", (project / ".envrc").read_text())

    # TRIAGE: asserts a comment-only nested .env is left in place with no .bak — a no-rename guard
    # visible only in the migration return value; `ai-spec doctor <proj>` never reveals which file
    # a migration would rename and `configure-recipes` (non-TTY rc 3) never runs the migration.
    def test_migrate_nested_empty_does_not_rename(self):
        """JD-6: comment-only nested .env must not be renamed to .env.bak."""
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            nested = project / "ai-specs" / ".env"
            nested.write_text("# only comments\n\n", encoding="utf-8")
            self.assertFalse(self.mod.migrate_nested_harness_env(project))
            self.assertTrue(nested.is_file())
            self.assertFalse((project / "ai-specs" / ".env.bak").exists())

    # TRIAGE: asserts migration parses `export`-prefixed nested .env lines, merges them, then
    # renames the nested file; the export-parse-merge-rename chain is TTY-wizard-only (doctor read-
    # only, configure-recipes rc 3 non-TTY), so the merged value is not exposed by any surface.
    def test_migrate_nested_export_lines_merged(self):
        """JD-6: export-prefixed nested .env keys must parse, merge, then rename."""
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            nested = project / "ai-specs" / ".env"
            nested.write_text('export TRELLO_TOKEN="from-export"\n', encoding="utf-8")
            self.assertTrue(self.mod.migrate_nested_harness_env(project))
            env_text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn("TRELLO_TOKEN=from-export", env_text)
            self.assertFalse(nested.exists())
            self.assertTrue((project / "ai-specs" / ".env.bak").is_file())

    # TRIAGE: asserts a non-export legacy .envrc stays in place with no silent .bak — a no-rename
    # parity guard; neither `doctor` (read-only OK/WARN lines) nor `configure-recipes` (rc 3
    # non-TTY, tree unchanged) performs the migration, so the preserved-file outcome is invisible.
    def test_migrate_legacy_envrc_empty_does_not_rename(self):
        """JD-6 parity: non-export legacy .envrc is left in place (no silent bak)."""
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            legacy = project / "ai-specs" / ".envrc"
            legacy.write_text("# no exports\n", encoding="utf-8")
            self.assertFalse(self.mod.migrate_legacy_envrc(project))
            self.assertTrue(legacy.is_file())
            self.assertFalse((project / "ai-specs" / ".envrc.bak").exists())

    def test_managed_block_is_current_discerns_stale_body(self):
        """JD-8 helper observable via doctor: stale vs current vs missing root .envrc block."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            home = self._cli_home_with_trello(root)
            project = root / "project"
            (project / "ai-specs").mkdir(parents=True)
            (project / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "p"\n\n'
                "[recipes.trello-mcp-workflow]\nenabled = true\n",
                encoding="utf-8",
            )
            (project / "AGENTS.md").write_text("# p\n", encoding="utf-8")
            start = "# managed-by: ai-specs (do not remove block)"
            end = "# end managed-by: ai-specs"

            # No root .envrc -> doctor reports the managed block as MISSING.
            missing = invoke(project, "doctor", cli_home=home)
            self.assertIn("project-root .envrc missing ai-specs managed block", missing.stdout)
            self.assertNotIn("stale ai-specs managed block", missing.stdout)

            # Old body (dotenv_if_exists ai-specs/.env) -> block present but STALE.
            stale = f"{start}\ndotenv_if_exists .env\ndotenv_if_exists ai-specs/.env\n{end}\n"
            (project / ".envrc").write_text(stale, encoding="utf-8")
            stale_out = invoke(project, "doctor", cli_home=home)
            self.assertIn("project-root .envrc has stale ai-specs managed block", stale_out.stdout)
            self.assertNotIn("project-root .envrc missing ai-specs managed block", stale_out.stdout)
            self.assertNotIn("has ai-specs managed block", stale_out.stdout)

            # Canonical body -> CURRENT (OK line, no stale/missing).
            current = f"{start}\ndotenv_if_exists .env\ndotenv_if_exists ai-specs.env\n{end}\n"
            (project / ".envrc").write_text(current + "\n", encoding="utf-8")
            current_out = invoke(project, "doctor", cli_home=home)
            self.assertIn("project-root .envrc has ai-specs managed block", current_out.stdout)
            self.assertNotIn("stale ai-specs managed block", current_out.stdout)
            self.assertNotIn("missing ai-specs managed block", current_out.stdout)

    # TRIAGE: asserts migrate_legacy_harness_env returns False when no legacy harness env exists;
    # the migration body only runs under the TTY wizard — `ai-specs configure-recipes <proj>` exits
    # 3 non-TTY and `ai-specs doctor <proj>` is read-only — so the no-op return value is not
    # exposed on any non-TTY surface.
    def test_migrate_noop_when_absent(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            self.assertFalse(self.mod.migrate_legacy_harness_env(project))

    # TRIAGE: asserts the offer path soft-fails (no ai-specs.env created) when prompt_env_vars raises;
    # offer_harness_env runs only in the TTY wizard — `ai-specs configure-recipes <proj>` exits 3
    # non-TTY and `ai-specs doctor <proj>` is read-only, so the non-creation on prompt error is not
    # visible on any non-TTY surface.
    def test_offer_harness_env_soft_fails_on_prompt_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.object(
                self.mod,
                "prompt_env_vars",
                side_effect=TypeError("password="),
            ):
                self.mod.offer_harness_env(project, offer_direnv_install=False)
            self.assertFalse((project / "ai-specs.env").exists())

    # TRIAGE: asserts the offer path runs `direnv allow <project_root>` when direnv is present and
    # writes ai-specs.env; direnv trust is a TTY interactive act (configure-recipes rc 3 non-TTY)
    # and doctor only *reads* env layout, so the exact `direnv allow` argv and the created file have
    # no CLI-exposed equivalent beyond unavailable directions.
    def test_offer_harness_env_invokes_direnv_allow(self):
        """When direnv is present, offer path runs `direnv allow <project_root>`."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            values = {"TRELLO_API_KEY": "k", "TRELLO_TOKEN": "t"}
            proc = MagicMock(returncode=0)
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.object(
                self.mod, "prompt_env_vars", return_value=values
            ), patch("shutil.which", return_value="/usr/bin/direnv"), patch(
                "subprocess.run", return_value=proc
            ) as run:
                self.mod.offer_harness_env(project, offer_direnv_install=False)
            allow_calls = [
                c
                for c in run.call_args_list
                if c.args and list(c.args[0][:2]) == ["direnv", "allow"]
            ]
            self.assertEqual(len(allow_calls), 1)
            self.assertEqual(allow_calls[0].args[0], ["direnv", "allow", str(project)])
            self.assertTrue((project / "ai-specs.env").is_file())

    # TRIAGE: asserts missing direnv is not fatal — offer still writes ai-specs.env and .envrc and
    # prints `brew install direnv` guidance; the guidance is console-only and the offer never runs
    # without a TTY (`configure-recipes` rc 3), while doctor read-only never installs direnv.
    def test_offer_harness_env_soft_fails_without_direnv(self):
        """Missing direnv does not abort; non-fatal install guidance is printed."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            values = {"TRELLO_API_KEY": "k", "TRELLO_TOKEN": "t"}
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.object(
                self.mod, "prompt_env_vars", return_value=values
            ), patch("shutil.which", return_value=None), patch(
                "subprocess.run", side_effect=FileNotFoundError("direnv")
            ), patch("builtins.print") as mock_print:
                # Soft-fail path only — isolate from direnv install offer.
                self.mod.offer_harness_env(project, offer_direnv_install=False)
            self.assertTrue((project / "ai-specs.env").is_file())
            self.assertTrue((project / ".envrc").is_file())
            guidance = " ".join(
                str(c.args[0]) for c in mock_print.call_args_list if c.args
            )
            self.assertIn("direnv", guidance.lower())
            self.assertIn("brew install direnv", guidance)

    # TRIAGE: asserts the TTY offer path calls dep_install.offer_and_install with the resolved
    # direnv plan and tty=True; the install offer is inherently interactive (`configure-recipes`
    # exits 3 non-TTY) and doctor read-only neither resolves nor prints install plans — the call
    # contract is unobservable through any non-TTY CLI surface.
    def test_offer_harness_env_offers_direnv_install_when_missing_tty(self):
        """When direnv is missing on a TTY, offer path calls dep_install.offer_and_install."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            values = {"TRELLO_API_KEY": "k", "TRELLO_TOKEN": "t"}
            dep_install = MagicMock()
            plan = MagicMock(
                binary="direnv",
                command=["brew", "install", "direnv"],
                kind="brew",
            )
            dep_install.resolve_install_plan.return_value = plan
            dep_install.offer_and_install.return_value = []
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.object(
                self.mod, "prompt_env_vars", return_value=values
            ), patch.object(self.mod, "_load_sibling", return_value=dep_install), patch.object(
                self.mod, "direnv_allow", return_value=False
            ), patch("shutil.which", return_value=None), patch.object(
                sys.stdin, "isatty", return_value=True
            ), patch.object(sys.stdout, "isatty", return_value=True):
                self.mod.offer_harness_env(project, offer_direnv_install=True)
            dep_install.resolve_install_plan.assert_called()
            self.assertEqual(
                dep_install.resolve_install_plan.call_args.args[0], "direnv"
            )
            dep_install.offer_and_install.assert_called_once()
            call_args, call_kwargs = dep_install.offer_and_install.call_args
            self.assertEqual(call_args[0], [plan])
            self.assertTrue(call_kwargs.get("tty"))

    # TRIAGE: asserts prompt_env_vars routes secret keys through questionary.password while the
    # wizard is TTY-only (configure-recipes exits 3 non-TTY); `doctor` never prompts and never
    # collects user-typed values, so the password/text/confirm routing is unobservable.
    def test_prompt_env_vars_uses_password_api_for_secrets(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            password = MagicMock()
            password.return_value.ask.return_value = "secret-key"
            text = MagicMock()
            text.return_value.ask.return_value = "plain"
            confirm = MagicMock()
            confirm.return_value.ask.return_value = True
            q = MagicMock(password=password, text=text, confirm=confirm)
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.dict(
                "sys.modules", {"questionary": q}
            ):
                # MODE not in trello toml — only secrets
                result = self.mod.prompt_env_vars(project)
            self.assertEqual(result["TRELLO_API_KEY"], "secret-key")
            password.assert_called()


class DepInstallTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            ROOT / "lib" / "_internal" / "dep_install.py", "dep_install_internal"
        )

    # TRIAGE: asserts resolve_install_plan for `npx` yields a guidance plan with an empty command;
    # plan resolution only feeds the TTY dependency gate (configure-recipes exits 3 non-TTY; recipe
    # add is interactive), and `doctor` prints at most a recipe-dep WARN with the binary and
    # install_url — never plan.kind/plan.command — so the resolved guidance is unobservable.
    def test_npx_guidance_only(self):
        plan = self.mod.resolve_install_plan(
            "npx", install_url="https://nodejs.org/en/download"
        )
        self.assertEqual(plan.kind, "guidance")
        self.assertEqual(plan.command, [])

    # TRIAGE: asserts resolve_install_plan for `bb` stays a kind=guidance plan; the only CLI
    # surface that mentions deps, `doctor`'s recipe-dep WARN, prints the binary/purpose/install_url
    # but never the resolved plan kind, and configure-recipes is rc 3 non-TTY — plan resolution
    # remains internal.
    def test_bb_guidance_only(self):
        plan = self.mod.resolve_install_plan("bb", install_url="https://example.com")
        self.assertEqual(plan.kind, "guidance")

    # TRIAGE: asserts resolve_install_plan keeps an unknown binary as guidance with empty command
    # and the right binary name; doctor's dep check reports only missing/unusable binary status
    # plus the install_url, and the non-TTY surface is rc 3 configure-recipes — the plan shape is
    # not exposed.
    def test_unknown_binary_guidance_only(self):
        """Binary outside _PACKAGE_MAP / _GUIDANCE_ONLY stays guidance with empty command."""
        plan = self.mod.resolve_install_plan("totally-unknown-bin")
        self.assertEqual(plan.kind, "guidance")
        self.assertEqual(plan.command, [])
        self.assertEqual(plan.binary, "totally-unknown-bin")

    # TRIAGE: asserts resolve_install_plan selects the brew command when brew is present on Darwin;
    # the brew/apt resolution is a plan-selection internal — `doctor` WARN shows the binary and
    # install_url but never the `brew install gh` argv, and configure-recipes is rc 3 non-TTY —
    # so the exact command is not CLI-observable.
    def test_brew_plan_when_brew_present(self):
        with patch.object(self.mod.shutil, "which", side_effect=lambda b: "/opt/brew" if b == "brew" else None), patch.object(
            self.mod.platform, "system", return_value="Darwin"
        ):
            plan = self.mod.resolve_install_plan("gh")
        self.assertEqual(plan.kind, "brew")
        self.assertEqual(plan.command, ["brew", "install", "gh"])

    # TRIAGE: asserts resolve_install_plan emits a sudo apt-get command for jq on Linux; the
    # apt-branch selection is invisible to `doctor` (binary/purpose/install_url WARN only) and the
    # interactive gate is rc 3 non-TTY, so the `sudo apt-get install` argv cannot be asserted.
    def test_apt_plan_on_linux(self):
        def which(b):
            if b == "apt-get":
                return "/usr/bin/apt-get"
            return None

        with patch.object(self.mod.shutil, "which", side_effect=which), patch.object(
            self.mod.platform, "system", return_value="Linux"
        ):
            plan = self.mod.resolve_install_plan("jq")
        self.assertEqual(plan.kind, "apt")
        self.assertEqual(plan.command[:3], ["sudo", "apt-get", "install"])

    # TRIAGE: asserts offer_and_install with tty=False is a no-op that never subprocesses; the
    # offer gate by design touches no CLI surface — `configure-recipes` is exit 3 non-TTY and
    # `doctor` read-only — so the empty result + run.assert_not_called is internal-only.
    def test_offer_non_tty_noop(self):
        plan = self.mod.InstallPlan(
            binary="jq",
            command=["brew", "install", "jq"],
            display="brew install jq",
            guidance_url="",
            kind="brew",
        )
        with patch.object(self.mod.subprocess, "run") as run:
            out = self.mod.offer_and_install([plan], tty=False)
        self.assertEqual(out, [])
        run.assert_not_called()

    # TRIAGE: asserts offer_and_install declines (no run) when the user cancels the confirm prompt;
    # the questionary decline is a TTY-only interaction (configure-recipes exits 3 non-TTY) and no
    # non-TTY verb surfaces the declined install — the run.assert_not_called is unobservable.
    def test_offer_decline_no_run(self):
        plan = self.mod.InstallPlan(
            binary="jq",
            command=["brew", "install", "jq"],
            display="brew install jq",
            guidance_url="",
            kind="brew",
        )
        confirm = MagicMock()
        confirm.return_value.ask.return_value = False
        q = MagicMock(confirm=confirm)
        with patch.dict("sys.modules", {"questionary": q}), patch.object(
            self.mod.subprocess, "run"
        ) as run:
            out = self.mod.offer_and_install([plan], tty=True)
        self.assertEqual(out, [])
        run.assert_not_called()

    # TRIAGE: asserts an accepted install runs subprocess and rechecks the binary; the accept-recheck
    # contract only exists in the TTY gate — `doctor` read-only never invokes installs and
    # `configure-recipes` rc 3 non-TTY — so run.assert_called_once and the recheck result are not
    # observable through the CLI.
    def test_offer_accept_runs_and_rechecks(self):
        plan = self.mod.InstallPlan(
            binary="jq",
            command=["brew", "install", "jq"],
            display="brew install jq",
            guidance_url="",
            kind="brew",
        )
        confirm = MagicMock()
        confirm.return_value.ask.return_value = True
        q = MagicMock(confirm=confirm)
        proc = MagicMock(returncode=0)
        with patch.dict("sys.modules", {"questionary": q}), patch.object(
            self.mod.subprocess, "run", return_value=proc
        ) as run, patch.object(self.mod.shutil, "which", return_value="/usr/bin/jq"):
            out = self.mod.offer_and_install([plan], tty=True)
        self.assertEqual(out, ["jq"])
        run.assert_called_once()


if __name__ == "__main__":
    unittest.main()