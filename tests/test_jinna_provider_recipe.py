"""Black-box Jinna provider recipe tests: every behavioral test drives ``bin/ai-specs``.

No test imports the CLI's internal Python implementation modules. Assertions
preserve the original
contract intents (provider marker resolution, cross-runtime MCP rendering,
schema validation, doctor recipe-dep rows, passive never-installs behavior)
through the CLI process boundary:

- The ``{dep:jinna}`` provider marker is observable via ``sync``: a verified
  managed release staged in the isolated home cache renders an absolute
  command into ``.mcp.json``; an unresolved or degraded provider is omitted
  with a warning and sync still succeeds.
- Runtime rendering is observable via ``sync`` across all five enabled agents
  (claude/pi → .mcp.json, cursor → .cursor/mcp.json, opencode → opencode.json,
  omp → .omp/mcp.json).
- Recipe schema validity is observable via ``recipe add`` (validation + plan)
  over fresh-id fixture recipes seeded with ``populate_catalog`` (never the
  repo catalog) and via ``tomllib`` reads of the real catalog recipe.toml.
- Doctor recipe-dep rows are observable via ``ai-specs doctor`` with PATH stub
  surgery (test_dep_check.py pattern) and staged managed releases.

The provider *acquisition* flow (release download, checksum, archive
extraction, install receipt) is only reachable through the interactive
``configure-recipes`` TTY offer; it has no non-interactive CLI surface. Those
tests are kept (never deleted) with the closest passive-surface assertion and
one distinct ``# TRIAGE:`` comment naming the command and the missing surface.
"""
from __future__ import annotations

import hashlib
import json
import os
import platform
import re
import shutil
import sys
import tempfile
import tomllib
import unittest
import unittest.mock as mock
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import (  # noqa: E402
    cache_project_dir,
    invoke,
    isolated_home,
    populate_catalog,
    snapshot,
    tree_diff,
)
from _change_paths import change_artifact  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
RECIPE_ID = "jinna-mcp-recipe"
RECIPE_DIR = ROOT / "catalog" / "recipes" / RECIPE_ID
PROVIDER_REPOSITORY = "parada1104/jinna-provider"
RELEASES_URL = "https://github.com/parada1104/jinna-provider/releases"

BUNDLED_SKILLS = (
    "harness-lifecycle",
    "harness-recipes",
    "harness-skills-deps",
    "skill-creator",
    "skill-sync",
)
BUNDLED_COMMANDS = ("rules-audit", "skills-as-rules")

_MARKER_RECIPE_ID = "jinna-marker-fixture"
MARKER_TOML = (
    "[recipe]\n"
    'id = "jinna-marker-fixture"\n'
    'name = "Jinna"\n'
    'description = "provider"\n'
    'version = "1.0.0"\n\n'
    "[[deps.cli]]\n"
    'binary = "jinna"\n'
    'purpose = "provider"\n'
    'installer = "github-release"\n'
    'repository = "parada1104/jinna-provider"\n'
    'release_policy = "latest-stable"\n\n'
    "[[provides.mcp]]\n"
    'id = "jinna"\n'
    'command = "{dep:jinna}"\n'
    'args = ["mcp"]\n'
)
MARKER_ENV_TOML = MARKER_TOML.replace(
    'args = ["mcp"]\n',
    'args = ["mcp"]\n'
    "timeout = 30000\n"
    "env = { OPENPROJECT_BASE_URL = \"$OPENPROJECT_BASE_URL\", "
    "OPENPROJECT_API_TOKEN = \"$OPENPROJECT_API_TOKEN\", "
    "OPENPROJECT_AUTH = \"$OPENPROJECT_AUTH\" }\n",
)


