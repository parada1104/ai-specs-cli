# ADR 0002 — TOML handling: purpose-built reader + line/segment editor

- **Status**: Accepted
- **Card**: [Go 05] Port the config layer: TOML, manifest, lock, schema (Go single-binary migration epic)
- **Context**: root Go module `ai-specs.dev/ai-specs`, go 1.24.13, zero third-party dependencies

## Context

The config layer reads and writes TOML in four places:

1. `lib/_internal/toml-read.py` — parses `ai-specs.toml` with stdlib `tomllib`
   and exposes normalized sections as JSON.
2. Manifest writes — five independent paths (parity contract §3): `recipe add`
   (literal append), `recipe configure` (line surgery), `recipe remove` /
   `skills remove` (segment delete by text), `skills add` (literal append via
   a hand-rolled serializer, `lib/_internal/toml_write.py:toml_value`).
3. `lib/_internal/recipe_schema.py` — parses `recipe.toml` with `tomllib` and
   validates it.
4. `lib/_internal/lock.py` — reads the lock with `tomllib`; writes it with a
   hand-rolled serializer (`_toml_string` escapes only `\` and `"`).

The manifest is a committed, user-owned file: `docs/go-migration-parity-contract.md`
§3 classifies manifest write formatting as **FROZEN** and rules out any Go TOML
library that marshals the whole document — reordering tables, dropping
comments, or re-quoting values is a parity failure. The zero-dependency
posture (ADR 0001) rules out vendoring a TOML library; the Go standard library
has no TOML parser.

## Decision

Two purpose-built components under the root module, no third-party code:

1. **A TOML value parser** (`internal/toml`) — a line-oriented reader for the
   TOML 1.0 subset that ai-specs manifests, recipe manifests, and lock files
   actually use: tables and array-of-tables, dotted and quoted keys, basic /
   literal / multi-line strings, integers (dec/hex/oct/bin), floats, booleans,
   arrays (including multi-line), and inline tables. Line endings follow
   tomllib's empirically pinned semantics: `\r\n` is accepted as a line ending
   everywhere (key/value pairs, table headers, comments, blank lines, and
   multi-line strings), a lone `\r` is rejected everywhere, and CRLF inside
   multi-line string values is normalized to `\n`, as tomllib does. It is a
   *reader only*:
   nothing in the ported layer ever re-serializes a parsed document, so
   round-trip fidelity of the writer question does not arise. Datetime values
   are out of the subset: ai-specs manifests never contain them, and Python's
   `json.dumps` cannot serialize the `datetime` objects `tomllib` returns, so
   a manifest with a datetime fails in both implementations.
2. **A line/segment editor** (`internal/config`) — the write surface is
   surgical text operations over raw file bytes, mirroring the Python heredocs:
   - append a rendered block at EOF (`skills add` pattern, values rendered by
     the `toml_value` port);
   - delete whole segments split on lines whose first character at column 0 is
     `[` (`recipe remove` / `skills remove` pattern), with the segment-matching
     regexes ported exactly;
   - parse-validate-and-restore: when the original file parses, the result
     must parse too or the write is refused and the original bytes stay
     untouched;
   - atomic replace (temp file in the target's directory + rename) preserving
     the original file mode;
   - universal-newline read translation: the write ops replicate Python
     `Path.read_text` semantics, so `\r\n` and lone `\r` become `\n` in memory
     and a CRLF manifest round-trips LF-normalized, matching the heredocs;
     guard-refused writes leave the original bytes untouched.

Rationale:

1. **The write surface is text, not a document.** All three write families in
   §3 operate on lines: placeholder comments (`= ""  # REQUIRED`), preserved
   indentation and inline comments with their exact pre-`#` space runs, and
   segment deletion that must tolerate a manifest that is not currently valid
   TOML. No marshaller can reproduce these; a line editor reproduces them
   trivially by construction.
2. **Round-trip fidelity is structural, not incidental.** Because the editor
   never re-serializes, untouched bytes are preserved by definition — the
   strongest possible round-trip guarantee.
3. **The reader only needs the manifest subset.** A full TOML 1.0
   implementation is justified only if something re-serializes; nothing does.
   The parser mirrors `tomllib`'s observable value types for the subset above,
   which is what every differential consumer (section JSON, schema
   validation, lock load) actually sees.
4. **Zero deps is the existing posture** (ADR 0001); `github.com/pelletier/go-toml`
   or `BurntSushi/toml` would both violate it *and* fail §3, since they are
   document marshallers at heart.
5. **Lock writes already have a Go authority.** `catalog/recipes/worktree-flow/gate/lockwrite.go`
   is the byte-exact port of `lock.py`'s writer; the root-module `internal/lock`
   port mirrors its conventions (same header constant, same escaping, same
   section order, same control-character refusal) so the two stay aligned.

## Consequences

- `tomllib` parse-*error* strings are Python-interpreter diagnostics; the Go
  parser's messages will differ in wording. These surface only as refuse-path
  text ("would produce invalid TOML: …") and are treated as diagnostic detail,
  not FROZEN surface. All *schema* validation messages (`recipe_schema.py`)
  are FROZEN and are reproduced character-for-character.
- New TOML features used by future manifests must be added to the parser
  explicitly; the subset boundary is enforced by tests.
- The parser and editor live in `internal/` and are testable in isolation;
  wiring into the dispatched verbs is later cards' work.
