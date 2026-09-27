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
