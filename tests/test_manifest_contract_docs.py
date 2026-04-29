import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
README = ROOT / "README.md"
TEMPLATE = ROOT / "templates" / "ai-specs.toml.tmpl"
REFERENCE = ROOT / "docs" / "ai-specs-toml.md"


class ManifestContractDocsTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.readme = README.read_text()
        cls.template = TEMPLATE.read_text()
        cls.reference = REFERENCE.read_text()

    def assertContainsAll(self, haystack, needles):
        for needle in needles:
            with self.subTest(needle=needle):
                self.assertIn(needle, haystack)

    def test_readme_links_manifest_reference_and_states_stable_surface(self):
        self.assertContainsAll(
            self.readme,
            [
                "`ai-specs/ai-specs.toml` in the project root is the ONLY V1 source of truth.",
                "See [`docs/ai-specs-toml.md`](docs/ai-specs-toml.md) for the complete manifest reference",
                "- `[project]`",
                "- `[[deps]]`",
                "- `[mcp.<name>]`",
                "`[agents]` is still supported as the fan-out selector, but the stable reference focuses on the sections users configure most directly today.",
                "- Missing `[agents]`, `[[deps]]`, and `[mcp]` remain valid and normalize to stable defaults.",
                "- `project.subrepos` remains validated by the existing root target resolver.",
                "- MCP `env` is the canonical field name.",
                "- MCP `environment` is still accepted as a tolerated input alias and normalizes to `env`.",
            ],
        )

    def test_readme_marks_out_of_scope_items_as_deferred(self):
        self.assertContainsAll(
            self.readme,
            [
                "Out of scope for this V1 contract (explicitly deferred to future changes):",
                "- precedence / merge policy beyond the currently implemented runtime behavior",
                "- standalone `[memory]` manifest configuration",
                "- bundle declarations and capability bindings",
                "- SDD/OpenSpec settings as part of the stable TOML reference",
            ],
        )

    def test_template_lists_same_surface_and_every_field_classification(self):
        self.assertContainsAll(
            self.template,
            [
                "# ONLY V1 source of truth del proyecto.",
                "#   [project]       optional section",
                "#   [agents]        optional section (default: enabled = [])",
                "#   [[deps]]        optional repeated section (ONLY required per entry: id, source)",
                "#   [mcp.<name>]    optional repeated section",
                "#   [sdd]           optional — spec-driven / OpenSpec (ver README y `ai-specs sdd --help`)",
                "# Optional section. `name` es opcional; default: \"\".",
                "# Optional field. Default: []. Solo acepta paths relativos válidos al root.",
                "# Optional section. `enabled` default: []. Solo estos reciben configs.",
                "#   id                  required; carpeta destino bajo skills/",
                "#   source              required; URL git (cualquier cosa que acepte `git clone`)",
                "#   path                optional; subdir del repo donde vive SKILL.md",
                "#   scope               optional; default [\"root\"]",
                "#   auto_invoke         optional; frases para la tabla Auto-invoke de AGENTS.md",
                "#   license             optional; SPDX id",
                "#   vendor_attribution  optional; autor/org upstream citado en la description",
                "#   command      optional; string o list",
                "#   args         optional; default []",
                "#   env          optional campo canónico; default {}",
                "#   environment  tolerated input alias de `env`; no nombre canónico",
                "#   timeout      optional; default null",
                "#   enabled      tolerated passthrough; no amplía el contrato V1",
            ],
        )

    def test_template_marks_out_of_scope_items_as_deferred(self):
        self.assertIn("# V1 NO agrega precedence, doctor ni [memory]; quedan deferidos a cambios futuros.", self.template)

    def test_manifest_reference_documents_stable_surface_and_fields(self):
        self.assertContainsAll(
            self.reference,
            [
                "# ai-specs.toml Reference",
                "`ai-specs/ai-specs.toml` in the project root is the only V1 source of truth.",
                "| `[project]` | optional | Project identity and root sync targets. |",
                "| `[agents]` | optional context | Selects which agent outputs receive generated config. |",
                "| `[[deps]]` | optional repeated table | Vendors external skills into `ai-specs/skills/<id>/`. |",
                "| `[mcp.<name>]` | optional repeated table | Declares MCP servers rendered to enabled agents. |",
                "| `project.name` | string | optional | `\"\"` |",
                "| `project.subrepos` | array of strings | optional | `[]` |",
                "| `agents.enabled` | array of strings | optional | `[]` |",
                "Supported values: `claude`, `cursor`, `opencode`, `codex`, `copilot`, `gemini`.",
                "| `deps.id` | string | required | none |",
                "| `deps.source` | string | required | none |",
            ],
        )

    def test_manifest_reference_does_not_document_unstable_manifest_surface(self):
        self.assertNotIn("## `[recipes.<id>]`", self.reference)
        self.assertNotIn("[recipes.runtime-memory-openmemory]", self.reference)
        self.assertNotIn("## `[sdd]`", self.reference)
        self.assertNotIn("artifact_store = \"filesystem\"", self.reference)

    def test_manifest_reference_documents_mcp_env_and_agent_rendering(self):
        self.assertContainsAll(
            self.reference,
            [
                "`env = [\"VAR\"]` references variables from the process environment.",
                "`env = { VAR = \"literal\" }` writes static literal values.",
                "`environment` is accepted as a tolerated input alias for `env`; do not use it as the canonical name.",
                "| Claude Code | `.mcp.json` | `mcpServers` | `env` | `$VAR` becomes `${VAR}` |",
                "| Cursor | `.cursor/mcp.json` | `mcpServers` | `env` | `$VAR` becomes `${VAR}` |",
                "| OpenCode | `opencode.json` | `mcp` | `environment` | `$VAR` becomes `{env:VAR}` |",
                "| Codex | `.codex/config.toml` | `[mcp_servers.<name>]` | `env` | `$VAR` becomes `${VAR}` |",
                "env = [\"TRELLO_API_KEY\", \"TRELLO_TOKEN\"]",
                "env = { MODE = \"local\" }",
            ],
        )


if __name__ == "__main__":
    unittest.main()
