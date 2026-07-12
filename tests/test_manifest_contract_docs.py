import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
README = ROOT / "README.md"
TEMPLATE = ROOT / "templates" / "ai-specs.toml.tmpl"
MANIFEST_DOC = ROOT / "docs" / "ai-specs-toml.md"
RECIPE_DOC = ROOT / "docs" / "recipe-schema.md"
GENERATED_TOML = ROOT / "ai-specs" / "ai-specs.toml"


def _extract_section_keys(text):
    """Extract ordered top-level TOML section keys from manifest text.

    Returns a list of first-component keys like ['project', 'agents', 'deps', ...].
    Handles both active and commented-out sections.
    """
    keys = []
    for line in text.splitlines():
        stripped = line.strip()
        # Match commented sections: # [project], # [[deps]], # [mcp.*]
        cmt = re.match(r'^#\s*\[\[?(\w+)', stripped)
        if cmt:
            key = cmt.group(1)
            if key not in keys:
                keys.append(key)
            continue
        # Match active sections: [project], [[deps]], [mcp.trello]
        active = re.match(r'^\[\[?(\w+)', stripped)
        if active:
            key = active.group(1)
            if key not in keys:
                keys.append(key)
    return keys


class ManifestContractDocsTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.readme = README.read_text()
        cls.template = TEMPLATE.read_text()
        cls.manifest_doc = MANIFEST_DOC.read_text()
        cls.recipe_doc = RECIPE_DOC.read_text()
        cls.generated_toml = GENERATED_TOML.read_text()

    def assertContainsAll(self, haystack, needles):
        for needle in needles:
            with self.subTest(needle=needle):
                self.assertIn(needle, haystack)

    def test_manifest_reference_lists_canonical_surface_and_compatibility_rules(self):
        self.assertContainsAll(
            self.manifest_doc,
            [
                "`ai-specs/ai-specs.toml` in the project root is the ONLY V1 source of truth.",
                "- `[project]`",
                "- `[agents]`",
                "- `[[deps]]`",
                "- `[mcp.<name>]`",
                "- `[recipes.<id>]`",
                "- `[recipes.<id>.config]`",
                "- `[[bindings]]`",
                "- `[tool]`",
                "- Missing `[agents]`, `[[deps]]`, and `[mcp]` remain valid and normalize to stable defaults.",
                "- `project.subrepos` remains validated by the existing root target resolver.",
                "- MCP `env` is the canonical field name.",
                "- MCP `environment` is still accepted as a tolerated input alias and normalizes to `env`.",
            ],
        )
        self.assertNotIn("[sdd]", self.generated_toml)
        self.assertNotIn("[sdd]", self.template)

    def test_manifest_reference_lists_every_v1_field_classification_row(self):
        self.assertContainsAll(
            self.manifest_doc,
            [
                "| `[project]` | `name` | optional, default `\"\"` |",
                "| `[project]` | `subrepos` | optional, default `[]`, validated as root-relative target paths |",
                "| `[agents]` | `enabled` | optional, default `[]` |",
                "| `[[deps]]` | `id`, `source` | only required minimum fields |",
                "| `[[deps]]` | `path`, `scope`, `auto_invoke`, `license`, `vendor_attribution`, `version` | optional passthrough fields consumed by vendoring/rendering |",
                "| `[mcp.<name>]` | `command` | optional |",
                "| `[mcp.<name>]` | `args` | optional, default `[]` |",
                "| `[mcp.<name>]` | `env` | optional canonical field, default `{}` |",
                "| `[mcp.<name>]` | `environment` | tolerated input alias of `env` |",
                "| `[mcp.<name>]` | `timeout` | optional |",
                "| `[mcp.<name>]` | `enabled` | tolerated passthrough field |",
                "| `[recipes.<id>]` | `enabled` | required; boolean — must be `true` to materialize |",
                "| `[recipes.<id>]` | `version` | required; exact string matching `recipe.toml` version |",
                "| `[recipes.<id>.config]` | `<key> = <value>` | optional per-recipe overrides; unknown keys warn and are ignored |",
                "| `[[bindings]]` | `capability`, `recipe` | optional explicit capability binding |",
                "| `[tool]` | `version` | optional; exact CLI version pin (semver) |",
                "| `[tool]` | `min_version` | optional; minimum acceptable CLI version (semver); mutually exclusive with `version` |",
                "| `[tool]` | `policy` | optional; `exact` or `min` (inferred from which version field is set) |",
            ],
        )

    def test_manifest_reference_marks_out_of_scope_items_as_deferred(self):
        self.assertContainsAll(
            self.manifest_doc,
            [
                "Out of scope for this V1 contract (explicitly deferred to future changes):",
                "- precedence / merge policy beyond the currently implemented runtime behavior",
            ],
        )

    def test_readme_links_to_dedicated_manifest_and_recipe_references(self):
        self.assertContainsAll(
            self.readme,
            [
                "[`docs/ai-specs-toml.md`](docs/ai-specs-toml.md)",
                "[`docs/recipe-schema.md`](docs/recipe-schema.md)",
                "[`docs/mcp-distribution.md`](docs/mcp-distribution.md)",
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

    def test_recipe_doc_does_not_contain_manifest_v2_additions(self):
        # Manifest-level bindings and config overrides belong in ai-specs-toml.md
        self.assertNotIn("## Manifest V2 Additions", self.recipe_doc)
        self.assertNotIn("## `[[bindings]]` table", self.recipe_doc)
        self.assertNotIn("## `[recipes.<id>.config]` override syntax", self.recipe_doc)

    def test_template_lists_same_surface_and_every_field_classification(self):
        self.assertContainsAll(
            self.template,
            [
                "# This is the ONLY V1 source of truth for the project.",
                "#   [project]       optional section",
                "#   [agents]        optional section (default: enabled = [])",
                "#   [[deps]]        optional repeated section (required per entry: id, source)",
                "#   [mcp.<name>]    optional repeated section",
                "#   [[bindings]]    optional repeated section (per entry: capability, recipe)",
                "#   [recipes.<id>]  optional — named bundles of skills, commands, templates,",
                "# name is optional; default: \"\"",
                "# subrepos is optional; default: []. Accepts root-relative paths only.",
                "# enabled is optional; default: []. Only these agents receive configs.",
                "#   id                  (req) target directory under skills/",
                "#   source              (req) git URL (anything `git clone` accepts)",
                "#   path                (opt) subdirectory inside the repo where SKILL.md lives",
                "#   scope               (opt) default [\"root\"]",
                "#   auto_invoke         (opt) trigger phrases for the AGENTS.md Auto-invoke table",
                "#   license             (opt) SPDX identifier",
                "#   vendor_attribution  (opt) upstream author/org, cited in skill description",
                "#   version             (opt) semantic version for metadata.version",
                "#   capability         (req) capability ID to bind",
                "#   recipe             (req) recipe ID that provides the capability",
                "#   command      optional; string or list",
                "#   args         optional; default []",
                "#   env          optional canonical field; default {}",
                "#   environment  tolerated input alias of `env`; not the canonical name",
                "#   timeout      optional; default null",
                "#   enabled      tolerated passthrough; does not extend the V1 contract",
            ],
        )

    def test_template_surface_does_not_claim_stale_deferrals(self):
        # The template no longer claims doctor/memory are deferred (both
        # are implemented); it also omits [sdd] from the surface listing.
        self.assertNotIn(
            "# V1 NO agrega precedence, doctor ni [memory]; quedan deferidos a cambios futuros.",
            self.template,
        )
        self.assertNotIn("deferidos", self.template)
        self.assertNotIn("#   [sdd]", self.template)

    def test_template_is_subset_of_generated_toml(self):
        template_keys = _extract_section_keys(self.template)
        generated_keys = _extract_section_keys(self.generated_toml)

        # Every section type in the template must appear in the generated TOML
        for key in template_keys:
            with self.subTest(section=key):
                self.assertIn(key, generated_keys,
                    f"Template section '{key}' not found in generated ai-specs.toml")

        # Verify same relative order for shared keys
        template_order = {k: i for i, k in enumerate(template_keys)}
        generated_order = {k: i for i, k in enumerate(generated_keys)}
        shared = sorted(
            [k for k in template_keys if k in generated_keys],
            key=lambda k: template_order[k],
        )
        for i in range(len(shared) - 1):
            a, b = shared[i], shared[i + 1]
            with self.subTest(order=f"{a} → {b}"):
                self.assertLess(
                    generated_order[a], generated_order[b],
                    f"Order mismatch: '{a}' should come before '{b}' in generated TOML",
                )


if __name__ == "__main__":
    unittest.main()
