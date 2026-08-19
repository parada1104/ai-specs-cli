# Judgment ledger: cleanup-merge-sequence

**Target (immutable):** `e1e8994` on `change/cleanup-merge-sequence`, base
`development` (`592dbf9`). 11 files, +499 / −53.

**Round:** 1. Two blind read-only judges, identical scope, launched in parallel.

## Counts

| | |
|---|---|
| Confirmed by both judges | 1 (critical) |
| Suspect (one judge) | 3 |
| Contradictions | 0 |

## What the judges confirmed is CORRECT

`syncBaseCleanup` is correctly isolated: it runs only after every pass returns
without failures, uses `--ff-only`, and is a no-op under `--dry-run`. Neither
judge found a path where the base is pulled while a candidate worktree still
holds a branch, which was the sequencing defect the change exists to fix.

Protected-branch enforcement survived the new stale-branch sweep. The base
branch is skipped explicitly even when the configured protected set is
incomplete, and `assertDeletable` still guards every destructive call.

## CONFIRMED — both judges

### C1 — path existence accepted as merge evidence (CRITICAL, both)
`gate/cleanup.go:394-424`

`noPRPathPresenceCleanup` classified a local branch as merged when every path it
touched merely *existed* in the base tree. It never compared content. Two
commits can touch the same path with entirely different bytes and never meet, so
a never-merged branch was scheduled for deletion — unrecoverable work loss on a
destructive path.

Both judges cited the same proof, and it is the strongest kind: **the change's
own test encoded the bug as the expected behaviour.**
`TestCleanupDiscoversStaleLocalBranchWithNoWorktree` writes `shared.txt` as
`"branch\n"` on the branch and `"landed\n"` on `main` independently, never
merges them, and then asserts `would remove stale-existing`.

Verified by the coordinator by reading the fixture rather than accepting the
reasoning: the two writes are independent commits with no merge between them.

## SUSPECT — one judge

| ID | Finding | Judge |
|---|---|---|
| S1 | Deleting the local branch before the remote one makes a remote failure unrecoverable: worktree and local branch are both gone, so the surviving remote branch is invisible to every later pass | B |
| S2 | `TestCleanupPreservesNewlinePathInTreeProof` cannot falsify the NUL-split regression it documents — "unmerged" is the outcome of *any* failed proof | B |
| S3 | The `SKILL.md` assertions are golden substring checks; they verify that phrases exist, not that the prescribed sequence is the implemented one | A |

## Round-one correction — all 4 addressed

| ID | Fix | Evidence |
|---|---|---|
| C1 | The fallback is **removed**, not repaired. Making it sound would mean comparing blob content — which is exactly `candidateHasCombinedTreeEquivalenceCleanup`, already run by `isMergedCleanup` a few lines above. The check was therefore unsound or redundant; there is no sound version that adds anything. A branch whose merge cannot be proven is preserved, per the change's own contract: refuse rather than guess. | RED first: the worker's fixture was rewritten to the correct expectation. `TestStaleBranchWithLandedContentIsStillRemovable` was added so the fix cannot silently disable the feature — preserving everything would be safe and useless. Falsifiability confirmed: forcing the refusal to `return true, "merged"` fails **both** new tests. |
| C1b | While reading the same function: the pull-request scan returned `false` on the first entry lacking a merge commit, so a head closed unmerged once and later reused for a pull request that *did* merge was misclassified as unmerged. It now scans every pull request for the head. | Found by the coordinator during the C1 correction, not reported by either judge. |
| S1 | Remote deletion now runs **before** local deletion, in both the linked-worktree loop and the stale-branch sweep. The remote step is the one that fails for reasons outside this machine; the local branch is the only handle a rerun has. | RED first: `TestRemoteDeletionFailureLeavesLocalBranchForRetry` failed with `local branch was deleted before the remote failure, leaving nothing for a rerun to retry`. The test does not stop at survival — it restores the remote and asserts the rerun completes both deletions, so recoverability is proven end to end rather than assumed. |
| S2 | Added `TestCleanupProvesMergeForNewlinePathInTree`, the falsifying half. The newline path is genuinely landed with identical content, so only a correctly reassembled path can prove it. | Sabotage confirmed the asymmetry Judge B described: replacing the NUL split with a newline split fails the **new** test and leaves the original one passing. |
| S3 | The order-sensitive assertion replaces phrase presence. `SKILL.md` was also stale on two counts after C1 and S1 — it still documented local-before-remote deletion and still offered the removed path-presence proof as valid evidence. Both corrected. | Sabotage confirmed: swapping the documented order fails `test_skill_documents_the_implemented_cleanup_order`. |

The spec delta was updated to match: the ordering requirement now states that the
local branch outlives remote deletion and why, and the stale-branch requirement
now demands content evidence and explicitly rejects path existence.

`catalog/recipes/worktree-flow/bin/SHA256SUMS` was regenerated with the canonical
`go1.24.13` toolchain after the final `cleanup.go` edit. A stale trust root
publishes zero release assets.

## Verification after correction

- `./tests/validate.sh` — see `verify-report.md`.
- Every correction was RED before it was green, and every one was re-sabotaged
  afterwards to prove the test can still fail.

## Disposition

Round one complete. No finding remains open.

`JUDGMENT: APPROVED ✅`
