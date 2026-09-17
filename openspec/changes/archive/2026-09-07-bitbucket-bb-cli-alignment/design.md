# Design: Align `bitbucket-pr-flow` with PHP `bb-cli`

## Context

`bitbucket-pr-flow` is the Bitbucket provider of `vcs-pr-flow`. It already
names the `bb` binary and renders as `Bitbucket (`bb` CLI)`, but live surfaces
still install and teach the TypeScript `@pilatos/bitbucket-cli` (paulvanderlei).
The CLI users actually run is Homebrew formula `bb-cli` (PHP, homepage
https://bb-cli.github.io). Shared binary name, different argv.

This change retargets that one provider. It does not add a runtime variant,
does not touch GitHub/GitLab recipes, and does not enable Bitbucket in this
repository's dogfood manifest.

Planning depth is Standard. Delivery is a single PR under the explicit 1200-line
budget; no chain or exception is required.

## Goals / Non-Goals

**Goals:**

- Make `bitbucket-pr-flow` identify PHP `bb-cli` as its sole upstream.
- Offer TTY opt-in `brew install bb-cli` for missing `bb`; never `brew install bb`.
- Rewrite recipe/docs/tests so executable `bb` invocations match the verified
  PHP surface, or stop with an explicit open-verification gap.
- Record apply-time evidence for remaining gaps without running `bb pr create`
  or `bb pr merge`.
- Declare host CLI metadata `version_check = "bb --version"` and
  `min_version = "1.4.1"` while keeping recipe version `1.3.0`.
- Positively verify PHP `bb-cli` identity and block a foreign `bb` with
  `bb-cli` / brew guidance.
- Redact agent-facing `bb auth show` to `Username` only.
- Delete the feature remote branch after merge; keep protected-head exclusions.
- Keep user-authorized warning wording and strip absolute host/worktree paths
  from apply-progress.

**Non-Goals:**

- Dual-runtime or npm / `@pilatos` path.
- Changing `_VCS_RECIPE_LABELS` or the binary name `bb`.
- Enabling `bitbucket-pr-flow` in `ai-specs/ai-specs.toml` / `.ai-specs.lock`.
- Executing `bb pr create`, `bb pr create --help`, or `bb pr merge`.
- Windows / non-brew non-apt installers.
- Doctor behavior changes (URL flows through existing WARN guidance).

## Frozen contracts

Do not touch:

- `openspec/specs/**` (deltas merge at archive).
- `lib/_internal/agents-render.py` `_VCS_RECIPE_LABELS` (`bitbucket-pr-flow` →
  `("Bitbucket", "bb")`).
- `lib/_internal/doctor.py` control flow (still check-only).
- This repo's dogfood `ai-specs/ai-specs.toml` and `.ai-specs.lock`.
- `catalog/recipes/git-pr-flow/**` and `catalog/recipes/gitlab-mr-flow/**`.

---

## Decision 1 — `dep_install` map change

Reuse the existing `InstallPlan` machinery from
`openspec/changes/archive/2026-07-25-deps-env-spoonfeed/design.md`. Do not add
resolvers, kinds, or confirmation UX.

### Exact diff shape

In `lib/_internal/dep_install.py`:

1. Add to `_PACKAGE_MAP`:

   ```python
   "bb": ("bb-cli", ""),
   ```

2. Remove `"bb"` from `_GUIDANCE_ONLY` so the set is only `npx`:

   ```python
   _GUIDANCE_ONLY = frozenset({"npx"})
   ```

3. Update the module comment that currently says "no blind Node / bb
   installs" so it names only `npx` (no blind Node install). Keep the empty
   string convention already documented on `_PACKAGE_MAP`: empty brew or apt
   side means that resolver is skipped.

No other production logic in `resolve_install_plan` or `offer_and_install`
changes. `config_wizard._dep_gate` already resolves plans for missing required
deps and calls `offer_and_install(..., tty=True)` only when both stdin and
stdout are TTYs. Doctor never imports `dep_install`.

### Resolver order (unchanged code, new map row)

`resolve_install_plan(binary, install_url=...)` already does:

1. If `binary in _GUIDANCE_ONLY` **or** `binary not in _PACKAGE_MAP` →
   `kind="guidance"`, `command=[]`, `display=install_url or "install '{binary}'
   manually"`.
2. Else unpack `(brew_formula, apt_pkg)`.
3. If `shutil.which("brew")` and `brew_formula` is non-empty and
   `platform.system()` is `Darwin` or `Linux` →
   `command=["brew", "install", brew_formula]`, `kind="brew"`.
4. Else if `shutil.which("apt-get")` and `apt_pkg` is non-empty and system is
   `Linux` → `command=["sudo", "apt-get", "install", "-y", apt_pkg]`,
   `kind="apt"`.
5. Else guidance (same shape as step 1).

After the map change, `bb` takes path 2 then 3 or 5. It never takes path 1 via
`_GUIDANCE_ONLY`.

### How the empty apt side degrades

For `"bb": ("bb-cli", "")`:

| Environment | Result |
|-------------|--------|
| Darwin + brew | `brew install bb-cli` |
| Linux + brew | `brew install bb-cli` (brew wins over apt, same as `gh`) |
| Linux, apt-get only | `apt_pkg` is `""` → step 4 is skipped → guidance with recipe `install_url` |
| No brew, no apt; Windows; other | guidance with recipe `install_url` |

`offer_and_install` already treats `kind == "guidance"` **or** empty `command`
as print-only (`→ {binary}: {display}`) and never runs a subprocess. Apt-only
`bb` therefore never constructs `sudo apt-get install`.

### TTY confirm vs non-TTY / doctor

- **TTY configure/init:** `_dep_gate` in `config_wizard.py` (~160–176) offers
  the plan, then `offer_and_install` uses `questionary.confirm(...,
  default=False)` before `subprocess.run`. Decline keeps the existing
  "Configure anyway?" gate.
- **Non-TTY:** `offer_and_install(..., tty=False)` returns `[]` immediately.
  `_dep_gate` also skips the offer when stdin/stdout are not TTYs.
- **Doctor:** `_check_recipe_cli_deps` (~635–661 in `doctor.py`) only calls
  `dep_check.check_project_deps`. Missing required `bb` stays WARN with
  `guidance=r.install_url`. No prompt, no brew/apt. After the recipe URL
  rewrite, that guidance string is the PHP homepage; doctor code is untouched.

`dep_check.py` already parses `version_check` / `min_version` from recipe
`[[deps.cli]]`. Hardening fills those fields on `bb`; no parser change.
PATH check shape is unchanged: `shutil.which("bb")`. Numeric version is not
identity; Decision 4 owns the PHP guard.

### `brew install bb` collision (regression-test concern)

Homebrew cask/formula `bb` is getbb.app, not Bitbucket. The map **must** pass
formula `bb-cli`.

**Substring trap:** `"brew install bb" in "brew install bb-cli"` is `True`.
Negative string assertions on the joined display **must not** use a raw
`"brew install bb"` containment check.

Pin the installer with exact argv and token-aware negatives:

- `plan.command == ["brew", "install", "bb-cli"]`
- `plan.display == "brew install bb-cli"`
- `plan.command != ["brew", "install", "bb"]`
- On guidance/apt-only plans: `plan.command == []` and `"bb-cli"` is not
  required in `display` (display is the `install_url`); assert `plan.kind ==
  "guidance"` and that `command` is not `["brew", "install", "bb"]`.

Replace `DepInstallTests.test_bb_guidance_only` in
`tests/test_env_scaffold.py` with mocked Darwin/Linux brew and Linux apt-only
cases. Keep `test_npx_guidance_only` and `test_offer_non_tty_noop` as
invariants that this change must not break.

---

## Decision 2 — Upstream verification for open gaps

Apply (task 5) is the only phase allowed to *promote* a gap into executable
guidance. Design-time fetches below are a snapshot; apply must re-fetch and
file the evidence.

### Where the docs live

The published site is https://bb-cli.github.io. It is a **Just the Docs /
Jekyll** site, not Starlight. Markdown lives in the **docs repo**
[`bb-cli/bb-cli.github.io`](https://github.com/bb-cli/bb-cli.github.io), not in
the PHP implementation repo [`bb-cli/bb-cli`](https://github.com/bb-cli/bb-cli).

Authoritative pages for this change:

| Topic | Published URL |
|-------|----------------|
| Homepage / identity | https://bb-cli.github.io |
| Install (binary / PHP runtime) | https://bb-cli.github.io/installation/ |
| Auth | https://bb-cli.github.io/authentication |
| Help / method inventory | https://bb-cli.github.io/help/ |
| Pull-request commands | https://bb-cli.github.io/docs/commands/pull-request.html |

Recipe `install_url` follows the GitHub sibling pattern (`https://cli.github.com/`):
use **https://bb-cli.github.io**. README may also link `/installation/` and
`/authentication`. Do not point `install_url` at paulvanderlei, npm, or a
Homebrew formula URL (Linux/guidance users need the project docs).

### Allowed probes vs forbidden discovery

Allowed:

- Fetch the URLs above (and the matching markdown from
  `bb-cli/bb-cli.github.io` if the HTML is incomplete).
- Read-only: `bb pr list`, `bb pr show` (optional extra args only if already
  documented on the PR page).

Forbidden:

- `bb pr create`, `bb pr create --help`, `bb pr merge`, and any mutating
  `bb pr` method (`approve`, `decline`, …).
- Inferring flags from TypeScript docs or from this repo's current goldens.

This worktree is a GitHub clone, not a Bitbucket repo. `bb pr list` / `bb pr
show` are **expected to fail** (auth error, "not a Bitbucket repository", or
equivalent). That failure is evidence that the probe was attempted, not
confirmation of create/merge argv. Record command, cwd, exit code, and a
redacted stderr snippet.

Optional: if apply has a Bitbucket-enabled directory already on disk, the same
two commands may be re-run there. Absence of such a repo is not a blocker;
docs + failed probes are enough to keep a gap closed.

### Decision rule (publish vs keep gap)

A flag, positional, or output contract becomes **executable guidance** only when
**all** of:

1. It appears on an authoritative page listed above (or the docs-repo
   markdown that generates that page), **and**
2. It does not require a forbidden command to observe, **and**
3. It is not contradicted by another authoritative page.

Otherwise keep the proposal's **Open verification gaps** label and tell agents
to **stop** rather than guess.

Methods that exist in `bb pr help` (`create`, `show`, `merge`, `list`, …) may
be *named*. Options and output shapes may be published only under the rule
above.

### Design-time snapshot (apply must re-confirm)

Fetched 2026-09-04 from https://bb-cli.github.io/docs/commands/pull-request.html
and https://bb-cli.github.io/authentication:

| Topic | Docs say | Publish now? |
|-------|----------|----------------|
| Create method | `bb pr create [source-branch] [options]` | Yes, method only |
| Create options | `-i`, `--title "..."`, `--description "..."` | Yes, those three |
| Create source | First positional; omitted → current branch | Yes, positional source. **Never** `--source` |
| Create destination | Documented as second positional in `bb pr create develop test 0`; comma-separated targets on the same page | Provisionally yes as second positional **if apply re-fetch still shows that line**; still never `--destination` |
| Create body | Not documented; PHP uses `--description` | Never `--body` |
| Show | `bb pr show <id> [unresolved]` — **comments**, not PR JSON | Yes as comments viewer. **Never** `--json --jq`. SHA guard stays a gap |
| Merge | `bb pr merge` (id implied) | Yes, method + id. **Never** `--strategy squash` or `--close-source-branch` until documented |
| Auth | `bb auth` / `bb auth save`; `bb auth show` | Yes those verbs. Exact prompts remain a gap; do not invent save flags |
| List | `bb pr list [branch]` (docs typo: "brach") | Probe + optional inspect. Destination-branch listing is documented |

`bb pr show` is **not** a drop-in for TypeScript `bb pr view --json --jq
'.source.commit.hash'`. Keep the approval-SHA *policy* (do not merge a moved
branch) as a stop-with-guidance step until a verified retrieval exists.
`bb pr commits` is documented as a method but is **not** an allowed probe;
do not treat its output shape as executable.

### Evidence file

Write findings into `openspec/changes/bitbucket-bb-cli-alignment/apply-progress.md`
(created during apply, not this phase). Minimum section:

```markdown
## Upstream verification (PHP bb-cli)

- Observed `bb version`: …
- Docs fetched (URL, date, excerpt or quote of the relevant argv): …
- Probes: `bb pr list` / `bb pr show` (command, cwd, exit, redacted stderr)
- Promoted to executable: …
- Remaining open gaps: …
```

After promotion, update only the matching README/command/skill/catalog lines.
Host `version_check` / `min_version` and the PHP identity guard are now in
scope (Decisions 3–4); do not widen further.

Dogfood isolation: do not `ai-specs sync` this repo's `ai-specs/` as a
Bitbucket enablement. Sync/eval coverage uses temporary fixtures. If a live
CLI smoke run mutates tracked dogfood files, revert them; quote the terminal
output in apply-progress / verify-report instead.

---

## Decision 3 — Recipe surface rewrite

One coherent identity: PHP `bb-cli`, binary still `bb`, recipe version still
`1.3.0` (no bump; examples currently say `1.1.0` and must catch up).

### File-by-file

| File | Change |
|------|--------|
| `catalog/recipes/bitbucket-pr-flow/recipe.toml` | `install_url = "https://bb-cli.github.io"`; rewrite `auto_switch_account` help to drop paulvanderlei and state there is no `auth switch` (point at `/authentication` if a URL is needed). Keep `binary = "bb"`, recipe `version = "1.3.0"`. Add `version_check = "bb --version"` and `min_version = "1.4.1"`. Brief `workflow_rules` already say `Bitbucket (bb CLI)` — leave renderer-facing identity. |
| `catalog/recipes/bitbucket-pr-flow/README.md` | PHP identity; `bb auth show` / `bb auth save`; enablement example `version = "1.3.0"`; document host floor `1.4.1` as distinct; identity guard (Decision 4); create/merge examples only if verified, else labeled gaps. Drop `--close-source-branch` as if it were a PHP flag; keep protected-head *policy*. |
| `…/commands/bb-pr-create.md` | Install blocker URL → bb-cli.github.io. Auth blocker → `bb auth save`, not `login`. Capture `bb auth show` and emit only `Username`; reject missing or multiple `Username` lines; never print `AppPassword`. Positive PHP identity guard via `bb --version`. Create step: verified PHP argv or an explicit **stop** note. Keep user authorization, preflight-before-push, dynamic remote, stop-after-create, no merge. |
| `…/skills/bitbucket-merge-workflow/SKILL.md` | Same auth/install retarget, redacted Username capture, and PHP identity guard. `pr show` only as comments (not JSON SHA). Merge: `bb pr merge <id>` without unverified flags. Preserve protected-head class, no-auto-merge, pre-merge archive, guardian, worktree/git cleanup for feature heads. After merge, explicitly delete the feature remote branch (`git push $REMOTE --delete`); never delete a protected head. Feature-head "close source branch" is git/UI policy, not `--close-source-branch`. |
| `docs/recipes-catalog.md` | Bitbucket section: PHP contract, `1.3.0` example, install-offer wording (`brew install bb-cli` on macOS). Do not edit unrelated recipes. |
| `docs/recipe-schema.md` | Known TTY map includes `bb` → formula `bb-cli` / empty apt; **only `npx` remains guidance-only**. |

`lib/_internal/doctor.py` and `agents-render.py`: no edits. Corrected
`install_url` appears in doctor WARN text automatically.

### Conservative create snippet (pending apply re-fetch)

If apply re-confirms the PR docs page, the create-only command becomes:

```bash
bb pr create <branch-name> <base_branch> --title "<title>" --description "<summary>"
```

Do not use `-i` as the agent default (interactive prompts). Do not publish
`--source`, `--destination`, or `--body`. If the second positional is no
longer documented at apply time, keep destination as an open gap and stop
before create.

### Tests: which file pins which surface

| Test | Pins |
|------|------|
| `tests/test_env_scaffold.py` `DepInstallTests` | Map: brew `bb-cli`, empty apt → guidance, never argv `["brew","install","bb"]`, `npx` still guidance-only, non-TTY noop |
| `tests/test_bitbucket_pr_flow_recipe.py` | Manifest URL/identity; `version_check = "bb --version"` and `min_version = "1.4.1"`; skill+command goldens: `bb auth save`/`show`, redacted Username-only capture, PHP identity guard, `bb pr create`/`show`/`merge`, recipe version `1.3.0` where asserted, **absence** of paulvanderlei / `@pilatos` / `bb auth login` / `bb pr view`; unverified flags either absent from executable examples or labeled as open gaps; explicit feature `git push $REMOTE --delete`; apply-progress has no absolute host/worktree paths; keep preflight-before-push, stop-after-create, no auto-merge, archive-before-merge order. Rename `test_skill_merge_closes_source_branch`. |
| `tests/test_recipes_catalog.py` `BitbucketPrFlowDocsContractTests` | README/catalog identity, recipe `1.3.0` distinct from host `1.4.1`, `bb --version`; schema/catalog no longer call `bb` guidance-only |
| `tests/test_sync_pipeline.py` | Fixture `version = '1.3.0'` (today `'1.1.0'` around the defaulted-base-branch Bitbucket workspace). Renderer assertion stays `Bitbucket (`bb` CLI)` |
| `tests/evals/scenarios/bitbucket-pr-flow/*` | PHP verbs; **do not require** `--close-source-branch` or `--strategy squash` / `--squash` as executable. Protected-head scenario: still forbid closing/deleting `development`; merge line may be `bb pr merge` without those flags. Feature-head scenario: require git/worktree cleanup, not the unverified bb close flag. `eval_vcs_pr_flow_live.py` scenario list unchanged; still no real remote merge |
| `tests/test_dep_check.py` | Unchanged (generic parser already covers `version_check` / `min_version`) |

Negative catalog/recipe assertions should search live Bitbucket surfaces only
(not GitLab `glab auth login`, not changelog `1.1.0` history).

### RED → GREEN per work unit

`tdd-flow` + `work-unit-commits`: tests travel with the behavior they pin. Do
not land a commit of red tests.

| Work unit | RED (local) | GREEN (same commit) |
|-----------|-------------|---------------------|
| WU1 Install map | Extend `DepInstallTests`; run `python3 -m unittest tests.test_env_scaffold.DepInstallTests` — current `test_bb_guidance_only` / new brew assertions fail | `_PACKAGE_MAP` / `_GUIDANCE_ONLY` edit; re-run the same command green |
| WU2 Recipe contract | Change goldens in `test_bitbucket_pr_flow_recipe.py` + catalog identity asserts; focused unittest goes red | Rewrite recipe.toml, README, command, skill, `docs/recipes-catalog.md`, `docs/recipe-schema.md` until those tests pass |
| WU3 Sync fixture + evals | `test_sync_pipeline` version `1.3.0`; eval scenario/prompt flag updates | Same commit; focused sync/catalog tests green. Evals stay `EVALS_LIVE=1` optional |
| WU4 Apply verification | n/a (docs/probes) | `apply-progress.md` evidence; only then promote remaining argv in surfaces + tighten tests |
| WU5 Hardening RED | Invert/add goldens for `version_check`/`min_version`, Username-only auth, PHP identity guard, feature `git push $REMOTE --delete`, apply-progress path hygiene; rename misleading merge test. Do not run tests in the RED-only planning slice. | Production surfaces in WU6 |
| WU6 Hardening GREEN | n/a until WU5 tests exist | Recipe.toml version metadata; skill/command identity + redacted auth + remote delete; docs host `1.4.1`; strip absolute paths from apply-progress |

Task 5 (upstream verification) may land inside WU2 if the design-time snapshot
still matches at apply start; otherwise WU2 ships conservative gap labels and
WU4 promotes. Never run create/merge to "finish" WU2.

---

## Decision 4 — Positive PHP `bb-cli` identity guard

Both CLIs install an executable named `bb`. `command -v bb` and
`dep_check._which` cannot tell them apart. `min_version = "1.4.1"` is a
numeric floor only; a foreign `bb` that prints a parseable version can still
pass dep check.

**Required preflight (skill and command), after `command -v bb`:**

1. Run `bb --version` (same command as recipe `version_check`).
2. Positively confirm PHP `bb-cli` identity (not "fail later on PHP verbs").
3. If the binary is not PHP `bb-cli`, **block** with guidance to install
   Homebrew formula `bb-cli` from https://bb-cli.github.io. Never propose
   `brew install bb`.

README keeps a short collision note. Live surfaces MUST NOT name `@pilatos`
or paulvanderlei even as a negative example; say "TypeScript Bitbucket CLI".

## Decision 5 — Redacted `bb auth show`

Observed PHP shape: `Username: <value>` and `AppPassword: <secret>`.

Agent-facing auth checks (skill Runtime Preflight and `bb-pr-create`) MUST:

1. Capture `bb auth show` output (never print the raw command result).
2. Emit only the `Username` line.
3. Reject when `Username` is missing or appears more than once.
4. Never print `AppPassword`.

Account-match comparison uses the single captured `Username` value. Empty or
ambiguous parse still stops; there is no `bb auth switch`.

## Decision 6 — Feature remote-branch deletion

After a merged feature PR, cleanup MUST explicitly delete the feature remote
branch (`git push $REMOTE --delete <branch-name>` or the worktree-cleanup
script's remote-deletion step) and verify absence. Protected heads (`main`,
`master`, `development`, `staging`, configured base / integration branch)
MUST remain excluded from UI close-source, worktree cleanup, local `-D`, and
remote delete.

Rename `test_skill_merge_closes_source_branch` so the test name matches this
policy (PHP merge does not take `--close-source-branch`; remote delete is a
separate git step for feature heads only).

## Decision 7 — Authorized warning wording and apply-progress hygiene

User-authorized warning locks on live surfaces:

- Never emit the three-token string `brew install bb`, even as "never do this".
  Write "never Homebrew formula/cask `bb` (getbb.app)".
- Never name `@pilatos` / paulvanderlei, even as a negative example.
- Never emit the substring `bb auth login`. Write "not a login subcommand".

`apply-progress.md` MUST NOT embed absolute host or worktree paths (no
`/Users/…`, `/home/…`, `/opt/homebrew/…`, or a concrete
`.worktrees/<slug>` absolute root). Refer to the change worktree relatively.

---

## Data flow

```mermaid
sequenceDiagram
  participant U as User
  participant W as config_wizard
  participant D as dep_check
  participant I as dep_install
  participant Doc as doctor
  participant R as recipe surfaces

  W->>D: check_cli_deps (shutil.which bb)
  alt missing required and TTY
    W->>I: resolve_install_plan("bb", install_url)
    alt brew + formula bb-cli
      I->>U: confirm brew install bb-cli
    else apt-only or no resolver
      I->>U: print install_url (no command)
    end
    W->>D: re-check
  end
  Doc->>D: check_project_deps
  Doc-->>U: WARN + install_url (never install)
  R-->>U: PHP verbs or labeled gap; stop if unverified
```

---

## Risks and rollback

| Risk | Mitigation |
|------|------------|
| `brew install bb` installs getbb.app | Map formula `bb-cli`; exact-argv tests; token-aware negatives; catalog wording |
| TypeScript `bb` still on PATH | Decision 4 positive identity guard; block with bb-cli/brew guidance |
| Publishing unverified create/merge/show flags | Decision 2 rule; tests forbid TypeScript argv unless labeled gap |
| TypeScript consumers see a breaking guidance change | Intended; no in-recipe variant |
| Numeric version without identity | `min_version` is a floor; Decision 4 is the identity control |
| Raw `bb auth show` leaks AppPassword | Decision 5: capture and emit Username only |
| Feature remote left after merge | Decision 6: explicit remote delete; protected heads excluded |
| Absolute paths in apply-progress | Decision 7: host/worktree paths stripped |
| Eval prompts still teach `--squash` / `--close-source-branch` | Update Bitbucket eval prompts/scenarios in WU3 |
| Substring-false-green on `"brew install bb"` | Exact `plan.command` equality |

**Rollback:** revert the change branch / PR. No data migration. Users who
already ran `brew install bb-cli` keep that formula; recipe docs and the
install map return to guidance-only `bb` with the previous URL.

---

## Rollout

Single PR on this worktree branch. First apply was Medium; hardening adds
version metadata, identity/auth/remote-delete goldens, and apply-progress
hygiene in the same change folder. Re-forecast at hardening GREEN before PR.

Validation (task 6, then task 8 after hardening): focused unittests listed
in tasks.md, then `./tests/run.sh` and `./tests/validate.sh` from the
repository root. Frozen contract diff check: no paulvanderlei / `@pilatos` /
`bb auth login` / `bb pr view` / `brew install bb` (as a three-token
command) / stale Bitbucket `1.3.0` vs `1.1.0` drift on live surfaces;
recipe `1.3.0` remains distinct from host `1.4.1`.

Parent close-out follows implementation and validation.
