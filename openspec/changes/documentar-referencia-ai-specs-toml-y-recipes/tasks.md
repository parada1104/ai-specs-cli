# Tasks: `documentar-referencia-ai-specs-toml-y-recipes`

## Phase 1: Source audit and doc contract framing

- [x] 1.1 Review current manifest and recipe contract sources in `README.md`, `docs/recipe-schema.md`, `templates/ai-specs.toml.tmpl`, `lib/_internal/toml-read.py`, and `lib/_internal/recipe_schema.py` to lock the current documented behavior.
- [x] 1.2 Review `tests/test_manifest_contract_docs.py` and related doc-contract coverage to identify which assertions should move from README-level contract text into dedicated reference docs.
- [x] 1.3 If the audit reveals a required runtime semantics change in `lib/_internal/toml-read.py` or `lib/_internal/recipe_schema.py`, stop implementation work and record it as an escalation instead of planning code changes in this change.

## Phase 2: Dedicated reference docs

- [x] 2.1 Create `docs/ai-specs-toml.md` as the dedicated canonical reference for the `ai-specs.toml` manifest, covering supported sections, field classifications, defaults, tolerated aliases, and explicit out-of-scope items.
- [x] 2.2 Refine `docs/recipe-schema.md` so it matches the existing recipe schema, terminology, and constraints implemented by the current code and specs.
- [x] 2.3 Ensure both reference docs use consistent wording for canonical fields, defaults, validation boundaries, and deferred items.

## Phase 3: README consolidation

- [x] 3.1 Update `README.md` to link to `docs/ai-specs-toml.md` and `docs/recipe-schema.md` as the dedicated references.
- [x] 3.2 Reduce duplicated manifest and recipe contract detail in `README.md` while keeping the essential overview and navigation needed for first-time readers.
- [x] 3.3 Preserve any README statements that must remain as top-level product guidance, but avoid maintaining duplicate contract tables or field-by-field definitions there.

## Phase 4: Doc-contract tests

- [x] 4.1 Extend `tests/test_manifest_contract_docs.py` to assert the new dedicated manifest reference exists and carries the required contract surface.
- [x] 4.2 Update doc-contract assertions so README checks focus on links and high-level guidance, while detailed contract assertions target `docs/ai-specs-toml.md` and `docs/recipe-schema.md`.
- [x] 4.3 Add or adjust recipe documentation assertions so the canonical recipe reference stays aligned with current schema behavior and documented constraints.

## Phase 5: Verification

- [x] 5.1 Run the focused doc-contract tests, primarily `tests/test_manifest_contract_docs.py`, and fix any failures caused by the documentation split.
- [x] 5.2 Run `./tests/run.sh` and resolve any regressions related to the changed docs or contract assertions.
- [x] 5.3 Run `./tests/validate.sh` and confirm the full validation suite remains green for the completed change.
