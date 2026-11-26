package cli

// `ai-specs plan-build-gate` — the CLI-served planning gate (cli-served-recipes
// T7, S3/F3, change-ledger-v2 T2).
//
// The verb no longer authorizes a write: in both execution modes it derives the
// change tier from the artifacts that exist (none, light, standard) and refuses
// only on an objective refutation R1–R2, each naming the fact that proves it and
// the exits that resolve it. A worktree with no artifact and no product-path
// divergence is nothing to grade: it exits 0, tier none.
//
// One mode: a pre-tool-use event on stdin. A production path write (the
// PLAN_BUILD_GATE_PATHS scope, default src lib catalog) is graded at the
// `work-start` ledger checkpoint first.
//
// The canonical spec tree (odd/specs/ or openspec/specs/) is the one protected
// path in either mode, because promotion is its only writer: a hand write there
// is refused as R1 unless the paired change carries a delta. Every other write
// under odd/ and every write under openspec/changes/ stays allowed.
//
// Exit codes: 0 allows the write, 2 refuses it. Malformed or unrelated input
// fails open, and an absent or unreadable refutation input never becomes an
// error: no fact proves a refutation, so the write is allowed.
//
// The merge-time guardian is separate and unchanged; this host only reads the
// same tier derivation and the same refutation rules.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/gates"
	"ai-specs.dev/ai-specs/internal/hook"
	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/tier"
	"ai-specs.dev/worktree-gate/runner"
	shared "ai-specs.dev/worktree-gate/shared"
)

const (
	// planBuildGateName is the program label of every message. The verb replaced
	// the script of the same name, so the prefix is unchanged.
	planBuildGateName = "plan-build-gate"
	// planBuildGateProdDirsEnv overrides the graded production directories.
	planBuildGateProdDirsEnv = gates.ProductDirsEnv
	// planBuildGateDefaultProdDirs is the default of that env.
	planBuildGateDefaultProdDirs = gates.ProductDirs
	// planBuildGateCheckpoint is the checkpoint this host grades.
	planBuildGateCheckpoint = "work-start"
)

// planBuildGateUsage is the verb's help text, on stdout for --help/-h.
const planBuildGateUsage = `Usage: ai-specs plan-build-gate [--explain]

Pre-tool-use hook: read one hook event (JSON) on stdin and grade the write. In
both modes the gate infers the change tier from the artifacts that exist (no
plan and no delta = none, either one = light, both = standard) and refuses only
when an objective refutation is proven: R1 a canonical spec (odd/specs/ or
openspec/specs/) written without its delta, R2 a project-declared high-risk path
without the evidence the project requires. Every refusal names the rule,
the fact and the exits. No plan, declaration or authorization is required for a
production write in any mode, and a worktree with no artifact and no product
change is nothing to grade.

The mode is read from [recipes.plan-build-flow.config] mode in
ai-specs/ai-specs.toml: "auto" (default), "odd" or "openspec". "auto" resolves
"odd" when the Gentle AI state directory <git-common-dir>/gentle-ai/ exists, and
"openspec" otherwise. An explicit mode wins over the inference.

Options:
  --explain           Also report the inferred tier and the rules that fired
  --help, -h          Show this help

Environment:
  PLAN_BUILD_GATE_PATHS  Space-separated production directories
                         (default: ` + planBuildGateDefaultProdDirs + `)

Exit codes:
  0  allow the write
  2  refuse the write
`