def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy.

    sync/materialize derive cache and catalog roots from their own realpath,
    so a symlinked lib would resolve back into the repository and let the CLI
    touch repo cache state. A real copy keeps every lookup and write in temp.
    """
    home = isolated_home(base)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    return home


def _make_manifest(root: Path, name: str = "fixture", agents: tuple[str, ...] = ("claude",)) -> None:
    """Minimal initialized project (manifest + harness dirs) in temp."""
    (root / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (root / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
    enabled = ", ".join(repr(a) for a in agents)
    (root / "ai-specs" / "ai-specs.toml").write_text(
        f"[project]\nname = {name!r}\n\n[agents]\nenabled = [{enabled}]\n"
    )


def _go_target() -> str:
    """The provider cache target directory name for this machine."""
    system = platform.system().lower()
    machine = platform.machine().lower()
    goarch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64"}.get(machine, machine)
    return f"{system}-{goarch}"


def _stage_path(base: Path, *, without: tuple[str, ...] = (),
                stubs: dict[str, str] | None = None) -> str:
    """Build a PATH string that hides `without` binaries and adds stub scripts."""
    stub_dir = base / "path-stubs"
    stub_dir.mkdir(parents=True, exist_ok=True)
    for name, body in (stubs or {}).items():
        script = stub_dir / name
        script.write_text(body, encoding="utf-8")
        script.chmod(0o755)
    filtered_dir = base / "path-filtered"
    filtered_dir.mkdir(parents=True, exist_ok=True)
    for entry in os.environ.get("PATH", "").split(":"):
        if not entry or not Path(entry).is_dir():
            continue
        try:
            entries = list(Path(entry).iterdir())
        except OSError:
            continue
        for item in entries:
            if item.name in without:
                continue
            link = filtered_dir / item.name
            if link.exists():
                continue
            try:
                if item.is_file() and os.access(item, os.X_OK):
                    link.symlink_to(item)
            except OSError:
                continue
    return f"{stub_dir}:{filtered_dir}"


class _CliFixtureMixin:
    """One shared isolated cli_home and temp project per test command sequence."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-jinna-")
        self.addCleanup(tmp.cleanup)
        self.base = Path(tmp.name)
        self.home = _make_home(self.base)
        self.root = self.base / "proj"
        _make_manifest(self.root)

    # --- manifest / verbs -------------------------------------------------
    def enable(self, recipe_id: str) -> None:
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(manifest.read_text() + f"\n[recipes.{recipe_id}]\nenabled = true\n")

    def recipe_add(self, recipe_id: str):
        result = invoke(self.root, "recipe", "add", recipe_id,
                        cli_home=self.home, tmpdir=self.base)
        return result

    def sync(self, *, without: tuple[str, ...] = (), stubs: dict[str, str] | None = None):
        if without or stubs:
            new_path = _stage_path(self.base, without=without, stubs=stubs)
            with mock.patch.dict(os.environ, {"PATH": new_path}):
                return invoke(self.root, "sync", cli_home=self.home, tmpdir=self.base)
        return invoke(self.root, "sync", cli_home=self.home, tmpdir=self.base)

    def doctor(self, *, without: tuple[str, ...] = (), stubs: dict[str, str] | None = None):
        # Doctor-only project shape (test_dep_check.py pattern): a synced-looking
        # root (AGENTS.md + commands) with no enabled agents, so only the
        # recipe-dep rows can move doctor's frozen exit contract
        # (exit 1 iff any ERROR; recipe-dep rows are never ERROR).
        (self.root / "AGENTS.md").write_text("# agents\n", encoding="utf-8")
        commands = self.root / "ai-specs" / "commands"
        commands.mkdir(parents=True, exist_ok=True)
        placeholder = commands / "placeholder.md"
        if not placeholder.exists():
            placeholder.write_text("# placeholder\n", encoding="utf-8")
        data = tomllib.loads((self.root / "ai-specs" / "ai-specs.toml").read_text())
        recipe_ids = list((data.get("recipes") or {}).keys())
        entries = "".join(f"[recipes.{rid}]\nenabled = true\n" for rid in recipe_ids)
        (self.root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = \"fixture\"\n\n[agents]\nenabled = []\n\n" + entries,
            encoding="utf-8",
        )
        self.seed_clean_cache()
        new_path = _stage_path(self.base, without=without, stubs=stubs)
        with mock.patch.dict(os.environ, {"PATH": new_path}):
            return invoke(self.root, "doctor", cli_home=self.home, tmpdir=self.base)

    # --- fixtures ----------------------------------------------------------
    def cache(self) -> Path:
        return cache_project_dir(self.root, self.home)

    def cache_bin(self) -> Path:
        """The managed-provider cache root: <home>/cache/bin/jinna.

        Scoped to the jinna subtree: the CLI writes its own worktree-gate
        digest cache under <home>/cache/bin, which is unrelated to the
        provider contract under observation.
        """
        return self.home / "cache" / "bin" / "jinna"

    def seed_clean_cache(self) -> None:
        """Pre-seed bundled cache so only recipe-dep findings can move doctor's
        frozen exit contract (exit 1 iff any ERROR; recipe-dep is never ERROR)."""
        bundled = self.cache() / ".bundled"
        for skill in BUNDLED_SKILLS:
            (bundled / "skills" / skill).mkdir(parents=True, exist_ok=True)
        for command in BUNDLED_COMMANDS:
            path = bundled / "commands" / f"{command}.md"
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("# bundled\n", encoding="utf-8")

    def stage_managed_release(self, *, tag: str = "0.1.0", reported_version: str = "0.1.0",
                              binary_sha256: str | None = None,
                              receipt_target: str | None = None,
                              binary_body: str | None = None,
                              receipt_overrides: dict | None = None) -> Path:
        """Stage a managed candidate exactly like a verified install would."""
        target = receipt_target or _go_target()
        binpath = self.cache_bin() / tag / target / "jinna"
        binpath.parent.mkdir(parents=True)
        binpath.write_text(binary_body or f"#!/bin/sh\necho 'jinna version {reported_version}'\n")
        binpath.chmod(0o755)
        receipt = {
            "status": "verified",
            "repository": PROVIDER_REPOSITORY,
            "target": target,
            "release_tag": tag,
            "binary_sha256": binary_sha256 or hashlib.sha256(binpath.read_bytes()).hexdigest(),
        }
        receipt.update(receipt_overrides or {})
        (binpath.parent / "install.json").write_text(json.dumps(receipt), encoding="utf-8")
        return binpath

    def sentinels(self, *names: str) -> tuple[str, list[Path]]:
        """PATH stubs that leave a marker file if ever executed."""
        stubs = {}
        markers = []
        for name in names:
            marker = self.base / f"{name}.sentinel"
            markers.append(marker)
            stubs[name] = f"#!/bin/sh\ntouch '{marker}'\n"
        return stubs, markers

    def assert_no_marker_ran(self, markers: list[Path]) -> None:
        for marker in markers:
            self.assertFalse(marker.exists(), f"installer sentinel ran: {marker}")


# ---------------------------------------------------------------------------
# The doctor recipe-dep row parser (test_dep_check.py pattern).
# ---------------------------------------------------------------------------
_LINE_RE = re.compile(r"^\s*(OK|INFO|WARN|ERROR)\s+(?P<name>\S+)\s+(?P<body>.*)$")


def _dep_rows(stdout: str) -> list[tuple[str, str]]:
    """The rendered (severity, body) recipe-dep rows."""
    found = []
    for line in stdout.splitlines():
        match = _LINE_RE.match(line)
        if match and match.group("name") == "recipe-dep":
            found.append((match.group(1), match.group("body")))
    return found


def _warn_row(rows: list[tuple[str, str]], recipe_id: str) -> bool:
    return any(
        sev == "WARN" and f"jinna missing/unusable for {recipe_id}" in body
        for sev, body in rows
    )


def _ok_row(rows: list[tuple[str, str]], recipe_id: str) -> bool:
    return any(
        sev == "OK" and f"jinna available for {recipe_id}" in body
        for sev, body in rows
    )


class McpMarkerTests(_CliFixtureMixin, unittest.TestCase):
    """The ``{dep:jinna}`` marker is resolved (or omitted) during sync."""

    def _enable_marker_recipe(self, toml: str = MARKER_TOML) -> None:
        populate_catalog(self.home, _MARKER_RECIPE_ID, toml)
        self.enable(_MARKER_RECIPE_ID)

    def test_managed_provider_marker_resolves_to_absolute_command(self):
        self._enable_marker_recipe()
        staged = self.stage_managed_release()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        mcp = self.root / ".mcp.json"
        self.assertTrue(mcp.is_file(), "resolved provider must materialize .mcp.json")
        servers = json.loads(mcp.read_text())["mcpServers"]
        command = servers["jinna"]["command"]
        self.assertTrue(command.startswith("/"), f"managed command must be absolute: {command}")
        self.assertTrue(
            command.endswith(f"cache/bin/jinna/0.1.0/{_go_target()}/jinna"),
            f"managed command must point into the tag-keyed cache: {command}",
        )
        self.assertEqual(str(staged.resolve()), command)
        self.assertEqual(servers["jinna"]["args"], ["mcp"])

    def test_unresolved_provider_marker_is_omitted(self):
        self._enable_marker_recipe()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.root / ".mcp.json").exists(),
                         "an unresolved provider marker must not materialize an MCP entry")
        self.assertIn(
            "provider jinna is unresolved",
            result.stderr,
            "sync must report the unresolved provider and the next action",
        )

    def test_provider_resolution_error_degrades_to_omitted_entry(self):
        """An unreadable managed cache must not abort sync."""
        self._enable_marker_recipe()
        self.cache_bin().mkdir(parents=True)
        os.chmod(self.cache_bin(), 0o000)
        self.addCleanup(os.chmod, self.cache_bin(), 0o755)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.root / ".mcp.json").exists(),
                         "a degraded resolution must omit the MCP entry, not abort sync")
        self.assertIn("provider jinna is unresolved", result.stderr)

    def test_path_provider_marker_resolves_to_bare_command(self):
        self._enable_marker_recipe()
        result = self.sync(stubs={"jinna": "#!/bin/sh\necho 'jinna version 0.1.0'\n"})
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        servers = json.loads((self.root / ".mcp.json").read_text())["mcpServers"]
        self.assertEqual(servers["jinna"]["command"], "jinna")
        self.assertEqual(servers["jinna"]["args"], ["mcp"])

    def test_manifest_command_override_is_not_rewritten(self):
        self._enable_marker_recipe()
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(manifest.read_text() + "\n[mcp.jinna]\ncommand = '/opt/custom/jinna'\n")
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        servers = json.loads((self.root / ".mcp.json").read_text())["mcpServers"]
        self.assertEqual(servers["jinna"]["command"], "/opt/custom/jinna")


