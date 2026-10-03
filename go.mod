// Module ai-specs-cli is the root Go module for the single-binary
// migration epic (https://trello.com/c/qwlHQ7Xa). It hosts the strangler
// command skeleton: a native `ai-specs` binary that implements the
// dispatcher contract of bin/ai-specs and execs the legacy lib/*.sh
// implementation for every not-yet-ported subcommand.
//
// Conventions mirror the worktree-gate module
// (catalog/recipes/worktree-flow/gate): canonical go1.24.13 toolchain,
// CGO_ENABLED=0 builds, and zero third-party dependencies.
module ai-specs.dev/ai-specs

go 1.24.13

// SX0a: the root binary invokes the gate's authoritative orphans decision
// in-process through the importable shared package instead of shelling out
// to the worktree-gate binary. The replace pins the import to the nested
// gate module in this repository (never a published release).
require ai-specs.dev/worktree-gate v0.0.0

replace ai-specs.dev/worktree-gate => ./catalog/recipes/worktree-flow/gate
