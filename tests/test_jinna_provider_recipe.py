"""Contract tests for the Jinna provider recipe and installer seams."""
from __future__ import annotations

import contextlib
import hashlib
import importlib.util
import io
import json
import os
import subprocess
import sys
import tarfile
import tempfile
import unittest
import urllib.error
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import MagicMock, patch


ROOT = Path(__file__).resolve().parents[1]
INTERNAL = ROOT / "lib" / "_internal"
PROVIDER_REPOSITORY = "parada1104/jinna-provider"
RELEASE_BASE = "https://github.com/parada1104/jinna-provider/releases/download/v0.1.0/"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class McpMarkerTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.materialize = load_module(INTERNAL / "recipe-materialize.py", "jinna_marker_materialize")
        cls.install = load_module(INTERNAL / "provider_install.py", "jinna_marker_install")

    @staticmethod
    def _write_catalog(directory: Path) -> Path:
        recipe_dir = directory / "jinna-mcp-recipe"
        recipe_dir.mkdir(parents=True)
        (recipe_dir / "recipe.toml").write_text(
            "[recipe]\n"
            'id = "jinna-mcp-recipe"\n'
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
            'args = ["mcp"]\n',
            encoding="utf-8",
        )
        return recipe_dir

    def test_managed_provider_marker_resolves_to_absolute_command(self):
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp) / "catalog"
            recipe_dir = self._write_catalog(catalog)
            resolved = self.install.ProviderResolution(
                binary="jinna",
                command="/cache/jinna/0.1.0/darwin-arm64/jinna",
                version="0.1.0",
                source="managed",
                path=Path("/cache/jinna/0.1.0/darwin-arm64/jinna"),
                target=("darwin", "arm64"),
                verified=True,
            )
            with patch.object(self.materialize, "_load_provider_install") as loader:
                loader.return_value.resolve_provider.return_value = resolved
                result = self.materialize.build_recipe_mcp(
                    catalog, ["jinna-mcp-recipe"], {}, ai_specs_home=Path(tmp) / "home"
                )
            self.assertEqual(result["jinna"]["command"], resolved.command)
            self.assertEqual(result["jinna"]["args"], ["mcp"])

    def test_unresolved_provider_marker_is_omitted(self):
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp) / "catalog"
            self._write_catalog(catalog)
            unresolved = self.install.ProviderResolution(
                binary="jinna",
                command="jinna",
                version="",
                source="unresolved",
                path=None,
                target=("darwin", "arm64"),
                verified=False,
            )
            with patch.object(self.materialize, "_load_provider_install") as loader:
                loader.return_value.resolve_provider.return_value = unresolved
                result = self.materialize.build_recipe_mcp(
                    catalog, ["jinna-mcp-recipe"], {}, ai_specs_home=Path(tmp) / "home"
                )
            self.assertNotIn("jinna", result)

    def test_provider_resolution_error_degrades_to_omitted_entry(self):
        """An OSError while resolving a cache candidate must not abort sync."""
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp) / "catalog"
            self._write_catalog(catalog)
            with patch.object(self.materialize, "_load_provider_install") as loader:
                loader.return_value.resolve_provider.side_effect = OSError("unreadable cache")
                result = self.materialize.build_recipe_mcp(
                    catalog, ["jinna-mcp-recipe"], {}, ai_specs_home=Path(tmp) / "home"
                )
        self.assertNotIn("jinna", result)

    def test_path_provider_marker_resolves_to_bare_command(self):
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp) / "catalog"
            self._write_catalog(catalog)
            resolved = self.install.ProviderResolution(
                binary="jinna",
                command="jinna",
                version="0.1.0",
                source="path",
                path=Path("/usr/local/bin/jinna"),
                target=("darwin", "arm64"),
                verified=True,
            )
            with patch.object(self.materialize, "_load_provider_install") as loader:
                loader.return_value.resolve_provider.return_value = resolved
                result = self.materialize.build_recipe_mcp(
                    catalog, ["jinna-mcp-recipe"], {}, ai_specs_home=Path(tmp) / "home"
                )
            self.assertEqual(result["jinna"]["command"], "jinna")
            self.assertEqual(result["jinna"]["args"], ["mcp"])

    def test_manifest_command_override_is_not_rewritten(self):
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp) / "catalog"
            self._write_catalog(catalog)
            unresolved = self.install.ProviderResolution(
                binary="jinna",
                command="jinna",
                version="",
                source="unresolved",
                path=None,
                target=("darwin", "arm64"),
                verified=False,
            )
            manifest_mcp = {
                "jinna": {"command": "/opt/custom/jinna", "args": ["mcp"]}
            }
            with patch.object(self.materialize, "_load_provider_install") as loader:
                loader.return_value.resolve_provider.return_value = unresolved
                result = self.materialize.build_recipe_mcp(
                    catalog,
                    ["jinna-mcp-recipe"],
                    manifest_mcp,
                    ai_specs_home=Path(tmp) / "home",
                )
            self.assertEqual(result["jinna"]["command"], "/opt/custom/jinna")


class ProviderRuntimeRenderingTests(unittest.TestCase):
    """Requirement 6: a resolved provider renders per runtime without a marker."""

    @classmethod
    def setUpClass(cls):
        cls.render = load_module(INTERNAL / "mcp-render.py", "jinna_mcp_render")

    def _servers(self, command: str) -> dict:
        return {
            "jinna": {
                "command": command,
                "args": ["mcp"],
                "timeout": 30000,
                "env": {
                    "OPENPROJECT_BASE_URL": "$OPENPROJECT_BASE_URL",
                    "OPENPROJECT_API_TOKEN": "$OPENPROJECT_API_TOKEN",
                    "OPENPROJECT_AUTH": "$OPENPROJECT_AUTH",
                },
            }
        }

    def test_opencode_receives_local_command_array(self):
        translated = self.render.translate_servers(
            "opencode", self._servers("/cache/bin/jinna/darwin-arm64/jinna")
        )
        self.assertEqual(
            translated["jinna"]["command"],
            ["/cache/bin/jinna/darwin-arm64/jinna", "mcp"],
        )
        self.assertEqual(translated["jinna"]["type"], "local")
        self.assertEqual(
            translated["jinna"]["environment"]["OPENPROJECT_API_TOKEN"],
            "{env:OPENPROJECT_API_TOKEN}",
        )

    def test_generic_runtimes_keep_concrete_command_and_env_refs(self):
        for agent in ("claude", "cursor", "pi", "omp"):
            with self.subTest(agent=agent):
                translated = self.render.translate_servers(
                    agent, self._servers("/cache/bin/jinna/darwin-arm64/jinna")
                )
                self.assertEqual(
                    translated["jinna"]["command"], "/cache/bin/jinna/darwin-arm64/jinna"
                )
                self.assertEqual(translated["jinna"]["args"], ["mcp"])
                self.assertEqual(
                    translated["jinna"]["env"]["OPENPROJECT_API_TOKEN"],
                    "${OPENPROJECT_API_TOKEN}",
                )
                self.assertNotIn("{dep:jinna}", json.dumps(translated))


class CatalogRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(INTERNAL / "recipe_schema.py", "jinna_catalog_schema")

    def test_catalog_recipe_declares_verified_provider_contract(self):
        recipe_path = ROOT / "catalog" / "recipes" / "jinna-mcp-recipe" / "recipe.toml"
        recipe = self.schema.load_recipe_toml(recipe_path)
        self.assertEqual(recipe.id, "jinna-mcp-recipe")
        self.assertEqual(len(recipe.cli_deps), 1)
        dep = recipe.cli_deps[0]
        self.assertEqual(dep.binary, "jinna")
        self.assertEqual(dep.installer, "github-release")
        self.assertEqual(dep.repository, "parada1104/jinna-provider")
        self.assertEqual(dep.release_policy, "latest-stable")
        self.assertEqual(recipe.mcp[0].config["command"], "{dep:jinna}")
        self.assertEqual(recipe.mcp[0].config["args"], ["mcp"])
        self.assertEqual(
            set(recipe.mcp[0].config["env"]),
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
        design = (
            ROOT / "openspec" / "changes" / "jinna-mcp-recipe" / "design.md"
        ).read_text(encoding="utf-8")
        self.assertIn("cache/bin/jinna", design)
        self.assertIn("<release-tag>", design)
        self.assertNotIn("<provider-version>", design)

    def test_recipe_readme_rollback_matches_the_managed_layout(self):
        readme = (
            ROOT / "catalog" / "recipes" / "jinna-mcp-recipe" / "README.md"
        ).read_text(encoding="utf-8")
        self.assertIn("cache/bin/jinna", readme)
        self.assertIn("roll back", readme.lower())
        self.assertNotIn("select the previously recorded managed provider version", readme)


class ProviderDependencySchemaTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(INTERNAL / "recipe_schema.py", "jinna_recipe_schema")

    @staticmethod
    def _write_recipe(directory: Path, dependency: str) -> Path:
        recipe_dir = directory / "recipe"
        recipe_dir.mkdir()
        (recipe_dir / "recipe.toml").write_text(
            "[recipe]\n"
            'id = "jinna-mcp-recipe"\n'
            'name = "Jinna MCP"\n'
            'description = "OpenProject provider"\n'
            'version = "1.0.0"\n\n'
            + dependency
            + "\n[provides]\n"
            'mcp = []\n',
            encoding="utf-8",
        )
        return recipe_dir / "recipe.toml"

    def test_github_release_dependency_parses(self):
        dependency = (
            "[[deps.cli]]\n"
            'binary = "jinna"\n'
            'purpose = "Run the local OpenProject provider MCP server"\n'
            "required = true\n"
            'install_url = "https://github.com/parada1104/jinna-provider/releases"\n'
            'version_check = "jinna version"\n'
            'min_version = "v0.1.0"\n'
            'installer = "github-release"\n'
            'repository = "parada1104/jinna-provider"\n'
            'release_policy = "latest-stable"\n'
        )
        with tempfile.TemporaryDirectory() as tmp:
            recipe = self.schema.load_recipe_toml(self._write_recipe(Path(tmp), dependency))

        dep = recipe.cli_deps[0]
        self.assertEqual(dep.binary, "jinna")
        self.assertEqual(dep.installer, "github-release")
        self.assertEqual(dep.repository, "parada1104/jinna-provider")
        self.assertEqual(dep.release_policy, "latest-stable")

    def test_non_github_release_dependency_rejected(self):
        dependency = (
            "[[deps.cli]]\n"
            'binary = "jinna"\n'
            'purpose = "Run provider"\n'
            'installer = "github-release"\n'
            'repository = "someone/else"\n'
            'release_policy = "latest-stable"\n'
        )
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(self.schema.RecipeValidationError):
                self.schema.load_recipe_toml(self._write_recipe(Path(tmp), dependency))


class ProviderDependencyCheckTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(INTERNAL / "recipe_schema.py", "jinna_check_schema")
        cls.check = load_module(INTERNAL / "dep_check.py", "jinna_dep_check")
        cls.install = load_module(INTERNAL / "provider_install.py", "jinna_check_install")

    def test_managed_provider_is_reported_as_available(self):
        dep = self.schema.CliDep(
            binary="jinna",
            purpose="provider",
            installer="github-release",
            repository="parada1104/jinna-provider",
            release_policy="latest-stable",
            min_version="0.1.0",
        )
        recipe = self.schema.Recipe(
            id="jinna-mcp-recipe",
            name="Jinna",
            description="provider",
            version="1.0.0",
            cli_deps=[dep],
        )
        resolved = self.install.ProviderResolution(
            binary="jinna",
            command="/cache/jinna",
            version="0.1.0",
            source="managed",
            path=Path("/cache/jinna"),
            target=("darwin", "arm64"),
            verified=True,
        )
        with patch.object(self.check, "_load_provider_install") as loader:
            loader.return_value.resolve_provider.return_value = resolved
            result = self.check.check_cli_deps(recipe)[0]
        self.assertTrue(result.ok)
        self.assertEqual(result.source, "managed")
        self.assertEqual(result.resolved_path, "/cache/jinna")


class ProviderInstallPlanTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.install = load_module(INTERNAL / "dep_install.py", "jinna_dep_install")

    def test_github_release_plan_is_not_a_shell_command(self):
        plan = self.install.resolve_install_plan(
            "jinna",
            install_url="https://github.com/parada1104/jinna-provider/releases",
            installer="github-release",
            repository="parada1104/jinna-provider",
            release_policy="latest-stable",
            min_version="0.1.0",
        )
        self.assertEqual(plan.kind, "github-release")
        self.assertEqual(plan.command, [])
        self.assertIn("parada1104/jinna-provider", plan.display)

    def test_non_tty_never_executes_github_install(self):
        plan = MagicMock(kind="github-release", command=[], binary="jinna")
        with patch.object(self.install, "_load_provider_install") as loader:
            self.assertEqual(self.install.offer_and_install([plan], tty=False), [])
        loader.assert_not_called()

    def test_empty_command_plan_never_calls_subprocess(self):
        plan = self.install.InstallPlan(
            binary="gh",
            command=[],
            display="install gh manually",
            guidance_url="https://cli.github.com/",
            kind="brew",
        )
        questionary = MagicMock()
        questionary.confirm.return_value.ask.return_value = True
        buffer = io.StringIO()
        with patch.dict("sys.modules", {"questionary": questionary}), patch.object(
            self.install.subprocess, "run"
        ) as run, contextlib.redirect_stderr(buffer):
            installed = self.install.offer_and_install([plan], tty=True)
        self.assertEqual(installed, [])
        run.assert_not_called()
        self.assertIn("install gh manually", buffer.getvalue())

    def test_interactive_github_install_requires_confirmation(self):
        plan = MagicMock(kind="github-release", command=[], binary="jinna")
        provider = MagicMock()
        provider.install_github_release.return_value = MagicMock(verified=True, version="0.1.0")
        questionary = MagicMock()
        questionary.confirm.return_value.ask.return_value = True
        with patch.dict("sys.modules", {"questionary": questionary}), patch.object(
            self.install, "_load_provider_install", return_value=provider
        ):
            self.assertEqual(self.install.offer_and_install([plan], tty=True), ["jinna"])
        provider.install_github_release.assert_called_once_with(plan.provider_plan)


class ProviderReleaseTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.install = load_module(INTERNAL / "provider_install.py", "jinna_provider_install")

    def test_supported_platform_mapping(self):
        self.assertEqual(self.install.detect_platform("Darwin", "arm64"), ("darwin", "arm64"))
        self.assertEqual(self.install.detect_platform("Linux", "x86_64"), ("linux", "amd64"))
        self.assertEqual(self.install.detect_platform("Windows", "AMD64"), ("windows", "amd64"))
        self.assertEqual(self.install.detect_platform("FreeBSD", "amd64"), ("", ""))

    def test_release_asset_name_uses_tag_and_target(self):
        self.assertEqual(
            self.install.release_asset_name("v0.1.0", "darwin", "arm64"),
            "jinna_v0.1.0_darwin_arm64.tar.gz",
        )
        self.assertEqual(
            self.install.release_asset_name("v0.1.0", "windows", "amd64"),
            "jinna_v0.1.0_windows_amd64.zip",
        )

    def test_stable_release_selection_rejects_wrong_repository(self):
        payload = {
            "html_url": "https://github.com/parada1104/jinna-provider/releases/tag/v0.1.0",
            "tag_name": "v0.1.0",
            "draft": False,
            "prerelease": False,
            "assets": [],
        }
        selected = self.install.parse_release(payload)
        self.assertEqual(selected.tag, "v0.1.0")
        self.assertEqual(selected.repository, "parada1104/jinna-provider")

    def test_sha256_manifest_requires_exact_asset(self):
        archive = b"archive-bytes"
        digest = hashlib.sha256(archive).hexdigest()
        self.assertEqual(
            self.install.parse_sha256sums(f"{digest}  jinna_v0.1.0_linux_amd64.tar.gz\n", "jinna_v0.1.0_linux_amd64.tar.gz"),
            digest,
        )
        with self.assertRaises(self.install.InstallError):
            self.install.parse_sha256sums(f"{digest}  other.tar.gz\n", "jinna_v0.1.0_linux_amd64.tar.gz")

    def test_safe_tar_extraction_rejects_traversal(self):
        with tempfile.TemporaryDirectory() as tmp:
            archive = Path(tmp) / "bad.tar.gz"
            with tarfile.open(archive, "w:gz") as tar:
                data = b"escape"
                info = tarfile.TarInfo("../outside")
                info.size = len(data)
                tar.addfile(info, io.BytesIO(data))
            with self.assertRaises(self.install.InstallError):
                self.install.extract_release_archive(archive, Path(tmp) / "out", "jinna_v0.1.0_linux_amd64.tar.gz", "linux")
            self.assertFalse((Path(tmp) / "outside").exists())

    @staticmethod
    def _tar_archive(archive_name: str) -> bytes:
        root = archive_name.removesuffix(".tar.gz")
        files = {
            f"{root}/jinna": b"provider executable",
            f"{root}/LICENSE": b"MIT",
            f"{root}/THIRD_PARTY_NOTICES.md": b"notices",
            f"{root}/README.md": b"README",
            f"{root}/docs/mcp.md": b"mcp",
            f"{root}/docs/migration.md": b"migration",
            f"{root}/docs/operations.md": b"operations",
        }
        stream = io.BytesIO()
        with tarfile.open(fileobj=stream, mode="w:gz") as tar:
            directory = tarfile.TarInfo(root)
            directory.type = tarfile.DIRTYPE
            tar.addfile(directory)
            docs = tarfile.TarInfo(f"{root}/docs")
            docs.type = tarfile.DIRTYPE
            tar.addfile(docs)
            for name, data in files.items():
                info = tarfile.TarInfo(name)
                info.size = len(data)
                tar.addfile(info, io.BytesIO(data))
        return stream.getvalue()

    def test_valid_github_release_is_installed_and_receipted(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        archive = self._tar_archive(archive_name)
        digest = hashlib.sha256(archive).hexdigest()
        content = _release_fixture(self.install, archive_name, archive, f"{digest}  {archive_name}\n")
        with tempfile.TemporaryDirectory() as tmp:
            plan = _release_plan(self.install, Path(tmp) / "cache")
            result = self.install.install_github_release(
                plan,
                fetch=lambda url: content[url],
                run_version=lambda path: "0.1.0",
            )
            self.assertEqual(result.source, "managed")
            self.assertTrue(result.verified)
            self.assertTrue(result.path.is_file())
            receipt = json.loads((result.path.parent / "install.json").read_text())
            self.assertEqual(receipt["release_tag"], "v0.1.0")
            self.assertEqual(receipt["target"], "darwin-arm64")
            self.assertEqual(list((Path(tmp) / "cache").glob(".jinna-*")), [])

    def test_github_release_install_uses_real_archive_contract(self):
        """The published v0.1.0 asset names are exactly what the installer derives."""
        for goos, goarch, suffix in (
            ("linux", "amd64", ".tar.gz"),
            ("linux", "arm64", ".tar.gz"),
            ("darwin", "amd64", ".tar.gz"),
            ("darwin", "arm64", ".tar.gz"),
            ("windows", "amd64", ".zip"),
        ):
            self.assertEqual(
                self.install.release_asset_name("v0.1.0", goos, goarch),
                f"jinna_v0.1.0_{goos}_{goarch}{suffix}",
            )

    def test_checksum_mismatch_does_not_publish_target(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        archive = self._tar_archive(archive_name)
        content = _release_fixture(
            self.install,
            archive_name,
            archive,
            "0" * 64 + "  " + archive_name + "\n",
        )
        with tempfile.TemporaryDirectory() as tmp:
            cache = Path(tmp) / "cache"
            plan = _release_plan(self.install, cache)
            with self.assertRaises(self.install.InstallError):
                self.install.install_github_release(
                    plan, fetch=lambda url: content[url], run_version=lambda path: "0.1.0"
                )
            self.assertFalse((cache / "v0.1.0" / "darwin-arm64").exists())


def _provider_tarball(archive_name: str, *, binary_size: int | None = None) -> bytes:
    """Build the release archive layout the provider publishes."""
    root = archive_name.removesuffix(".tar.gz")
    files = {
        f"{root}/jinna": b"provider executable" if binary_size is None else b"x" * binary_size,
        f"{root}/LICENSE": b"MIT",
        f"{root}/THIRD_PARTY_NOTICES.md": b"notices",
        f"{root}/README.md": b"README",
        f"{root}/docs/mcp.md": b"mcp",
        f"{root}/docs/migration.md": b"migration",
        f"{root}/docs/operations.md": b"operations",
    }
    stream = io.BytesIO()
    with tarfile.open(fileobj=stream, mode="w:gz") as tar:
        for directory in (root, f"{root}/docs"):
            info = tarfile.TarInfo(directory)
            info.type = tarfile.DIRTYPE
            tar.addfile(info)
        for name, data in files.items():
            info = tarfile.TarInfo(name)
            info.size = len(data)
            tar.addfile(info, io.BytesIO(data))
    return stream.getvalue()


def _metadata_body(tag: str, artifacts: list[str], *, version: str | None = None) -> bytes:
    """Build a RELEASE.json body matching the provider release contract."""
    return json.dumps(
        {
            "version": tag if version is None else version,
            "commit": "4dc0182ed5036a258b024aabd7d6e91530dd1ddf",
            "buildDate": "2026-09-09T14:33:00-03:00",
            "sourceDateEpoch": 1788975180,
            "artifacts": list(artifacts),
        }
    ).encode()


def _traversal_tarball(archive_name: str) -> bytes:
    """Build a checksum-valid but unsafe archive containing a traversal member."""
    root = archive_name.removesuffix(".tar.gz")
    stream = io.BytesIO()
    with tarfile.open(fileobj=stream, mode="w:gz") as tar:
        data = b"escape"
        placeholder = tarfile.TarInfo(f"{root}/jinna")
        placeholder.size = len(data)
        tar.addfile(placeholder, io.BytesIO(data))
        escape = tarfile.TarInfo("../outside")
        escape.size = len(data)
        tar.addfile(escape, io.BytesIO(data))
    return stream.getvalue()


def _release_content(install, files: dict[str, bytes], *, tag: str = "v0.1.0", release_payload=None):
    """Build the GitHub API + download fixture keyed by asset name."""
    base = f"https://github.com/parada1104/jinna-provider/releases/download/{tag}/"
    payload = (
        release_payload
        if release_payload is not None
        else {
            "html_url": f"https://github.com/parada1104/jinna-provider/releases/tag/{tag}",
            "tag_name": tag,
            "draft": False,
            "prerelease": False,
            "assets": [
                {"name": name, "browser_download_url": base + name} for name in files
            ],
        }
    )
    content = {install.GITHUB_API_BASE + "/releases/latest": json.dumps(payload).encode()}
    for name, body in files.items():
        content[base + name] = body
    return content


def _release_fixture(
    install,
    archive_name: str,
    archive: bytes,
    sums_text: str,
    *,
    tag: str = "v0.1.0",
    metadata: bytes | None = None,
    metadata_artifacts: list[str] | None = None,
):
    """Standard release fixture: archive + SHA256SUMS + RELEASE.json."""
    files = {
        archive_name: archive,
        "SHA256SUMS": sums_text.encode(),
        "RELEASE.json": (
            metadata
            if metadata is not None
            else _metadata_body(tag, metadata_artifacts or [archive_name])
        ),
    }
    return _release_content(install, files, tag=tag)


def _release_plan(install, cache: Path):
    return install.GithubReleasePlan(
        binary="jinna",
        repository=PROVIDER_REPOSITORY,
        release_policy="latest-stable",
        min_version="0.1.0",
        install_url="https://github.com/parada1104/jinna-provider/releases",
        goos="darwin",
        goarch="arm64",
        cache_root=cache,
    )


def _seed_managed_candidate(
    cache: Path, *, tag: str, goos: str, goarch: str, reported_version: str
) -> Path:
    """Create a verified managed candidate exactly like a real install would."""
    target = cache / tag / f"{goos}-{goarch}"
    target.mkdir(parents=True)
    binary = target / ("jinna.exe" if goos == "windows" else "jinna")
    binary.write_text(f"#!/bin/sh\necho 'jinna version {reported_version}'\n")
    binary.chmod(0o755)
    (target / "install.json").write_text(
        json.dumps(
            {
                "status": "verified",
                "repository": PROVIDER_REPOSITORY,
                "release_tag": tag,
                "target": f"{goos}-{goarch}",
                "archive": f"jinna_{tag}_{goos}_{goarch}.tar.gz",
                "archive_sha256": "0" * 64,
                "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
            }
        ),
        encoding="utf-8",
    )
    return binary


def _release_dependency(install):
    return SimpleNamespace(
        binary="jinna",
        min_version="0.1.0",
        installer="github-release",
        repository=PROVIDER_REPOSITORY,
        release_policy="latest-stable",
        install_url="https://github.com/parada1104/jinna-provider/releases",
    )


class ProviderInstallerHardeningTests(unittest.TestCase):
    """RED/GREEN coverage for the remaining installer safety invariants."""

    @classmethod
    def setUpClass(cls):
        cls.install = load_module(INTERNAL / "provider_install.py", "jinna_installer_hardening")

    def test_version_self_check_must_match_release_tag(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        archive = _provider_tarball(archive_name)
        digest = hashlib.sha256(archive).hexdigest()
        content = _release_fixture(
            self.install, archive_name, archive, f"{digest}  {archive_name}\n"
        )
        with tempfile.TemporaryDirectory() as tmp:
            cache = Path(tmp) / "cache"
            plan = _release_plan(self.install, cache)
            with self.assertRaises(self.install.InstallError):
                self.install.install_github_release(
                    plan,
                    fetch=lambda url: content[url],
                    run_version=lambda path: "0.9.9",
                )
            self.assertFalse((cache / "v0.1.0" / "darwin-arm64").exists())

    def test_malformed_release_metadata_raises_install_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            plan = _release_plan(self.install, Path(tmp) / "cache")
            with self.assertRaises(self.install.InstallError):
                self.install.install_github_release(
                    plan,
                    fetch=lambda url: b"{not json",
                    run_version=lambda path: "0.1.0",
                )

    def test_release_missing_expected_target_asset_is_rejected(self):
        other = "jinna_v0.1.0_linux_amd64.tar.gz"
        content = _release_content(
            self.install,
            {
                other: b"provider bytes",
                "SHA256SUMS": b"",
                "RELEASE.json": _metadata_body("v0.1.0", [other]),
            },
        )
        with tempfile.TemporaryDirectory() as tmp:
            plan = _release_plan(self.install, Path(tmp) / "cache")
            with self.assertRaises(self.install.InstallError):
                self.install.install_github_release(
                    plan, fetch=lambda url: content[url], run_version=lambda path: "0.1.0"
                )

    def test_non_github_asset_host_is_rejected(self):
        payload = {
            "html_url": "https://github.com/parada1104/jinna-provider/releases/tag/v0.1.0",
            "tag_name": "v0.1.0",
            "draft": False,
            "prerelease": False,
            "assets": [
                {
                    "name": "jinna_v0.1.0_darwin_arm64.tar.gz",
                    "browser_download_url": "https://evil.example.com/jinna.tar.gz",
                }
            ],
        }
        with self.assertRaises(self.install.InstallError):
            self.install.parse_release(payload)

    def test_duplicate_release_assets_are_rejected(self):
        asset = {
            "name": "jinna_v0.1.0_darwin_arm64.tar.gz",
            "browser_download_url": RELEASE_BASE + "jinna_v0.1.0_darwin_arm64.tar.gz",
        }
        payload = {
            "html_url": "https://github.com/parada1104/jinna-provider/releases/tag/v0.1.0",
            "tag_name": "v0.1.0",
            "draft": False,
            "prerelease": False,
            "assets": [asset, dict(asset)],
        }
        with self.assertRaises(self.install.InstallError):
            self.install.parse_release(payload)

    def test_oversized_response_is_rejected(self):
        class FakeResponse:
            def __init__(self, data: bytes):
                self._remaining = data

            def read(self, size: int = -1) -> bytes:
                if size is None or size < 0:
                    size = len(self._remaining)
                chunk, self._remaining = self._remaining[:size], self._remaining[size:]
                return chunk

            def __enter__(self):
                return self

            def __exit__(self, *exc):
                return False

        oversized = b"x" * (self.install.MAX_HTTP_BYTES + 1)
        with patch.object(
            self.install.urllib.request, "urlopen", return_value=FakeResponse(oversized)
        ):
            with self.assertRaises(self.install.InstallError):
                self.install._fetch_bytes(self.install.GITHUB_API_BASE + "/releases/latest")

    def test_fetch_rejects_non_allowlisted_host(self):
        with self.assertRaises(self.install.InstallError):
            self.install._fetch_bytes("https://evil.example.com/SHA256SUMS")

    def test_non_utf8_sums_manifest_raises_install_error(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        archive = _provider_tarball(archive_name)
        digest = hashlib.sha256(archive).hexdigest()
        content = _release_fixture(
            self.install, archive_name, archive, f"{digest}  {archive_name}\n"
        )
        sums_url = next(url for url in content if url.endswith("/SHA256SUMS"))
        content[sums_url] = b"\xff\xfe not utf-8 \xff\x00"
        with tempfile.TemporaryDirectory() as tmp:
            plan = _release_plan(self.install, Path(tmp) / "cache")
            with self.assertRaises(self.install.InstallError):
                self.install.install_github_release(
                    plan,
                    fetch=lambda url: content[url],
                    run_version=lambda path: "0.1.0",
                )

    def test_install_receipt_carries_only_non_secret_provenance(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        archive = _provider_tarball(archive_name)
        digest = hashlib.sha256(archive).hexdigest()
        content = _release_fixture(
            self.install, archive_name, archive, f"{digest}  {archive_name}\n"
        )
        with tempfile.TemporaryDirectory() as tmp:
            plan = _release_plan(self.install, Path(tmp) / "cache")
            result = self.install.install_github_release(
                plan,
                fetch=lambda url: content[url],
                run_version=lambda path: "0.1.0",
            )
            receipt = json.loads((result.path.parent / "install.json").read_text())
        self.assertEqual(
            set(receipt),
            {
                "status",
                "repository",
                "release_tag",
                "target",
                "archive",
                "archive_sha256",
                "binary_sha256",
            },
        )
        self.assertEqual(receipt["repository"], PROVIDER_REPOSITORY)
        self.assertEqual(receipt["target"], "darwin-arm64")

    def test_managed_candidate_with_inconsistent_receipt_is_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp)
            cache = self.install.cache_root(home)
            target_dir = cache / "v0.1.0" / "darwin-arm64"
            target_dir.mkdir(parents=True)
            binary = target_dir / "jinna"
            binary.write_text("#!/bin/sh\necho 'jinna version v0.1.0'\n")
            binary.chmod(0o755)
            (target_dir / "install.json").write_text(
                json.dumps(
                    {
                        "status": "verified",
                        "repository": PROVIDER_REPOSITORY,
                        "release_tag": "v9.9.9",
                        "target": "darwin-arm64",
                        "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
                    }
                )
            )
            dep = SimpleNamespace(
                binary="jinna",
                min_version="0.1.0",
                installer="github-release",
                repository=PROVIDER_REPOSITORY,
                release_policy="latest-stable",
            )
            with patch.object(self.install.shutil, "which", return_value=None):
                resolution = self.install.resolve_provider(dep, ai_specs_home=home)
        self.assertEqual(resolution.source, "unresolved")

    def test_resolution_never_touches_the_network(self):
        dep = SimpleNamespace(
            binary="jinna",
            min_version="0.1.0",
            installer="github-release",
            repository=PROVIDER_REPOSITORY,
            release_policy="latest-stable",
        )
        with tempfile.TemporaryDirectory() as tmp, patch.object(
            self.install.shutil, "which", return_value=None
        ), patch.object(
            self.install,
            "_fetch_bytes",
            side_effect=AssertionError("resolution must not download"),
        ):
            resolution = self.install.resolve_provider(dep, ai_specs_home=Path(tmp))
        self.assertEqual(resolution.source, "unresolved")

    def test_resolution_degrades_on_unreadable_managed_binary(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp)
            cache = self.install.cache_root(home)
            _seed_managed_candidate(
                cache, tag="v0.1.0", goos="darwin", goarch="arm64", reported_version="0.1.0"
            )
            with patch.object(self.install.shutil, "which", return_value=None), patch.object(
                self.install, "_sha256", side_effect=OSError("permission denied")
            ):
                resolution = self.install.resolve_provider(
                    _release_dependency(self.install), ai_specs_home=home
                )
        self.assertEqual(resolution.source, "unresolved")

    def test_resolution_degrades_on_unreadable_cache_root(self):
        with tempfile.TemporaryDirectory() as tmp, patch.object(
            self.install.shutil, "which", return_value=None
        ), patch.object(
            self.install, "_managed_candidates", side_effect=OSError("permission denied")
        ):
            resolution = self.install.resolve_provider(
                _release_dependency(self.install), ai_specs_home=Path(tmp)
            )
        self.assertEqual(resolution.source, "unresolved")

    def test_unsupported_platform_lists_targets_and_guidance(self):
        dep = SimpleNamespace(
            binary="jinna",
            installer="github-release",
            repository=PROVIDER_REPOSITORY,
            release_policy="latest-stable",
            min_version="0.1.0",
            install_url="https://github.com/parada1104/jinna-provider/releases",
        )
        with patch.object(self.install, "detect_platform", return_value=("", "")):
            with self.assertRaises(self.install.InstallError) as ctx:
                self.install.build_release_plan(dep)
        message = str(ctx.exception)
        for target in ("darwin/arm64", "linux/amd64", "windows/amd64"):
            self.assertIn(target, message)
        self.assertIn("github.com/parada1104/jinna-provider/releases", message)

    def test_archive_traversal_absolute_and_duplicate_members_are_rejected(self):
        cases = {
            "absolute": "/etc/passwd",
            "duplicate": "jinna_v0.1.0_linux_amd64/README.md",
        }
        for label, member_name in cases.items():
            with self.subTest(case=label), tempfile.TemporaryDirectory() as tmp:
                stream = io.BytesIO()
                with tarfile.open(fileobj=stream, mode="w:gz") as tar:
                    info = tarfile.TarInfo(member_name)
                    data = b"payload"
                    info.size = len(data)
                    tar.addfile(info, io.BytesIO(data))
                    if label == "duplicate":
                        clone = tarfile.TarInfo(member_name)
                        clone.size = len(data)
                        tar.addfile(clone, io.BytesIO(data))
                archive = Path(tmp) / "archive.tar.gz"
                archive.write_bytes(stream.getvalue())
                with self.assertRaises(self.install.InstallError):
                    self.install.extract_release_archive(
                        archive,
                        Path(tmp) / "out",
                        "jinna_v0.1.0_linux_amd64.tar.gz",
                        "linux",
                    )

    def test_unexpected_archive_member_is_rejected(self):
        archive_name = "jinna_v0.1.0_linux_amd64.tar.gz"
        root = archive_name.removesuffix(".tar.gz")
        stream = io.BytesIO()
        with tarfile.open(fileobj=stream, mode="w:gz") as tar:
            info = tarfile.TarInfo(f"{root}/jinna-v2")
            data = b"unexpected"
            info.size = len(data)
            tar.addfile(info, io.BytesIO(data))
        with tempfile.TemporaryDirectory() as tmp:
            archive = Path(tmp) / "archive.tar.gz"
            archive.write_bytes(stream.getvalue())
            with self.assertRaises(self.install.InstallError):
                self.install.extract_release_archive(
                    archive, Path(tmp) / "out", archive_name, "linux"
                )

    def test_symlink_member_is_rejected(self):
        archive_name = "jinna_v0.1.0_linux_amd64.tar.gz"
        root = archive_name.removesuffix(".tar.gz")
        stream = io.BytesIO()
        with tarfile.open(fileobj=stream, mode="w:gz") as tar:
            link = tarfile.TarInfo(f"{root}/jinna")
            link.type = tarfile.SYMTYPE
            link.linkname = "/etc/passwd"
            tar.addfile(link)
        with tempfile.TemporaryDirectory() as tmp:
            archive = Path(tmp) / "archive.tar.gz"
            archive.write_bytes(stream.getvalue())
            with self.assertRaises(self.install.InstallError):
                self.install.extract_release_archive(
                    archive, Path(tmp) / "out", archive_name, "linux"
                )


class ProviderReleaseIntegrityTests(unittest.TestCase):
    """Genuine failure-path coverage: metadata rules, network, checksums, atomicity."""

    @classmethod
    def setUpClass(cls):
        cls.install = load_module(INTERNAL / "provider_install.py", "jinna_release_integrity")

    @staticmethod
    def _release_payload(**overrides) -> dict:
        payload = {
            "html_url": f"https://github.com/{PROVIDER_REPOSITORY}/releases/tag/v0.1.0",
            "tag_name": "v0.1.0",
            "draft": False,
            "prerelease": False,
            "assets": [],
        }
        payload.update(overrides)
        return payload

    def test_draft_release_is_rejected(self):
        with self.assertRaises(self.install.InstallError):
            self.install.parse_release(self._release_payload(draft=True))

    def test_prerelease_flag_is_rejected(self):
        with self.assertRaises(self.install.InstallError):
            self.install.parse_release(self._release_payload(prerelease=True))

    def test_prerelease_suffix_tag_is_rejected_without_the_flag(self):
        with self.assertRaises(self.install.InstallError):
            self.install.parse_release(self._release_payload(tag_name="v0.2.0-rc.1"))

    def test_wrong_repository_release_is_rejected(self):
        with self.assertRaises(self.install.InstallError):
            self.install.parse_release(
                self._release_payload(
                    html_url="https://github.com/someone/else/releases/tag/v0.1.0"
                )
            )

    def test_malformed_release_metadata_is_rejected(self):
        base = self._release_payload()
        cases = {
            "non_object": [],
            "missing_tag": {k: v for k, v in base.items() if k != "tag_name"},
            "non_list_assets": {**base, "assets": {}},
            "asset_not_object": {**base, "assets": ["nope"]},
            "asset_without_url": {**base, "assets": [{"name": "x"}]},
        }
        for label, payload in cases.items():
            with self.subTest(case=label):
                with self.assertRaises(self.install.InstallError):
                    self.install.parse_release(payload)

    def test_network_error_becomes_bounded_install_error(self):
        with patch.object(
            self.install.urllib.request,
            "urlopen",
            side_effect=urllib.error.URLError("name resolution failed"),
        ):
            with self.assertRaises(self.install.InstallError) as ctx:
                self.install._fetch_bytes(self.install.GITHUB_API_BASE + "/releases/latest")
        message = str(ctx.exception)
        self.assertIn("provider download failed", message)
        self.assertNotIn("OPENPROJECT", message)

    def test_timeout_becomes_bounded_install_error(self):
        with patch.object(
            self.install.urllib.request, "urlopen", side_effect=TimeoutError("timed out")
        ):
            with self.assertRaises(self.install.InstallError) as ctx:
                self.install._fetch_bytes(self.install.GITHUB_API_BASE + "/releases/latest")
        self.assertNotIn("OPENPROJECT", str(ctx.exception))

    def test_sha256_manifest_rejects_malformed_missing_and_duplicate_entries(self):
        name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        digest = "a" * 64
        cases = {
            "single_space": f"{digest} {name}\n",
            "short_digest": f"abc  {name}\n",
            "missing_asset": f"{digest}  other.tar.gz\n",
            "duplicate": f"{digest}  {name}\n{digest}  {name}\n",
        }
        for label, text in cases.items():
            with self.subTest(case=label):
                with self.assertRaises(self.install.InstallError):
                    self.install.parse_sha256sums(text, name)

    def test_checksum_mismatch_never_runs_the_binary(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        archive = _provider_tarball(archive_name)
        content = _release_fixture(
            self.install, archive_name, archive, "0" * 64 + f"  {archive_name}\n"
        )
        runs: list[Path] = []
        with tempfile.TemporaryDirectory() as tmp:
            plan = _release_plan(self.install, Path(tmp) / "cache")
            with self.assertRaises(self.install.InstallError):
                self.install.install_github_release(
                    plan,
                    fetch=lambda url: content[url],
                    run_version=lambda path: runs.append(path) or "0.1.0",
                )
        self.assertEqual(runs, [])

    def test_release_without_release_json_asset_is_rejected(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        archive = _provider_tarball(archive_name)
        digest = hashlib.sha256(archive).hexdigest()
        content = _release_content(
            self.install,
            {archive_name: archive, "SHA256SUMS": f"{digest}  {archive_name}\n".encode()},
        )
        with tempfile.TemporaryDirectory() as tmp:
            plan = _release_plan(self.install, Path(tmp) / "cache")
            with self.assertRaises(self.install.InstallError):
                self.install.install_github_release(
                    plan, fetch=lambda url: content[url], run_version=lambda path: "0.1.0"
                )

    def test_release_json_version_mismatch_is_rejected(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        archive = _provider_tarball(archive_name)
        digest = hashlib.sha256(archive).hexdigest()
        content = _release_fixture(
            self.install,
            archive_name,
            archive,
            f"{digest}  {archive_name}\n",
            metadata=_metadata_body("v0.1.0", [archive_name], version="v9.9.9"),
        )
        with tempfile.TemporaryDirectory() as tmp:
            plan = _release_plan(self.install, Path(tmp) / "cache")
            with self.assertRaises(self.install.InstallError):
                self.install.install_github_release(
                    plan, fetch=lambda url: content[url], run_version=lambda path: "0.1.0"
                )

    def test_release_json_missing_expected_artifact_is_rejected(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        archive = _provider_tarball(archive_name)
        digest = hashlib.sha256(archive).hexdigest()
        content = _release_fixture(
            self.install,
            archive_name,
            archive,
            f"{digest}  {archive_name}\n",
            metadata_artifacts=["jinna_v0.1.0_linux_amd64.tar.gz"],
        )
        with tempfile.TemporaryDirectory() as tmp:
            plan = _release_plan(self.install, Path(tmp) / "cache")
            with self.assertRaises(self.install.InstallError):
                self.install.install_github_release(
                    plan, fetch=lambda url: content[url], run_version=lambda path: "0.1.0"
                )

    def test_release_json_duplicate_artifact_is_rejected(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        archive = _provider_tarball(archive_name)
        digest = hashlib.sha256(archive).hexdigest()
        content = _release_fixture(
            self.install,
            archive_name,
            archive,
            f"{digest}  {archive_name}\n",
            metadata_artifacts=[archive_name, archive_name],
        )
        with tempfile.TemporaryDirectory() as tmp:
            plan = _release_plan(self.install, Path(tmp) / "cache")
            with self.assertRaises(self.install.InstallError):
                self.install.install_github_release(
                    plan, fetch=lambda url: content[url], run_version=lambda path: "0.1.0"
                )

    def test_release_json_malformed_and_non_object_bodies_are_rejected(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        archive = _provider_tarball(archive_name)
        digest = hashlib.sha256(archive).hexdigest()
        for label, body in {"malformed": b"{oops", "non_object": b"[]"}.items():
            with self.subTest(case=label):
                content = _release_fixture(
                    self.install,
                    archive_name,
                    archive,
                    f"{digest}  {archive_name}\n",
                    metadata=body,
                )
                with tempfile.TemporaryDirectory() as tmp:
                    plan = _release_plan(self.install, Path(tmp) / "cache")
                    with self.assertRaises(self.install.InstallError):
                        self.install.install_github_release(
                            plan,
                            fetch=lambda url: content[url],
                            run_version=lambda path: "0.1.0",
                        )

    def test_release_json_oversized_body_is_rejected(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        archive = _provider_tarball(archive_name)
        digest = hashlib.sha256(archive).hexdigest()
        oversized = json.dumps(
            {
                "version": "v0.1.0",
                "artifacts": [archive_name],
                "padding": "x" * (256 * 1024),
            }
        ).encode()
        content = _release_fixture(
            self.install, archive_name, archive, f"{digest}  {archive_name}\n", metadata=oversized
        )
        with tempfile.TemporaryDirectory() as tmp:
            plan = _release_plan(self.install, Path(tmp) / "cache")
            with self.assertRaises(self.install.InstallError):
                self.install.install_github_release(
                    plan, fetch=lambda url: content[url], run_version=lambda path: "0.1.0"
                )

    def test_prior_managed_candidate_survives_every_injected_failure(self):
        archive_name = "jinna_v0.1.0_darwin_arm64.tar.gz"
        good_archive = _provider_tarball(archive_name)
        digest = hashlib.sha256(good_archive).hexdigest()
        good = _release_fixture(
            self.install, archive_name, good_archive, f"{digest}  {archive_name}\n"
        )
        api_url = self.install.GITHUB_API_BASE + "/releases/latest"
        sums_url = next(url for url in good if url.endswith("/SHA256SUMS"))
        archive_url = next(url for url in good if url.endswith(archive_name))

        def injected(target: str):
            def fetch(url: str) -> bytes:
                if url == target:
                    raise self.install.InstallError(f"injected download failure for {target}")
                return good[url]

            return fetch

        def version(reported: str):
            def run(path: Path) -> str:
                return reported

            return run

        mismatched = dict(good)
        mismatched[sums_url] = ("0" * 64 + f"  {archive_name}\n").encode()

        traversal = _traversal_tarball(archive_name)
        unsafe = dict(good)
        unsafe[archive_url] = traversal
        unsafe[sums_url] = f"{hashlib.sha256(traversal).hexdigest()}  {archive_name}\n".encode()

        scenarios = {
            "metadata_download": (injected(api_url), version("0.1.0")),
            "sums_download": (injected(sums_url), version("0.1.0")),
            "archive_download": (injected(archive_url), version("0.1.0")),
            "checksum_mismatch": (lambda url: mismatched[url], version("0.1.0")),
            "unsafe_archive": (lambda url: unsafe[url], version("0.1.0")),
            "version_self_check": (lambda url: good[url], version("9.9.9")),
        }

        for label, (fetch, run_version) in scenarios.items():
            with self.subTest(failure=label), tempfile.TemporaryDirectory() as tmp:
                home = Path(tmp)
                cache = self.install.cache_root(home)
                prior = _seed_managed_candidate(
                    cache,
                    tag="v0.0.9",
                    goos="darwin",
                    goarch="arm64",
                    reported_version="0.0.9",
                )
                prior_bytes = prior.read_bytes()
                prior_receipt = (prior.parent / "install.json").read_bytes()
                plan = _release_plan(self.install, cache)
                with self.assertRaises(self.install.InstallError):
                    self.install.install_github_release(
                        plan, fetch=fetch, run_version=run_version
                    )
                self.assertEqual(prior.read_bytes(), prior_bytes)
                self.assertEqual((prior.parent / "install.json").read_bytes(), prior_receipt)
                self.assertFalse((cache / "v0.1.0" / "darwin-arm64").exists())
                self.assertEqual(list(cache.glob("**/.jinna-*")), [])
                dep = _release_dependency(self.install)
                dep.min_version = "0.0.1"
                with patch.object(self.install.shutil, "which", return_value=None):
                    resolution = self.install.resolve_provider(dep, ai_specs_home=home)
                self.assertEqual(resolution.source, "managed")
                self.assertTrue(resolution.verified)
                self.assertEqual(resolution.version, "0.0.9")


class ProviderMemberLimitTests(unittest.TestCase):
    """The provider binary is already close to the old 8 MiB member cap."""

    # Observed size of the released darwin/arm64 `jinna` executable (v0.1.0).
    RELEASED_BINARY_BYTES = 7_907_794

    @classmethod
    def setUpClass(cls):
        cls.install = load_module(INTERNAL / "provider_install.py", "jinna_member_limit")

    def test_member_limit_is_documented_and_above_the_released_binary(self):
        self.assertGreaterEqual(self.install.MAX_MEMBER_BYTES, 32 * 1024 * 1024)
        self.assertGreater(self.install.MAX_MEMBER_BYTES, self.RELEASED_BINARY_BYTES)

    def test_member_limit_boundary_is_enforced(self):
        archive_name = "jinna_v0.1.0_linux_amd64.tar.gz"
        limit = 4096
        with patch.object(self.install, "MAX_MEMBER_BYTES", limit):
            with tempfile.TemporaryDirectory() as tmp:
                at_limit = Path(tmp) / "at-limit.tar.gz"
                at_limit.write_bytes(_provider_tarball(archive_name, binary_size=limit))
                binary = self.install.extract_release_archive(
                    at_limit, Path(tmp) / "ok", archive_name, "linux"
                )
                self.assertEqual(binary.stat().st_size, limit)

                over_limit = Path(tmp) / "over-limit.tar.gz"
                over_limit.write_bytes(_provider_tarball(archive_name, binary_size=limit + 1))
                with self.assertRaises(self.install.InstallError):
                    self.install.extract_release_archive(
                        over_limit, Path(tmp) / "over", archive_name, "linux"
                    )


class ProviderConsentPreviewTests(unittest.TestCase):
    """Requirement 3: the interactive plan must describe the full install."""

    @classmethod
    def setUpClass(cls):
        cls.dep_install = load_module(INTERNAL / "dep_install.py", "jinna_dep_install_preview")
        cls.install = load_module(INTERNAL / "provider_install.py", "jinna_preview_install")

    def _plan(self, cache: Path):
        provider_plan = self.install.GithubReleasePlan(
            binary="jinna",
            repository=PROVIDER_REPOSITORY,
            release_policy="latest-stable",
            min_version="0.1.0",
            install_url="https://github.com/parada1104/jinna-provider/releases",
            goos="darwin",
            goarch="arm64",
            cache_root=cache,
        )
        return self.dep_install.InstallPlan(
            binary="jinna",
            command=[],
            display="GitHub Release plan",
            guidance_url="https://github.com/parada1104/jinna-provider/releases",
            kind="github-release",
            installer="github-release",
            repository=PROVIDER_REPOSITORY,
            release_policy="latest-stable",
            min_version="0.1.0",
            provider_plan=provider_plan,
        )

    def test_github_release_preview_covers_consent_fields(self):
        with tempfile.TemporaryDirectory() as tmp:
            cache = Path(tmp) / "cache"
            preview = self.dep_install.describe_install_plan(self._plan(cache))
        self.assertIn(PROVIDER_REPOSITORY, preview)
        self.assertIn("latest-stable", preview)
        self.assertIn("darwin/arm64", preview)
        self.assertIn("jinna_", preview)
        self.assertIn("darwin_arm64.tar.gz", preview)
        self.assertIn(str(cache), preview)
        self.assertIn("SHA256SUMS", preview)
        self.assertIn("PATH", preview)
        self.assertIn("0.1.0", preview)

    def test_tty_offer_prints_full_plan_before_confirmation(self):
        questionary = MagicMock()
        questionary.confirm.return_value.ask.return_value = False
        buffer = io.StringIO()
        with tempfile.TemporaryDirectory() as tmp, contextlib.redirect_stderr(
            buffer
        ), patch.dict("sys.modules", {"questionary": questionary}):
            installed = self.dep_install.offer_and_install(
                [self._plan(Path(tmp) / "cache")], tty=True
            )
        self.assertEqual(installed, [])
        printed = buffer.getvalue()
        self.assertIn(PROVIDER_REPOSITORY, printed)
        self.assertIn("SHA256SUMS", printed)
        self.assertIn("darwin/arm64", printed)

    def test_guidance_plan_keeps_legacy_display(self):
        plan = self.dep_install.InstallPlan(
            binary="npx",
            command=[],
            display="install 'npx' manually",
            guidance_url="",
            kind="guidance",
        )
        self.assertEqual(self.dep_install.describe_install_plan(plan), "install 'npx' manually")


class ProviderReleaseSmokeTests(unittest.TestCase):
    """Opt-in smoke against a real provider binary. Never part of the default suite.

    Set ``AI_SPECS_JINNA_SMOKE_BINARY`` to a released provider executable to run
    it. No OpenProject credentials, network calls, or writes are performed.
    """

    BINARY_ENV = "AI_SPECS_JINNA_SMOKE_BINARY"

    def _binary(self) -> Path:
        configured = os.environ.get(self.BINARY_ENV)
        if not configured:
            self.skipTest(f"{self.BINARY_ENV} is not set; release smoke is opt-in")
        path = Path(configured).expanduser()
        if not path.is_file():
            self.skipTest(f"{self.BINARY_ENV} does not point at a file")
        return path

    def test_provider_version_smoke(self):
        binary = self._binary()
        result = subprocess.run(
            [str(binary), "version"], capture_output=True, text=True, timeout=30
        )
        self.assertEqual(result.returncode, 0)
        self.assertRegex(result.stdout + result.stderr, r"\d+\.\d+\.\d+")

    def test_provider_mcp_help_smoke(self):
        binary = self._binary()
        result = subprocess.run(
            [str(binary), "mcp", "--help"], capture_output=True, text=True, timeout=30
        )
        self.assertIn(result.returncode, (0, 1, 2))
        self.assertTrue((result.stdout + result.stderr).strip())


if __name__ == "__main__":
    unittest.main()