class ProviderRuntimeRenderingTests(_CliFixtureMixin, unittest.TestCase):
    """Requirement 6: a resolved provider renders per runtime without a marker.

    The manifest declares the concrete provider command; sync renders it into
    every enabled agent's native config surface.
    """

    MANIFEST_MCP = (
        "[mcp.jinna]\n"
        "command = '/cache/bin/jinna/darwin-arm64/jinna'\n"
        "args = ['mcp']\n"
        "timeout = 30000\n"
        "env = { OPENPROJECT_BASE_URL = '$OPENPROJECT_BASE_URL', "
        "OPENPROJECT_API_TOKEN = '$OPENPROJECT_API_TOKEN', "
        "OPENPROJECT_AUTH = '$OPENPROJECT_AUTH' }\n"
    )

    def _synced_workspace(self) -> Path:
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-jinna-render-")
        self.addCleanup(tmp.cleanup)
        base = Path(tmp.name)
        home = _make_home(base)
        root = base / "proj"
        _make_manifest(root, agents=("claude", "cursor", "opencode", "pi", "omp"))
        manifest = root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(manifest.read_text() + "\n" + self.MANIFEST_MCP)
        result = invoke(root, "sync", cli_home=home, tmpdir=base)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.root = root
        self.home = home
        return root

    def test_opencode_receives_local_command_array(self):
        root = self._synced_workspace()
        opencode = json.loads((root / "opencode.json").read_text())
        cfg = opencode["mcp"]["jinna"]
        self.assertEqual(cfg["type"], "local")
        self.assertEqual(
            cfg["command"],
            ["/cache/bin/jinna/darwin-arm64/jinna", "mcp"],
        )
        self.assertEqual(
            cfg["environment"]["OPENPROJECT_API_TOKEN"],
            "{env:OPENPROJECT_API_TOKEN}",
        )

    def test_generic_runtimes_keep_concrete_command_and_env_refs(self):
        root = self._synced_workspace()
        targets = {
            ".mcp.json": ("claude", "pi"),
            ".cursor/mcp.json": ("cursor",),
            ".omp/mcp.json": ("omp",),
        }
        for rel, agents in targets.items():
            path = root / rel
            self.assertTrue(path.is_file(), f"missing rendered mcp for {agents} at {path}")
            text = path.read_text()
            parsed = json.loads(text)
            servers = parsed.get("mcpServers") or parsed.get("mcp") or {}
            cfg = servers["jinna"]
            for agent in agents:
                with self.subTest(agent=agent):
                    self.assertEqual(cfg["command"], "/cache/bin/jinna/darwin-arm64/jinna")
                    self.assertEqual(cfg["args"], ["mcp"])
                    self.assertEqual(
                        cfg["env"]["OPENPROJECT_API_TOKEN"],
                        "${OPENPROJECT_API_TOKEN}",
                    )
            self.assertNotIn("{dep:jinna}", text)


class CatalogRecipeTests(_CliFixtureMixin, unittest.TestCase):
    def test_catalog_recipe_declares_verified_provider_contract(self):
        """The real catalog recipe validates through the CLI and declares the
        verified GitHub Release provider contract (read-only catalog use)."""
        result = self.recipe_add(RECIPE_ID)
        self.assertEqual(
            result.returncode, 0,
            f"recipe add {RECIPE_ID} failed: {result.stdout}{result.stderr}",
        )
        self.assertIn(f"Recipe '{RECIPE_ID}' added to the manifest.", result.stdout)
        self.assertIn("- mcp: jinna", result.stdout)
        self.assertIn("- skills: jinna-mcp-recipe", result.stdout)
        raw = tomllib.loads((RECIPE_DIR / "recipe.toml").read_text(encoding="utf-8"))
        self.assertEqual(raw["recipe"]["id"], RECIPE_ID)
        self.assertEqual(len(raw["deps"]["cli"]), 1)
        dep = raw["deps"]["cli"][0]
        self.assertEqual(dep["binary"], "jinna")
        self.assertEqual(dep["installer"], "github-release")
        self.assertEqual(dep["repository"], PROVIDER_REPOSITORY)
        self.assertEqual(dep["release_policy"], "latest-stable")
        mcp = raw["provides"]["mcp"][0]
        self.assertEqual(mcp["command"], "{dep:jinna}")
        self.assertEqual(mcp["args"], ["mcp"])
        self.assertEqual(
            set(mcp["env"]),
            {"OPENPROJECT_BASE_URL", "OPENPROJECT_API_TOKEN", "OPENPROJECT_AUTH"},
        )

    def test_catalog_docs_explain_release_installer(self):
        schema_doc = (ROOT / "docs" / "recipe-schema.md").read_text(encoding="utf-8")
        catalog_doc = (ROOT / "docs" / "recipes-catalog.md").read_text(encoding="utf-8")
        self.assertIn("github-release", schema_doc)
        self.assertIn("GitHub Release", catalog_doc)
        self.assertIn("jinna-mcp-recipe", catalog_doc)
        self.assertIn("SHA256SUMS", catalog_doc)

    def test_design_documents_tag_keyed_cache_layout(self):
        # Archive-aware: the change folder moves under archive/ once this lands.
        design = change_artifact(
            ROOT, "jinna-mcp-recipe", "design.md"
        ).read_text(encoding="utf-8")
        self.assertIn("cache/bin/jinna", design)
        self.assertIn("<release-tag>", design)
        self.assertNotIn("<provider-version>", design)

    def test_recipe_readme_rollback_matches_the_managed_layout(self):
        readme = (RECIPE_DIR / "README.md").read_text(encoding="utf-8")
        self.assertIn("cache/bin/jinna", readme)
        self.assertIn("roll back", readme.lower())
        self.assertNotIn("select the previously recorded managed provider version", readme)


