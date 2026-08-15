# README — worktree-gate binary digests

This directory commits **only text**: the expected SHA-256 digests of the
published `worktree-gate` release assets, one line per target in the form

```
<sha256>  worktree-gate-<goos>-<goarch>
```

**No binaries are ever committed here** (design D4: `git clone` is the install
channel; binaries in git would cost every user ~15-25 MB per release forever).

The committed digests are the trust root (D5): the network supplies only bytes,
the repository supplies the expected digest. A downloaded asset whose SHA-256
does not match this file is deleted and never executed. Existing executable
cache hits are revalidated against this trust root, the current `VERSION`, and
`--selftest`; stale or unknown candidates are quarantined before re-acquisition.
A successful cache install also writes an atomic `<binary>.verified` receipt
containing the verified version, digest, and self-test result. The launcher
rejects a cache binary without that receipt.

The supported matrix (spec "Multi-arch build matrix and reproducibility") is
`darwin/arm64`, `darwin/amd64`, `linux/amd64` and `linux/arm64`. Builds are
reproducible (`CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`, version stamped
at link time), so any reviewer with a Go toolchain can regenerate a target and
compare its digest:

```
scripts/build-gate.sh
shasum -a 256 dist/worktree-gate-*
```

Digests are regenerated and committed as part of each release (spec: "Published
digests MUST be regenerated and committed as part of the release, and MUST
match the published assets").
