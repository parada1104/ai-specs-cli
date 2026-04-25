import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
README = ROOT / "README.md"
TEMPLATE = ROOT / "templates" / "ai-specs.toml.tmpl"


class ManifestContractDocsTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.readme = README.read_text()
        cls.template = TEMPLATE.read_text()

    def assertContainsAll(self, haystack, needles):
        for needle in needles:
            with self.subTest(needle=needle):
                self.assertIn(needle, haystack)

    def test_readme_lists_entire_canonical_surface_and_compatibility_rules(self):
        self.assertContainsAll(
            self.readme,
            [
                "`ai-specs/ai-specs.toml` in the project root is the ONLY V1 source of truth.",
                "No other manifest sections are part of the canonical V1 surface for this change.",
                "- `[project]`",
                "- `[agents]`",
                "- `[[deps]]`",
                "- `[mcp.<name>]`",
                "- Missing `[agents]`, `[[deps]]`, and `[mcp]` remain valid and normalize to stable defaults.",
                "- `project.subrepos` remains validated by the existing root target resolver.",
                "- MCP `env` is the canonical field name.",
                "- MCP `environment` is still accepted as a tolerated input alias and normalizes to `env`.",
            ],
        )

    def test_readme_lists_every_v1_field_classification_row(self):
        self.assertContainsAll(
            self.readme,
            [
                "| `[project]` | `name` | optional, default `\"\"` |",
                "| `[project]` | `subrepos` | optional, default `[]`, validated as root-relative target paths |",
                "| `[agents]` | `enabled` | optional, default `[]` |",
                "| `[[deps]]` | `id`, `source` | only required minimum fields |",
                "| `[[deps]]` | `path`, `scope`, `auto_invoke`, `license`, `vendor_attribution` | optional passthrough fields consumed by vendoring/rendering |",
                "| `[mcp.<name>]` | `command` | optional |",
                "| `[mcp.<name>]` | `args` | optional, default `[]` |",
                "| `[mcp.<name>]` | `env` | optional canonical field, default `{}` |",
                "| `[mcp.<name>]` | `environment` | tolerated input alias of `env` |",
                "| `[mcp.<name>]` | `timeout` | optional |",
                "| `[mcp.<name>]` | `enabled` | tolerated passthrough field |",
            ],
        )

    def test_readme_marks_out_of_scope_items_as_deferred(self):
        self.assertContainsAll(
            self.readme,
            [
                "Out of scope for this V1 contract (explicitly deferred to future changes):",
                "- precedence / merge policy beyond the currently implemented runtime behavior",
                "- `ai-specs doctor`",
                "- `[memory]`",
                "- introducing new manifest sections",
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
                "# Ninguna otra sección forma parte de la superficie canónica V1 en este cambio.",
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


if __name__ == "__main__":
    unittest.main()
