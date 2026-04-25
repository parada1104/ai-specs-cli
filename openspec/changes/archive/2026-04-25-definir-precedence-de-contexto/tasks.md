## 1. Test Coverage Foundation

- [x] 1.1 Add RED documentation tests proving `docs/ai/context-precedence.md` contains the exact MVP order, source-class definitions, conflict examples, and documentation-first boundary.
- [x] 1.2 Add RED generated-output coverage proving `ai-specs sync` renders an `AGENTS.md` reference to `docs/ai/context-precedence.md` and includes the compact precedence order.
- [x] 1.3 Run the targeted new tests before implementation and capture the expected failures in apply progress.

## 2. Canonical Documentation

- [x] 2.1 Create `docs/ai/context-precedence.md` with the normative source order, source definitions, conflict examples, and audit checklist.
- [x] 2.2 Add a concise README pointer to the canonical context precedence doc without duplicating the full rule as a second source of truth.

## 3. Generated Agent Instructions

- [x] 3.1 Update `lib/_internal/agents-md-render.py` so generated `AGENTS.md` includes a compact context precedence section referencing `docs/ai/context-precedence.md`.
- [x] 3.2 Regenerate root generated artifacts with the normal sync flow so `AGENTS.md` reflects the renderer change.

## 4. Verification

- [x] 4.1 Run targeted tests for context precedence documentation and generated output, then fix failures tied to this change.
- [x] 4.2 Run `./tests/run.sh` and fix failures tied to this change.
- [x] 4.3 Run `./tests/validate.sh` and record the result.
