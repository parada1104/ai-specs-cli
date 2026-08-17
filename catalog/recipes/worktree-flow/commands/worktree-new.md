# worktree-new

Create an isolated git worktree for a change that will write files.

> This is a generated Markdown command executed by the agent — there is NO
> executable `/worktree-new` helper and no new routing surface. The create
> contract below is enforced by `util.resolve_request_context` /
> `util.resolve_subrepo` (see `lib/_internal/util.py`).

## When to use

Before implementing a change, writing artifacts, or editing code. Skip it for
pure exploration that writes nothing.

## Request context: owner vs planning root

Resolve one request context from proven Git facts
(`util.resolve_request_context`):

- **owner_root** — the repository that owns code and VCS work for the request
  (`git rev-parse --show-toplevel`).
- **planning_root** — the canonical planning-artifact tree. A subrepo request
  plans under the **proven superproject**; a superrepo request plans under the
  superproject itself.
- Missing, ambiguous, detached, or uninitialized topology fails safe: no owner
  inference beyond the toplevel and no planning-root exception.

## Topology

Resolve `repo_topology` from `[recipes.worktree-flow.config]` (default `auto`).
`auto` with initialized submodules → `monorepo-submodules`; otherwise
`standalone`. `monorepo-apps` is naming-only and uses the same create command as
`standalone`.

## Steps

1. Resolve a branch slug from the change name (kebab-case, e.g. `feat/x` →
   directory `feat-x`).
2. Ensure `<worktrees_dir>/` (default `.worktrees/`) is listed in `.gitignore`
   at the **superproject** root (shared layout for all topologies).
3. Create the worktree off the integration branch using the topology below.
4. Do all file-writing work for the change inside that worktree.

### standalone / monorepo-apps

```bash
git worktree add <worktrees_dir>/<slug> -b <branch> <integration_branch>
```

`<subrepo>` MUST be absent here (ignore with a warning if passed).

### monorepo-submodules

Require or infer `<subrepo>`, then create with **`git -C`** and an **absolute**
destination under the superproject `worktrees_dir`:

```bash
super_abs="$(git -C "$super_root" rev-parse --show-toplevel)"
git -C "$super_abs/<subrepo_path>" worktree add \
  "$super_abs/<worktrees_dir>/<subrepo>-<slug>" \
  -b <branch> <integration_branch>
```

Destination MUST be absolute. A relative path under `git -C <subrepo>` resolves
*inside* the submodule and is incorrect.

A request whose context is the **superrepo** MUST NOT infer a subrepo: creation
requires an explicit, validated `<subrepo>`. Without one, hard-error **before
any `git worktree add`**.

#### `<subrepo>` resolution

This command's `<subrepo>` selection is validated by `util.resolve_subrepo` (see `lib/_internal/util.py`).

1. **cwd inference** via `git rev-parse --show-toplevel` (do **not** use
   `--show-superproject-working-tree` — it is empty from linked worktrees):
   - If toplevel is an initialized submodule path under the super root → that
     path.
   - Else if toplevel is `<super>/<worktrees_dir>/<name>-<slug>`, pick the
     **longest** initialized submodule path `P` such that the basename starts
     with `P-` (disambiguates `alquimia-front` vs `alquimia-front-web`).
2. Reconcile with an explicit `<subrepo>` arg: mismatch → hard error naming
   both the inferred and explicit values.
3. Validate path-first against `.gitmodules`, then unique **name**; unknown or
   ambiguous name → hard error (use the path to disambiguate).
4. Reject uninitialized modules (`git submodule status` `-` prefix); tell the
   user to run `git submodule update --init <path>`.

## Planning root

Artifact phases (`plan-build-flow`), the pre-merge guardian, and renderers
resolve `openspec/changes/<slug>/` against the request **planning root**, never
the process cwd or a subrepo primary checkout. A subrepo request writes its
plans only under `<super>/openspec/changes/<slug>/`.

## Notes

- One worktree per change; keep the branch name and directory slug identical.
- Under submodules the directory is `<worktrees_dir>/<subrepo>-<slug>` (shared
  superproject `.worktrees/`, not a per-module `.worktrees/`).
- Preserve unrelated worktrees — never remove a worktree you did not create.
- Use `worktrees_dir` and `integration_branch` from
  `[recipes.worktree-flow.config]` in `ai-specs/ai-specs.toml`.
