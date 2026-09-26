"""End-to-end contract test for the standalone Jinna consumer recipe flow.

This codifies the already-shipped product behavior so a future recipe or
materialization change cannot silently regress the isolated consumer path:

    ai-specs init --no-tui
    ai-specs recipe add jinna-mcp-recipe
    ai-specs sync
    ai-specs doctor

The fixture stays network-free and self-contained:

* the consumer project lives in a temporary directory outside the repository;
* ``AI_SPECS_HOME`` points at a temporary CLI home whose catalog/lib/bundled
  assets are symlinks back to this worktree, so no durable state is written;
* a fake ``jinna`` executable is first on ``PATH`` and answers only
  ``jinna version``. Any other invocation (notably ``jinna mcp``) fails and is
  recorded, proving the passive lifecycle never starts the provider server.

The behavior under test already ships, so the first run of this test is the
baseline GREEN: it exists to freeze the contract, not to drive a product fix.
"""
from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path


sys.path.insert(0, str(Path(__file__).resolve().parent))
from _cache_paths import resolved_skills_dir

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
RECIPE_ID = "jinna-mcp-recipe"
JINNA_SKILL_ID = "jinna-mcp-recipe"
DOGFOOD_MANIFEST = ROOT / "ai-specs" / "ai-specs.toml"

# Directories the CLI resolves from AI_SPECS_HOME (symlinked from this worktree).
CLI_HOME_LINKS = ("catalog", "lib", "bundled-skills", "bundled-commands", "templates")

OPENPROJECT_ENV = {
    "OPENPROJECT_BASE_URL": "https://openproject.example.com",
    "OPENPROJECT_API_TOKEN": "dummy-token",
    "OPENPROJECT_AUTH": "basic",
}
JINNA_ENV_NAMES = tuple(OPENPROJECT_ENV)

FAKE_JINNA_SCRIPT = """#!/usr/bin/env bash
printf '%s\\n' "$1" >> "$JINNA_FAKE_LOG"
if [ "${1:-}" = "version" ]; then
  echo "jinna version 0.1.0"
  exit 0
fi
echo "unexpected jinna invocation: $*" >&2
exit 3
"""


class JinnaConsumerRecipeFlowTests(unittest.TestCase):
    def test_standalone_consumer_lifecycle_materializes_local_jinna_mcp(self):
        dogfood_before = DOGFOOD_MANIFEST.read_bytes()

        with tempfile.TemporaryDirectory(prefix="jinna-consumer-") as tmp:
            tmp = Path(tmp)
            consumer = tmp / "consumer"
            consumer.mkdir()
            self.assertFalse(
                consumer.is_relative_to(ROOT),
                "the consumer project must live outside the repository",
            )

            cli_home = tmp / "cli-home"
            cli_home.mkdir()
            for name in CLI_HOME_LINKS:
                os.symlink(ROOT / name, cli_home / name, target_is_directory=True)

            bin_dir = tmp / "bin"
            bin_dir.mkdir()
            invocation_log = tmp / "jinna-invocations.log"
            fake_jinna = bin_dir / "jinna"
            fake_jinna.write_text(FAKE_JINNA_SCRIPT, encoding="utf-8")
            fake_jinna.chmod(0o755)

            env = self._subprocess_env(cli_home, bin_dir, invocation_log)

            for command in (
                [str(CLI), "init", "--no-tui"],
                [str(CLI), "recipe", "add", RECIPE_ID],
                [str(CLI), "sync"],
                [str(CLI), "doctor"],
            ):
                result = subprocess.run(
                    command,
                    cwd=consumer,
                    env=env,
                    capture_output=True,
                    text=True,
                    timeout=300,
                )
                self.assertEqual(
                    result.returncode,
                    0,
                    f"{' '.join(command)} exited {result.returncode}\n"
                    f"--- stdout ---\n{result.stdout}\n--- stderr ---\n{result.stderr}",
                )

            manifest = tomllib.loads(
                (consumer / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
            )
            self.assertTrue(
                manifest["recipes"][RECIPE_ID]["enabled"],
                "the consumer manifest must enable the jinna recipe",
            )

            readme = consumer / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
            self.assertTrue(readme.is_file(), "the recipe README was not materialized")
            self.assertTrue(readme.read_text(encoding="utf-8").strip())

            skill = resolved_skills_dir(consumer, cli_home=cli_home) / JINNA_SKILL_ID / "SKILL.md"
            self.assertTrue(skill.is_file(), "the Jinna recipe skill was not materialized")

            self._assert_mcp_json(consumer / ".mcp.json")
            self._assert_mcp_json(consumer / ".cursor" / "mcp.json")
            self._assert_opencode(consumer / "opencode.json")

            logged = invocation_log.read_text(encoding="utf-8") if invocation_log.exists() else ""
            invocations = logged.split()
            self.assertTrue(invocations, "the fake jinna version check never ran")
            self.assertEqual(
                set(invocations),
                {"version"},
                "the passive lifecycle must only ask the provider for its version",
            )

        self.assertEqual(
            DOGFOOD_MANIFEST.read_bytes(),
            dogfood_before,
            "the isolated consumer flow must not mutate this worktree's manifest",
        )

    @staticmethod
    def _subprocess_env(cli_home: Path, bin_dir: Path, invocation_log: Path) -> dict[str, str]:
        env = dict(os.environ)
        env.update(OPENPROJECT_ENV)
        env["AI_SPECS_HOME"] = str(cli_home)
        env["JINNA_FAKE_LOG"] = str(invocation_log)
        env["PATH"] = f"{bin_dir}{os.pathsep}{env.get('PATH', '')}"
        return env

    def _assert_mcp_json(self, path: Path) -> None:
        self.assertTrue(path.is_file(), f"{path} was not materialized")
        server = json.loads(path.read_text(encoding="utf-8"))["mcpServers"]["jinna"]
        self.assertEqual(server["command"], "jinna")
        self.assertEqual(server["args"], ["mcp"])
        self.assertEqual(server["timeout"], 30000)
        for name in JINNA_ENV_NAMES:
            self.assertEqual(server["env"][name], f"${{{name}}}")

    def _assert_opencode(self, path: Path) -> None:
        self.assertTrue(path.is_file(), f"{path} was not materialized")
        server = json.loads(path.read_text(encoding="utf-8"))["mcp"]["jinna"]
        self.assertEqual(server["command"], ["jinna", "mcp"])
        self.assertEqual(server["timeout"], 30000)
        for name in JINNA_ENV_NAMES:
            self.assertEqual(server["environment"][name], f"{{env:{name}}}")


if __name__ == "__main__":
    unittest.main()
