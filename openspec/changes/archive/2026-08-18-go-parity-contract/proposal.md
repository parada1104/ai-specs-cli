# Parity contract + CLI surface matrix

## Tracker

- **card_id**: `6a84e758a52c749278855a14`
- **shortLink**: `wUldHID4`
- **url**: https://trello.com/c/wUldHID4
- **list**: In Progress
- **epic**: https://trello.com/c/qwlHQ7Xa

## Planning depth

**tasks-only.** This is a read-only investigation whose entire deliverable is one
committed document. It writes no production code, changes no behavior, and adds
no runtime surface, so a spec and a design would restate the document itself.

## Intent

Produce the authoritative inventory of the CLI's observable surface, so the Go
migration is verified against a written contract instead of an implicit one
spread across 103 test files, 19 shell dispatchers and 29 Python modules.

## Scope

- Enumerate all 14 subcommands: flags, exit codes, stdout/stderr contracts,
  TTY branching, filesystem effects, environment variables, ordering guarantees.
- Classify every surface FROZEN / TOLERANT / FREE with a rationale.
- Record behavioral inconsistencies as defects to be filed separately, never
  normalized silently during the port.
- Conclude with a written go/no-go.

## Out of scope

- Writing any Go code.
- Fixing any recorded defect. Each becomes its own card.

## Outcome

**GO.** Delivered as `docs/go-migration-parity-contract.md`. Three findings
materially change the epic plan:

1. A whole-document Go TOML library is not viable for manifest writes — five
   write paths operate on text and preserve comments, ordering and untouched
   bytes. This settles the ADR that card [Go 05] was going to open.
2. `sync.sh` extracts structured data from `recipe-materialize.py`'s
   human-readable stdout by grep. Cosmetic output changes break the pipeline
   silently while both implementations coexist.
3. Three commands documented as read-only are not: `doctor`, `refresh-bundled`
   and `hub`.

35 defects recorded (D1–D35), 4 of them data-loss class.

## Branch and merge policy

Per the epic contract: this change branches from `epic/go-single-binary` and its
PR targets `epic/go-single-binary`. It never targets `development` or `main`.
