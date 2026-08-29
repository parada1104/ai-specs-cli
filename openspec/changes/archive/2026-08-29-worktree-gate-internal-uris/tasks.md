# Tasks: make worktree-gate URI and cwd resolution safe

Depth: standard

## Tasks

1. **Specify cwd precedence** — relative candidates use event `cwd`, process
   `$PWD` is fallback only, and absolute candidates remain unchanged.
2. **Add internal URI allowlist** — explicitly bypass known non-filesystem
   protocols before Git path classification.
3. **Preserve unknown URI safety** — verify `https://`, `file://`, and
   `custom://` do not receive the internal bypass.
4. **Add cwd regression tests** — cover unrelated process cwd plus external
   event cwd, and protected relative destinations that must still block.
5. **Add shell cwd boundary tests** — cover relative shell writes using event
   cwd and document limited `cd` handling/fail-open behavior.
6. **Update worktree-flow delta spec** — record URI scope, cwd precedence,
   scenarios, and non-goals.
7. **Run focused verification** — execute gate integration tests and inspect
   exit-code/stderr behavior.
8. **Run repository verification** — execute `./tests/run.sh` and fix regressions.

## Review workload forecast

- Expected surface: hook, integration tests, delta spec, and possibly changelog.
- Standard review risk: path-resolution correctness and bypass scope.
- Adversarial cases: missing cwd, unrelated process cwd, relative paths inside
  and outside the protected repo, unknown schemes, and dynamic shell `cd`.
***