// runPlanBuildGate grades one pre-tool-use event from stdin.
func runPlanBuildGate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	explain := false
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			fmt.Fprint(stdout, planBuildGateUsage)
			return 0
		case "--explain":
			explain = true
		default:
			fmt.Fprintf(stderr, "%s: unexpected argument '%s'\n", planBuildGateName, arg)
			fmt.Fprintf(stderr, "Run 'ai-specs %s --help' for usage.\n", planBuildGateName)
			return 2
		}
	}
	event, err := io.ReadAll(stdin)
	if err != nil {
		return 0
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0
	}
	target, ok := hook.NormalizeEvent(event, cwd)
	if !ok || target.Path == "" || target.Probe == "" {
		return 0
	}
	// The target repository is authoritative: no work tree means nothing to gate.
	if worktreeGateGit(target.Probe, "rev-parse", "--is-inside-work-tree") != "true" {
		return 0
	}
	toplevel := worktreeGateGit(target.Probe, "rev-parse", "--show-toplevel")
	if toplevel == "" {
		return 0
	}
	repoRoot := shared.RealPath(toplevel)
	abs := target.Path
	if !planBuildGateUnder(repoRoot, abs) {
		return 0
	}
	rel := planBuildGateRel(repoRoot, abs)
	// The execution mode is resolved once.
	mode := planbuild.Resolve(repoRoot, planBuildGateCommonDir(target.Probe))
	// Gitignored agent configuration is machine setup, never a production change.
	if planBuildGateAgentConfig(rel) {
		return 0
	}
	// Artifact writes are unconditional at the nearest repository root.
	if planBuildGateUnder(filepath.Join(repoRoot, "openspec", "changes"), abs) {
		return 0
	}
	// The canonical spec tree is the one protected path in either mode: promotion
	// is its only writer, so a hand write there is R1.
	if planBuildGateUnder(planbuild.SpecsBase(repoRoot, mode), abs) {
		tool := target.Tool
		if tool == "" {
			tool = "edit"
		}
		return planBuildGateCanonicalSpec(repoRoot, target.Probe, mode, tool, rel, explain, stderr)
	}
	if mode == planbuild.ModeOdd && planBuildGateUnder(filepath.Join(repoRoot, "odd"), abs) {
		return 0
	}
	change := ""
	if name, ok := planbuild.ChangeFromBranch(planbuild.Branch(target.Probe)); ok {
		change = name
	}
	// S16: a worktree with no plan, no delta and no product-path divergence is not a
	// change, so it owes no ledger row, no tier and no refutation, wherever it writes.
	// The ledger checkpoint is a pending decision, not a tier, so it still comes first.
	tool := target.Tool
	if tool == "" {
		tool = "edit"
	}
	// Work-start fires before the first non-read-only production write, whether
	// or not a change folder exists.
	if planBuildGateProduction(rel) && !planBuildGateGradeLedger(repoRoot, stderr) {
		return 2
	}
	if planBuildGateWorkstream(repoRoot, mode, change) {
		if explain {
			fmt.Fprintf(stderr, "%s: tier %s; refutations %s\n", planBuildGateName,
				planBuildGateInferredTier(repoRoot, mode, change), planBuildGateRules(nil))
		}
		return 0
	}
	// The tier is inferred, never required: no plan, declaration or authorization
	// is a precondition, and only a proven refutation refuses.
	blockers := gates.WriteRefutations(repoRoot, mode, change, rel)
	if explain {
		fmt.Fprintf(stderr, "%s: tier %s; refutations %s\n", planBuildGateName,
			planBuildGateInferredTier(repoRoot, mode, change), planBuildGateRules(blockers))
	}
	for _, blocker := range blockers {
		fmt.Fprintf(stderr, "%s: refusing to %s '%s' — %s\n", planBuildGateName, tool, rel, blocker)
	}
	if len(blockers) > 0 {
		return 2
	}
	return 0
}

// planBuildGateWorkstream reports a worktree that is not a change at all: no plan,
// no delta and no product-path divergence from the integration base (S16). A change
// is conservative — any product-path divergence is enough — and an unresolvable
// base is graded rather than exempted.
func planBuildGateWorkstream(repoRoot, mode, change string) bool {
	if change == "" || planbuild.HasDelta(repoRoot, mode, change) {
		return false
	}
	if info, err := os.Stat(planbuild.TasksPath(repoRoot, mode, change)); err == nil && !info.IsDir() {
		return false
	}
	entries, ok := planbuild.DiffEntries(repoRoot, gates.BaseRef(repoRoot, nil))
	if !ok {
		return false
	}
	for _, entry := range entries {
		if planBuildGateProduction(entry.Path) {
			return false
		}
	}
	return true
}

// planBuildGateInferredTier derives the change tier from the artifacts the
// change carries, through the one derivation both gates share.
func planBuildGateInferredTier(repoRoot, mode, change string) string {
	if change == "" {
		return tier.None
	}
	return tier.Derive(
		planbuild.PlanIsValidForMode(repoRoot, mode, change),
		planbuild.HasDelta(repoRoot, mode, change))
}

