# ADR 0001 — CLI framework: stdlib `flag` + explicit dispatch

- **Status**: Accepted
- **Card**: [Go 04] Go module bootstrap + command skeleton (Go single-binary migration epic)
- **Context**: root Go module `ai-specs.dev/ai-specs`, go 1.24.13

## Context

The Go single-binary migration must reproduce the dispatcher contract of
`bin/ai-specs` (14 verbs, aliases `-v/--version` and `-h/--help`, bare
invocation rewritten to `hub`, unknown-command usage error with exit 2). The
surface is verb-dispatch: each verb forwards positional args and a handful of
flags to a legacy `lib/*.sh` implementation today, and will own its flags
inside its future ported Go module tomorrow. The root launcher itself parses
no flags beyond recognizing the verb.

The repo holds a zero-third-party-dependency posture for Go code: the
worktree-gate module (`catalog/recipes/worktree-flow/gate`) and this root
module's `go.mod` both document and practice it.

Popular Go CLI frameworks (cobra, urfave/cli) were considered for the root
dispatcher.

## Decision

Use the Go standard library only: explicit dispatch over a verb table
(`internal/cli`), with `flag` available to future ported modules for their
per-verb flags. Reject cobra and urfave-cli.

Rationale:

1. **Parity is byte-exact.** The parity contract freezes help text, error
   strings, and exit codes. Framework-generated usage/help output (cobra's
   auto-usage on unknown flags, urfave's templates) would fight the byte-exact
   contract at every seam; suppressing it means configuring the framework
   away, at which point it adds cost and no value.
2. **The root parses no flags.** `bin/ai-specs` matches verbs and shifts the
   rest through. A `map[verb]route` plus two native handlers is the whole
   dispatcher; a framework's flag model (persistent flags, command trees) has
   nothing to bind to.
3. **Flags belong to ported verbs.** Each verb's flag surface lives with its
   future Go module (e.g. `upgrade`'s `--dry-run/--force`), where a local
   `flag.FlagSet` per verb mirrors the existing gate-module convention.
4. **Aliases are trivial.** `-v/--version` and `-h/--help` are two map
   entries, not a framework feature.
5. **Zero deps is the existing, documented posture** — smaller binary, no
   dependency drift, `CGO_ENABLED=0` builds stay self-contained.

## Consequences

- Dispatch code is manual: adding a verb is one table entry (shim) or one
  handler (native). Acceptable at 14 verbs.
- No automatic shell-completion generation. The legacy CLI has none; parity
  does not require it.
- No automatic usage synthesis: unknown-command output is hand-written to
  match the frozen two-line stderr contract exactly.
- Each ported verb that needs flags adopts `flag` locally, following the gate
  module's `run(args, stdin, stdout, stderr) int` in-process-test pattern.
- **Revisit trigger**: if a ported verb grows a subcommand-heavy UX (nested
  commands, shared flag inheritance across a tree) that hand-rolled dispatch
  makes error-prone, re-evaluate a framework for that module only — not for
  the root dispatcher.
