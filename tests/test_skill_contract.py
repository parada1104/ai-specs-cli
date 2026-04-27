import importlib.util
import json
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "lib" / "_internal" / "skill_contract.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class SkillContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(MODULE_PATH, "skill_contract_internal")

    def write_skill(self, tmp: Path, content: str) -> Path:
        path = tmp / "SKILL.md"
        path.write_text(content)
        return path

    def test_parses_canonical_local_skill(self):
        with tempfile.TemporaryDirectory() as tmp:
            skill_path = self.write_skill(
                Path(tmp),
                "---\n"
                "name: local-demo\n"
                "description: >\n"
                "  Demo local skill. Trigger: When syncing locally.\n"
                "license: Apache-2.0\n"
                "metadata:\n"
                "  author: fixture-suite\n"
                "  version: \"1.0\"\n"
                "  scope: [root]\n"
                "  auto_invoke:\n"
                "    - \"Sync local metadata\"\n"
                "---\n\n"
                "# Demo\n",
            )

            skill = self.mod.from_local_skill(skill_path)

            self.assertEqual(skill["name"], "local-demo")
            self.assertEqual(skill["description_summary"], "Demo local skill")
            self.assertEqual(skill["license"], "Apache-2.0")
            self.assertEqual(skill["metadata"]["author"], "fixture-suite")
            self.assertEqual(skill["metadata"]["scope"], ["root"])
            self.assertEqual(skill["metadata"]["auto_invoke"], ["Sync local metadata"])

    def test_compatibility_mode_normalizes_scalar_auto_invoke_and_missing_fields(self):
        with tempfile.TemporaryDirectory() as tmp:
            skill_path = self.write_skill(
                Path(tmp),
                "---\n"
                "name: legacy-skill\n"
                "description: Legacy skill.\n"
                "metadata:\n"
                "  scope: [root]\n"
                "  auto_invoke: \"Legacy trigger\"\n"
                "---\n",
            )

            skill = self.mod.from_local_skill(skill_path, compatibility=True)

            self.assertEqual(skill["license"], "Apache-2.0")
            self.assertEqual(skill["metadata"]["author"], "unknown")
            self.assertEqual(skill["metadata"]["version"], "1.0")
            self.assertEqual(skill["metadata"]["auto_invoke"], ["Legacy trigger"])
            self.assertTrue(skill["warnings"])

    def test_from_dep_generates_canonical_provenance_metadata(self):
        dep = {
            "id": "vendored-demo",
            "source": "https://example.com/demo.git",
            "scope": ["root"],
            "auto_invoke": ["Sync vendored metadata"],
            "license": "MIT",
            "vendor_attribution": "example-org",
        }
        upstream_text = (
            "---\n"
            "name: upstream-demo\n"
            "description: >\n"
            "  Upstream description. Trigger: Anything upstream.\n"
            "---\n\n"
            "# Upstream\n"
        )

        skill = self.mod.from_dep(dep, upstream_text)
        rendered = self.mod.render_skill_markdown(skill)

        self.assertEqual(skill["name"], "vendored-demo")
        self.assertEqual(skill["metadata"]["source"], dep["source"])
        self.assertEqual(skill["metadata"]["vendor_attribution"], "example-org")
        self.assertEqual(skill["metadata"]["author"], "example-org")
        self.assertIn('vendor_attribution: "example-org"', rendered)
        self.assertIn('source: "https://example.com/demo.git"', rendered)
        self.assertIn("# Upstream", rendered)

    def test_sync_metadata_requires_scope_and_auto_invoke_together(self):
        with tempfile.TemporaryDirectory() as tmp:
            skill_path = self.write_skill(
                Path(tmp),
                "---\n"
                "name: broken-sync\n"
                "description: Broken sync metadata.\n"
                "license: Apache-2.0\n"
                "metadata:\n"
                "  author: fixture-suite\n"
                "  version: \"1.0\"\n"
                "  scope: [root]\n"
                "---\n",
            )

            skill = self.mod.from_local_skill(skill_path)
            with self.assertRaises(self.mod.SkillContractError) as ctx:
                self.mod.validate_sync_metadata(skill, path=skill_path)

            self.assertEqual(ctx.exception.field, "metadata.auto_invoke")
            self.assertIn("required", ctx.exception.message)

    def test_canonical_local_skill_without_sync_metadata_remains_valid(self):
        with tempfile.TemporaryDirectory() as tmp:
            skill_path = self.write_skill(
                Path(tmp),
                "---\n"
                "name: local-docs\n"
                "description: Local docs helper.\n"
                "license: Apache-2.0\n"
                "metadata:\n"
                "  author: fixture-suite\n"
                "  version: \"1.0\"\n"
                "---\n\n"
                "# Docs Helper\n",
            )

            skill = self.mod.from_local_skill(skill_path)

            self.assertEqual(skill["name"], "local-docs")
            self.assertEqual(skill["license"], "Apache-2.0")
            self.assertEqual(skill["metadata"]["author"], "fixture-suite")
            self.assertEqual(skill["metadata"]["version"], "1.0")
            self.assertNotIn("scope", skill["metadata"])
            self.assertNotIn("auto_invoke", skill["metadata"])
            self.assertEqual(skill["warnings"], [])

    def test_cli_sync_metadata_returns_json_payload(self):
        with tempfile.TemporaryDirectory() as tmp:
            skill_path = self.write_skill(
                Path(tmp),
                "---\n"
                "name: cli-demo\n"
                "description: CLI demo.\n"
                "license: Apache-2.0\n"
                "metadata:\n"
                "  author: fixture-suite\n"
                "  version: \"1.0\"\n"
                "  scope: [root]\n"
                "  auto_invoke:\n"
                "    - \"Run CLI metadata\"\n"
                "---\n",
            )

            proc = subprocess.run(
                ["python3", str(MODULE_PATH), "sync-metadata", str(skill_path)],
                text=True,
                capture_output=True,
                check=False,
            )

            self.assertEqual(proc.returncode, 0, proc.stderr)
            payload = json.loads(proc.stdout)
            self.assertTrue(payload["enabled"])
            self.assertEqual(payload["name"], "cli-demo")


if __name__ == "__main__":
    unittest.main()
