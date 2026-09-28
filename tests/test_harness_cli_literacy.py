"""Black-box tests for always-on harness CLI literacy skills + AGENTS.md pointer.

File-content assertions (bundled skill/AGENTS.md/CLI-help surfaces) read the
repository's observable artifacts directly. Behavioral surfaces drive the CLI
process boundary:

- ``bin/ai-specs refresh-bundled`` flattens CLI-bundled skills/commands into
  the isolated home's per-project cache (``cache/projects/<key>/.bundled``)
  with zero in-project writes.
- ``bin/ai-specs sync`` renders the AGENTS.md harness CLI-literacy pointer.

No test may import ``lib/_internal`` modules. The one internal surface with
no bin/ai-specs verb (per-skill sync-metadata validation) is kept as a
process-boundary invocation of the validator script, marked with a distinct
``# TRIAGE:`` comment.
"""
from __future__ import annotations

import json
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import cache_project_dir, invoke, temp_project  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
BUNDLED = ROOT / "bundled-skills"
SKILL_CONTRACT = ROOT / "lib" / "_internal" / "skill_contract.py"

HARNESS_SKILLS = (
    "harness-lifecycle",
    "harness-recipes",
    "harness-skills-deps",
)

POINTER_NEEDLES = (
    "harness-lifecycle",
    "harness-recipes",
    "harness-skills-deps",
)


