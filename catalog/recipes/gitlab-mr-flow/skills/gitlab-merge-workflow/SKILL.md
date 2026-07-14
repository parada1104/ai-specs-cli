[SKILL.md#6F02]
1:---
2:name: gitlab-merge-workflow
3:description: >
4:  Provider-oriented merge workflow for feature branches created in worktrees.
5:  Uses the configured base branch from
6:  [recipes.gitlab-mr-flow.config] (base_branch). GitLab via the glab CLI.
7:license: MIT
8:metadata:
9:  author: ai-specs
10:  version: "1.0"
11:  generatedBy: "manual-runtime"
12:  scope: [root]
13:  auto_invoke:
14:    - "Creating a merge request on GitLab"
15:    - "Merging a feature branch via GitLab MR"
16:    - "Cleaning up a worktree after merge"
17:    - "Finishing work on a feature branch"
18:    - "Syncing development after a merge"
19:---
20:
21:# GitLab Merge Workflow
22:
23:Use this skill only when the user explicitly asks to create an MR, merge, finish
24:a branch, or clean up after merge on GitLab.
25:
26:Use the configured base branch from `[recipes.gitlab-mr-flow.config]` (`base_branch`).
27:This recipe implements GitLab through the `glab` CLI. Honor any no-push/no-merge rules
28:declared for the project.
29:
30:## Preconditions
31:
32:- User explicitly requested MR/merge/cleanup.
33:- Working branch belongs to one focused change.
34:- Worktree has no unrelated uncommitted changes.
35:- Required verification evidence is complete or the user accepts the gap.
- A change folder under `openspec/changes/<slug>/` (excluding `archive/`) exists
  on the branch with at least `tasks.md` committed. If missing, stop before PR
  creation and complete planning first.
36:- `glab` is installed and authenticated.
37:
38:## Runtime Preflight
39:
40:Before any push or MR creation, verify the GitLab CLI is available:
41:
42:```bash
43:command -v glab
44:```
45:
46:If `glab` is not found, stop and report:
47:
48:> **Blocker**: `glab` is not installed. Install it from https://gitlab.com/gitlab-org/cli
49:> and retry.
50:
51:Then verify authentication:
52:
53:```bash
54:glab auth status
55:```
56:
57:If authentication fails, stop and report:
58:
59:> **Blocker**: `glab` is not authenticated. Run `glab auth login` and retry.
60:
61:Then verify `jq` is available (required for SHA pinning during merge):
62:
63:```bash
64:command -v jq
65:```
66:
67:If `jq` is not found, stop and report:
68:
 69:> **Blocker**: `jq` is not installed. Install it from https://jqlang.github.io/jq/download/ and retry.
 70:
 71:Then run **Runtime Preflight: Account Match** when `expected_owner` is set in
 72:`[recipes.gitlab-mr-flow.config]` (skip when empty — no extra CLI calls):
 73:
 74:```bash
 75:# Runtime Preflight: Account Match (GitLab)
 76:EXPECTED_OWNER="{config.expected_owner}"
 77:if [ -n "$EXPECTED_OWNER" ]; then
 78:  ACTIVE=$(glab auth status 2>&1 | awk '
 79:    /Logged in to gitlab\.com account/ {
 80:      if (match($0, /account [^ ]+ \(/))      { a=substr($0, RSTART+8, RLENGTH-2) }
 81:      else if (match($0, /account [^ ]+$/))   { a=substr($0, RSTART+8) }
 82:    }
 83:    /Active account: true/ { print a }' | head -1)
 84:  if [ "$ACTIVE" != "$EXPECTED_OWNER" ]; then
 85:    echo "**Blocker**: active glab account is '$ACTIVE'; expected '$EXPECTED_OWNER'."
 86:    echo "glab has no 'auth switch'. Run: glab auth login   (or export GLAB_TOKEN=<token>)."
 87:    return 1
 88:  fi
 89:fi
 90:```
 91:
 92:## Workflow
 93:
 94:1. Inspect current branch, worktree path, and `git status`.
 95:2. Run or confirm any verification required before merge.
 96:3. Run **Runtime Preflight** (CLI checks + account match above).
 97:4. Resolve the GitLab remote and push the feature branch explicitly:
76:
77:```bash
78:REMOTE=$(git remote | grep -E '^(origin|gitlab|upstream)$' | head -1 || echo "origin")
79:git push -u $REMOTE <branch-name>
80:```
81:
82:> **Note**: The remote is resolved dynamically to support repos where the GitLab remote is named `gitlab` or `upstream` instead of `origin`. Falls back to `origin` if no known name matches.
83:
84:4. Create a merge request with the configured base branch:
85:
86:```bash
87:glab mr create --source-branch <branch-name> --target-branch <base_branch> --title "<title>" --description "<summary and verification>" --yes
88:```
89:
90:5. STOP. Do not merge. Report the MR URL and wait for explicit user approval.
91:
92:6. Before merging, capture the approved MR head SHA to prevent merging unreviewed commits:
93:
94:```bash
95:APPROVED_SHA=$(glab mr view <mr-number> --output json | jq -r '.sha')
96:```
97:
98:7. Before merging, archive and record SDD/OpenSpec artifacts for the change
   while still on the review branch. The archive boundary is the pre-merge
   branch state — never defer this step until after the merge lands on the base
   branch. Commit and push any archive commits to the review branch before
   proceeding.

8. Merge only after explicit user approval, required checks/review, the
   pre-merge archive step above, and pinning the approved SHA:
99:
100:```bash
101:glab mr merge <mr-number> --squash --yes --remove-source-branch --sha $APPROVED_SHA
102:```
103:
104:> **Note**: The `--sha` flag ensures that only the reviewed commit is merged. If the branch was updated between approval and merge, the command will fail, preventing unreviewed commits from being merged.
105:
106:9. After the MR is merged, navigate to the main repo root first (the agent may
107:   be running inside the worktree, and removing it while `$PWD` points there
108:   causes `fatal: Unable to read current working directory`). Then remove the
109:   worktree and force-delete the local branch:
110:
111:```bash
112:cd <main-repo-root>
113:git worktree remove <absolute-path-to-worktree>
114:git branch -D <branch-name>
115:```
116:
117:> **Note**: `git branch -D` (capital D) is required because `glab mr merge --squash`
118:> rewrites history — the feature branch commits are not ancestors of the target
119:> branch, so `git branch -d` would refuse with "not fully merged". Force-delete
120:> is safe here because the MR was already merged.
121:
122:10. Sync the integration branch:
123:
124:```bash
125:git checkout <base_branch>
126:git pull --ff-only origin <base_branch>
127:```
128:
129:## Guardrails
130:
131:- Never merge locally with `git merge` for feature work that should go through MR.
132:- Never push, merge, delete branches, or remove worktrees without explicit user instruction.
133:- Never remove a worktree before confirming the MR is merged and no uncommitted work remains.
134:- Preserve unrelated changes; stop and ask if cleanup would touch them.
135:- Never use implicit push options on `glab mr create` — always push explicitly before creating the MR.
136:- Never use options that merge without explicit user approval.
137:- If `glab` is unavailable or unauthenticated, stop with the exact blocker before pushing or creating an MR.
138: