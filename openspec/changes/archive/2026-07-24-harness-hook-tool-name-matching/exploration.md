## Exploration: harness-hook-tool-name-matching

**Trigger:** Live incident in this repo's own dogfood session. An
omp-family orchestrator session delegated an edit of `lib/sync.sh` /
`lib/sync-agent.sh` to a `task` subagent. The subagent edited both files
directly on the `development` branch, in the main worktree, while
`worktree-flow:worktree-gate` was generated, wired, and enabled
(`WORKTREE_GATE_PROTECTED="main development"`). The gate never fired — no
block, no warning, nothing. The user noticed only because the files showed
up as dirty in `git status` afterward.

**Correction during this exploration:** the orchestrator's first hypothesis
(a tool-name regex mismatch — this session's tools are named `_edit`/`_write`
with a leading underscore, which a `^(?:Edit|Write|...)$` matcher would never
match) was tested directly and **refuted**: a direct, non-delegated write
attempted in the main worktree on `development` during this exploration was
correctly blocked by the same gate, with the exact expected error text from
`worktree-gate.sh:80`. So the regex/tool-naming path works for
directly-invoked tools in this session. The real mechanism is documented
elsewhere in this very repo (see Decision 1) and is a different, better-known
failure mode: **the runtime's pre-tool-use hook event does not fire for
subagent-delegated tool calls at all.**

---

### Architecture Map

```
catalog/recipes/<recipe>/recipe.toml
  [[provides.hooks]] { id, event, script, matcher? }
        ▼
recipe-materialize.py → resolved-hooks.json
        ▼
hooks-render.py render(agent) — per-agent dispatch (lib/_internal/hooks-render.py:373-416)
        ├─ claude   → .claude/settings.json  PreToolUse (native, exit 2 = block)
        ├─ cursor   → warn-and-skip for file-write matchers (no pre-file-write event at all)
        ├─ opencode → .opencode/plugin/<id>.ts   tool.execute.before  (DOCUMENTED: does not fire for subagent/MCP calls — opencode#5894, #2319)
        ├─ pi       → .pi/extensions/<id>.ts     pi.on("tool_call", …)  (docs/runtime-hooks.md claims "✅ covers all tool calls" — UNVERIFIED against subagent delegation in this exploration)
        └─ omp      → .omp/extensions/<id>.ts    pi.on("tool_call", …)  (structurally identical handler to pi's; ABSENT from docs/runtime-hooks.md's compatibility tables entirely)
```

Both `render_pi` and `render_omp` register the exact same
`pi.on("tool_call", (call) => { ... })` handler shape
(`.pi/extensions/worktree-flow-worktree-gate.ts:12-13`,
`.omp/extensions/worktree-flow-worktree-gate.ts:12-13`) against two different
import packages (`@earendil-works/pi-coding-agent` for pi,
`@oh-my-pi/pi-coding-agent` for omp). Whether that host API dispatches
`tool_call` events for a delegated `task`/subagent's own tool invocations —
or only for the top-level session's own tool calls — is a property of the
*host extension runtime*, not of anything `ai-specs` generates. This
exploration found no evidence either package's `tool_call` event is
specified, tested, or documented (by ai-specs or upstream) to include
subagent-originated calls.

---

### Decision 1: What does this repo already know?

`docs/runtime-hooks.md` has an explicit "Known gaps" section (lines 61-69)
and a "Status" table (lines 103-110):

| harness | `pre-tool-use` blocking | notes |
|---|---|---|
| claude | ✅ | native exit-2 |
| pi | ✅ | **covers all tool calls** |
| opencode | ✅ (primary agent) | **not subagent/MCP tool calls** |
| cursor | ⚠️ | no pre-file-write hook; shell/MCP gates only |