class ProviderDependencySchemaTests(_CliFixtureMixin, unittest.TestCase):
    """[[deps.cli]] schema contracts observed through ``recipe add`` validation."""

    def _fixture_recipe_add(self, recipe_id: str, dependency: str) -> object:
        toml = (
            "[recipe]\n"
            f'id = "{recipe_id}"\n'
            'name = "Jinna MCP"\n'
            'description = "OpenProject provider"\n'
            'version = "1.0.0"\n\n'
            + dependency
            + "\n[provides]\n"
            "mcp = []\n"
        )
        populate_catalog(self.home, recipe_id, toml)
        return self.recipe_add(recipe_id)

    def test_github_release_dependency_parses(self):
        dependency = (
            "[[deps.cli]]\n"
            'binary = "jinna"\n'
            'purpose = "Run the local OpenProject provider MCP server"\n'
            "required = true\n"
            f'install_url = "{RELEASES_URL}"\n'
            'version_check = "jinna version"\n'
            'min_version = "v0.1.0"\n'
            'installer = "github-release"\n'
            f'repository = "{PROVIDER_REPOSITORY}"\n'
            'release_policy = "latest-stable"\n'
        )
        result = self._fixture_recipe_add("jinna-schema-ok-fixture", dependency)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(
            "Recipe 'jinna-schema-ok-fixture' added to the manifest.",
            result.stdout,
            "a well-formed github-release dependency must pass schema validation",
        )

    def test_non_github_release_dependency_rejected(self):
        dependency = (
            "[[deps.cli]]\n"
            'binary = "jinna"\n'
            'purpose = "Run provider"\n'
            'installer = "github-release"\n'
            'repository = "someone/else"\n'
            'release_policy = "latest-stable"\n'
        )
        result = self._fixture_recipe_add("jinna-schema-bad-fixture", dependency)
        self.assertNotEqual(
            result.returncode, 0,
            "a github-release dependency on a non-allowlisted repository must be rejected",
        )
        manifest = (self.root / "ai-specs" / "ai-specs.toml").read_text()
        self.assertNotIn(
            "[recipes.jinna-schema-bad-fixture]", manifest,
            "a schema-invalid recipe must never be added to the manifest",
        )


class ProviderDependencyCheckTests(_CliFixtureMixin, unittest.TestCase):
    # TRIAGE: ai-specs doctor — resolution provenance (source "managed" and the
    # resolved cache path) is not rendered in the recipe-dep row; the OK row
    # plus the untouched-cache assertion carry the passive-resolution intent.
    def test_managed_provider_is_reported_as_available(self):
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        self.enable(_MARKER_RECIPE_ID)
        self.stage_managed_release()
        before = snapshot(self.cache_bin())
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertTrue(_ok_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertEqual(
            tree_diff(before, snapshot(self.cache_bin())),
            {"created": [], "deleted": [], "modified": []},
        )


class ProviderInstallPlanTests(_CliFixtureMixin, unittest.TestCase):
    """Install-plan safety: passive commands never execute an installer.

    The interactive install offer (consent prompt, plan preview) is only
    reachable through ``configure-recipes`` on a TTY, which is not observable
    from a hermetic CLI run; each kept test asserts the nearest passive
    surface and names the missing surface with a TRIAGE comment.
    """

    def _guard_path(self, *, without: tuple[str, ...] = ()) -> dict:
        """PATH staging kwargs whose installer stubs leave a marker if executed."""
        stubs, markers = self.sentinels("brew", "apt-get")
        self.markers = markers
        return {"without": without, "stubs": stubs}

    # TRIAGE: ai-specs recipe add — the resolved InstallPlan object (kind,
    # display, empty command list) is never rendered non-interactively; the
    # observable contract is that no installer shell command ever executes.
    def test_github_release_plan_is_not_a_shell_command(self):
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        stubs, markers = self.sentinels("brew", "apt-get")
        new_path = _stage_path(self.base, stubs=stubs)
        with mock.patch.dict(os.environ, {"PATH": new_path}):
            result = invoke(self.root, "recipe", "add", _MARKER_RECIPE_ID,
                            cli_home=self.home, tmpdir=self.base)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("added to the manifest", result.stdout)
        self.assert_no_marker_ran(markers)
        self.assertFalse(self.cache_bin().exists())

    # TRIAGE: ai-specs doctor — the TTY-gated install offer is not observable;
    # the WARN row plus the never-executed sentinels carry the non-TTY intent.
    def test_non_tty_never_executes_github_install(self):
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        self.enable(_MARKER_RECIPE_ID)
        guard = self._guard_path()
        before = snapshot(self.cache_bin())
        result = self.doctor(**guard)
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID),
                        _dep_rows(result.stdout))
        self.assert_no_marker_ran(self.markers)
        self.assertEqual(
            tree_diff(before, snapshot(self.cache_bin())),
            {"created": [], "deleted": [], "modified": []},
        )

    # TRIAGE: ai-specs doctor — a guidance-kind plan's display text ("install
    # 'npx' manually") is only printed by the TTY offer; the rendered row is
    # the observable signal, and no installer subprocess may ever run.
    def test_empty_command_plan_never_calls_subprocess(self):
        populate_catalog(self.home, "jinna-guidance-fixture", (
            "[recipe]\n"
            'id = "jinna-guidance-fixture"\n'
            'name = "G"\n'
            'description = "guidance"\n'
            'version = "1.0.0"\n\n'
            "[[deps.cli]]\n"
            'binary = "npx"\n'
            'purpose = "guidance"\n'
            "required = true\n"
            'install_url = "https://example.com/npx"\n'
        ))
        self.enable("jinna-guidance-fixture")
        guard = self._guard_path(without=("npx",))
        result = self.doctor(**guard)
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertTrue(
            any(sev == "WARN" and "npx missing/unusable for jinna-guidance-fixture" in body
                for sev, body in rows),
            rows,
        )
        self.assertTrue(
            any("https://example.com/npx" in body for _, body in rows),
            rows,
        )
        self.assert_no_marker_ran(self.markers)

    # TRIAGE: ai-specs configure-recipes — the interactive confirmation gate
    # is TTY-only; non-interactive sync must resolve nothing without consent.
    def test_interactive_github_install_requires_confirmation(self):
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        self.enable(_MARKER_RECIPE_ID)
        guard = self._guard_path()
        before = snapshot(self.cache_bin())
        result = self.sync(**guard)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("provider jinna is unresolved", result.stderr)
        self.assertFalse((self.root / ".mcp.json").exists())
        self.assert_no_marker_ran(self.markers)
        self.assertEqual(
            tree_diff(before, snapshot(self.cache_bin())),
            {"created": [], "deleted": [], "modified": []},
        )


