# Delta for worktree-flow

## ADDED Requirements

### Requirement: Go gate implementation of record with behavioral parity

The `worktree-flow` worktree gate SHALL be implemented as a single autocontained Go
binary (`worktree-gate`) built from source in the repository. The binary SHALL be the
implementation of record; the Bash implementation is retained only as a frozen reference
and rollback path.

The binary MUST accept the same normalized event JSON on stdin that the Bash gate accepts
and MUST honour the same exit-code contract: exit `0` to allow, exit `2` to block. No
other exit code may be produced for a well-formed invocation, and usage or flag errors
MUST also exit `0`.

Block reasons and all warnings MUST be written to stderr. stdout MUST remain empty except
for the diagnostic flags `--version`, `--selftest` and `--explain`.

Resolution precedence MUST be preserved exactly:

- `gate_mode`: `WORKTREE_GATE_MODE` env → stamped value → `always`.
- `gate_scope`: `WORKTREE_GATE_SCOPE` env → stamped value → `auto`.
- `repo_topology`: stamped value only. There MUST NOT be an environment override.
- protected branches: `WORKTREE_GATE_PROTECTED` env → stamped/default `main development`.

An invalid value at any level MUST emit the existing warning on stderr and fall back one
level. `gate_mode = off` MUST exit `0` before `gate_scope` and `repo_topology` are
resolved, so a disabled gate emits no scope or topology warning.

The binary MUST preserve every fail-open path of the Bash implementation: unparseable or
non-object stdin JSON, absent write candidates, an unusable event `cwd`, a candidate
outside any repository, a candidate inside a linked worktree, a non-protected branch,
unproven topology under an applicable exception, and any internal error.

Gate policy MUST NOT change. Candidate extraction, the internal URI allowlist, the
`.claude` local-configuration exception, topology classification, the `openspec/changes`
central-planning exception, and all block message text MUST remain byte-identical to the
frozen Bash reference.

#### Scenario: Structured write on a protected branch is blocked identically

- GIVEN the main worktree is checked out on a protected branch
- AND the gate is materialized with the Go implementation resolved
- WHEN a path-mode event targets a file inside that worktree
- THEN the binary exits `2`
- AND the stderr message is byte-identical to the frozen Bash reference for the same event

#### Scenario: Shell write-bypass detection is preserved

- GIVEN the main worktree is checked out on a protected branch
- WHEN a shell-mode event carries a command that redirects, `tee`s, `sed -i`s, `cp`/`mv`s,
  or calls a Python, Node or Ruby write API targeting a path inside that worktree
- THEN the binary exits `2` with the shell-mode message
- AND the extracted candidate list reported by `--explain` equals the list the frozen Bash
  reference extracts for the same command

#### Scenario: Quote-delimiter pairing survives the regex port

- GIVEN a command containing `open("path', 'w')` with mismatched string delimiters
- WHEN the gate extracts candidates
- THEN no candidate is extracted from that call
- AND a matching-delimiter form such as `open("path", "w")` still yields the candidate

#### Scenario: Unbalanced shell quoting fails open

- GIVEN a shell-mode event whose command contains an unterminated quote
- WHEN the gate tokenizes the command
- THEN tokenization yields no tokens
- AND the gate exits `0`

#### Scenario: Repo topology has no environment override

- GIVEN the gate is stamped with `repo_topology = auto`
- AND `WORKTREE_REPO_TOPOLOGY` is set to `standalone` in the environment
- WHEN the gate resolves configuration
- THEN the environment value is ignored
- AND the decision matches the stamped `auto` behavior

#### Scenario: Disabled gate emits no downstream warnings

- GIVEN the gate is stamped with `gate_mode = off`
- AND the stamped `gate_scope` is invalid
- WHEN any event is delivered
- THEN the gate exits `0`
- AND no `gate_scope` warning is written to stderr

#### Scenario: Sibling-prefix directory is not treated as inside the repository

- GIVEN a protected repository at a path such as `/w/repo`
- WHEN a candidate resolves to `/w/repo-evil/file.txt`
- THEN the candidate is not considered inside the repository
- AND the gate exits `0`