// planBuildGateRules names the rules that fired, for --explain.
func planBuildGateRules(blockers []string) string {
	if len(blockers) == 0 {
		return "none"
	}
	rules := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		rule, _, _ := strings.Cut(blocker, ":")
		rules = append(rules, rule)
	}
	return strings.Join(rules, ", ")
}

// planBuildGateCommonDir resolves git's common directory against the probe
// directory, through the one helper the promoter shares.
func planBuildGateCommonDir(probeDir string) string {
	return planbuild.CommonDir(probeDir)
}

// planBuildGateCanonicalSpec grades a write to the canonical spec tree: a hand
// write there is a promotion bypass (R1), so it passes only when the branch
// pairs a change whose delta promotion would promote. A passing canonical write
// still owes the --explain report.
func planBuildGateCanonicalSpec(repoRoot, probeDir, mode, tool, rel string, explain bool, stderr io.Writer) int {
	change := ""
	if name, ok := planbuild.ChangeFromBranch(planbuild.Branch(probeDir)); ok {
		if planbuild.HasDelta(repoRoot, mode, name) {
			// A delta answers R1, never R2: the pending target is still graded.
			blockers := gates.HighRiskRefutation(repoRoot, mode, name, rel)
			if explain {
				fmt.Fprintf(stderr, "%s: tier %s; refutations %s\n", planBuildGateName,
					planBuildGateInferredTier(repoRoot, mode, name), planBuildGateRules(blockers))
			}
			for _, blocker := range blockers {
				fmt.Fprintf(stderr, "%s: refusing to %s '%s' — %s\n", planBuildGateName, tool, rel, blocker)
			}
			if len(blockers) > 0 {
				return 2
			}
			return 0
		}
		change = name
	}
	fmt.Fprintf(stderr, "%s: refusing to %s '%s' — %s\n", planBuildGateName, tool, rel,
		gates.CanonicalSpecRefusal(mode, change, rel))
	return 2
}

// planBuildGateUnder reports whether path is container or below it. Component
// aware, so a lookalike sibling never matches.
func planBuildGateUnder(container, path string) bool {
	if path == container {
		return true
	}
	return strings.HasPrefix(path, container+string(filepath.Separator))
}

// planBuildGateRel is the shell host's `"${abs#"$repo_root"/}"` with its `.`
// fallback for the repository root itself.
func planBuildGateRel(root, abs string) string {
	if abs == root {
		return "."
	}
	return strings.TrimPrefix(abs, root+string(filepath.Separator))
}

// planBuildGateAgentConfig reports the shell host's gitignored-config patterns:
// `<any>/.claude/settings*.json` and `<any>/.claude/hooks/*`.
func planBuildGateAgentConfig(rel string) bool {
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if part != ".claude" || i+1 >= len(parts) {
			continue
		}
		next := parts[i+1]
		if next == "hooks" && i+2 < len(parts) {
			return true
		}
		if i+2 == len(parts) {
			if matched, err := filepath.Match("settings*.json", next); err == nil && matched {
				return true
			}
		}
	}
	return false
}

// planBuildGateProduction reports whether the first segment of rel is one of the
// configured production directories.
func planBuildGateProduction(rel string) bool {
	dirs := os.Getenv(planBuildGateProdDirsEnv)
	// `${prod_dirs// /}`: only spaces make the value blank.
	if strings.ReplaceAll(dirs, " ", "") == "" {
		dirs = planBuildGateDefaultProdDirs
	}
	first, _, _ := strings.Cut(rel, "/")
	for _, dir := range strings.Fields(dirs) {
		if first == dir {
			return true
		}
	}
	return false
}

// --- Ledger work-start checkpoint -------------------------------------------