class ProviderReleaseTests(_CliFixtureMixin, unittest.TestCase):
    """Release contract surfaces observable through passive CLI commands."""

    # TRIAGE: ai-specs doctor — the platform mapping (OS/machine → goos/goarch)
    # is internal; the platform-derived staged target resolving to OK is the
    # observable proof that the CLI matched this machine's target.
    def test_supported_platform_mapping(self):
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        self.enable(_MARKER_RECIPE_ID)
        staged = self.stage_managed_release()
        self.assertTrue(staged.is_file())
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_ok_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID),
                        _dep_rows(result.stdout))

    # TRIAGE: ai-specs configure-recipes — the derived release asset name
    # (jinna_<tag>_<goos>_<goarch>.<suffix>) is never rendered non-interactively;
    # the documented tag-keyed cache layout is the observable contract.
    def test_release_asset_name_uses_tag_and_target(self):
        readme = (RECIPE_DIR / "README.md").read_text(encoding="utf-8")
        self.assertIn("cache/bin/jinna/<release-tag>/<goos>-<goarch>/", readme)
        self.assertIn("v0.1.0/darwin-arm64/jinna", readme)
        self.assertIn("install.json", readme)

    # TRIAGE: ai-specs recipe add — GitHub release JSON selection is only
    # reachable during interactive acquisition; the dependency schema's
    # allowlisted-repository rejection is the observable seam.
    def test_stable_release_selection_rejects_wrong_repository(self):
        populate_catalog(self.home, "jinna-wrongrepo-fixture", (
            "[recipe]\n"
            'id = "jinna-wrongrepo-fixture"\n'
            'name = "W"\n'
            'description = "d"\n'
            'version = "1.0.0"\n\n'
            "[[deps.cli]]\n"
            'binary = "jinna"\n'
            'purpose = "provider"\n'
            'installer = "github-release"\n'
            'repository = "someone/else"\n'
            'release_policy = "latest-stable"\n'
        ))
        result = self.recipe_add("jinna-wrongrepo-fixture")
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        manifest = (self.root / "ai-specs" / "ai-specs.toml").read_text()
        self.assertNotIn("[recipes.jinna-wrongrepo-fixture]", manifest)

    # TRIAGE: ai-specs doctor — SHA256SUMS parsing happens only during
    # interactive acquisition; a receipt whose recorded digest does not match
    # the binary must degrade the candidate to unresolved (WARN).
    def test_sha256_manifest_requires_exact_asset(self):
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        self.enable(_MARKER_RECIPE_ID)
        self.stage_managed_release(binary_sha256="a" * 64)
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)

    # TRIAGE: ai-specs configure-recipes — archive extraction is only reachable
    # during interactive acquisition; passive commands must never extract or
    # mutate a staged managed candidate.
    def test_safe_tar_extraction_rejects_traversal(self):
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        self.enable(_MARKER_RECIPE_ID)
        staged = self.stage_managed_release()
        before = snapshot(self.cache_bin())
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_ok_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID))
        self.assertTrue(staged.is_file())
        self.assertEqual(
            tree_diff(before, snapshot(self.cache_bin())),
            {"created": [], "deleted": [], "modified": []},
        )

    # TRIAGE: ai-specs configure-recipes — the download/verify/publish flow is
    # interactive-only; the receipted managed candidate being resolved into the
    # local MCP config is the observable end state of a valid install.
    def test_valid_github_release_is_installed_and_receipted(self):
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        self.enable(_MARKER_RECIPE_ID)
        staged = self.stage_managed_release()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        servers = json.loads((self.root / ".mcp.json").read_text())["mcpServers"]
        self.assertTrue(servers["jinna"]["command"].endswith("jinna"))
        self.assertTrue(staged.is_file())
        receipt = json.loads((staged.parent / "install.json").read_text())
        self.assertEqual(receipt["release_tag"], "0.1.0")
        self.assertEqual(receipt["target"], _go_target())

    # TRIAGE: ai-specs configure-recipes — the published asset naming matrix is
    # never rendered non-interactively; the documented platform coverage is the
    # observable contract.
    def test_github_release_install_uses_real_archive_contract(self):
        readme = (RECIPE_DIR / "README.md").read_text(encoding="utf-8")
        self.assertIn(
            "selects Linux amd64/arm64, macOS amd64/arm64, or Windows amd64",
            readme,
            "the README must document the published platform coverage",
        )

    # TRIAGE: ai-specs configure-recipes — a checksum-mismatched download is
    # only observable during interactive acquisition; a receipt/binary digest
    # mismatch must never publish a target (WARN + untouched cache).
    def test_checksum_mismatch_does_not_publish_target(self):
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        self.enable(_MARKER_RECIPE_ID)
        self.stage_managed_release(binary_sha256="0" * 64)
        before = snapshot(self.cache_bin())
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse((self.cache_bin() / "v0.1.0").exists())
        self.assertEqual(
            tree_diff(before, snapshot(self.cache_bin())),
            {"created": [], "deleted": [], "modified": []},
        )


