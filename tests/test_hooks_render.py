"""Black-box tests for per-harness runtime-hook wiring.

Every test drives ``bin/ai-specs sync`` through its process boundary:
hook-declaring recipes are staged into an isolated catalog (fresh recipe ids,
real non-symlinked recipe dirs, never touching the repository catalog), the
project manifest enables the agents under test, and the assertions observe
the per-agent hook artifacts ``sync`` writes (.claude/settings.json, .cursor
wrappers + hooks.json, .opencode plugins, .pi/.omp extensions) plus sync
stderr warnings. No test may import ``lib/_internal`` modules.

Contract reference: openspec/changes/recipe-runtime-hooks/specs/
runtime-hook-distribution/spec.md.

WorkspaceContextProcessBoundaryTests relocate the sync-generated adapter into
a temporary materialized installation and execute it through the Node harness
with a recording ``spawnSync`` double, observing the actual process boundary
(executable path, event cwd, options cwd, fail-open behavior) — never
generated source text.
"""
from __future__ import annotations

import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home, populate_catalog  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]

NODE_HARNESS = (
    ROOT / "tests" / "fixtures" / "workspace-context" / "opencode_process_boundary.mjs"
)
NODE = shutil.which("node")

# Fresh catalog recipe ids (never colliding with repository catalog recipes).
RECIPE_SHELL = "hookbb-shell"
RECIPE_FILEWRITE = "hookbb-filewrite"
RECIPE_STOP = "hookbb-stop"
HOOK_SCRIPT_REL = "hooks/gate.sh"

HOOK_SCRIPT_BODY = "#!/usr/bin/env bash\n# launcher marker\nexit 0\n"


def mcp_record_bytes(raw: bytes):
    import tomllib

    data = tomllib.loads(raw.decode())
    return data.get("mcp", {}), data.get("mcp-block", {})


def _make_home(base: Path) -> Path:
    """Isolated CLI install root.

    ``lib/`` is not installed: sync/materialize are the native binary, which
    derives its cache and catalog roots from this home.
    """
    home = isolated_home(base)
    (home / "lib").unlink(missing_ok=True)
    return home


def _hook_recipe_toml(recipe_id: str, hook_id: str, event: str, matcher: str) -> str:
    return (
        f'[recipe]\nid = "{recipe_id}"\nname = "{recipe_id}"\n'
        f'description = "test recipe {recipe_id}"\nversion = "1.0.0"\n\n'
        "[[provides.hooks]]\n"
        f'id = "{hook_id}"\n'
        f'event = "{event}"\n'
        f'script = "{HOOK_SCRIPT_REL}"\n'
        f'matcher = "{matcher}"\n'
        "blocking = true\n"
        f'description = "test hook {hook_id}"\n'
    )