def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy.

    The CLI derives cache and catalog roots from its own realpath, so a
    symlinked lib would resolve back into the repository and let the CLI
    touch repo cache state. A real copy keeps every lookup and write in temp.
    """
    home = base / "cli-home"
    home.mkdir(parents=True, exist_ok=True)
    for entry in ("bin", "lib", "catalog", "VERSION", "bundled-skills",
                  "bundled-commands", "templates", "scripts"):
        src = ROOT / entry
        dest = home / entry
        if src.exists() and not dest.exists():
            dest.symlink_to(src)
    (home / "cache").mkdir(parents=True, exist_ok=True)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    return home


def sync_metadata_payload(path: Path) -> dict:
    """Validate one skill's sync metadata through the validator's CLI surface.

    # TRIAGE: python3 lib/_internal/skill_contract.py sync-metadata — no
    bin/ai-specs verb exposes per-skill sync-metadata validation; the
    process-boundary invocation preserves the original contract (enabled,
    root scope, auto_invoke) without importing the module.
    """
    proc = subprocess.run(
        [sys.executable, str(SKILL_CONTRACT), "sync-metadata", str(path)],
        text=True, capture_output=True, check=False, input="",
    )
    assert proc.returncode == 0, proc.stderr or proc.stdout
    return json.loads(proc.stdout)


def public_cli_commands() -> set[str]:
    """Parse top-level command names from `ai-specs help`."""
    text = (ROOT / "bin" / "ai-specs").read_text()
    # help heredoc lists: "  init [path] ..." etc.
    cmds: set[str] = set()
    in_help = False
    for line in text.splitlines():
        if line.startswith("help|-h|--help)"):
            in_help = True
            continue
        if in_help and line.strip() == "EOF":
            break
        if not in_help:
            continue
        m = re.match(r"\s{2}([a-z][a-z0-9-]*)\b", line)
        if m:
            cmds.add(m.group(1))
    # Bare hub is routed when no subcommand; still a public entry.
    cmds.add("hub")
    return cmds


class HarnessCliLiteracyTests(unittest.TestCase):
    def setUp(self):
        self._td, self.project = temp_project(
            name="literacy-fixture", agents=("claude",)
        )
        self._home_base = Path(tempfile.mkdtemp(prefix="ai-specs-literacy-home-"))
        self.home = _make_home(self._home_base)
        self.addCleanup(self._td.cleanup)

    def _bundled_cache(self, subpath: str) -> Path:
        return cache_project_dir(self.project, self.home) / ".bundled" / subpath

    def _refresh_bundled(self, *flags: str):
        return invoke(self.project, "refresh-bundled", *flags, cli_home=self.home)

    def test_bundled_harness_skills_exist(self):
        for name in HARNESS_SKILLS:
            path = BUNDLED / name / "SKILL.md"
            self.assertTrue(path.is_file(), f"missing {path.relative_to(ROOT)}")

    def test_harness_skills_frontmatter_valid(self):
        for name in HARNESS_SKILLS:
            path = BUNDLED / name / "SKILL.md"
            meta = sync_metadata_payload(path)
            self.assertTrue(meta["enabled"], name)
            self.assertIn("root", meta["scope"], name)
            self.assertTrue(meta["auto_invoke"], name)

    def test_refresh_bundled_flattens_harness_skills_to_cache(self):
        proc = self._refresh_bundled()
        self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
        bundled_root = self._bundled_cache("skills")
        for name in HARNESS_SKILLS:
            self.assertTrue(
                (bundled_root / name / "SKILL.md").is_file(),
                f"not flattened to cache: {name}",
            )
            # CLI-bundled skills must NOT leak into the committed project surface.
            self.assertFalse(
                (self.project / "ai-specs" / "skills" / name).exists(),
                f"leaked into project surface: {name}",
            )

    def test_refresh_bundled_migrates_inproject_copy_via_lock_hash(self):
        """An untouched in-project bundled copy from an older CLI (differs from
        source, but recorded in the legacy lock) is removed on refresh-bundled —
        while the legacy [skills.*] hashes are still in memory."""
        import hashlib
        old_dir = self.project / "ai-specs" / "skills" / "harness-lifecycle"
        old_dir.mkdir(parents=True)
        old = old_dir / "SKILL.md"
        content = "# harness-lifecycle (older CLI, untouched)\n"
        old.write_text(content)
        h = hashlib.sha256(content.encode()).hexdigest()
        (self.project / "ai-specs" / ".ai-specs.lock").write_text(
            f'[skills."harness-lifecycle"]\n"SKILL.md" = "{h}"\n'
        )
        proc = self._refresh_bundled()
        self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
        self.assertFalse(
            old_dir.exists(),
            "untouched in-project copy should be migrated out via lock hash",
        )

    def test_refresh_bundled_init_flattens_commands_without_writing_project(self):
        (self.project / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
        proc = self._refresh_bundled("--init")
        self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
        bundled_root = self._bundled_cache("commands")
        self.assertTrue((bundled_root / "rules-audit.md").is_file())
        self.assertTrue((bundled_root / "skills-as-rules.md").is_file())
        self.assertEqual(
            sorted(p.name for p in (self.project / "ai-specs" / "commands").glob("*.md")),
            [],
            "bundled commands must not be written into ai-specs/commands/",
        )
        self.assertFalse(
            list((self.project / "ai-specs" / "commands").glob("*.new")),
            "refresh-bundled must never write .new sidecars",
        )

    def test_refresh_bundled_removes_byte_identical_command_leftover(self):
        (self.project / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
        leftover = self.project / "ai-specs" / "commands" / "rules-audit.md"
        leftover.write_text((ROOT / "bundled-commands" / "rules-audit.md").read_text())
        proc = self._refresh_bundled()
        self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
        self.assertFalse(leftover.exists(), "byte-identical bundled command must be removed")
        self.assertFalse((self.project / "ai-specs" / "commands" / "rules-audit.md.new").exists())

    def test_refresh_bundled_keeps_customized_command_with_notice(self):
        (self.project / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
        customized = self.project / "ai-specs" / "commands" / "rules-audit.md"
        customized.write_text("# rules-audit (locally edited)\n")
        proc = self._refresh_bundled()
        self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
        self.assertTrue(customized.exists(), "customized copy must be preserved")
        out = proc.stdout + proc.stderr
        self.assertIn("customized", out)
        self.assertFalse((self.project / "ai-specs" / "commands" / "rules-audit.md.new").exists())

    def test_agents_render_emits_harness_literacy_pointer(self):
        proc = invoke(self.project, "sync", cli_home=self.home)
        self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
        text = (self.project / "AGENTS.md").read_text()
        self.assertIn("## Useful Commands", text)
        for needle in POINTER_NEEDLES:
            self.assertIn(needle, text)
        self.assertRegex(
            text,
            r"[Hh]arness|ai-specs harness|harness operations",
        )
        # Bundled harness skills resolve via agent fan-out / cache — never
        # claim they live on the committed project surface.
        self.assertNotIn("under `ai-specs/skills/`", text)
        self.assertNotIn("under ai-specs/skills/", text)

    def test_harness_lifecycle_documents_cache_flatten(self):
        text = (BUNDLED / "harness-lifecycle" / "SKILL.md").read_text()
        self.assertRegex(text, r"\.bundled|cache.*bundled|flatten", re.I)
        self.assertNotIn("SKILL.md.new", text)
        self.assertNotRegex(
            text,
            r"First install copies into `ai-specs/skills/",
        )

    def test_refresh_prints_tracked_leftover_remediation(self):
        """After removing leftovers, refresh prints git rm --cached guidance."""
        project = self.project
        subprocess.run(["git", "init", "-q", str(project)], check=True, input="")
        subprocess.run(
            ["git", "-C", str(project), "config", "user.email", "t@example.com"],
            check=True, input="",
        )
        subprocess.run(
            ["git", "-C", str(project), "config", "user.name", "t"],
            check=True, input="",
        )
        leftover = project / "ai-specs" / "skills" / "skill-creator"
        leftover.mkdir(parents=True)
        (leftover / "SKILL.md").write_text(
            (BUNDLED / "skill-creator" / "SKILL.md").read_text()
        )
        subprocess.run(["git", "-C", str(project), "add", "-A"], check=True, input="")
        subprocess.run(
            ["git", "-C", str(project), "commit", "-qm", "track leftover"],
            check=True, input="",
        )
        proc = self._refresh_bundled()
        self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
        self.assertFalse(leftover.exists())
        out = proc.stdout + proc.stderr
        self.assertIn("git rm -r --cached", out)
        self.assertIn("skill-creator", out)
        # The printed remediation names the exact tracked path the
        # leftover-detection helper computes.
        self.assertIn("ai-specs/skills/skill-creator", out)
        tracked = subprocess.run(
            ["git", "-C", str(project), "ls-files", "ai-specs/skills/skill-creator"],
            capture_output=True, text=True, check=True, input="",
        ).stdout
        self.assertIn("SKILL.md", tracked)
        # TRIAGE: the tracked_bundled_skill_leftovers helper agreement has no
        # CLI-observable surface; the printed remediation guidance plus the
        # surviving git-tracked path observe the same contract.

    def test_harness_skill_commands_match_cli_help(self):
        known = public_cli_commands()
        self.assertIn("sync", known)
        self.assertIn("recipe", known)
        # Only real invocations: `ai-specs <cmd>` or a line starting with ai-specs.
        pattern = re.compile(
            r"(?:`ai-specs\s+([a-z][a-z0-9-]*)|^ai-specs\s+([a-z][a-z0-9-]*))",
            re.MULTILINE,
        )
        unknown: list[str] = []
        for name in HARNESS_SKILLS:
            path = BUNDLED / name / "SKILL.md"
            self.assertTrue(path.is_file(), f"missing {path}")
            for match in pattern.finditer(path.read_text()):
                cmd = match.group(1) or match.group(2)
                if cmd not in known:
                    unknown.append(f"{name}:{cmd}")
        self.assertEqual(unknown, [], f"unknown CLI commands in literacy skills: {unknown}")

    def test_harness_recipes_documents_assisted_configure_contract(self):
        text = (BUNDLED / "harness-recipes" / "SKILL.md").read_text()
        for needle in ("recipe configure", "inspect", "recommend", "approval", "--sync", "preserve", "report"):
            self.assertIn(needle, text.lower(), needle)
        self.assertIn("no-secret", text.lower())

    def test_lifecycle_cross_links_noninteractive_helper(self):
        text = (BUNDLED / "harness-lifecycle" / "SKILL.md").read_text().lower()
        self.assertIn("recipe configure", text)


if __name__ == "__main__":
    unittest.main()
