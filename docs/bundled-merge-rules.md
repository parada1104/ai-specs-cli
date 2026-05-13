# Bundled Merge Rules

How `ai-specs sync` (or `ai-specs refresh-bundled`) reconciles your edits
against upstream changes using a SHA-256 baseline tracked in
`ai-specs/.ai-specs.lock`. Commit the lock file so teammates stay on the
same baseline.

| Your file vs baseline | Upstream changed? | Outcome |
|-----------------------|-------------------|---------|
| Untouched             | Yes               | Auto-updated to the new CLI version |
| Untouched             | No                | Nothing (silent) |
| Customized            | Yes               | CLI version saved as `<name>.new` alongside yours — diff and merge by hand |
| Customized            | No                | Your edits stand |
| Deleted by you        | —                 | Respected (stops tracking) |

The lock file records what the CLI shipped to your project last time.
Bundled skills (`skill-creator`, `skill-sync`) and catalog skills vendored
via `[[deps]]` all participate in the same lock-based update mechanism.