**`omp` is absent from every table in this document** — the abstract→native
event map (line 36), the per-harness distribution table (line 49), and the
status table (line 105) all list only `claude | cursor | opencode | pi`.
Yet `hooks-render.py` has a full `render_omp()` (lines 320-368),
`lib/_internal/platform.sh:133-152` documents omp as a first-class agent with
its own `runtime_hooks_target`, and `README.md`/`CHANGELOG.md` list omp as a
supported harness alongside the other four. **This documentation was never
updated when omp support shipped** — there's a real 5th platform in code with
zero entry in the one document meant to be "the single source of truth"
(the doc's own words, line 33) for hook behavior per harness.

The `pi | ✅ | covers all tool calls` claim is stated as fact but this
exploration found no test, eval scenario, or code comment substantiating it
specifically for *subagent-delegated* tool calls — only the *absence* of a
caveat, in contrast to opencode's explicit sourced caveat (`opencode#5894`).
An absent caveat is not the same as a verified claim.

**Finding:** there is no catalog, test, or eval coverage answering "does this
harness's pre-tool-use hook fire for a subagent's tool calls?" for pi or omp.
The opencode caveat exists because someone hit and filed it upstream; nothing
suggests an equivalent investigation was ever done for pi/omp, and this
session's own incident is direct evidence that at least this omp-family
build behaves like the *undocumented* case, not the claimed "✅ covers all
tool calls" case.

---

### Decision 2: Live-eval coverage doesn't (and structurally can't) catch this

`tests/evals/lib/harness.py` invokes real `claude`/`cursor-agent`/`opencode`/
`pi`/`omp` CLI binaries directly, with `wire_runtime_hooks()`
(`harness.py:109`) doing exactly what `sync-agent.sh` does. But every eval
scenario drives the target runtime with a **single top-level agent
completing the task itself** — none of the live gate scenarios in
`eval_plan_build_flow_live.py` (`requires_hook` scenarios, lines 236-239)
spin up a nested subagent/delegated tool call to see whether the gate still
fires one level down. `NO_FILE_WRITE_HOOK_RUNTIMES = frozenset({"cursor-agent"})`
(`harness.py:101`) captures the *known* opencode subagent gap only implicitly
(by skipping cursor-agent for an unrelated reason — cursor has no
pre-file-write event at all, not a subagent issue) — it does not model
"subagent tool calls" as its own eval dimension for **any** runtime,
including opencode, where the gap is already documented. **The eval suite,
as designed today, cannot distinguish "gate fires for direct edits" from
"gate fires for delegated/subagent edits too" for any of the 4 runtimes that
support blocking hooks.** This is why the gap was invisible until a real
multi-agent session (this one) happened to delegate the exact file the gate
protects.

---

### Decision 3: Is the tool-naming regex a real, separate bug?

Yes — smaller in blast radius, but real, and worth fixing in the same change
since it touches the identical three renderer functions:

- `render_pi`/`render_omp` (`hooks-render.py:290,343`) use
  `new RegExp(`^(?:${MATCHER})$`, "i")` — case-insensitive.
- `render_opencode` (`hooks-render.py:239`) uses
  `new RegExp(`^(?:${MATCHER})$`)` — **no `"i"` flag**, inconsistent with its
  siblings, with no comment explaining the asymmetry.

[INFERENCE] If OpenCode's real `tool.execute.before` reports lowercase tool
ids (as the pi/omp renderer's own comment asserts is generally true of
"pi/omp tool names": `hooks-render.py:288-290,341-343`), then
`^(?:Edit|Write|MultiEdit|NotebookEdit)$` without the `i` flag never matches
a real OpenCode edit/write call, independent of any subagent question. This
was not independently re-verified against a live OpenCode binary during this
exploration (no such binary available in-session) — flag for confirmation in
design/proposal, e.g. by running `eval_plan_build_flow_live.py`'s
`requires_hook` scenario against a real `opencode` binary and checking
whether the gate actually blocks.

This is a genuinely distinct, narrower bug from the subagent-visibility gap:
even a top-level, non-delegated OpenCode tool call would plausibly bypass the
gate today, whereas pi/omp's case-insensitive regex is correct for
top-level calls (confirmed empirically in this exploration) and only fails
one level down, for subagents.

---

### Decision 4: Blast radius

Recipes shipping `[[provides.hooks]]` (grepped `catalog/recipes/*/recipe.toml`):

| Recipe | Hook id | Matcher |
|---|---|---|
| `worktree-flow` | `worktree-gate` | `Edit\|Write\|MultiEdit\|NotebookEdit` |
| `plan-build-flow` | `plan-build-gate` | `Edit\|Write\|MultiEdit\|NotebookEdit` |

Both are the project's two enforcement-critical gates. Any project on this
recipe set, using pi/omp/opencode with a delegation-capable agent (any
harness offering a "spawn subagent"/"task" primitive, which is now standard
across omp, pi, and reportedly opencode/claude-adjacent tooling), has a
silent bypass whenever work is delegated rather than done inline — which is
precisely the pattern this project's own `AGENTS.md`/orchestrator rules
*encourage* ("delegation is the default," "fan work out to task subagents").
**The gate and the delegation-first workflow this project mandates are
currently in direct, silent tension**: following the recommended workflow
(delegate multi-file changes to subagents) is the exact scenario that
defeats the gate meant to keep those same changes off protected branches.

---

### Decision 5: Remediation options

| Option | Description | Pros | Cons |
|---|---|---|---|
| A. Document + fail loud, don't silently trust | Update `docs/runtime-hooks.md` to add the missing `omp` row, mark pi/omp's subagent coverage as **unverified** (not "✅") pending evidence, and add explicit guidance: "hooks in this recipe family are known/suspected not to cover subagent tool calls on opencode/pi/omp; do not rely on them as the sole guard for delegation-heavy workflows" | Zero code risk; ships immediately; honest about current state | Doesn't fix the gap — an agent can still ignore the doc, and the point of a gate is to not depend on the agent reading docs |
| B. Orchestrator-level guard in `AGENTS.md`/runtime rules | Add an explicit rule (already partially present: "Ask before destructive git operations"; extend to "verify current worktree/branch before delegating any write-capable subagent") requiring the orchestrator to check `git worktree`/branch state itself before dispatching a write task, independent of the runtime hook | Defense-in-depth at the layer that actually failed here (the orchestrator delegated without checking); doesn't depend on unreliable runtime hook events | Prompt-level guidance, not enforced by tooling — same class of "hope the agent reads it" weakness as A, just at a different layer; should be paired with a real check |
| C. `sync-agent.sh` / `init.sh` doctor check: verify current-worktree-state before any generation step that could imply active work | Not applicable here — doctor/sync don't run per-tool-call, they run at sync time. Not a fit for this specific gap (runtime tool-call gating), noted only to rule out | — | Wrong layer; doctor cannot intercept individual tool calls at agent runtime |
| D. Escalate upstream: file/confirm the omp/pi equivalent of opencode#5894 | Since `pi`/`omp`'s `tool_call` event subagent-visibility is unverified (Decision 1), determine ground truth by reading `@oh-my-pi/pi-coding-agent` / `@earendil-works/pi-coding-agent` source (or filing upstream) rather than assuming either the "✅" claim or this session's single incident generalizes | Turns an assumption into a fact; corrects `docs/runtime-hooks.md` accurately either way | Outside this repo's control if the answer is "the API genuinely doesn't dispatch subagent tool calls" — becomes an upstream feature request, not something `ai-specs` can fix unilaterally |
| E. Fix the separate opencode case-sensitivity regex bug (Decision 3) | Add the missing `"i"` flag to `render_opencode`'s generated regex, matching `render_pi`/`render_omp` | Small, safe, immediately correct regardless of the subagent question | Does not address the subagent-visibility gap (opencode's subagent gap is a *host API* limitation per opencode#5894, not a regex bug — fixing the regex only helps opencode's *top-level* tool calls) |

**Recommendation: D first (determine ground truth for pi/omp), then A + B
together, plus E as a small independent fix.**

The core problem is an **unverified compatibility claim** ("pi: ✅ covers all
tool calls") contradicted by one real incident. Before writing more code or
docs, confirm via source/upstream (D) whether omp/pi's `tool_call` truly
never fires for subagent calls (matching opencode) or whether this
particular omp build/version has a bug or config gap opencode's contract
doesn't have. That answer determines whether the fix is "correct the docs
and add an orchestrator-level guard" (A+B, if the limitation is inherent to
the host API) or "file/fix a real omp defect" (if it's a regression relative
to what the API is supposed to support). Either way, A (accurate docs) and B
(orchestrator-level defense-in-depth, since a tool-call hook is proven
unreliable at least for delegated work) should ship regardless of D's
answer — a security-relevant gate should never have its only enforcement
layer be "an event that may or may not fire depending on delegation depth,"
undocumented either way. E is small, safe, and independent — ship it
alongside without waiting on D.

---

### Key Risks

1. **This project's own recommended workflow (delegate to subagents by
   default) is the exact trigger for the gap.** Any fix framed purely as "fix
   the hook" without also addressing "the orchestrator should not rely
   solely on a hook it cannot verify fires for delegated work" leaves the
   same class of incident able to recur under normal, encouraged usage.
2. **The omp compatibility claim was inherited from pi without being
   re-verified**, and omp isn't even documented as a distinct row — future
   omp-specific behavior changes (fork drift from upstream pi) have no
   tracked baseline to regress against.
3. **Decision 3's opencode claim is [INFERENCE]** — not independently
   re-verified against a live OpenCode binary in this exploration.
4. **Fixing only the regex (E) would give false confidence** — it looks like
   "the hook gap is fixed" while the actual reported incident (subagent
   delegation) remains completely unaddressed by a regex change.

---

**Proposal** → `openspec/changes/harness-hook-tool-name-matching/proposal.md`.
Recommend scoping the proposal to: (1) verify pi/omp subagent tool-call
visibility against source/upstream, record the finding; (2) correct
`docs/runtime-hooks.md` (add missing `omp` row; downgrade "✅ covers all tool
calls" to accurate, sourced status); (3) add an orchestrator-level
pre-delegation worktree/branch check to the project's runtime rules,
independent of any runtime hook; (4) fix `render_opencode`'s missing
case-insensitive regex flag as a small, unrelated-but-adjacent correction.