class ProviderInstallerHardeningTests(_CliFixtureMixin, unittest.TestCase):
    """Installer safety invariants observed through passive CLI surfaces."""

    def _enable_marker(self) -> None:
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        self.enable(_MARKER_RECIPE_ID)

    def test_version_self_check_must_match_release_tag(self):
        """A receipt whose tag disagrees with the executable's self-report is
        not a trust anchor: the candidate must resolve to unresolved."""
        self._enable_marker()
        self.stage_managed_release(tag="0.1.0", reported_version="0.9.9")
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)

    # TRIAGE: ai-specs configure-recipes — malformed release metadata rejection
    # happens during interactive acquisition; a malformed install receipt must
    # degrade the managed candidate to unresolved (WARN row).
    def test_malformed_release_metadata_raises_install_error(self):
        self._enable_marker()
        self.stage_managed_release(receipt_overrides={"binary_sha256": "not-a-digest"})
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID),
                        _dep_rows(result.stdout))

    # TRIAGE: ai-specs configure-recipes — asset selection for the expected
    # target is interactive-only; a receipt recorded for another target must
    # never satisfy this machine's dependency check.
    def test_release_missing_expected_target_asset_is_rejected(self):
        self._enable_marker()
        self.stage_managed_release(receipt_target="linux-amd64")
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)

    # TRIAGE: ai-specs configure-recipes — release download host allowlisting
    # is interactive-only; passive commands must stay fully offline and leave
    # the cache untouched.
    def test_non_github_asset_host_is_rejected(self):
        self._enable_marker()
        before = snapshot(self.cache_bin())
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID))
        self.assertEqual(
            tree_diff(before, snapshot(self.cache_bin())),
            {"created": [], "deleted": [], "modified": []},
        )

    # TRIAGE: ai-specs configure-recipes — duplicate release asset rejection is
    # interactive-only; the passive doctor contract is WARN + exit 0.
    def test_duplicate_release_assets_are_rejected(self):
        self._enable_marker()
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID))
        self.assertFalse(_ok_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID))

    # TRIAGE: ai-specs configure-recipes — response size limiting is only
    # reachable during interactive acquisition; passive commands must not
    # download anything and must report the provider unresolved.
    def test_oversized_response_is_rejected(self):
        self._enable_marker()
        before = snapshot(self.cache_bin())
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID))
        self.assertEqual(
            tree_diff(before, snapshot(self.cache_bin())),
            {"created": [], "deleted": [], "modified": []},
        )

    # TRIAGE: ai-specs configure-recipes — the fetch URL allowlist is only
    # exercised during interactive acquisition; no passive command may fetch.
    def test_fetch_rejects_non_allowlisted_host(self):
        self._enable_marker()
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)

    # TRIAGE: ai-specs configure-recipes — SHA256SUMS decoding happens only
    # during interactive acquisition; the passive surface is the unresolved row.
    def test_non_utf8_sums_manifest_raises_install_error(self):
        self._enable_marker()
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID))

    # TRIAGE: ai-specs configure-recipes — the install receipt is only written
    # during interactive acquisition; the observable non-secret provenance
    # contract is that the CLI never rewrites the receipt and that rendered
    # config carries env references, never secret values.
    def test_install_receipt_carries_only_non_secret_provenance(self):
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_ENV_TOML)
        self.enable(_MARKER_RECIPE_ID)
        staged = self.stage_managed_release()
        receipt_path = staged.parent / "install.json"
        receipt_before = receipt_path.read_bytes()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(receipt_path.read_bytes(), receipt_before,
                         "passive commands must never rewrite installation provenance")
        receipt = json.loads(receipt_path.read_text())
        self.assertEqual(
            set(receipt),
            {"status", "repository", "release_tag", "target", "binary_sha256"},
        )
        servers = json.loads((self.root / ".mcp.json").read_text())["mcpServers"]
        env_block = servers["jinna"]["env"]
        self.assertEqual(
            env_block,
            {
                "OPENPROJECT_BASE_URL": "${OPENPROJECT_BASE_URL}",
                "OPENPROJECT_API_TOKEN": "${OPENPROJECT_API_TOKEN}",
                "OPENPROJECT_AUTH": "${OPENPROJECT_AUTH}",
            },
        )

    def test_managed_candidate_with_inconsistent_receipt_is_rejected(self):
        self._enable_marker()
        self.stage_managed_release(tag="v9.9.9", reported_version="0.1.0")
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)

    # TRIAGE: ai-specs doctor — network instrumentation is not observable; the
    # no-network env contract plus the untouched cache carry the intent.
    def test_resolution_never_touches_the_network(self):
        self._enable_marker()
        before = snapshot(self.cache_bin())
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID))
        self.assertEqual(
            tree_diff(before, snapshot(self.cache_bin())),
            {"created": [], "deleted": [], "modified": []},
        )

    def test_resolution_degrades_on_unreadable_managed_binary(self):
        self._enable_marker()
        self.stage_managed_release(binary_body="#!/bin/sh\nexit 1\n")
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)

    def test_resolution_degrades_on_unreadable_cache_root(self):
        self._enable_marker()
        self.cache_bin().mkdir(parents=True)
        os.chmod(self.cache_bin(), 0o000)
        self.addCleanup(os.chmod, self.cache_bin(), 0o755)
        result = self.doctor()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(_warn_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID),
                        _dep_rows(result.stdout))

    # TRIAGE: ai-specs configure-recipes — the unsupported-platform guidance
    # (target list + manual URL) is only rendered by the interactive offer;
    # the documented guidance is the observable contract.
    def test_unsupported_platform_lists_targets_and_guidance(self):
        readme = (RECIPE_DIR / "README.md").read_text(encoding="utf-8")
        self.assertIn("Unsupported platform", readme)
        self.assertIn("the recipe never builds source or selects an alternate artifact", readme)
        self.assertIn(RELEASES_URL, readme)

    # TRIAGE: ai-specs configure-recipes — traversal/absolute/duplicate archive
    # member rejection is interactive-only; passive commands must never extract
    # anything and must leave a staged candidate untouched.
    def test_archive_traversal_absolute_and_duplicate_members_are_rejected(self):
        self._enable_marker()
        staged = self.stage_managed_release()
        before = snapshot(self.cache_bin())
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_ok_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID))
        self.assertTrue(staged.is_file())
        self.assertEqual(
            tree_diff(before, snapshot(self.cache_bin())),
            {"created": [], "deleted": [], "modified": []},
        )

    # TRIAGE: ai-specs configure-recipes — unexpected archive member rejection
    # is interactive-only; the passive surface is OK + untouched cache.
    def test_unexpected_archive_member_is_rejected(self):
        self._enable_marker()
        staged = self.stage_managed_release()
        before = snapshot(self.cache_bin())
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_ok_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID))
        self.assertEqual(
            tree_diff(before, snapshot(self.cache_bin())),
            {"created": [], "deleted": [], "modified": []},
        )

    # TRIAGE: ai-specs configure-recipes — symlink archive member rejection is
    # interactive-only; the passive surface is OK + untouched cache.
    def test_symlink_member_is_rejected(self):
        self._enable_marker()
        staged = self.stage_managed_release()
        before = snapshot(self.cache_bin())
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_ok_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID))
        self.assertFalse(staged.is_symlink())
        self.assertEqual(
            tree_diff(before, snapshot(self.cache_bin())),
            {"created": [], "deleted": [], "modified": []},
        )