#### Scenario: Nonexistent write target still canonicalizes through symlinks

- GIVEN a protected repository reached through a symlinked parent directory
- WHEN a candidate names a file that does not exist yet inside that repository
- THEN canonicalization resolves the existing prefix through the symlink and appends the
  remaining components
- AND the gate blocks with exit `2`

---

### Requirement: Portable launcher indirection with a stable materialized path

The recipe SHALL distribute a thin launcher at `hooks/worktree-gate.sh` whose materialized
path MUST remain `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh`, so every
harness's rendered wiring continues to reference the same `script_path` with no renderer
change and no re-render churn.

The launcher MUST be POSIX-shell compatible under bash 3.2: no `mapfile`, no associative
arrays, and no case-conversion parameter expansion.

The launcher MUST carry the stamped `gate_mode`, `gate_scope`, `repo_topology`,
`gate_impl` and expected binary version, and MUST retain the staleness sentinel that
materialization uses to detect an outdated materialized gate, so already-synced projects
are upgraded rather than silently preserved.

The launcher MUST resolve an implementation in this order, first hit winning:

1. an executable named by `WORKTREE_GATE_BIN`;
2. a project-local pinned binary under the recipe's materialized `bin/` directory;
3. the version-keyed cached binary for the detected platform;
4. the frozen Bash implementation, when `gate_impl` permits it;
5. otherwise, a single explanatory warning on stderr followed by exit `0`.

The launcher MUST hand off with `exec` so stdin is passed through unmodified and the
implementation's exit code is returned without translation. The launcher MUST NOT compute
a digest of the binary on the invocation path unless `WORKTREE_GATE_VERIFY` requests it.

Platform detection MUST map `Darwin` to `darwin` and `Linux` to `linux`, and MUST map
`arm64` and `aarch64` to `arm64` and `x86_64` and `amd64` to `amd64`. An unrecognized
platform MUST NOT resolve a binary.

#### Scenario: Materialized path is unchanged for every harness

- GIVEN a project with claude, cursor, opencode, pi and omp enabled
- WHEN hooks are rendered after this change
- THEN every rendered artifact still references
  `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh`
- AND the rendered bytes are identical to the pre-change output

#### Scenario: Launcher execs the resolved binary and passes stdin through

- GIVEN a resolvable gate binary
- WHEN a harness invokes the launcher with event JSON on stdin
- THEN the binary receives the identical JSON
- AND the launcher's exit status is the binary's exit status

#### Scenario: Explicit binary override wins

- GIVEN `WORKTREE_GATE_BIN` names an executable
- WHEN the launcher resolves an implementation
- THEN that executable is used
- AND neither the project pin nor the cache is consulted

#### Scenario: No usable implementation fails open loudly

- GIVEN no override, no project pin, no cached binary, and `gate_impl` forbidding the Bash
  path
- WHEN the launcher runs
- THEN exactly one explanatory warning naming the missing binary is written to stderr
- AND the launcher exits `0`

#### Scenario: Pre-Go materialized gate is upgraded, not preserved

- GIVEN a project whose materialized `worktree-gate.sh` predates this change
- WHEN `ai-specs sync` runs
- THEN the materialized file is replaced by the stamped launcher
- AND no staleness warning suppresses the upgrade

#### Scenario: Translated shell architecture still resolves a usable binary

- GIVEN an Apple Silicon host whose shell reports `x86_64` from `uname -m`
- WHEN the launcher detects the platform
- THEN it selects the `darwin-amd64` target
- AND the gate operates normally

---

### Requirement: Binary acquisition, verification and cache layout

Gate binaries MUST NOT be committed to the repository. The repository SHALL commit only
the expected SHA-256 digests for each published target, and those committed digests SHALL
be the trust root for verification.

`ai-specs sync` SHALL acquire the gate binary for the host platform when the
`worktree-flow` recipe is enabled and `gate_impl` is `auto` or `go`. The acquired binary
MUST be stored at
`$AI_SPECS_HOME/cache/bin/worktree-gate/<cli-version>/<goos>-<goarch>/worktree-gate`, so
each CLI version resolves its own binary.

