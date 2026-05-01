import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
README = ROOT / "README.md"
TEMPLATE = ROOT / "templates" / "ai-specs.toml.tmpl"
MANIFEST_DOC = ROOT / "docs" / "ai-specs-toml.md"
RECIPE_DOC = ROOT / "docs" / "recipe-schema.md"


class ManifestContractDocsTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.readme = README.read_text()
        cls.template = TEMPLATE.read_text()
        cls.manifest_doc = MANIFEST_DOC.read_text()
        cls.recipe_doc = RECIPE_DOC.read_text()

    def assertContainsAll(self, haystack, needles):
        for needle in needles:
            with self.subTest(needle=needle):
                self.assertIn(needle, haystack)

    def test_manifest_reference_lists_canonical_surface_and_compatibility_rules(self):
        self.assertContainsAll(
            self.manifest_doc,
            [
                "`ai-specs/ai-specs.toml` in the project root is the ONLY V1 source of truth.",
                "Omission of `[sdd]` remains valid for projects not using SDD.",
                "- `[project]`",
                "- `[agents]`",
                "- `[[deps]]`",
                "- `[mcp.<name>]`",
                "- `[recipes.<id>]`",
                "- `[recipes.<id>.config]`",
                "- `[[bindings]]`",
                "- `[sdd]` (optional)",
                "- Missing `[agents]`, `[[deps]]`, and `[mcp]` remain valid and normalize to stable defaults.",
                "- `project.subrepos` remains validated by the existing root target resolver.",
                "- MCP `env` is the canonical field name.",
                "- MCP `environment` is still accepted as a tolerated input alias and normalizes to `env`.",
            ],
        )

    def test_manifest_reference_lists_every_v1_field_classification_row(self):
        self.assertContainsAll(
            self.manifest_doc,
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
                "| `[recipes.<id>]` | `enabled` | required; boolean - must be `true` to materialize |",
                "| `[recipes.<id>]` | `version` | required; exact string matching `recipe.toml` version |",
                "| `[recipes.<id>.config]` | `<key> = <value>` | optional per-recipe overrides; unknown keys warn and are ignored |",
                "| `[[bindings]]` | `capability`, `recipe` | optional explicit capability binding |",
                "| `[sdd]` | `enabled`, `provider`, `artifact_store` | optional; `provider` = `openspec` in v1 |",
            ],
        )

    def test_manifest_reference_marks_out_of_scope_items_as_deferred(self):
        self.assertContainsAll(
            self.manifest_doc,
            [
                "Out of scope for this V1 contract (explicitly deferred to future changes):",
                "- precedence / merge policy beyond the currently implemented runtime behavior",
                "- `[memory]` (distinct from `[sdd].artifact_store = memory`)",
            ],
        )

    def test_readme_links_to_dedicated_manifest_and_recipe_references(self):
        self.assertContainsAll(
            self.readme,
            [
                "[`docs/ai-specs-toml.md`](docs/ai-specs-toml.md)",
                "[`docs/recipe-schema.md`](docs/recipe-schema.md)",
                "[`docs/ai/sdd.md`](docs/ai/sdd.md)",
                "Use the README for overview and navigation.",
            ],
        )

    def test_recipe_reference_covers_current_v2_contract_and_boundaries(self):
        self.assertContainsAll(
            self.recipe_doc,
            [
                "[`docs/ai-specs-toml.md`](ai-specs-toml.md)",
                "## `[sdd]` recipe metadata",
                "| `threshold` | string | no | Optional ceremony level: `trivial`, `local_fix`, `behavior_change`, or `domain_change` |",
                "Missing `required` causes a validation error.",
                "The current validator treats\n`type` as descriptive metadata",
                "| `condition` | string | no | `\"not_exists\"` (default) — skip if target already exists |",
                "Only `source` and `target` are part of the supported docs contract.",
                "## Reference recipe: `trello-mcp-workflow`",
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


if __name__ == "__main__":
    unittest.main()