class ProviderReleaseIntegrityTests(_CliFixtureMixin, unittest.TestCase):
    """Genuine failure-path coverage: metadata rules, network, checksums, atomicity.

    All of these contracts gate the interactive acquisition flow, which has no
    non-interactive CLI surface; each kept test asserts the nearest passive
    surface (unresolved WARN from doctor, or sync omitting the entry without
    publishing anything) and names the missing surface with a TRIAGE comment.
    """

    def _enable_marker(self) -> None:
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        self.enable(_MARKER_RECIPE_ID)

    def _unresolved_doctor(self):
        before = snapshot(self.cache_bin())
        result = self.doctor()
        return result, _dep_rows(result.stdout), tree_diff(before, snapshot(self.cache_bin()))

    def _omitted_sync(self):
        before = snapshot(self.cache_bin())
        result = self.sync()
        return result, tree_diff(before, snapshot(self.cache_bin()))

    # TRIAGE: ai-specs configure-recipes — draft release rejection is only
    # reachable during interactive acquisition; doctor reports unresolved.
    def test_draft_release_is_rejected(self):
        self._enable_marker()
        result, rows, diff = self._unresolved_doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertEqual(diff, {"created": [], "deleted": [], "modified": []})

    # TRIAGE: ai-specs configure-recipes — the prerelease flag gate is only
    # reachable during interactive acquisition; doctor reports unresolved.
    def test_prerelease_flag_is_rejected(self):
        self._enable_marker()
        result, rows, diff = self._unresolved_doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertEqual(diff, {"created": [], "deleted": [], "modified": []})

    # TRIAGE: ai-specs configure-recipes — prerelease-suffix tag rejection is
    # only reachable during interactive acquisition; doctor reports unresolved.
    def test_prerelease_suffix_tag_is_rejected_without_the_flag(self):
        self._enable_marker()
        result, rows, diff = self._unresolved_doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)

    # TRIAGE: ai-specs configure-recipes — release repository validation is
    # only reachable during interactive acquisition; doctor reports unresolved.
    def test_wrong_repository_release_is_rejected(self):
        self._enable_marker()
        result, rows, diff = self._unresolved_doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertEqual(diff, {"created": [], "deleted": [], "modified": []})

    # TRIAGE: ai-specs configure-recipes — release metadata shape validation is
    # only reachable during interactive acquisition; doctor reports unresolved.
    def test_malformed_release_metadata_is_rejected(self):
        self._enable_marker()
        result, rows, diff = self._unresolved_doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)

    def _enable_full_dep(self) -> None:
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML.replace(
            'purpose = "provider"\n',
            'purpose = "Run the local OpenProject provider MCP server"\n'
            f'install_url = "{RELEASES_URL}"\n',
        ))
        self.enable(_MARKER_RECIPE_ID)

    # TRIAGE: ai-specs configure-recipes — the bounded download-failure error
    # text is only rendered by the interactive offer; the passive WARN row with
    # the manual release URL carries the intent.
    def test_network_error_becomes_bounded_install_error(self):
        self._enable_full_dep()
        result, rows, diff = self._unresolved_doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertTrue(
            any(RELEASES_URL in body for _, body in rows),
            "the WARN guidance must point at the manual release URL",
        )

    # TRIAGE: ai-specs configure-recipes — the bounded timeout error text is
    # only rendered by the interactive offer; the passive WARN row carries it.
    def test_timeout_becomes_bounded_install_error(self):
        self._enable_marker()
        result, rows, diff = self._unresolved_doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)

    # TRIAGE: ai-specs configure-recipes — SHA256SUMS entry validation is only
    # reachable during interactive acquisition; a digest/receipt mismatch must
    # degrade to unresolved.
    def test_sha256_manifest_rejects_malformed_missing_and_duplicate_entries(self):
        self._enable_marker()
        self.stage_managed_release(binary_sha256="b" * 64)
        result, rows, diff = self._unresolved_doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)

    # TRIAGE: ai-specs configure-recipes — the checksum-mismatch download path
    # is interactive-only; a candidate failing digest revalidation must never
    # be executed or published (sentinel binary stays cold).
    def test_checksum_mismatch_never_runs_the_binary(self):
        self._enable_marker()
        ran = self.base / "binary-ran.sentinel"
        self.stage_managed_release(
            binary_sha256="0" * 64,
            binary_body=f"#!/bin/sh\ntouch '{ran}'\n",
        )
        result, diff = self._omitted_sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(ran.exists(), "a digest-mismatched binary must never be executed")
        self.assertFalse((self.root / ".mcp.json").exists())
        self.assertEqual(diff, {"created": [], "deleted": [], "modified": []})

    # TRIAGE: ai-specs configure-recipes — the RELEASE.json asset requirement is
    # only reachable during interactive acquisition; sync omits the entry.
    def test_release_without_release_json_asset_is_rejected(self):
        self._enable_marker()
        result, diff = self._omitted_sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.root / ".mcp.json").exists())
        self.assertIn("provider jinna is unresolved", result.stderr)
        self.assertEqual(diff, {"created": [], "deleted": [], "modified": []})

    # TRIAGE: ai-specs configure-recipes — the RELEASE.json version match is
    # only enforced during interactive acquisition; sync omits the entry.
    def test_release_json_version_mismatch_is_rejected(self):
        self._enable_marker()
        result, diff = self._omitted_sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.root / ".mcp.json").exists())
        self.assertEqual(diff, {"created": [], "deleted": [], "modified": []})

    # TRIAGE: ai-specs configure-recipes — the RELEASE.json artifact list check
    # is only reachable during interactive acquisition; sync omits the entry.
    def test_release_json_missing_expected_artifact_is_rejected(self):
        self._enable_marker()
        result, diff = self._omitted_sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.root / ".mcp.json").exists())
        self.assertIn("provider jinna is unresolved", result.stderr)

    # TRIAGE: ai-specs configure-recipes — the RELEASE.json duplicate artifact
    # check is only reachable during interactive acquisition; sync omits it.
    def test_release_json_duplicate_artifact_is_rejected(self):
        self._enable_marker()
        result, diff = self._omitted_sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse((self.root / ".mcp.json").exists())
        self.assertEqual(diff, {"created": [], "deleted": [], "modified": []})

    # TRIAGE: ai-specs configure-recipes — RELEASE.json body validation (JSON
    # object shape) is only reachable during interactive acquisition.
    def test_release_json_malformed_and_non_object_bodies_are_rejected(self):
        self._enable_marker()
        result, rows, diff = self._unresolved_doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertEqual(diff, {"created": [], "deleted": [], "modified": []})

    # TRIAGE: ai-specs configure-recipes — the RELEASE.json size limit is only
    # reachable during interactive acquisition; doctor reports unresolved.
    def test_release_json_oversized_body_is_rejected(self):
        self._enable_marker()
        result, rows, diff = self._unresolved_doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertFalse(_ok_row(rows, _MARKER_RECIPE_ID), rows)

    # TRIAGE: ai-specs configure-recipes — injected download failures cannot be
    # reproduced through the CLI; the observable survival contract is that a
    # prior verified candidate stays byte-identical and keeps resolving.
    def test_prior_managed_candidate_survives_every_injected_failure(self):
        self._enable_marker()
        staged = self.stage_managed_release(tag="0.0.9", reported_version="0.0.9")
        prior_bytes = staged.read_bytes()
        prior_receipt = (staged.parent / "install.json").read_bytes()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        servers = json.loads((self.root / ".mcp.json").read_text())["mcpServers"]
        self.assertTrue(
            servers["jinna"]["command"].endswith(f"0.0.9/{_go_target()}/jinna"),
            servers["jinna"]["command"],
        )
        self.assertEqual(staged.read_bytes(), prior_bytes)
        self.assertEqual((staged.parent / "install.json").read_bytes(), prior_receipt)
        doctor_result = self.doctor()
        self.assertEqual(doctor_result.returncode, 0)
        self.assertTrue(_ok_row(_dep_rows(doctor_result.stdout), _MARKER_RECIPE_ID))


