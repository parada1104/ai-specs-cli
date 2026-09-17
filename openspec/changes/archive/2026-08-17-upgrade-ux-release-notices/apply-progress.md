# Apply progress: upgrade-experience

## WU0 — Exclusion list verification (gate for WU4)

The runtime reference count was treated as evidence, not proof, so the
exclusion list was verified three ways before any path was excluded.

**1. Static reads.** No shell or Python read of `$AI_SPECS_HOME`,
`ai_specs_home`, or `cli_home` resolves into `openspec/`, `tests/`, `.github/`
or `tmp/`. No generic traversal (`glob`, `rglob`, `iterdir`, `walk`, `find`)
runs over the CLI home either, so nothing can reach an excluded subtree
indirectly.

**2. The one near-miss.** `lib/_internal/premerge_guardian.py` reads
`openspec/changes/...`, but line 444 shows it resolves against
`Path(repo_root)` — the **consumer project**, never the CLI install. Not a
conflict.

**3. Empirical.** A narrowed clone was driven end-to-end:

| Command | Result |
|---|---|
| `ai-specs --version` | `0.22.0` |
| `ai-specs help` | full command list |
| `ai-specs init <scratch>` | exit 0 |
| `ai-specs sync <scratch>` | `✓ ai-specs sync complete` |
| `ai-specs doctor <scratch>` | 18 OK, 0 INFO, 1 WARN, 0 ERROR |

The single WARN is `no [mcp.*] entries declared` — an empty scratch project,
unrelated to narrowing.

**Conclusion:** the exclusion list stands as specified. No shrink required.

## Footprint measured (WU4)

Narrowing a real clone of `main`:

| | Files (excluding `.git/`) |
|---|---|
| Before | 1842 |
| After | 958 |

48% fewer files. `git status --porcelain` is empty afterwards, so a narrowed
checkout does not trip the dirty-tree guard on the next upgrade.

## Deviations from the plan

**Summary bullets were added in two passes.** The first GREEN emitted only
version and date lines, which satisfied every spec scenario and every test.
Rendering the real output showed that was too thin to be useful, so bullets
were added — RED first (`test_bullets_come_from_added_and_changed` and
friends).

Rendering again exposed a defect no assertion had caught: real changelog
bullets are 400+ character paragraphs, so the "summary" was another wall of
text. `_condense()` was added, again RED first, capping bullets at the first
sentence or 100 characters on a word boundary.

Both defects were only visible by looking at real output. Worth recording:
the spec scenarios were satisfied at a point where the feature was not yet
good.

**A separating blank line** was added after the report block; the output ran
straight into the symlink-verification line.

## Test corrections made during the work

Two failures were defects in the tests, not the code, and both are worth
remembering:

1. **Degradation tests mutated the local tree.** Deleting or corrupting
   `CHANGELOG.md` in the fake install makes the tree dirty, so `upgrade` aborted
   with exit 3 and the test measured the dirty-tree guard instead of parser
   degradation. Fixed by inducing degradation in the **published** state
   (`publish_mutated`), which is what a real user actually receives.

2. **The old-git shim never matched.** It checked `$1 == "sparse-checkout"`,
   but real calls are `git -C <dir> sparse-checkout …`, so the subcommand is
   never `$1`. The shim now scans every argument. Before the fix the fallback
   test passed for the wrong reason — the real git ran and narrowing succeeded.

3. **`test_notice_does_not_bleed_into_the_next_subsection`** began failing once
   summary bullets landed, because its sentinel is a `### Fixed` bullet and now
   legitimately appears in the summary. The assertion was narrowed to the
   notice block, which is what the requirement is actually about.

## Verification

- `./tests/validate.sh` in the change worktree: **exit 0, 1772 tests, 0
  failures** (up from 1684 on `development`).
- `bash -n` clean on `install.sh`, `lib/upgrade.sh`,
  `lib/_internal/narrow-checkout.sh`.
- All 70 upgrade-related tests pass with narrowing wired in, so no safety
  behavior regressed.
