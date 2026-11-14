# Troubleshooting: the launcher heals a missing native binary

`bin/ai-specs` is a thin launcher: it execs `$AI_SPECS_HOME/dist/ai-specs`
(the native Go CLI) with your arguments. Everything users see — verbs, help,
exit codes — comes from that binary.

## After `ai-specs upgrade` from 0.24.x

The upgrade that ships with 0.24.x only fast-forwards the git checkout
(`git fetch` + `git merge --ff-only`). It never installs the compiled binary,
so a 0.24.x install ends up with the 0.25.0 tree and **no**
`$AI_SPECS_HOME/dist/ai-specs`.

You do not have to do anything. The first verb after the upgrade finds the
binary missing and acquires it once:

1. It downloads the release asset for your platform
   (`ai-specs-<darwin|linux>-<arm64|amd64>`) from the release of the version in
   `$AI_SPECS_HOME/VERSION`.
2. It verifies the SHA-256 **before** installing, against the committed digest
   in `$AI_SPECS_HOME/bin/SHA256SUMS`. A mismatch deletes the download and
   installs nothing.
3. With no committed digest for your platform, no download is attempted; if
   `go` is on `PATH` it builds from the checkout instead
   (`go build ./cmd/ai-specs`).
4. The verified bytes are installed atomically at
   `<real home>/dist/ai-specs` (mode `0755`). The home directory is
   canonicalized, so a symlinked home receives `dist/` under its real path.

The acquisition writes progress to **stderr** only. The verb's stdout is
untouched, so anything that pipes `ai-specs <verb>` sees exactly the same
output as before.

If neither route works, every verb exits 1 with:

```text
ai-specs: native binary not found at <home>/dist/ai-specs
  No verified release asset for this platform and no usable Go toolchain.
  Re-run the installer: curl -fsSL https://raw.githubusercontent.com/parada1104/ai-specs-cli/main/install.sh | bash
  Or build it with scripts/build-cli.sh, point AI_SPECS_RELEASE_BASE at a mirror, or set AI_SPECS_GO_BIN.
```

Re-running `install.sh` is the supported fix; it installs the entry point and
the `dist/` copy together.

## Environment overrides

| Variable | Effect |
| --- | --- |
| `AI_SPECS_GO_BIN` | Pins the binary to exec. A pinned binary is **never** replaced: if the path is not executable the launcher fails immediately, with no acquisition attempt. The Go CLI sets this for the processes it starts. |
| `AI_SPECS_RELEASE_BASE` | Download base for the release asset (default: this repo's GitHub release for `VERSION`). Point it at a mirror when GitHub is unreachable. |
| `AI_SPECS_TRUST_ROOT_FILE` | File holding the expected digests (default: `<home>/bin/SHA256SUMS`). Only this file authorizes a download. |
| `AI_SPECS_HOME` | The checkout the launcher serves and writes `dist/` into (default: the launcher's parent directory). |

## Related

- `docs/go-migration-parity-contract.md` — what the native CLI must preserve.
- `install.sh` — the installer: verified asset, `dist/` copy, entry point.