Acquisition MUST verify the downloaded bytes against the committed digest **before** the
binary is installed or executed. On mismatch the downloaded file MUST be deleted, a
warning MUST be emitted, the binary MUST NOT be executed, and the failure MUST be
recorded for diagnostics.

Installation MUST be atomic: download to a temporary file in the destination directory,
verify, set mode `0755`, then rename into place, so a partial download can never be
executed. A newly installed binary MUST pass its self-test before being considered usable.

Acquisition MUST NOT fail `ai-specs sync`. Any failure — no network, unsupported platform,
digest mismatch, failed self-test — MUST warn and degrade.

An opt-in local build from the in-repo Go source MUST be available for offline and
air-gapped installs and MUST write into the same cache layout. A Go toolchain MUST NOT be
required to install or use the CLI.

#### Scenario: Verified acquisition installs a usable binary

- GIVEN the host platform is in the published matrix
- AND the downloaded asset's SHA-256 matches the committed digest
- WHEN `ai-specs sync` runs
- THEN the binary is installed at the version-keyed cache path with mode `0755`
- AND its self-test passes

#### Scenario: Digest mismatch never executes the artifact

- GIVEN a downloaded asset whose SHA-256 does not match the committed digest
- WHEN acquisition verifies it
- THEN the file is deleted
- AND the binary is never executed
- AND a warning is emitted and the mismatch is recorded for diagnostics

#### Scenario: Partial download is never installed

- GIVEN a download that terminates before completion
- WHEN acquisition runs
- THEN the temporary file is discarded
- AND no file is renamed into the cache path

#### Scenario: Offline sync degrades instead of failing

- GIVEN no network access and no cached binary
- WHEN `ai-specs sync` runs with `gate_impl = auto`
- THEN sync succeeds
- AND a warning states that the Bash implementation is in effect

#### Scenario: Unsupported platform degrades with a warning

- GIVEN a host platform outside the published matrix
- WHEN acquisition runs
- THEN no binary is installed
- AND a warning names the unsupported platform and the available alternatives

#### Scenario: Opt-in local build populates the same cache path

- GIVEN a Go toolchain is present and a local build is requested
- WHEN acquisition runs
- THEN the binary is built from the in-repo source into the version-keyed cache path
- AND no network access is required

---

### Requirement: `gate_impl` configuration

The `worktree-flow` recipe SHALL expose a `gate_impl` configuration key with enum
`auto | go | bash` and default `auto`.

- `auto` MUST prefer the Go binary and fall back to the frozen Bash implementation when no
  binary is usable.
- `go` MUST use only the Go binary; when none is usable the gate MUST fail open and the
  condition MUST be reported as an error by diagnostics.
- `bash` MUST use the frozen Bash implementation and MUST NOT require any binary,
  network access, or Go toolchain.

An invalid `gate_impl` value MUST be rejected at sync time with a message listing the
allowed values, consistent with `gate_scope` and `repo_topology` validation. The resolved
value MUST be stamped into the materialized launcher.

#### Scenario: Default configuration prefers the Go binary

- GIVEN `gate_impl` is unset
- WHEN the recipe is materialized
- THEN the launcher is stamped with `auto`
- AND a usable Go binary is preferred over the Bash implementation

#### Scenario: Bash mode requires no binary

- GIVEN `gate_impl = bash`
- WHEN `ai-specs sync` runs with no network and no cached binary
- THEN sync succeeds without acquiring a binary
- AND the frozen Bash implementation answers gate events with unchanged behavior

#### Scenario: Go mode surfaces a missing binary as an error

- GIVEN `gate_impl = go`
- AND no usable binary is resolvable
- WHEN a gate event is delivered
- THEN the gate exits `0`
- AND diagnostics report an error stating the gate is not enforcing

#### Scenario: Invalid value is rejected at sync

- GIVEN `gate_impl = rust`
- WHEN `ai-specs sync` runs
- THEN sync fails with a message listing `auto`, `go` and `bash`

---

### Requirement: Multi-arch build matrix and reproducibility