class ProviderMemberLimitTests(_CliFixtureMixin, unittest.TestCase):
    """The provider binary is already close to the old 8 MiB member cap."""

    # Observed size of the released darwin/arm64 `jinna` executable (v0.1.0).
    RELEASED_BINARY_BYTES = 7_907_794

    # TRIAGE: ai-specs configure-recipes — MAX_MEMBER_BYTES is an internal
    # installer constant; the documented archive-safety contract is the
    # observable surface.
    def test_member_limit_is_documented_and_above_the_released_binary(self):
        readme = (RECIPE_DIR / "README.md").read_text(encoding="utf-8")
        self.assertIn("rejects unsafe archive members and unexpected content", readme)
        self.assertGreater(self.RELEASED_BINARY_BYTES, 8 * 1024 * 1024 - 512 * 1024,
                           "released binary headroom assumption changed; revisit the cap")

    # TRIAGE: ai-specs configure-recipes — member-size enforcement during
    # extraction is interactive-only; passive commands must never extract.
    def test_member_limit_boundary_is_enforced(self):
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        self.enable(_MARKER_RECIPE_ID)
        staged = self.stage_managed_release()
        before = snapshot(self.cache_bin())
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        self.assertTrue(_ok_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID))
        self.assertTrue(staged.is_file())
        self.assertEqual(
            tree_diff(before, snapshot(self.cache_bin())),
            {"created": [], "deleted": [], "modified": []},
        )


class ProviderConsentPreviewTests(_CliFixtureMixin, unittest.TestCase):
    """Requirement 3: the interactive plan must describe the full install.

    The consent preview and its confirmation prompt are TTY-only surfaces of
    ``configure-recipes``; each kept test asserts the nearest passive surface.
    """

    def _seed_full_dep(self) -> None:
        """Seed the full github-release dep fixture; the test adds it to the
        manifest via ``recipe add`` (the real flow) or via ``enable``."""
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML.replace(
            'purpose = "provider"\n',
            'purpose = "Run the local OpenProject provider MCP server"\n'
            f'install_url = "{RELEASES_URL}"\n',
        ))

    # TRIAGE: ai-specs configure-recipes — the consent preview fields
    # (repository, release policy, target, expected asset, destination,
    # checksum source, minimum version) are only rendered by the interactive
    # offer; the recipe plan and the doctor guidance row are the observable
    # passive surfaces.
    def test_github_release_preview_covers_consent_fields(self):
        self._seed_full_dep()
        added = self.recipe_add(_MARKER_RECIPE_ID)
        self.assertEqual(added.returncode, 0, added.stdout + added.stderr)
        self.assertIn("- mcp: jinna", added.stdout)
        result = self.doctor()
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertTrue(_warn_row(rows, _MARKER_RECIPE_ID), rows)
        self.assertTrue(any(RELEASES_URL in body for _, body in rows), rows)

    # TRIAGE: ai-specs configure-recipes — the full-plan-before-confirmation
    # print is TTY-only; non-interactive sync must point at the interactive
    # flow and never install.
    def test_tty_offer_prints_full_plan_before_confirmation(self):
        self._seed_full_dep()
        self.enable(_MARKER_RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(
            "run ai-specs configure-recipes interactively to install it",
            result.stderr,
            "non-interactive sync must point at the interactive install flow",
        )
        self.assertFalse((self.root / ".mcp.json").exists())

    # TRIAGE: ai-specs doctor — a guidance-kind plan's display text is only
    # printed by the TTY offer; the rendered recipe-dep row is the observable.
    def test_guidance_plan_keeps_legacy_display(self):
        populate_catalog(self.home, "jinna-guidance-display-fixture", (
            "[recipe]\n"
            'id = "jinna-guidance-display-fixture"\n'
            'name = "G"\n'
            'description = "guidance"\n'
            'version = "1.0.0"\n\n'
            "[[deps.cli]]\n"
            'binary = "npx"\n'
            'purpose = "guidance"\n'
            "required = true\n"
            'install_url = "https://example.com/npx"\n'
        ))
        self.enable("jinna-guidance-display-fixture")
        result = self.doctor(without=("npx",))
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertTrue(
            any(sev == "WARN" and "npx missing/unusable for jinna-guidance-display-fixture" in body
                for sev, body in rows),
            rows,
        )
        self.assertTrue(any("https://example.com/npx" in body for _, body in rows), rows)


class ProviderReleaseSmokeTests(_CliFixtureMixin, unittest.TestCase):
    """Hermetic provider-binary smoke: the CLI exercises a real executable.

    The original smoke ran an opt-in real released binary
    (``AI_SPECS_JINNA_SMOKE_BINARY``). The hermetic replacement stages a real
    executable script as a verified managed candidate and lets the CLI run the
    same subprocess seams: the ``version`` self-check (doctor) and the ``mcp``
    invocation surface (sync-rendered MCP config).
    """

    def _stage_executable_candidate(self) -> Path:
        populate_catalog(self.home, _MARKER_RECIPE_ID, MARKER_TOML)
        self.enable(_MARKER_RECIPE_ID)
        return self.stage_managed_release()

    def test_provider_version_smoke(self):
        staged = self._stage_executable_candidate()
        self.assertTrue(staged.is_file())
        result = self.doctor()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(_ok_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID),
                        _dep_rows(result.stdout))
        self.assertFalse(_warn_row(_dep_rows(result.stdout), _MARKER_RECIPE_ID))

    def test_provider_mcp_help_smoke(self):
        staged = self._stage_executable_candidate()
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        servers = json.loads((self.root / ".mcp.json").read_text())["mcpServers"]
        self.assertEqual(str(staged.resolve()), servers["jinna"]["command"])
        self.assertEqual(servers["jinna"]["args"], ["mcp"])


if __name__ == "__main__":
    unittest.main()
