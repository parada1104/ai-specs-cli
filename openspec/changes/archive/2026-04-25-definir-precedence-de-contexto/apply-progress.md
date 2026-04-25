# Apply Progress: definir-precedence-de-contexto

## Current Status

- Mode: Strict TDD
- Tasks complete: 10/10
- Current phase: apply complete, ready for verify

## TDD Evidence

### RED: context precedence docs/generated output

Command:

```bash
python3 -m unittest tests.test_context_precedence_docs
```

Result: expected failure.

Observed failures:
- `docs/ai/context-precedence.md` is missing.
- `README.md` does not yet point to the context precedence doc.
- Generated `AGENTS.md` does not yet reference `docs/ai/context-precedence.md` or include the compact precedence order.

This confirms tasks 1.1 and 1.2 are RED before implementation.

### GREEN: context precedence docs/generated output

Command:

```bash
python3 -m unittest tests.test_context_precedence_docs
```

Result: PASS, 3 tests.

Covered behavior:
- `docs/ai/context-precedence.md` contains the exact MVP order once, source definitions, conflict examples, audit checklist, and MVP boundary.
- `README.md` points to the canonical doc without duplicating the precedence order.
- `ai-specs sync` renders `AGENTS.md` with the compact order and canonical doc link when the doc exists.

### GREEN: full test suite

Command:

```bash
./tests/run.sh
```

Result: PASS, 22 tests.

### GREEN: validation

Command:

```bash
./tests/validate.sh
```

Result: PASS, 22 tests plus py_compile and bash syntax checks.