The gate binary SHALL be published for `darwin/arm64`, `darwin/amd64`, `linux/amd64` and
`linux/arm64`. `windows/amd64` is explicitly out of scope while the gate is wired through
POSIX shell launchers.

Builds MUST be static and dependency-free: `CGO_ENABLED=0`, no third-party Go modules.
Builds MUST be reproducible: path prefixes trimmed, VCS stamping disabled, and the CLI
version injected at link time. Building the same source with the same toolchain and target
MUST produce identical bytes, so the committed digests are independently verifiable.

Published digests MUST be regenerated and committed as part of the release, and MUST match
the published assets.

#### Scenario: Every supported target builds

- GIVEN the build script and a Go toolchain
- WHEN all supported targets are built
- THEN one static binary is produced per target
- AND none links against a C runtime

#### Scenario: Repeated builds are byte-identical

- GIVEN the same source, toolchain and target
- WHEN the target is built twice
- THEN both artifacts have the same SHA-256

#### Scenario: Committed digests match published assets

- GIVEN a release with published assets
- WHEN each asset's SHA-256 is compared with the committed digest file
- THEN every digest matches

#### Scenario: darwin/arm64 asset executes on Apple Silicon

- GIVEN the released `darwin/arm64` asset
- WHEN it is executed on Apple Silicon hardware
- THEN it runs and its self-test passes

---

### Requirement: Diagnostics for gate implementation health

`ai-specs doctor` SHALL report the gate's resolved implementation, binary path and
version, and any fallback currently in effect, because a fail-open gate is otherwise
invisible.

Severity MUST be:

- OK when the Go binary is resolved, its version matches the stamped version, and its
  self-test passes;
- INFO when `gate_impl = bash` is configured explicitly;
- WARN when `gate_impl = auto` is silently falling back to Bash, or the binary version
  does not match the stamped version;
- ERROR when `gate_impl = go` has no usable binary, or a digest mismatch was recorded at
  the last acquisition.

#### Scenario: Healthy Go gate reports OK

- GIVEN a resolvable binary whose version matches the stamp and whose self-test passes
- WHEN `ai-specs doctor` runs
- THEN the gate check reports OK with the resolved path and version

#### Scenario: Silent fallback is surfaced as a warning

- GIVEN `gate_impl = auto` and no usable binary
- WHEN `ai-specs doctor` runs
- THEN the gate check reports WARN stating the Bash implementation is in effect

#### Scenario: Non-enforcing gate is surfaced as an error

- GIVEN `gate_impl = go` and no usable binary
- WHEN `ai-specs doctor` runs
- THEN the gate check reports ERROR stating the gate is failing open
- AND the message names the expected binary path

#### Scenario: Recorded digest mismatch is surfaced as an error

- GIVEN a digest mismatch was recorded at the last acquisition
- WHEN `ai-specs doctor` runs
- THEN the gate check reports ERROR
- AND the message states the artifact was rejected and never executed

---

### Requirement: Gate invocation performance budget

A single gate invocation SHALL spawn exactly one implementation process. The gate MUST NOT
spawn one interpreter process per candidate write path.

Git facts SHALL be memoized within an invocation: repository root, git directory, common
directory, current branch, and submodule records MUST be derived at most once per resolved
directory per invocation, regardless of how many candidate paths an event yields.

For an event yielding multiple candidates in a repository with submodules, the Go
implementation MUST issue strictly fewer `git` subprocess invocations than the frozen Bash
implementation for the same event.

Digest verification MUST NOT occur on the invocation path unless explicitly requested by
environment variable.

#### Scenario: One process per invocation

- GIVEN a shell-mode event yielding four candidate write paths
- WHEN the gate runs
- THEN exactly one implementation process is spawned

#### Scenario: Git facts are memoized across candidates

- GIVEN a shell-mode event yielding four candidate paths inside one repository with two
  initialized submodules
- WHEN the gate resolves each candidate
- THEN the number of `git` invocations is strictly lower than the frozen Bash
  implementation issues for the same event
- AND the resulting decision is identical

#### Scenario: No hashing on the hot path by default

- GIVEN a resolvable cached binary and no verification request
- WHEN the launcher runs
- THEN no digest of the binary is computed
