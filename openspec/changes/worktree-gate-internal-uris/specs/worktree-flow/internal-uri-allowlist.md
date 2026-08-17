# Spec: internal URI allowlist in worktree gate

## Requirements

- REQ-1: Path-mode writes to known internal harness URIs must not be blocked
  by the worktree gate, regardless of branch.
- REQ-2: Known internal URI schemes: `xd://`, `skill://`, `rule://`,
  `agent://`, `history://`, `artifact://`, `local://`, `vault://`,
  `mcp://`, `issue://`, `pr://`, `omp://`.
- REQ-3: The URI allowlist applies in PATH mode only. In SHELL mode a
  URI-looking token is a literal write target and MUST NOT bypass
  classification.
- REQ-4: A known scheme that masks a filesystem path must be classified
  normally: candidates carrying `../` traversal or an absolute path after the
  scheme never receive the internal-URI bypass, even in PATH mode.
- REQ-5: Unknown URI schemes (`https://`, `file://`, `custom://`, etc.)
  must still go through normal gating.
- REQ-6: A usable event `cwd` is a non-empty absolute existing directory.
  Relative or nonexistent event `cwd` falls back to the hook process `$PWD`.
- REQ-7: All existing gating behavior (non-URI paths, `.claude` exceptions,
  scope logic, shell-mode heuristics) must remain unchanged.

## Scenarios

### S1: xd://resolve on protected branch -> allowed
Event: Write with file_path="xd://resolve" on development branch.
Result: exit 0 (allowed).

### S2: artifact://abc on protected branch -> allowed
Event: Write with file_path="artifact://abc123" on development branch.
Result: exit 0 (allowed).

### S3: local://plan.md on protected branch -> allowed
Event: Write with file_path="local://plan.md" on main branch.
Result: exit 0 (allowed).

### S4: vault://path/doc on protected branch -> allowed
Event: Write with file_path="vault://hermes-vault/doc.md" on development.
Result: exit 0 (allowed).

### S5: skill://test/init on protected branch -> allowed
Event: Write with file_path="skill://testing/init" on main branch.
Result: exit 0 (allowed).

### S6: https:// prefixed repo path -> blocked (unknown scheme)
Event: Write with file_path="https://example.com/src.py" on main branch,
where src.py exists inside the repo.
Result: exit 2 (blocked) — unknown scheme falls through to normal gating.

### S7: Normal filesystem path on protected branch -> blocked
Event: Write with file_path="src/app.py" on development branch.
Result: exit 2 (blocked) — regression protection.

### S8: xd:// absolute path after scheme -> blocked
Event: Write with file_path="xd:///abs/repo/src.py" on protected branch,
where /abs/repo/src.py exists inside the repo.
Result: exit 2 (blocked) — absolute-path-masked candidate is classified
normally.

### S9: xd:// traversal-masked repo path -> blocked
Event: Write with file_path="xd://repo/../repo/src/app.py" on protected branch.
Result: exit 2 (blocked) — traversal-masked candidate is classified normally.

### S10: shell-mode URI-looking literal -> blocked
Event: Bash command "echo x > xd:///abs/repo/src.py" on protected branch.
Result: exit 2 (blocked) — SHELL mode never applies the URI bypass.

### S11: relative event cwd -> process cwd fallback
Event: Write with file_path="src/app.py" and cwd="relative/dir", process cwd
inside protected repo on protected branch.
Result: exit 2 (blocked) — unusable event cwd falls back to process `$PWD`.

### S12: nonexistent event cwd -> process cwd fallback
Event: Write with file_path="src/app.py" and cwd pointing to a nonexistent
directory, process cwd inside protected repo on protected branch.
Result: exit 2 (blocked) — unusable event cwd falls back to process `$PWD`.