// planBuildGateGradeLedger grades `work-start` in process and returns whether
// the write may continue. It prints the same lines and takes the same blocking
// decision as the shell host; a missing verdict allows, as the host's empty
// output did. No mode flag is forwarded: Go resolves env, the witness-bound
// recipe config and the warn default.
func planBuildGateGradeLedger(repoRoot string, stderr io.Writer) bool {
	var out bytes.Buffer
	rc := runner.Run(planBuildGateLedgerArgs(repoRoot), strings.NewReader(""), &out, io.Discard, "unknown")
	verdict := out.String()
	if verdict == "" {
		return true
	}
	decision := planBuildGateField(verdict, "decision")
	if decision == "" {
		return true
	}
	reason := planBuildGateField(verdict, "reason")
	active := planBuildGateField(verdict, "active")
	if rc == 2 || decision == "block" {
		fmt.Fprintf(stderr, "%s: blocked at %s — %s\n",
			planBuildGateName, planBuildGateCheckpoint, planBuildGateOr(reason, "missing tracked item"))
		return false
	}
	if decision == "ask" && reason == "identity_unavailable" {
		// No identity means no durable key for a checkpoint-scoped answer: report
		// and proceed without inferring a decision.
		fmt.Fprintf(stderr, "%s: %s cannot be keyed (identity_unavailable); "+
			"reporting and proceeding without recording a decision.\n", planBuildGateName, planBuildGateCheckpoint)
		return true
	}
	if decision == "ask" {
		return planBuildGateAsk(repoRoot, verdict, stderr)
	}
	if active == "1" && decision != "allow" {
		fmt.Fprintf(stderr, "%s: %s at %s — %s\n",
			planBuildGateName, decision, planBuildGateCheckpoint, planBuildGateOr(reason, "no primary item"))
	}
	return true
}

// planBuildGateLedgerArgs is the predicate call the shell host made through the
// installed CLI.
func planBuildGateLedgerArgs(repoRoot string) []string {
	return []string{"--ledger", "--checkpoint", planBuildGateCheckpoint, "--project-root", repoRoot}
}

// planBuildGateAsk offers the human opt-out for an ask verdict. Unlike the
// Tracker gate this host blocks: no terminal, a refused answer or a failed
// record all refuse the write.
func planBuildGateAsk(repoRoot, verdict string, stderr io.Writer) bool {
	for _, side := range []string{"local", "remote", "code", "git"} {
		value := planBuildGateField(verdict, "prompt.evidence."+side)
		fmt.Fprintf(stderr, "  %s: %s\n", side, planBuildGateOr(value, "(unavailable)"))
	}
	fmt.Fprintf(stderr, "  choices: %s\n", planBuildGateField(verdict, "prompt.choices"))
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %s needs a decision but no terminal is available; "+
			"no opt-out was recorded; blocking.\n", planBuildGateName, planBuildGateCheckpoint)
		return false
	}
	defer tty.Close()
	fmt.Fprintf(stderr, "%s: opt out of the %s checkpoint? [y/N] ", planBuildGateName, planBuildGateCheckpoint)
	answer, _ := bufio.NewReader(tty).ReadString('\n')
	switch strings.TrimSpace(answer) {
	case "y", "Y", "yes", "YES", "Yes":
		decide := fmt.Sprintf(`{"checkpoint":%q,"kind":"opt-out","choice":"continue"}`, planBuildGateCheckpoint)
		rc := runner.Run(append(planBuildGateLedgerArgs(repoRoot), "--decide", decide),
			strings.NewReader(""), io.Discard, io.Discard, "unknown")
		if rc == 0 {
			fmt.Fprintf(stderr, "%s: opt-out recorded for %s; proceeding.\n", planBuildGateName, planBuildGateCheckpoint)
			return true
		}
		fmt.Fprintf(stderr, "%s: failed to record the %s opt-out; blocking.\n", planBuildGateName, planBuildGateCheckpoint)
		return false
	}
	fmt.Fprintf(stderr, "%s: no opt-out recorded for %s; blocking.\n", planBuildGateName, planBuildGateCheckpoint)
	return false
}

// planBuildGateField reads one dotted path of a JSON object with the text rules
// of `ai-specs hook field`, which the shell host used. "" on any error.
func planBuildGateField(doc, path string) string {
	decoder := json.NewDecoder(strings.NewReader(doc))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return ""
	}
	for _, key := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return ""
		}
		next, found := object[key]
		if !found {
			return ""
		}
		value = next
	}
	return fieldText(value)
}

// planBuildGateOr returns value, or fallback when it is empty.
func planBuildGateOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
