package cli

// `ai-specs plan-build-gate` — the CLI-served planning gate (cli-served-recipes
// T7, S3/F3).
//
// Port of the retired catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh.
// That shell host was an acquisition/JSON bridge to the Go ledger predicate and
// to the Go-owned submodule topology proof; this verb makes the same calls in
// process, so the refusals, the exit codes and the fail-open posture are the
// ones the recipe shipped.
//
// One mode: a pre-tool-use event on stdin. A production path write (the
// PLAN_BUILD_GATE_PATHS scope, default src/lib/catalog) is graded at the
// `work-start` ledger checkpoint first, then refused until the plan of the
// repository's execution mode exists. In `openspec` mode that is the tasks.md
// of the change the branch pairs (openspec/changes/<change>/tasks.md) under the
// nearest repository root, or the only active change when the branch pairs
// none; ambiguity refuses instead of accepting any tasks.md. For a proven
// submodule topology the central planning root keeps its own plan scan.
// In `odd` mode it is the paired plan odd/tasks/<change>.md at the repository
// root, and the planning tree is odd/ instead of openspec/changes/. The mode
// selects the plan tree and never waives the plan requirement (S10).
//
// Exit codes: 0 allows the write, 2 refuses it. Malformed, unrelated or
// unprovable input fails open, except the topology proof, which fails closed
// onto the nearest-root gate exactly as the shell host did.
//
// No run-time configuration comes from the sync snapshot: the shell host
// stamped nothing on this hook, and the ledger mode is resolved by the
// predicate from env, the witness-bound recipe config and its warn default (S2).

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/hook"
	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/worktree-gate/runner"
	shared "ai-specs.dev/worktree-gate/shared"
)

const (
	// planBuildGateName is the program label of every message. The verb replaced
	// the script of the same name, so the prefix is unchanged.
	planBuildGateName = "plan-build-gate"
	// planBuildGateProdDirsEnv overrides the graded production directories.
	planBuildGateProdDirsEnv = "PLAN_BUILD_GATE_PATHS"
	// planBuildGateDefaultProdDirs is the default of that env.
	planBuildGateDefaultProdDirs = "src lib catalog"
	// planBuildGateCheckpoint is the checkpoint this host grades.
	planBuildGateCheckpoint = "work-start"
)

// planBuildGateUsage is the verb's help text, on stdout for --help/-h.
const planBuildGateUsage = `Usage: ai-specs plan-build-gate

Pre-tool-use hook: read one hook event (JSON) on stdin and refuse a production
path write until the plan of the repository's execution mode exists. In
"openspec" mode the plan is the paired change's
openspec/changes/<change>/tasks.md: the branch's last segment pairs a change
folder, or the only active change is used. In "odd" mode the plan is the paired file
odd/tasks/<change>.md, where <change> is the last segment of the current branch
name (feat/thing and fix/thing both pair with odd/tasks/thing.md). Planning
artifacts under openspec/changes/ and, in "odd" mode, under odd/ are always
writable, and the agent's own .claude config is never graded.

The mode is read from [recipes.plan-build-flow.config] mode in
ai-specs/ai-specs.toml: "auto" (default), "odd" or "openspec". "auto" resolves
"odd" when the Gentle AI state directory <git-common-dir>/gentle-ai/ exists, and
"openspec" otherwise. An explicit mode wins over the inference.

Options:
  --help, -h          Show this help

Environment:
  PLAN_BUILD_GATE_PATHS  Space-separated production directories
                         (default: ` + planBuildGateDefaultProdDirs + `)

Exit codes:
  0  allow the write
  2  refuse the write
`

// planBuildGateNearestRefusal is the shell host's nearest-root refusal.
const planBuildGateNearestRefusal = "%s: refusing to %s '%s' — no active change folder " +
	"(openspec/changes/<slug>/tasks.md) found. Classify the change and write the plan " +
	"first, then implement. Writing planning artifacts under openspec/changes/ is never " +
	"blocked.\n"

