"""Pin the skill-sync consumer to the native `ai-specs skills validate` and
`skills field` verbs.

bundled-skills/skill-sync/assets/sync.sh must validate each SKILL.md through the
native binary (AI_SPECS_GO_BIN) and extract its fields with `skills field`, not
the retired python3 JSON one-liners. A stub binary records its argv, so this
test fails if the script goes back to python3 (mutation check for GO-20 U2e).
"""
from __future__ import annotations

import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SYNC_SH = ROOT / "bundled-skills" / "skill-sync" / "assets" / "sync.sh"

SKILL = """---
name: demo
description: Demo skill.
license: Apache-2.0
metadata:
  author: tests
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Run demo"
---
Body.
"""

STUB = """#!/bin/sh
printf '%s\\n' "$*" >> "$STUB_LOG"
if [ "$STUB_FAIL" = "1" ]; then
  echo '{"error": {"field": "name", "message": "is required", "path": null}}' >&2
  exit 1
fi
if [ "$1 $2" = "skills field" ]; then
  cat > /dev/null
  case "$3" in
    name) echo demo ;;
    scope) echo root ;;
    auto_invoke) echo 'Run demo' ;;
  esac
  exit 0
fi
echo '{"enabled": true, "name": "demo", "scope": ["root"], "auto_invoke": ["Run demo"], "warnings": []}'
"""


class SkillSyncConsumerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="skill-sync-consumer-"))
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)
        # Four levels below the project root, as in the installed cache layout.
        self.proj = self.tmp / "proj"
        self.script = self.proj / ".bundled" / "skills" / "skill-sync" / "assets" / "sync.sh"
        self.script.parent.mkdir(parents=True)
        shutil.copy(SYNC_SH, self.script)
        (self.proj / "ai-specs").mkdir()
        (self.proj / "ai-specs" / "ai-specs.toml").write_text('[project]\nname = "demo"\n')
        self.skill = self.proj / "ai-specs" / "skills" / "demo" / "SKILL.md"
        self.skill.parent.mkdir(parents=True)
        self.skill.write_text(SKILL)
        self.stub = self.tmp / "ai-specs-stub"
        self.stub.write_text(STUB)
        self.stub.chmod(0o755)
        self.log = self.tmp / "argv.log"

    def run_sync(self, fail=False):
        env = dict(os.environ)
        env.pop("AI_SPECS_HOME", None)
        env.update(
            AI_SPECS_GO_BIN=str(self.stub),
            STUB_LOG=str(self.log),
            STUB_FAIL="1" if fail else "0",
        )
        return subprocess.run(
            ["bash", str(self.script)], cwd=self.proj, env=env,
            text=True, capture_output=True, check=False,
        )

    def test_validates_each_skill_through_the_native_verb(self):
        proc = self.run_sync()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("All skills have sync metadata", proc.stdout)
        lines = self.log.read_text().splitlines()
        self.assertRegex(lines[0], r"^skills validate .*ai-specs/skills/demo/SKILL\.md$")
        for expected in ("^skills field name$", "^skills field scope$", "^skills field auto_invoke$"):
            self.assertTrue(
                any(re.search(expected, line) for line in lines),
                f"missing {expected} in {lines}",
            )
        self.assertNotIn("python3", SYNC_SH.read_text())

    def test_invalid_skill_is_reported_as_invalid(self):
        proc = self.run_sync(fail=True)
        self.assertIn("invalid", proc.stdout)
        self.assertIn("is required", proc.stderr)


if __name__ == "__main__":
    unittest.main()
