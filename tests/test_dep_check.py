"""Black-box tests for recipe CLI dependency checking (dep_check.py).

Every test drives ``bin/ai-specs doctor`` and asserts the rendered
``recipe-dep`` rows through the CLI process boundary. No test may import
``lib/_internal`` modules.

Staging (proven in test_doctor.py):
- an isolated install root with a REAL lib copy (doctor derives cache roots
  from its own realpath) and an empty catalog, where the test recipe is
  materialized into a real per-recipe directory;
- PATH surgery is the only black-box lever over ``shutil.which`` and version
  probes inside the doctor process: stub executables are placed first on PATH
  and unwanted real binaries are filtered out.

Severity contract rendered by doctor for a recipe-dep row:
  OK    — found and usable (min_version satisfied or none declared)
  WARN  — required dep missing/unusable (includes below-minimum versions)
  INFO  — optional dep missing
and the frozen exit contract: exit 1 iff at least one ERROR row (recipe-dep
rows are never ERROR).
"""
from __future__ import annotations

import hashlib
import json
import os
import platform
import re
import shutil
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import cache_project_dir, invoke, isolated_home, snapshot, tree_diff  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]

BUNDLED_SKILLS = (
    "harness-lifecycle",
    "harness-recipes",
    "harness-skills-deps",
    "skill-creator",
    "skill-sync",
)
BUNDLED_COMMANDS = ("rules-audit", "skills-as-rules")

_LINE_RE = re.compile(r"^\s*(OK|INFO|WARN|ERROR)\s+(?P<name>\S+)\s+(?P<body>.*)$")


def _dep_rows(stdout: str) -> list[tuple[str, str]]:
    """The rendered (severity, body) recipe-dep rows."""
    found = []
    for line in stdout.splitlines():
        match = _LINE_RE.match(line)
        if match and match.group("name") == "recipe-dep":
            found.append((match.group(1), match.group("body")))
    return found


def _make_home(base: Path) -> Path:
    """Isolated CLI home with a REAL lib copy and an EMPTY catalog."""
    home = isolated_home(base, catalog=False)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor"),
    )
    return home


def _seed_clean_cache(root: Path, home: Path) -> None:
    """Pre-seed the per-project bundled cache so only the dep check under test
    can influence the frozen exit-code contract (exit 1 iff any ERROR)."""
    bundled = cache_project_dir(root, home) / ".bundled"
    for skill in BUNDLED_SKILLS:
        (bundled / "skills" / skill).mkdir(parents=True, exist_ok=True)
    for command in BUNDLED_COMMANDS:
        path = bundled / "commands" / f"{command}.md"
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("# bundled\n", encoding="utf-8")


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


def _go_target() -> str:
    """The provider cache target directory name for this machine."""
    system = platform.system().lower()
    machine = platform.machine().lower()
    goarch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64"}.get(machine, machine)
    return f"{system}-{goarch}"


def _cli_dep(binary: str, purpose: str, *, required: bool = True,
             install_url: str = "", version_check: str = "",
             min_version: str = "") -> str:
    lines = [
        f"binary = \"{binary}\"",
        f"purpose = \"{purpose}\"",
        f"required = {'true' if required else 'false'}",
    ]
    if install_url:
        lines.append(f"install_url = \"{install_url}\"")
    if version_check:
        lines.append(f"version_check = \"{version_check}\"")
    if min_version:
        lines.append(f"min_version = \"{min_version}\"")
    return "[[deps.cli]]\n" + "\n".join(lines) + "\n"


class DepCheckTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.home = _make_home(self.base)

    def _write_project(self, recipe_tomls: dict[str, str]) -> Path:
        """Materialize catalog recipes (real dirs, never the repo catalog) and
        a project manifest enabling all of them."""
        for recipe_id, body in recipe_tomls.items():
            catalog = self.home / "catalog" / "recipes" / recipe_id
            catalog.mkdir(parents=True, exist_ok=True)
            header = (
                "[recipe]\n"
                f"id = \"{recipe_id}\"\n"
                f"name = \"{recipe_id.title()}\"\n"
                "description = \"D\"\n"
                "version = \"1.0\"\n\n"
            )
            (catalog / "recipe.toml").write_text(header + body, encoding="utf-8")
        project = self.base / "project"
        (project / "ai-specs").mkdir(parents=True)
        (project / "AGENTS.md").write_text("# agents\n")
        _seed_clean_cache(project, self.home)
        (project / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
        (project / "ai-specs" / "commands" / "placeholder.md").write_text("# placeholder\n")
        entries = "\n".join(
            f"[recipes.{recipe_id}]\nenabled = true\nversion = \"1.0\"\n"
            for recipe_id in recipe_tomls
        )
        (project / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = \"demo\"\n\n[agents]\nenabled = []\n\n" + entries,
            encoding="utf-8",
        )
        return project

    def _doctor(self, project: Path, *, without: tuple[str, ...] = (),
                stubs: dict[str, str] | None = None):
        new_path = _stage_path(self.base, without=without, stubs=stubs)
        with patch.dict(os.environ, {"PATH": new_path}):
            return invoke(project, "doctor", cli_home=self.home, tmpdir=self.base)

    def _single(self, dep_toml: str) -> Path:
        return self._write_project({"demo-recipe": dep_toml})

    def test_found_binary_ok(self):
        project = self._single(_cli_dep(
            "gh", "PRs", install_url="https://cli.github.com/",
        ))
        result = self._doctor(project, stubs={"gh": "#!/bin/sh\nexit 0\n"})
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertIn(("OK", "gh available for demo-recipe"), rows)

    def test_missing_binary_not_ok(self):
        project = self._single(_cli_dep(
            "gh", "PRs", install_url="https://cli.github.com/",
        ))
        result = self._doctor(project, without=("gh",))
        rows = _dep_rows(result.stdout)
        warn_rows = [(sev, body) for sev, body in rows if sev == "WARN"]
        self.assertTrue(warn_rows)
        self.assertIn("gh missing/unusable for demo-recipe", warn_rows[0][1])
        self.assertIn("https://cli.github.com/", warn_rows[0][1])
        # Frozen exit contract: a WARN-only recipe-dep finding keeps exit 0.
        self.assertEqual(result.returncode, 0)

    # TRIAGE: ai-specs doctor — the parsed version string ("2.40.0") is not
    # rendered in the recipe-dep row; only found/ok reach the CLI surface.
    def test_version_meets_min(self):
        project = self._single(_cli_dep(
            "verok", "versioned", version_check="verok --version",
            min_version="2.0.0",
        ))
        result = self._doctor(
            project, stubs={"verok": "#!/bin/sh\necho \"verok 2.40.0\"\n"})
        self.assertEqual(result.returncode, 0)
        self.assertIn(("OK", "verok available for demo-recipe"), _dep_rows(result.stdout))

    # TRIAGE: ai-specs doctor — the below-minimum detail ("found 1.9.0 <
    # required 2.0.0") is not rendered; the WARN row is the observable signal.
    def test_version_below_min(self):
        project = self._single(_cli_dep(
            "verlow", "versioned", version_check="verlow --version",
            min_version="2.0.0",
        ))
        result = self._doctor(
            project, stubs={"verlow": "#!/bin/sh\necho \"verlow 1.9.0\"\n"})
        rows = _dep_rows(result.stdout)
        self.assertTrue(
            any(sev == "WARN" and "verlow missing/unusable for demo-recipe" in body
                for sev, body in rows),
            rows,
        )
        self.assertFalse(any(sev == "OK" and "verlow" in body for sev, body in rows))

    # TRIAGE: ai-specs doctor — the "version unknown" detail is not rendered;
    # the non-blocking OK row is the observable signal.
    def test_unparseable_version_does_not_block(self):
        project = self._single(_cli_dep(
            "verweird", "versioned", version_check="verweird --version",
            min_version="2.0.0",
        ))
        result = self._doctor(
            project, stubs={"verweird": "#!/bin/sh\necho weird\n"})
        self.assertEqual(result.returncode, 0)
        self.assertIn(
            ("OK", "verweird available for demo-recipe"),
            _dep_rows(result.stdout),
        )

    def test_optional_missing_not_failure(self):
        project = self._single(_cli_dep("optool", "nice to have", required=False))
        result = self._doctor(project, without=("optool",))
        rows = _dep_rows(result.stdout)
        self.assertTrue(
            any(sev == "INFO" and "optional optool not found for demo-recipe" in body
                for sev, body in rows),
            rows,
        )
        # Frozen exit contract: exit 1 iff at least one ERROR; INFO is healthy.
        self.assertEqual(result.returncode, 0)

    # TRIAGE: ai-specs doctor — a failing version probe degrades to an empty
    # version internally; doctor renders only the non-blocking OK row.
    def test_version_check_subprocess_error_degrades(self):
        project = self._single(_cli_dep(
            "verfail", "versioned", version_check="verfail --version",
            min_version="2.0.0",
        ))
        result = self._doctor(
            project, stubs={"verfail": "#!/bin/sh\nexit 1\n"})
        self.assertEqual(result.returncode, 0)
        self.assertIn(
            ("OK", "verfail available for demo-recipe"),
            _dep_rows(result.stdout),
        )

    def test_check_project_deps_aggregates(self):
        project = self._write_project({
            "alpha": _cli_dep("tool-a", "A"),
            "beta": _cli_dep("tool-b", "B"),
        })
        result = self._doctor(project, stubs={
            "tool-a": "#!/bin/sh\nexit 0\n",
            "tool-b": "#!/bin/sh\nexit 0\n",
        })
        self.assertEqual(result.returncode, 0)
        rows = _dep_rows(result.stdout)
        self.assertIn(("OK", "tool-a available for alpha"), rows)
        self.assertIn(("OK", "tool-b available for beta"), rows)

    def _provider_dep(self) -> str:
        return _cli_dep(
            "jinna", "provider",
            install_url="https://github.com/parada1104/jinna-provider/releases",
            min_version="0.1.0",
        ) + (
            'installer = "github-release"\n'
            'repository = "parada1104/jinna-provider"\n'
            'release_policy = "latest-stable"\n'
        )

    def _stage_managed_release(self) -> Path:
        """A verified managed provider release in the isolated home cache."""
        target = _go_target()
        binpath = (
            self.home / "cache" / "bin" / "jinna" / "0.1.0" / target / "jinna"
        )
        binpath.parent.mkdir(parents=True)
        binpath.write_text("#!/bin/sh\necho \"jinna 0.1.0\"\n")
        binpath.chmod(0o755)
        (binpath.parent / "install.json").write_text(json.dumps({
            "status": "verified",
            "repository": "parada1104/jinna-provider",
            "target": target,
            "release_tag": "0.1.0",
            "binary_sha256": hashlib.sha256(binpath.read_bytes()).hexdigest(),
        }), encoding="utf-8")
        return binpath

    # TRIAGE: ai-specs doctor — resolution provenance (source "managed" and the
    # resolved cache path) is not rendered in the recipe-dep row; the OK row
    # plus the untouched-cache assertion carry the passive-resolution intent.
    def test_github_release_dep_reports_resolution_without_downloading(self):
        """Requirement 2/3: passive dependency checks never mutate or download."""
        project = self._single(self._provider_dep())
        self._stage_managed_release()
        before = snapshot(self.home / "cache")
        result = self._doctor(project)
        self.assertEqual(result.returncode, 0)
        self.assertIn(
            ("OK", "jinna available for demo-recipe"),
            _dep_rows(result.stdout),
        )
        self.assertEqual(
            tree_diff(before, snapshot(self.home / "cache")),
            {"created": [], "deleted": [], "modified": []},
        )

    # TRIAGE: ai-specs doctor — the unresolved detail ("not found for
    # darwin/arm64") and source "unresolved" are not rendered; the WARN row
    # plus the untouched-cache assertion carry the passive-resolution intent.
    def test_github_release_dep_unresolved_is_not_ok(self):
        project = self._single(self._provider_dep())
        before = snapshot(self.home / "cache")
        result = self._doctor(project)
        rows = _dep_rows(result.stdout)
        self.assertTrue(
            any(sev == "WARN" and "jinna missing/unusable for demo-recipe" in body
                for sev, body in rows),
            rows,
        )
        self.assertFalse(any(sev == "OK" and "jinna" in body for sev, body in rows))
        self.assertEqual(
            tree_diff(before, snapshot(self.home / "cache")),
            {"created": [], "deleted": [], "modified": []},
        )

    def test_version_ge(self):
        # _version_ge semantics through the CLI: zero-padding of short tuples
        # ((2,0) >= (2,0,0)), major-only compares ((10,) >= (9,)), and the
        # failing direction ((2,0) < (2,1)).
        project = self._write_project({"demo-recipe": (
            _cli_dep("pad-a", "pad", version_check="pad-a --version", min_version="2.0.0")
            + _cli_dep("big-b", "big", version_check="big-b --version", min_version="9")
            + _cli_dep("low-c", "low", version_check="low-c --version", min_version="2.1")
        )})
        result = self._doctor(project, stubs={
            "pad-a": "#!/bin/sh\necho \"pad-a 2.0\"\n",
            "big-b": "#!/bin/sh\necho \"big-b 10\"\n",
            "low-c": "#!/bin/sh\necho \"low-c 2.0\"\n",
        })
        rows = _dep_rows(result.stdout)
        self.assertIn(("OK", "pad-a available for demo-recipe"), rows)
        self.assertIn(("OK", "big-b available for demo-recipe"), rows)
        self.assertTrue(
            any(sev == "WARN" and "low-c missing/unusable for demo-recipe" in body
                for sev, body in rows),
            rows,
        )

    def test_parse_version(self):
        # _parse_version semantics through the CLI: dotted numbers are exacted
        # from mixed probe output ("mix 2.40.0" → exactly 2.40.0), and text
        # without digits parses empty (does not block).
        project = self._write_project({"demo-recipe": (
            _cli_dep("mix-a", "parse", version_check="mix-a --version", min_version="2.40.1")
            + _cli_dep("mix-b", "parse", version_check="mix-b --version", min_version="2.40.0")
            + _cli_dep("nop-c", "parse", version_check="nop-c --version", min_version="2.0.0")
        )})
        result = self._doctor(project, stubs={
            "mix-a": "#!/bin/sh\necho \"mix-a 2.40.0\"\n",
            "mix-b": "#!/bin/sh\necho \"mix-b 2.40.0\"\n",
            "nop-c": "#!/bin/sh\necho nope\n",
        })
        rows = _dep_rows(result.stdout)
        # 2.40.0 < 2.40.1 proves the probe output parsed to exactly (2, 40, 0).
        self.assertTrue(
            any(sev == "WARN" and "mix-a missing/unusable for demo-recipe" in body
                for sev, body in rows),
            rows,
        )
        self.assertIn(("OK", "mix-b available for demo-recipe"), rows)
        # No digits → () → unknown version does not block.
        self.assertIn(("OK", "nop-c available for demo-recipe"), rows)


if __name__ == "__main__":
    unittest.main()