// planBuildGateOddPlanRefusal is the odd-mode refusal for a missing or
// non-regular paired plan.
const planBuildGateOddPlanRefusal = "%s: refusing to %s '%s' — ODD mode is active but the ODD plan " +
	"'odd/tasks/%s.md' is missing, is not a regular file, or resolves outside the repository " +
	"for branch '%s'. Write the plan first, or declare mode = \"openspec\" in " +
	"ai-specs/ai-specs.toml to use the OpenSpec flow. Writing planning artifacts under odd/ is " +
	"never blocked.\n"

// planBuildGateOddBranchRefusal is the odd-mode refusal for an unresolvable branch.
const planBuildGateOddBranchRefusal = "%s: refusing to %s '%s' — ODD mode is active but the current " +
	"branch cannot be resolved (detached HEAD), so no ODD plan can be paired. Write the plan at " +
	"'odd/tasks/<change>.md' on the change branch, or declare mode = \"openspec\" in " +
	"ai-specs/ai-specs.toml to use the OpenSpec flow.\n"

// planBuildGateCentralRefusal is the shell host's central-root refusal.
const planBuildGateCentralRefusal = "%s: refusing to %s '%s' — no active change folder " +
	"found under central planning root '%s/openspec/changes/' for submodule '%s'. " +
	"Classify the change and write the central plan first; writes under that planning " +
	"tree are allowed.\n"

// planBuildGateAmbiguousRefusal refuses when the change tree holds two or more
// active changes and the branch pairs none; the resolver error names every
// candidate and the --change remedy.
const planBuildGateAmbiguousRefusal = "%s: refusing to %s '%s' — %s\n"

// planBuildGateNoChangeRefusal refuses when no active change exists and the
// branch names none; the resolver error names the plan path to write.
const planBuildGateNoChangeRefusal = "%s: refusing to %s '%s' — %s\n"