class _HookFixtureMixin(unittest.TestCase):
    """One shared isolated cli_home and temp project per test command sequence."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-hooksbb-")
        self.addCleanup(tmp.cleanup)
        # Resolve once: the CLI reports real paths (/var -> /private/var on
        # macOS), so an unresolved fixture root would never match its output.
        self.base = Path(tmp.name).resolve()
        self.home = _make_home(self.base)
        self.root = self.base / "proj"
        (self.root / "ai-specs" / "skills").mkdir(parents=True)
        (self.root / "ai-specs" / "commands").mkdir()
        self._recipe_id = ""

    def seed_recipe(self, recipe_id: str, hook_id: str, *,
                    event: str = "pre-tool-use", matcher: str = "Bash") -> None:
        """Stage one hook-declaring catalog recipe into the isolated home."""
        rdir = populate_catalog(
            self.home, recipe_id, _hook_recipe_toml(recipe_id, hook_id, event, matcher)
        )
        script = rdir / HOOK_SCRIPT_REL
        script.parent.mkdir(parents=True, exist_ok=True)
        script.write_text(HOOK_SCRIPT_BODY)
        self._recipe_id = recipe_id

    @property
    def script_path(self) -> str:
        """Project-relative materialized launcher path for the seeded recipe."""
        return f"ai-specs/recipes/{self._recipe_id}/{HOOK_SCRIPT_REL}"

    @property
    def shim_base(self) -> str:
        return f"{self._recipe_id}-{self._hook_id}"

    def write_manifest(self, *agents: str) -> None:
        enabled = ", ".join(f"'{a}'" for a in agents)
        (self.root / "ai-specs" / "ai-specs.toml").write_text(
            f"[project]\nname = 'hookbb'\n\n[agents]\nenabled = [{enabled}]\n\n"
            f"[recipes.{self._recipe_id}]\nenabled = true\n"
        )

    def sync(self):
        result = invoke(self.root, "sync", cli_home=self.home)
        self.assertEqual(
            result.returncode, 0,
            f"sync failed: {result.stdout}{result.stderr}",
        )
        return result

    def seed_and_sync(self, recipe_id: str, hook_id: str, *agents: str, **kw):
        self._hook_id = hook_id
        self.seed_recipe(recipe_id, hook_id, **kw)
        self.write_manifest(*agents)
        return self.sync()


class HooksRenderTests(_HookFixtureMixin):
    def test_recipe_mcp_nan_does_not_remove_user_fields_on_resync(self):
        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("claude")
        (self.root / "AGENTS.md").write_text("# S4 probe\n")
        manifest = self.root / "ai-specs/ai-specs.toml"
        # MCP is rendered only when the manifest declares servers; without this
        # table sync-agent skips MCP entirely and the recipe never reaches it.
        manifest.write_text(manifest.read_text() + "\n[mcp.demo]\ncommand = 'echo'\n")
        recipe = self.base / "recipe-mcp.json"
        recipe.write_text('{"delta":{"command":"echo","env":{"K":NaN}}}\n')
        target = self.root / ".mcp.json"
        original = '{"manual":{"setting":"keep me"}}\n'
        target.write_text(original)

        def reject_constant(name):
            raise ValueError(f"bare {name} is not JSON")

        for attempt in (1, 2):
            with self.subTest(attempt=attempt):
                result = invoke(
                    self.root, "sync-agent", "--claude",
                    "--source-root", str(self.root), "--target", str(self.root),
                    "--recipe-mcp", str(recipe), cli_home=self.home,
                    append_root=False, tmpdir=self.base,
                )
                # Invalid recipe input is refused with one line naming the file.
                # The fixture root is a normalization root, so the line keeps
                # the file name under <TEMP>.
                self.assertEqual(result.returncode, 1, result.stderr)
                self.assertEqual(
                    result.stderr,
                    "error: <TEMP>/recipe-mcp.json: invalid JSON: non-finite "
                    "constant NaN/Infinity\n",
                )
                text = target.read_text()
                json.loads(text, parse_constant=reject_constant)
                self.assertEqual(text, original)
                self.assertFalse(list(target.parent.glob(".mcp.json.bak*")))

    def test_valid_numeric_extremes_survive_two_syncs(self):
        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("claude")
        manifest = self.root / "ai-specs/ai-specs.toml"
        manifest.write_text(manifest.read_text() + "\n[mcp.demo]\ncommand = 'echo'\n")

        targets = [self.root / ".claude/settings.json", self.root / ".mcp.json"]
        targets[0].parent.mkdir()
        seed = (
            '{"plus":1e400,"minus":-1e400,"zero":-0,"small":1e-400,'
            '"big":123456789012345678901234567890}'
        )
        for target in targets:
            target.write_text(seed)

        for attempt in (1, 2):
            with self.subTest(attempt=attempt):
                self.assertEqual(self.sync().stderr, "")
                for target in targets:
                    self.assertFalse(list(target.parent.glob(target.name + ".bak*")))
                    self.assertLessEqual(
                        {"plus", "minus", "zero", "small", "big"},
                        json.loads(target.read_text()).keys(),
                    )

    def test_invalid_hook_and_mcp_json_backed_up_before_regeneration(self):
        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("claude")
        manifest = self.root / "ai-specs/ai-specs.toml"
        manifest.write_text(manifest.read_text() + "\n[mcp.demo]\ncommand = 'echo'\n")
        hook, mcp = ".claude/settings.json", ".mcp.json"
        original = b'{\r\n  "user": true,\r\n}\r\n'
        (self.root / hook).parent.mkdir()
        for rel in (hook, mcp):
            (self.root / rel).write_bytes(original)
        result = self.sync()
        self.assertTrue(result.stdout.endswith("\n✓ ai-specs sync complete\n"))
        self.assertEqual(result.stderr, "".join(
            f"warning: invalid JSON in <TEMP>/{rel}; backed up to <TEMP>/{rel}.bak\n"
            for rel in (hook, mcp)
        ))
        for rel in (hook, mcp):
            self.assertEqual((self.root / (rel + ".bak")).read_bytes(), original)
        hooks = json.loads((self.root / hook).read_text())["hooks"]["PreToolUse"]
        self.assertEqual([entry["_ai_specs_managed"] for entry in hooks],
                         [f"ai-specs:hooks:{RECIPE_SHELL}:shell-gate"])
        self.assertEqual(json.loads((self.root / mcp).read_text()),
                         {"mcpServers": {"demo": {"command": "echo", "args": [], "env": {}}}})

        manifest.write_text(manifest.read_text().split("[recipes.")[0])
        edited = b'{ // keep this comment\n}\n'
        (self.root / hook).write_bytes(edited)
        result = self.sync()
        self.assertTrue(result.stdout.endswith("\n✓ ai-specs sync complete\n"))
        self.assertEqual(result.stderr, f"warning: invalid JSON in <TEMP>/{hook}; "
                         f"backed up to <TEMP>/{hook}.bak.1\n")
        self.assertEqual(json.loads((self.root / hook).read_text()), {})
        self.assertEqual((self.root / (hook + ".bak")).read_bytes(), original)
        self.assertEqual((self.root / (hook + ".bak.1")).read_bytes(), edited)

    def test_mcp_merge_keeps_user_servers_and_prunes_managed_ones(self):
        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("claude")
        manifest = self.root / "ai-specs/ai-specs.toml"
        manifest.write_text(manifest.read_text() + "\n[mcp.demo]\ncommand = 'echo'\n")
        target = self.root / ".mcp.json"
        target.write_text('{"mcpServers": {"mine": {"command": "user-cmd"}}}\n')

        # The first sync finds a server it never recorded: one upgrade notice.
        self.assertEqual(
            self.sync().stderr,
            "    ! <TEMP>/.mcp.json: MCP servers not recorded as "
            "ai-specs-managed: mine (remove them if ai-specs generated them)\n",
        )
        self.assertEqual(json.loads(target.read_text())["mcpServers"], {
            "mine": {"command": "user-cmd"},
            "demo": {"command": "echo", "args": [], "env": {}},
        })
        first = target.read_text()
        self.assertEqual(self.sync().stderr, "")
        self.assertEqual(target.read_text(), first)

        # Removing the manifest server deletes only the managed entry, and it
        # is deleted even though no [mcp.*] server remains.
        manifest.write_text(manifest.read_text().replace(
            "\n[mcp.demo]\ncommand = 'echo'\n", ""))
        self.assertEqual(self.sync().stderr, "")
        self.assertEqual(json.loads(target.read_text())["mcpServers"],
                         {"mine": {"command": "user-cmd"}})
        self.assertNotIn('"demo"', (self.root / "ai-specs/.ai-specs.lock").read_text())

    def test_codex_mcp_collision_survives_writes_and_prune(self):
        import tomllib

        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("codex")
        manifest = self.root / "ai-specs/ai-specs.toml"
        base = manifest.read_text()
        target = self.root / ".codex/config.toml"
        target.parent.mkdir()

        for header in ("same", "'same'", '"same"', r'"\u0073ame"'):
            with self.subTest(header=header):
                target.write_text(
                    f'[mcp_servers.{header}]\ncommand = "user-command"\n'
                )
                user = target.read_bytes()
                manifest.write_text(
                    base + '\n[mcp.same]\ncommand = "manifest-command"\n'
                    + '[mcp.new]\ncommand = "new-command"\n'
                )
                result = self.sync()
                self.assertEqual(
                    result.stderr,
                    '    ! preserved user MCP server "same" in '
                    '<TEMP>/.codex/config.toml (not recorded as ai-specs-managed)\n',
                )
                servers = tomllib.loads(target.read_text())["mcp_servers"]
                self.assertEqual(servers["same"]["command"], "user-command")
                self.assertEqual(servers["new"]["command"], "new-command")
                # Everything outside the managed block is byte-identical.
                self.assertTrue(target.read_bytes().startswith(user))
                manifest.write_text(base)
                self.sync()
                self.assertEqual(
                    tomllib.loads(target.read_text())["mcp_servers"],
                    {"same": {"command": "user-command"}},
                )

    def test_codex_prune_keeps_escaped_table_text_inside_multiline_string(self):
        import tomllib

        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("codex")
        manifest = self.root / "ai-specs/ai-specs.toml"
        base = manifest.read_text()
        manifest.write_text(base + '\n[mcp.same]\ncommand = "echo"\n')
        self.sync()

        target = self.root / ".codex/config.toml"
        user = (
            b'user_note = """\n[mcp_servers."\\u0073ame"]\n'
            b'this is user text, not a table\n"""\n'
        )
        target.write_bytes(user + b"\n" + target.read_bytes())
        manifest.write_text(base)
        self.assertEqual(self.sync().stderr, "")

        self.assertEqual(target.read_bytes(), user)
        self.assertEqual(tomllib.loads(target.read_text()), tomllib.loads(user.decode()))

    def test_codex_mcp_inline_server_is_not_duplicated(self):
        import tomllib

        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("codex")
        manifest = self.root / "ai-specs/ai-specs.toml"
        base = manifest.read_text()
        manifest.write_text(
            base + '\n[mcp.same]\ncommand = "manifest"\n'
            + '[mcp.other]\ncommand = "managed"\n'
        )
        target = self.root / ".codex/config.toml"
        target.parent.mkdir()
        original = b'[mcp_servers]\nsame = { command = "user" }\n'
        target.write_bytes(original)

        result = self.sync()
        self.assertEqual(
            result.stderr,
            '    ! preserved user MCP server "same" in '
            '<TEMP>/.codex/config.toml (not recorded as ai-specs-managed)\n',
        )
        servers = tomllib.loads(target.read_text())["mcp_servers"]
        self.assertEqual(servers["same"], {"command": "user"})
        self.assertEqual(servers["other"]["command"], "managed")

        manifest.write_text(base)
        self.sync()
        self.assertEqual(target.read_bytes(), original)

    def test_codex_array_row_does_not_hide_root_mcp_assignment(self):
        import tomllib

        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("codex")
        manifest = self.root / "ai-specs/ai-specs.toml"
        base = manifest.read_text()
        manifest.write_text(base + '\n[mcp.same]\ncommand = "managed"\n')
        self.sync()

        target = self.root / ".codex/config.toml"
        original = (
            b'user_choices = [\n  ["same"]\n]\n'
            b'mcp_servers.same.command = "user"\n'
        )
        expected = tomllib.loads(original.decode())
        target.write_bytes(original)
        manifest.write_text(base + '\n[mcp.same]\ncommand = "replacement"\n')

        result = invoke(self.root, "sync", cli_home=self.home)
        self.assertIn(result.returncode, (0, 1), result.stderr)
        self.assertEqual(target.read_bytes(), original)
        self.assertEqual(tomllib.loads(target.read_text()), expected)

    def test_codex_prune_keeps_comment_for_next_user_table(self):
        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("codex")
        manifest = self.root / "ai-specs/ai-specs.toml"
        base = manifest.read_text()
        manifest.write_text(base + '\n[mcp.same]\ncommand = "echo"\n')
        self.sync()

        target = self.root / ".codex/config.toml"
        user = b"# User note for the features below.\n[features]\nshell_tool = true\n"
        target.write_bytes(target.read_bytes() + b"\n" + user)
        manifest.write_text(base)
        self.sync()

        self.assertEqual(target.read_bytes(), user)

    def test_codex_marker_example_inside_user_string_is_never_removed(self):
        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("codex")
        target = self.root / ".codex/config.toml"
        target.parent.mkdir()
        original = (
            b'developer_instructions = """\nExample:\n'
            b'# managed-by: ai-specs (do not remove block): MCP servers\n'
            b'[mcp_servers.same]\ncommand="example"\n'
            b'# end managed-by: ai-specs: MCP servers\nEnd example.\n"""\n'
        )
        target.write_bytes(original)

        result = invoke(self.root, "sync", cli_home=self.home)
        self.assertIn(result.returncode, (0, 1), result.stderr)
        self.assertEqual(target.read_bytes(), original)

    def test_codex_moved_begin_marker_cannot_delete_user_table(self):
        import tomllib

        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("codex")
        manifest = self.root / "ai-specs/ai-specs.toml"
        base = manifest.read_text()
        manifest.write_text(base + '\n[mcp.same]\ncommand = "managed"\n')
        target = self.root / ".codex/config.toml"
        target.parent.mkdir()
        user = b"[features]\nshell_tool = true\n"
        target.write_bytes(user)
        self.sync()

        marker = b"# managed-by: ai-specs (do not remove block): MCP servers\n"
        moved = marker + user + target.read_bytes().split(marker, 1)[1]
        target.write_bytes(moved)
        manifest.write_text(base)

        result = invoke(self.root, "sync", cli_home=self.home)
        if result.returncode == 1:
            self.assertEqual(target.read_bytes(), moved)
        else:
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(tomllib.loads(target.read_text()), tomllib.loads(user.decode()))

    def test_codex_dotted_manifest_name_cannot_modify_user_server(self):
        import tomllib

        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("codex")
        manifest = self.root / "ai-specs/ai-specs.toml"
        manifest.write_text(
            manifest.read_text() + '\n[mcp."dev.server"]\ncommand = "owned"\n'
        )
        target = self.root / ".codex/config.toml"
        target.parent.mkdir()
        original = b'[mcp_servers.dev]\ncommand = "user"\n'
        target.write_bytes(original)

        result = invoke(self.root, "sync", cli_home=self.home)
        if result.returncode == 1:
            self.assertEqual(target.read_bytes(), original)
        else:
            self.assertEqual(result.returncode, 0, result.stderr)
            servers = tomllib.loads(target.read_text())["mcp_servers"]
            self.assertEqual(servers["dev"], {"command": "user"})
            self.assertEqual(servers["dev.server"]["command"], "owned")

    def test_codex_moved_end_cannot_delete_unowned_mcp_server(self):
        import tomllib

        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("codex")
        manifest = self.root / "ai-specs/ai-specs.toml"
        base = manifest.read_text()
        manifest.write_text(base + '\n[mcp.same]\ncommand = "managed"\n')
        self.sync()

        target = self.root / ".codex/config.toml"
        end = b"# end managed-by: ai-specs: MCP servers\n"
        user = b'[mcp_servers.user_owned]\ncommand = "keep-user"\n'
        target.write_bytes(target.read_bytes() + b"\n" + user)
        moved = target.read_bytes().replace(end, b"", 1) + b"\n" + end
        target.write_bytes(moved)
        manifest.write_text(base)

        result = invoke(self.root, "sync", cli_home=self.home)
        if result.returncode == 1:
            self.assertEqual(target.read_bytes(), moved)
        else:
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(
                tomllib.loads(target.read_text())["mcp_servers"],
                {"user_owned": {"command": "keep-user"}},
            )

    def test_codex_moved_end_cannot_delete_user_comment(self):
        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("codex")
        manifest = self.root / "ai-specs/ai-specs.toml"
        base = manifest.read_text()
        manifest.write_text(base + '\n[mcp.same]\ncommand = "managed"\n')
        self.sync()

        target = self.root / ".codex/config.toml"
        end = b"# end managed-by: ai-specs: MCP servers\n"
        note = b"# User note for features: keep enabled on this machine.\n"
        before = target.read_bytes() + b"\n" + note + b"[features]\nshell_tool = true\n"
        target.write_bytes(before)
        moved = before.replace(end, b"", 1).replace(note, note + end, 1)
        target.write_bytes(moved)
        manifest.write_text(base)

        result = invoke(self.root, "sync", cli_home=self.home)
        if result.returncode == 1:
            self.assertEqual(target.read_bytes(), moved)
        else:
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn(note, target.read_bytes())

    def test_codex_prune_does_not_delete_inline_user_comment(self):
        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("codex")
        manifest = self.root / "ai-specs/ai-specs.toml"
        base = manifest.read_text()
        manifest.write_text(base + '\n[mcp.same]\ncommand = "managed"\n')
        self.sync()

        target = self.root / ".codex/config.toml"
        lock = self.root / "ai-specs/.ai-specs.lock"
        note = b"# User note: keep this explanation."
        annotated = target.read_bytes().replace(
            b"[mcp_servers.same]\n", b"[mcp_servers.same] " + note + b"\n", 1
        )
        self.assertIn(note, annotated)
        target.write_bytes(annotated)
        original_lock = lock.read_bytes()
        manifest.write_text(base)

        result = invoke(self.root, "sync", cli_home=self.home)
        if result.returncode == 1:
            self.assertEqual(target.read_bytes(), annotated)
            # The AGENTS.md brief baseline is legitimately restamped for the
            # changed manifest, so compare the MCP ownership record, which is the
            # state this refusal must leave untouched.
            import tomllib

            def mcp_record(path):
                data = tomllib.loads(path.read_text())
                return data.get("mcp", {}), data.get("mcp-block", {})

            self.assertEqual(mcp_record(lock), mcp_record_bytes(original_lock))
        else:
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn(note, target.read_bytes())

    def test_codex_hashless_managed_block_does_not_lose_inline_comment(self):
        import re

        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("codex")
        manifest = self.root / "ai-specs/ai-specs.toml"
        base = manifest.read_text()
        manifest.write_text(base + '\n[mcp.same]\ncommand = "managed"\n')
        self.sync()

        target = self.root / ".codex/config.toml"
        lock = self.root / "ai-specs/.ai-specs.lock"
        note = b"# An inline note from the user: retain this."
        annotated = target.read_bytes().replace(
            b"[mcp_servers.same]\n", b"[mcp_servers.same] " + note + b"\n", 1
        )
        self.assertIn(note, annotated)
        hashless, count = re.subn(
            rb'^\[mcp-block\."\.codex/config\.toml"\]\n'
            rb'sha256 = "[0-9a-f]{64}"\n\n?',
            b"", lock.read_bytes(), flags=re.M,
        )
        self.assertEqual(count, 1)
        lock.write_bytes(hashless)
        target.write_bytes(annotated)
        manifest.write_text(base)

        result = invoke(self.root, "sync", cli_home=self.home)
        if result.returncode == 1:
            self.assertEqual(target.read_bytes(), annotated)
            # The AGENTS.md brief baseline is restamped for the changed manifest,
            # so compare the MCP ownership record this refusal must leave alone.
            self.assertEqual(mcp_record_bytes(lock.read_bytes()),
                             mcp_record_bytes(hashless))
        else:
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn(note, target.read_bytes())

        import hashlib
        import tomllib

        # The refusal above proved the annotation survived. Now the user
        # explicitly removes the block and asks sync to rebuild it.
        target.write_bytes(b"")
        manifest.write_text(base + '\n[mcp.same]\ncommand = "recovered"\n')
        self.sync()

        rendered = target.read_bytes()
        self.assertEqual(
            tomllib.loads(rendered.decode())["mcp_servers"]["same"]["command"],
            "recovered",
        )
        self.assertEqual(
            tomllib.loads(lock.read_text())["mcp-block"][".codex/config.toml"]["sha256"],
            hashlib.sha256(rendered).hexdigest(),
        )

    def test_corrupt_mcp_target_is_backed_up_when_pruning_last_server(self):
        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("claude")
        manifest = self.root / "ai-specs/ai-specs.toml"
        base = manifest.read_text()
        manifest.write_text(base + "\n[mcp.demo]\ncommand = 'echo'\n")
        self.sync()

        target = self.root / ".mcp.json"
        corrupt = b'{"mcpServers":{"demo":{"command":"echo"}},}\r\n'
        target.write_bytes(corrupt)
        manifest.write_text(base)
        result = self.sync()

        self.assertIn(
            "warning: invalid JSON in <TEMP>/.mcp.json; "
            "backed up to <TEMP>/.mcp.json.bak\n",
            result.stderr,
        )
        self.assertEqual((self.root / ".mcp.json.bak").read_bytes(), corrupt)
        self.assertNotIn("demo", json.loads(target.read_text()).get("mcpServers", {}))
        self.assertNotIn("[mcp.", (self.root / "ai-specs/.ai-specs.lock").read_text())

    def test_warns_about_unrecorded_mcp_on_upgrade_after_manifest_removal(self):
        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("claude")
        target = self.root / ".mcp.json"
        # Old generated output has no per-target ownership record.
        original = b'{"mcpServers":{"legacy":{"command":"echo","args":[],"env":{}}}}\n'
        target.write_bytes(original)

        result = self.sync()
        self.assertRegex(
            result.stderr, r'(?im)^.*(?:unrecorded|not recorded|ownership).*$'
        )
        self.assertEqual(target.read_bytes(), original)

        # The notice is recorded once and never repeated.
        self.assertEqual(self.sync().stderr, "")
        self.assertEqual(target.read_bytes(), original)

        manifest = self.root / "ai-specs/ai-specs.toml"
        base = manifest.read_text()
        manifest.write_text(base + "\n[mcp.legacy]\ncommand = 'replacement'\n")
        self.assertIn(
            'preserved user MCP server "legacy"', self.sync().stderr
        )
        self.assertEqual(target.read_bytes(), original)

        manifest.write_text(base)
        self.assertEqual(self.sync().stderr, "")
        self.assertEqual(target.read_bytes(), original)
        self.assertIn(
            '[mcp.".mcp.json"]\n',
            (self.root / "ai-specs/.ai-specs.lock").read_text(),
        )

    def test_removed_recipe_prunes_all_wiring(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "claude", "cursor",
                           "opencode", "pi", "omp")
        paths = [self.root / self.script_path]
        paths += [self.root / f"{directory}/{self.shim_base}.{extension}"
                  for directory, extension in ((".cursor/hooks", "sh"),
                      (".opencode/plugin", "ts"), (".pi/extensions", "ts"),
                      (".omp/extensions", "ts"))]
        self.assertTrue(all(path.is_file() for path in paths))
        manifest = self.root / "ai-specs/ai-specs.toml"
        manifest.write_text(manifest.read_text().split("[recipes.")[0])
        self.sync()
        paths.append(self.root / f"ai-specs/recipes/{RECIPE_SHELL}")
        self.assertEqual([str(p.relative_to(self.root)) for p in paths if p.exists()], [])
        for config in (".claude/settings.json", ".cursor/hooks.json"):
            self.assertNotIn(f"ai-specs:hooks:{RECIPE_SHELL}:", (self.root / config).read_text())
        self.assertNotIn(self.script_path, (self.root / "ai-specs/.ai-specs.lock").read_text())

    def test_missing_subrepo_shim_loses_ownership_before_path_reuse(self):
        import tomllib

        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("pi")
        manifest = self.root / "ai-specs/ai-specs.toml"
        manifest.write_text(manifest.read_text().replace(
            "[project]\n", '[project]\nsubrepos = ["sub-a"]\n', 1
        ))
        (self.root / "sub-a").mkdir()
        self.sync()

        rel = f"sub-a/.pi/extensions/{RECIPE_SHELL}-shell-gate.ts"
        shim = self.root / rel
        lock = self.root / "ai-specs/.ai-specs.lock"
        self.assertIn(rel, tomllib.loads(lock.read_text())["hooks"])

        shim.unlink()
        manifest.write_text(manifest.read_text().split("[recipes.")[0])
        self.sync()
        self.assertNotIn(rel, tomllib.loads(lock.read_text()).get("hooks", {}))

        user = b"// user-authored replacement, not a generated shim\n"
        shim.write_bytes(user)
        self.sync()
        self.assertEqual(shim.read_bytes(), user)

    def test_omitted_subrepo_shim_record_cannot_redeem_a_handwritten_file(self):
        import tomllib

        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        self.write_manifest("pi")
        manifest = self.root / "ai-specs/ai-specs.toml"
        with_subrepo = manifest.read_text().replace(
            "[project]\n", '[project]\nsubrepos = ["sub-a"]\n', 1
        )
        manifest.write_text(with_subrepo)
        (self.root / "sub-a").mkdir()
        self.sync()

        rel = f"sub-a/.pi/extensions/{RECIPE_SHELL}-shell-gate.ts"
        shim = self.root / rel
        lock = self.root / "ai-specs/.ai-specs.lock"
        self.assertIn(rel, tomllib.loads(lock.read_text())["hooks"])

        # The subrepo leaves the manifest, so no sync visits it any more: its
        # ownership record must still lose whatever authority the shim no
        # longer backs.
        manifest.write_text(with_subrepo.split("[recipes.")[0].replace(
            'subrepos = ["sub-a"]\n', ''
        ))
        shim.unlink()
        self.sync()
        self.assertNotIn(rel, tomllib.loads(lock.read_text()).get("hooks", {}))

        # Re-adding the subrepo (recipe still disabled) must not delete a
        # hand-authored file that now sits at the recorded path.
        user = b"// user-authored replacement, not a generated shim\n"
        shim.write_bytes(user)
        manifest.write_text(with_subrepo.split("[recipes.")[0])
        self.sync()
        self.assertEqual(shim.read_bytes(), user)

    def test_edited_shim_is_pruned_by_its_ownership_record(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "claude", "cursor",
                           "opencode", "pi", "omp")
        shims = [self.root / f"{directory}/{self.shim_base}.{extension}"
                 for directory, extension in ((".cursor/hooks", "sh"),
                     (".opencode/plugin", "ts"), (".pi/extensions", "ts"),
                     (".omp/extensions", "ts"))]
        import tomllib

        lock = self.root / "ai-specs/.ai-specs.lock"
        recorded = tomllib.loads(lock.read_text())["hooks"]
        owner = f"ai-specs:hooks:{RECIPE_SHELL}:shell-gate"
        for shim in shims:
            self.assertEqual(recorded[shim.relative_to(self.root).as_posix()], owner)
        # Edit every shim and strip its generated marker: the ownership record,
        # not the marker, is what licenses the prune.
        for shim in shims:
            shim.write_text("// locally edited, marker stripped\n")
        manifest = self.root / "ai-specs/ai-specs.toml"
        manifest.write_text(manifest.read_text().split("[recipes.")[0])
        self.sync()
        self.assertEqual([str(p.relative_to(self.root)) for p in shims if p.exists()], [])
        self.assertNotIn("[hooks]", lock.read_text())

    def test_claude_pretooluse_managed_block(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "claude")
        settings = json.loads((self.root / ".claude" / "settings.json").read_text())
        pre = settings["hooks"]["PreToolUse"]
        # Find the managed entry for this hook
        entry = next(e for e in pre if e.get("matcher") == "Bash")
        cmd = entry["hooks"][0]["command"]
        self.assertIn(self.script_path, cmd)
        self.assertEqual(entry["hooks"][0]["type"], "command")

    def test_cursor_hooks_json(self):
        # Use a Bash-matcher hook so Cursor has a real target (beforeShellExecution)
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "cursor")
        wrapper = self.root / ".cursor" / "hooks" / f"{self.shim_base}.sh"
        self.assertTrue(wrapper.is_file(), "cursor wrapper should be generated")
        wtext = wrapper.read_text()
        self.assertIn("GENERATED by ai-specs", wtext)
        self.assertIn('"permission":"deny"', wtext.replace(" ", ""))
        hooks_json = json.loads((self.root / ".cursor" / "hooks.json").read_text())
        self.assertIn("beforeShellExecution", json.dumps(hooks_json))

    def test_opencode_plugin_shim(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "opencode")
        plugin = self.root / ".opencode" / "plugin" / f"{self.shim_base}.ts"
        self.assertTrue(plugin.is_file(), "opencode plugin should be generated")
        text = plugin.read_text()
        self.assertIn("// GENERATED by ai-specs", text)
        self.assertIn("tool.execute.before", text)
        self.assertIn("throw", text)
        self.assertIn(self.script_path, text)
        # OpenCode tool ids may be lowercase while recipe matchers use
        # Claude-style names — same case-insensitive contract as pi/omp.
        self.assertIn('new RegExp(`^(?:${MATCHER})$`, "i")', text)

    def test_pi_extension_shim(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "pi")
        ext = self.root / ".pi" / "extensions" / f"{self.shim_base}.ts"
        self.assertTrue(ext.is_file(), "pi extension should be generated")
        text = ext.read_text()
        self.assertIn("// GENERATED by ai-specs", text)
        self.assertIn("@earendil-works/pi-coding-agent", text)
        self.assertIn("ExtensionAPI", text)
        self.assertIn('pi.on("tool_call"', text)
        self.assertIn("block: true", text)
        # Pi exposes the tool name as camelCase `toolName` (verified June 2026);
        # reading only snake_case would make the matcher never fire. Lock it in.
        self.assertIn("call?.toolName", text)
        # pi/omp tool names are lowercase (write, edit) while matchers use
        # Claude-style names (Write, Edit) — the regex must be case-insensitive
        # (live-verified June 2026: case-sensitive matcher never fired in omp).
        self.assertIn('new RegExp(`^(?:${MATCHER})$`, "i")', text)
        # pi/omp tool input uses `path`, not `file_path`; the event must
        # normalize so hook scripts reading file_path keep working.
        self.assertIn("rawInput.file_path ?? rawInput.path ?? rawInput.notebook_path", text)

    def test_omp_extension_shim(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "omp")
        ext = self.root / ".omp" / "extensions" / f"{self.shim_base}.ts"
        self.assertTrue(ext.is_file(), "omp extension should be generated")
        text = ext.read_text()
        self.assertIn("// GENERATED by ai-specs", text)
        self.assertIn("@oh-my-pi/pi-coding-agent", text)
        self.assertIn("ExtensionAPI", text)
        self.assertIn('pi.on("tool_call"', text)
        self.assertIn("block: true", text)
        # omp exposes the tool name as camelCase `toolName` (same as pi, verified June 2026).
        self.assertIn("call?.toolName", text)
        # Live-verified June 2026 in omp: tool names arrive lowercase (write,
        # bash, read) and tool input uses `path`. Case-insensitive matcher +
        # input normalization are required for the gate to ever fire.
        self.assertIn('new RegExp(`^(?:${MATCHER})$`, "i")', text)
        self.assertIn("rawInput.file_path ?? rawInput.path ?? rawInput.notebook_path", text)

    def test_unsupported_event_warns_and_skips(self):
        # `stop` has no opencode mapping -> warn + skip for opencode, present for claude
        result = self.seed_and_sync(RECIPE_STOP, "stopper", "opencode", "claude",
                                    event="stop", matcher="")
        # No plugin file emitted for opencode
        self.assertFalse((self.root / ".opencode" / "plugin" / f"{self.shim_base}.ts").exists())
        joined = result.stderr
        self.assertIn(RECIPE_STOP, joined)
        self.assertIn("stopper", joined)
        self.assertIn("stop", joined)
        self.assertIn("opencode", joined)
        # claude supports stop
        settings = json.loads((self.root / ".claude" / "settings.json").read_text())
        self.assertIn("Stop", settings["hooks"])

    def test_cursor_no_prefilewrite_target_warns_and_skips(self):
        result = self.seed_and_sync(RECIPE_FILEWRITE, "worktree-gate", "cursor",
                                    matcher="Edit|Write|MultiEdit|NotebookEdit")
        wrapper = self.root / ".cursor" / "hooks" / f"{self.shim_base}.sh"
        self.assertFalse(wrapper.exists(), "cursor must skip file-write gates")
        joined = result.stderr.lower()
        self.assertIn("cursor", joined)
        self.assertIn("worktree-gate", result.stderr)

    def test_render_idempotent(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "claude")
        settings_path = self.root / ".claude" / "settings.json"
        first = settings_path.read_bytes()
        self.sync()
        second = settings_path.read_bytes()
        self.assertEqual(first, second, "second render must be byte-identical")

    def test_user_hook_preserved(self):
        self._hook_id = "shell-gate"
        self.seed_recipe(RECIPE_SHELL, "shell-gate")
        claude_dir = self.root / ".claude"
        claude_dir.mkdir(parents=True)
        (claude_dir / "settings.json").write_text(json.dumps({
            "model": "opus",
            "hooks": {
                "PreToolUse": [
                    {"matcher": "UserOwned", "hooks": [
                        {"type": "command", "command": "/usr/bin/true"}
                    ]}
                ]
            }
        }, indent=2))
        self.write_manifest("claude")
        self.sync()
        settings = json.loads((claude_dir / "settings.json").read_text())
        # Sibling user key preserved
        self.assertEqual(settings["model"], "opus")
        matchers = [e.get("matcher") for e in settings["hooks"]["PreToolUse"]]
        self.assertIn("UserOwned", matchers)  # user hook preserved
        self.assertIn("Bash", matchers)       # managed hook added


