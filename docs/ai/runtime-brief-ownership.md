# Runtime brief ownership

`AGENTS.md` is a generated runtime brief, but existing repositories may already
contain hand-written agent instructions. ai-specs uses lock-backed provenance to
avoid replacing those bytes by inference.

## Existing `AGENTS.md`

On the first sync after this policy is installed:

- an absent file is rendered and recorded in `ai-specs/.ai-specs.lock`;
- a file whose bytes exactly match the bytes ai-specs would render is adopted
  silently and recorded;
- every other file with no baseline is preserved as `untracked`.

The renderer does not infer ownership from headings, an introductory blockquote,
or any other recognizable shape. A stale existing brief is intentionally a
one-time visible interruption rather than a guessed adoption that could overwrite
hand-written context on the next sync.

A preserved brief reports both available exits:

```text
ai-specs sync --adopt-brief
```

records the current bytes as the managed baseline. Add this marker at the top:

```html
<!-- ai-specs:runtime-brief -->
```

to keep the file permanently user-owned. `ai-specs doctor` reports the same
ownership state and guidance.

## After adoption

A baseline-matching brief is `managed_current` and produces no extra output. If
the manifest changes while the user has not edited `AGENTS.md`, it becomes
`managed_stale`; sync regenerates it and records the new baseline. If the user
edits the file, it becomes `user_modified`; sync preserves it and does not offer
a force refresh because runtime briefs may contain irreplaceable context.

## Entry points and opt-outs

The ownership decision is inside `agents-render.py`, so `init`, `sync`, and
`sync-agent` use the same classifier. `[brief].render = false` still skips
rendering before classification. The marker remains an unconditional opt-out.

`CLAUDE.md` is not the exposed surface: it is a relative symlink to `AGENTS.md`,
and `make_relative_symlink` refuses to replace a pre-existing regular file. A
regular `CLAUDE.md` therefore causes sync to fail rather than being clobbered.