// runPlanBuildGate grades one pre-tool-use event from stdin.
func runPlanBuildGate(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	for _, arg := range args {
		switch arg {
		case "--help", "-h":
			fmt.Fprint(stdout, planBuildGateUsage)
			return 0
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
	// The execution mode is resolved once and selects the plan tree; it never
	// waives the plan requirement.
	mode := planbuild.Resolve(repoRoot, planBuildGateCommonDir(target.Probe))
	// Gitignored agent configuration is machine setup, never a production change.
	if planBuildGateAgentConfig(rel) {
		return 0
	}
	// Artifact writes are unconditional at the nearest repository root.
	if planBuildGateUnder(filepath.Join(repoRoot, "openspec", "changes"), abs) {
		return 0
	}
	if mode == planbuild.ModeOdd && planBuildGateUnder(filepath.Join(repoRoot, "odd"), abs) {
		return 0
	}
	if !planBuildGateProduction(rel) {
		return 0
	}
	tool := target.Tool
	if tool == "" {
		tool = "edit"
	}
	// Work-start fires before the first non-read-only production write, whether
	// or not a change folder exists. The artifact rule below is separate.
	if !planBuildGateGradeLedger(repoRoot, stderr) {
		return 2
	}
	if mode == planbuild.ModeOdd {
		// The plan is per repository root in odd mode, so the submodule topology
		// proof does not apply.
		return planBuildGateOddPlan(repoRoot, target.Probe, tool, rel, stderr)
	}
	change, err := planbuild.ResolveChange(repoRoot, planbuild.ModeOpenspec, planbuild.Branch(target.Probe), "")
	if err == nil && planbuild.PlanIsValidForMode(repoRoot, planbuild.ModeOpenspec, change) {
		return 0
	}
	if central, sub, proven := planBuildGateCentralProof(repoRoot, target.Probe); proven {
		// The strict resolver applies to the nearest root on purpose: the central
		// root's branch is not the submodule's, so only the proven topology proof
		// selects the central plan tree.
		if planBuildGateHasPlan(central) {
			return 0
		}
		fmt.Fprintf(stderr, planBuildGateCentralRefusal, planBuildGateName, tool, rel, central, sub)
		return 2
	}
	switch {
	case errors.Is(err, planbuild.ErrAmbiguousChange):
		fmt.Fprintf(stderr, planBuildGateAmbiguousRefusal, planBuildGateName, tool, rel, err)
	case errors.Is(err, planbuild.ErrNoChange):
		fmt.Fprintf(stderr, planBuildGateNoChangeRefusal, planBuildGateName, tool, rel, err)
	default:
		fmt.Fprintf(stderr, planBuildGateNearestRefusal, planBuildGateName, tool, rel)
	}
	return 2
}

// planBuildGateCommonDir resolves git's common directory against the probe
// directory, through the one helper the promoter shares.
func planBuildGateCommonDir(probeDir string) string {
	return planbuild.CommonDir(probeDir)
}

// planBuildGateOddPlan grades the odd-mode plan: the paired plan of the current
// branch, a regular file at odd/tasks/<change>.md.
func planBuildGateOddPlan(repoRoot, probeDir, tool, rel string, stderr io.Writer) int {
	branch := planbuild.Branch(probeDir)
	change, ok := planbuild.ChangeFromBranch(branch)
	if !ok {
		fmt.Fprintf(stderr, planBuildGateOddBranchRefusal, planBuildGateName, tool, rel)
		return 2
	}
	if planbuild.PlanIsValid(repoRoot, change) {
		return 0
	}
	fmt.Fprintf(stderr, planBuildGateOddPlanRefusal, planBuildGateName, tool, rel, change, branch)
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

// planBuildGateHasPlan reports whether root holds an active change folder: a
// change directory whose tasks.md resolves to a regular file. It serves the
// central planning root only: the nearest root resolves its change through the
// branch ladder. A match alone is not enough — a directory or a dangling
// symlink named tasks.md would let a production write through with no readable
// plan (R3-001); os.Stat follows symlinks. The changes directory is read
// literally, never as a glob pattern, so a repository path containing '[', '*' or
// '?' cannot wedge the gate.
func planBuildGateHasPlan(root string) bool {
	changes, err := os.ReadDir(filepath.Join(root, "openspec", "changes"))
	if err != nil {
		return false
	}
	for _, change := range changes {
		plan := filepath.Join(root, "openspec", "changes", change.Name(), "tasks.md")
		if info, err := os.Stat(plan); err == nil && info.Mode().IsRegular() {
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

// --- Central planning root (verified topology proof) ------------------------

// planBuildGateCentralRootQuerier runs the gate module's read-only topology
// proof from the probe directory, the way the retired host asked the installed
// CLI: `cd <probe> && ai-specs worktree-gate --resolve-central-root`. The proof
// resolves upward from the process directory and lives unexported in the
// imported gate module, so it needs the probe directory as its working
// directory; a child process has one of its own, while os.Chdir would move every
// concurrent caller in this process and race their relative paths (R3-002).
// Tests substitute it.
var planBuildGateCentralRootQuerier = func(probeDir string) (string, bool) {
	exe, err := planBuildGateTopologyExecutable()
	if err != nil {
		return "", false
	}
	cmd := exec.Command(exe, "worktree-gate", "--resolve-central-root")
	cmd.Dir = probeDir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return out.String(), true
}

// planBuildGateTopologyExecutable is the CLI the child process runs: this one.
// Tests substitute it.
var planBuildGateTopologyExecutable = os.Executable

// planBuildGateCentralProof validates the proof against this repository. A
// missing binary, a nonzero exit, a malformed or empty payload, or a central
// root that does not contain the nearest repository root leaves the topology
// unproven, so the caller fails closed onto the nearest-root gate.
func planBuildGateCentralProof(repoRoot, probeDir string) (string, string, bool) {
	out, ok := planBuildGateCentralRootQuerier(probeDir)
	if !ok || strings.TrimSpace(out) == "" {
		return "", "", false
	}
	cand := planBuildGateField(out, "central_root")
	sub := planBuildGateField(out, "submodule")
	if !filepath.IsAbs(cand) {
		return "", "", false
	}
	if strings.ContainsAny(cand+sub, "\t\r\n") {
		return "", "", false
	}
	resolved, err := filepath.EvalSymlinks(cand)
	if err != nil {
		return "", "", false
	}
	if resolved == repoRoot || !planBuildGateUnder(resolved, repoRoot) {
		return "", "", false
	}
	return resolved, sub, true
}
