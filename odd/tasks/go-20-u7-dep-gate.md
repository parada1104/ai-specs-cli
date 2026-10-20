# Go 20 / U7-1 — the CLI-dep gate, native

Unit document for **U7 item 1** of the program (`odd/tasks/go-20-python-exit.md`). Base
`epic/go-single-binary` at `494b028`; worktree `.worktrees/go-20-u7-dep-gate`, branch
`change/go-20-u7-dep-gate`.

## Specs

- S1 — Scope (verbatim, program doc `## Next`, item 1): "**The native CLI-dep gate** — `_dep_gate` +
  `dep_install` are the last interactive surface with no Go side. Porting it retires **S37** (the
  `runLegacyInitDepGate` seam in `init`'s wizard) and the matching seam in `configure-recipes`, which
  is the only reason `runShim` still exists."
- S2 — It is sliced before it is opened (program S6, verbatim): "one unit = one small PR against the
  epic with its own review; a verb port is sliced further when it does not fit one review." Measured:
  `_dep_gate` is ~40 lines standing on `dep_install.py` (240: plans, consent prompt, brew/apt
  subprocesses) and on `provider_install.py`'s install half (597: network, checksums, extraction),
  while its CHECK half exists in Go only as `internal/doctor`'s unexported probe. Slices: **T1** the
  check, the plans and the panel; **T2** the provider install half; **T3** the gate itself and the
  seam's retirement; **T4** the dead code and the program row.
- S3 — The plan half reproduces `dep_install.resolve_install_plan`'s DISPLAY byte for byte:
  brew is `brew install <formula>`; apt is `sudo apt-get install -y <pkg>`; guidance is the declared
  `install_url`, or `install '<binary>' manually` when there is none; a provider plan is
  `GitHub Release <repository> (<release_policy>) for <goos>/<goarch>`.
- S4 — The panel reproduces `config_wizard._render_dep_panel`'s line byte for byte (format string
  verbatim from the legacy source): `f"{status:4s}  {r.binary:12s}  {r.purpose}{detail}{hint}"`, with
  `status` = `OK` when the dep is ok, `WARN` when it is required and not ok, `INFO` otherwise;
  `detail` = `f" ({r.detail})"` only when the detail is non-empty; `hint` = `f" → {r.install_url}"`
  only when a URL is declared AND the dep is not ok.
- S5 — One authoritative grader per behaviour (project rule): the probe moves OUT of `internal/doctor`
  into `internal/deps` and doctor delegates to it; nothing is copied.
- S6 — T1 changes no user-visible behaviour: `doctor`'s output is identical and the gate keeps running
  through S37's seam on all three of its callers until T3.
- S7 — The provider plan's validation errors are `provider_install.build_release_plan`'s, verbatim,
  because the gate surfaces them inside `install offer skipped: <exc>`: "provider dependency is not the
  allowlisted GitHub Release installer", "provider release policy must be latest-stable", and the
  platform message "provider has no artifact for this platform; supported targets: <list>.<guidance>".
- S8 — Out of scope for T1: no subprocess install, no download, no wiring change, no deletion.
- S9 — Evidence per slice: RED observed before GREEN; the plan differential drives the REAL
  `dep_install.resolve_install_plan`/`describe_install_plan` against `deps.ResolvePlan`/`Describe`
  across the matrix (brew, apt, both, neither, Darwin, Linux, guidance-only, unknown binary, and the
  provider plan with its rejection paths); then `go test ./cmd/... ./internal/...`, the parity corpus,
  `gofmt -l`, and the manifest diff.

## Tasks

| ID | Task | Specs | Commit |
| --- | --- | --- | --- |
| T1 | `internal/provider.BuildReleasePlan` + `internal/deps` (the probe promoted, plans resolving, panel rendering); doctor delegates | S3-S7, S9 | — |
| T2 | `internal/provider`'s install half: download, SHA256SUMS verification, extraction, managed receipt | S1 | — |
| T3 | `deps.Gate` (panel, per-plan offer, install, recheck, `Configure anyway?`) wired into init's wizard, `configure-recipes` and `recipe add`; S37's seam and driver deleted | S1, S4, S6 | — |
| T4 | Verify `runShim`/`routeShim`/`shims` are unreachable and delete them; update the program doc's Progress row | S1 | — |

## Log

- L1 — Request (verbatim, 2026-10-19): "si, continuemos", answering the hand-off plan's first item
  ("The native CLI-dep gate … is the only reason `runShim` still exists"), after the program closed
  U6e with `var shims = map[string]route{}` and left U7's scope written in
  `odd/tasks/go-20-python-exit.md`.
- L2 — Slicing measured before opening (S2): the gate is not one PR. `dep_install.offer_and_install`
  runs `brew`/`apt-get` through `subprocess.run(timeout=300)` and delegates the GitHub-release kind to
  `provider_install.install_github_release` (download, `SHA256SUMS`, extraction, managed receipt) —
  delivery-adjacent surface (program risk: a mistake here installs bytes on a user's machine), so it
  gets its own slice and its own review. T1 stops at the read-only half: what to offer, and how to
  display it.
- L3 — T1 finding: the probe had NO shared authority. `internal/doctor/depcheck.go` held it
  unexported and package-private (`checkOne`, `checkProjectDeps`, `checkRecipeCLIDeps`), and the gate
  needs the per-recipe form (`dep_check.check_cli_deps`), which doctor's port did not expose (its
  `checkProjectDeps` walks the manifest). Promoting it into `internal/deps` is what S5 requires;
  doctor now calls `deps.CheckProject` and keeps only its check mapping.
- L4 — T1 finding, recorded not fixed (parity contract debt, U2's): `docs/go-migration-parity-contract.md`
  §8 "Network and external process surface" does NOT list the gate's installs (`brew install`,
  `sudo apt-get install -y`, the provider download), although they are the tree's most consequential
  external calls. T2/T3 own the row once the installs are native.