class WorkspaceContextProcessBoundaryTests(_HookFixtureMixin):
    """Deterministic Node process-boundary tests for generated adapters.

    Render the generated adapter for a real hook through ``bin/ai-specs sync``,
    relocate it into a temporary installation (as sync would materialize it),
    and execute it through the Node harness with a recording ``spawnSync``
    double. The assertions observe the actual process boundary: executable
    path, event cwd, options cwd, and fail-open behavior — never generated
    source text.

    Covers OpenCode directory normalization and child cwd (1.2), Pi/OMP
    module-location launcher paths with process-cwd events (1.3), and the
    preserved Claude/Cursor project-directory wiring (1.4).
    """

    RUNTIME_DIRS = {
        "opencode": Path(".opencode") / "plugin",
        "pi": Path(".pi") / "extensions",
        "omp": Path(".omp") / "extensions",
    }

    def setUp(self):
        super().setUp()
        self.assertTrue(NODE, "node is required for the process-boundary harness")
        self.assertTrue(NODE_HARNESS.is_file(),
                        f"missing harness fixture: {NODE_HARNESS.relative_to(ROOT)}")
        self.unrelated = self.base / "unrelated-cwd"
        self.unrelated.mkdir()
        # Node's chdir resolves symlinks (macOS /var -> /private/var), so the
        # process-cwd value the adapter sees is the real path.
        self.unrelated_real = str(self.unrelated.resolve())
        self.valid_dir = self.base / "valid-dir"
        self.valid_dir.mkdir()

    # --- helpers -----------------------------------------------------------

    def _render(self, agent: str) -> Path:
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", agent)
        return self.root / self.RUNTIME_DIRS[agent] / f"{self.shim_base}.ts"

    def _install(self) -> tuple[Path, Path]:
        """Temporary materialized installation: launcher + relocated adapter."""
        install = self.base / "install"
        launcher = install / self.script_path
        launcher.parent.mkdir(parents=True, exist_ok=True)
        launcher.write_text(HOOK_SCRIPT_BODY)
        launcher.chmod(0o755)
        return install, launcher

    def _relocate(self, agent: str, generated: Path, install: Path) -> Path:
        target = install / self.RUNTIME_DIRS[agent] / generated.name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(generated, target)
        return target

    def _run_harness(self, kind: str, plugin: Path, *,
                     directory=None, directory_json=None, tool=None,
                     input_json=None, args_json=None, status=None,
                     stderr=None, error=None, throw=None) -> dict:
        cmd = [NODE, str(NODE_HARNESS), "--kind", kind,
               "--plugin", str(plugin), "--cwd", str(self.unrelated)]
        if directory is not None:
            cmd += ["--directory", str(directory)]
        if directory_json is not None:
            cmd += ["--directory-json", json.dumps(directory_json)]
        if tool is not None:
            cmd += ["--tool", tool]
        if input_json is not None:
            cmd += ["--input-json", json.dumps(input_json)]
        if args_json is not None:
            cmd += ["--args-json", json.dumps(args_json)]
        if status is not None:
            cmd += ["--status", str(status)]
        if stderr is not None:
            cmd += ["--stderr", stderr]
        if error is not None:
            cmd += ["--error", error]
        if throw is not None:
            cmd += ["--throw", throw]
        proc = subprocess.run(cmd, capture_output=True, text=True, check=False,
                              input="")
        self.assertEqual(proc.returncode, 0,
                         f"harness failed: {proc.stderr}\nstdout: {proc.stdout}")
        return json.loads(proc.stdout)

    def _assert_single_record(self, res: dict) -> dict:
        self.assertEqual(len(res["records"]), 1, res)
        return res["records"][0]

    # --- OpenCode directory normalization and child cwd (1.2) --------------

    def test_opencode_valid_directory_trimmed_for_event_and_child_cwd(self):
        generated = self._render("opencode")
        install, launcher = self._install()
        relocated = self._relocate("opencode", generated, install)
        # Directory carries outer whitespace; the normalized value must drive
        # both the event cwd and the spawnSync child cwd.
        res = self._run_harness(
            "opencode", relocated,
            directory=f"  {self.valid_dir}  ",
            args_json={"file_path": "x.py"},
        )
        rec = self._assert_single_record(res)
        self.assertEqual(rec["command"], str(launcher.resolve()),
                         "launcher path must be module-derived and absolute")
        self.assertEqual(rec["input"]["cwd"], str(self.valid_dir),
                         "event cwd must be the outer-trimmed directory")
        self.assertEqual(rec["optionsCwd"], str(self.valid_dir),
                         "child spawnSync cwd must equal the trimmed directory")
        self.assertFalse(res["outcome"]["threw"], res)

    def test_opencode_unrelated_process_cwd_still_finds_launcher(self):
        # Harness runs from unrelated-cwd; the valid directory is the event
        # target. The launcher must come from module location, not $PWD.
        generated = self._render("opencode")
        install, launcher = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated,
            directory=str(self.valid_dir),
            args_json={"file_path": "x.py"},
        )
        rec = self._assert_single_record(res)
        self.assertEqual(rec["command"], str(launcher.resolve()))
        self.assertEqual(rec["optionsCwd"], str(self.valid_dir))

    def test_opencode_absent_directory_falls_back_to_process_cwd(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness("opencode", relocated, args_json={"file_path": "x.py"})
        rec = self._assert_single_record(res)
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real)
        self.assertEqual(rec["optionsCwd"], self.unrelated_real)

    def test_opencode_non_string_directory_falls_back(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated, directory_json=123,
            args_json={"file_path": "x.py"})
        rec = self._assert_single_record(res)
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real)
        self.assertEqual(rec["optionsCwd"], self.unrelated_real)

    def test_opencode_whitespace_only_directory_falls_back(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated, directory="   ",
            args_json={"file_path": "x.py"})
        rec = self._assert_single_record(res)
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real)
        self.assertEqual(rec["optionsCwd"], self.unrelated_real)

    def test_opencode_relative_directory_falls_back(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated, directory="relative/path",
            args_json={"file_path": "x.py"})
        rec = self._assert_single_record(res)
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real)
        self.assertEqual(rec["optionsCwd"], self.unrelated_real)

    def test_opencode_nonexistent_directory_falls_back(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated,
            directory=str(self.base / "does-not-exist"),
            args_json={"file_path": "x.py"})
        rec = self._assert_single_record(res)
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real)
        self.assertEqual(rec["optionsCwd"], self.unrelated_real)

    def test_opencode_non_directory_value_falls_back(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        plain_file = self.base / "plain-file.txt"
        plain_file.write_text("x")
        res = self._run_harness(
            "opencode", relocated, directory=str(plain_file),
            args_json={"file_path": "x.py"})
        rec = self._assert_single_record(res)
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real)
        self.assertEqual(rec["optionsCwd"], self.unrelated_real)

    def test_opencode_status_two_is_the_only_block(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated, directory=str(self.valid_dir),
            status=2, stderr="blocked by fixture",
            args_json={"file_path": "x.py"})
        self.assertTrue(res["outcome"]["threw"], res)
        self.assertIn("blocked by fixture", res["outcome"]["error"])

    def test_opencode_child_error_fails_open(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated, directory=str(self.valid_dir),
            error="spawn failed", args_json={"file_path": "x.py"})
        self.assertFalse(res["outcome"]["threw"],
                         "a child spawn error must fail open, not throw")

    def test_opencode_child_throw_fails_open(self):
        generated = self._render("opencode")
        install, _ = self._install()
        relocated = self._relocate("opencode", generated, install)
        res = self._run_harness(
            "opencode", relocated, directory=str(self.valid_dir),
            throw="child threw", args_json={"file_path": "x.py"})
        self.assertFalse(res["outcome"]["threw"],
                         "a thrown child-process exception must fail open")

    # --- Pi/OMP module-location launcher paths (1.3) -----------------------

    def _assert_process_cwd_event(self, res: dict, launcher: Path) -> None:
        rec = self._assert_single_record(res)
        self.assertEqual(rec["command"], str(launcher.resolve()),
                         "pi/omp launcher must be module-derived and absolute")
        self.assertEqual(rec["input"]["cwd"], self.unrelated_real,
                         "pi/omp event cwd must stay process.cwd()")
        self.assertEqual(rec["optionsCwd"], None,
                         "pi/omp child process must keep inheriting process cwd")
        self.assertFalse(res["outcome"]["block"], res)

    def test_pi_relocated_extension_finds_launcher_from_module_location(self):
        generated = self._render("pi")
        install, launcher = self._install()
        relocated = self._relocate("pi", generated, install)
        res = self._run_harness("pi", relocated, tool="bash",
                                input_json={"path": "x.py"})
        self._assert_process_cwd_event(res, launcher)
        self.assertEqual(res["outcome"]["block"], False)
        # No workspace root may be claimed: the event cwd must not be the
        # installation root.
        self.assertNotEqual(res["records"][0]["input"]["cwd"], str(install))

    def test_omp_relocated_extension_finds_launcher_from_module_location(self):
        generated = self._render("omp")
        install, launcher = self._install()
        relocated = self._relocate("omp", generated, install)
        res = self._run_harness("omp", relocated, tool="bash",
                                input_json={"path": "x.py"})
        self._assert_process_cwd_event(res, launcher)
        self.assertNotEqual(res["records"][0]["input"]["cwd"], str(install))

    # --- preserved Claude/Cursor wiring (1.4) ------------------------------

    def test_claude_script_keeps_project_dir_variable(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "claude")
        settings = json.loads((self.root / ".claude" / "settings.json").read_text())
        entry = next(e for e in settings["hooks"]["PreToolUse"]
                     if e.get("matcher") == "Bash")
        cmd = entry["hooks"][0]["command"]
        self.assertIn("$CLAUDE_PROJECT_DIR/", cmd)
        self.assertIn(self.script_path, cmd)

    def test_cursor_wrapper_keeps_project_dir_variable(self):
        self.seed_and_sync(RECIPE_SHELL, "shell-gate", "cursor")
        wrapper = self.root / ".cursor" / "hooks" / f"{self.shim_base}.sh"
        self.assertIn("$CURSOR_PROJECT_DIR/", wrapper.read_text())

    def test_cursor_filewrite_limitation_preserved(self):
        # The adapter change must not replace Cursor's missing pre-file-write
        # hook: a file-write matcher still warns and skips for cursor.
        result = self.seed_and_sync(RECIPE_FILEWRITE, "worktree-gate", "cursor",
                                    matcher="Edit|Write|MultiEdit|NotebookEdit")
        wrapper = self.root / ".cursor" / "hooks" / f"{self.shim_base}.sh"
        self.assertFalse(wrapper.exists())
        self.assertIn("no pre-file-write hook", result.stderr)


if __name__ == "__main__":
    unittest.main()
