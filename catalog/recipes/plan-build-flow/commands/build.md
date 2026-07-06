# /build — Implement, validate, and close an authorized plan

Operationalize the `plan-build-flow` skill for building. Load that skill and
follow its phase mapping and degradation policies before doing anything else
below.

## Steps

1. **Load the skill.** Read the `plan-build-flow` skill and follow its phase
   mapping (Section 2), worktree deference (Section 7), and archive-tail
   graceful no-op rules (Section 8).

2. **Resolve the change.** Resolve the target change-slug and the artifact
   store used by the prior `/plan`: use the argument if given, otherwise the
   single outstanding plan if there is exactly one, otherwise ask the user
   which change to build.

3. **Confirm authorization.** Confirm the resolved plan was reviewed and
   authorized by the human. If it was not, stop and point the user back to
   `/plan` instead of building an unauthorized plan.

4. **Defer to worktree isolation when enabled.** If an isolated-worktree
   workflow is enabled in this project, ensure the implementation work runs
   inside that change's dedicated worktree, following that workflow's own
   conventions. If no worktree workflow is enabled, build in the current
   working tree.

5. **Implement and validate.** Run implementation against the authorized plan,
   then validate the result.

6. **Run the archive tail.** Close the change folder (always completes).
   Write the vault/canonical-store summary and the tracker comment when those
   integrations are enabled; otherwise no-op each with an informative note.
   The rest of the closing step completes regardless of whether those optional
   channels ran.

7. **Report completion.** Tell the user what was built, what was validated,
   and that the change was closed — including a note for any optional output
   channel that was skipped.

Never surface internal phase names to the user — speak only in terms of
"plan" and "build".
