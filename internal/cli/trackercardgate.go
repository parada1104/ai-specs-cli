package cli

// `ai-specs tracker-card-gate` — the CLI-served Tracker card gate
// (cli-served-recipes T6, S3/F2).
//
// Port of the retired catalog/recipes/trello-mcp-workflow/hooks/
// tracker-card-gate.sh. That shell host was only an acquisition/JSON bridge to
// the Go ledger predicate; this verb makes the same calls in process, so the
// verdicts, the reported lines, the exit codes and the advisory posture are the
// ones the recipe shipped.
//
// Two modes, selected by argv exactly as the script did:
//
//	hook mode  no --root/--checkpoint/--stage. One pre-tool-use event on stdin
//	           maps a production path write to `apply-start` and `gh pr create`
//	           to `pr-review`.
//	host mode  `--root <root> --checkpoint pre-merge|archive-close`, or the
//	           retained `--stage pre-merge|pre-archive` alias, grades one
//	           Tracker lifecycle checkpoint directly.
//
// Exit codes: 0 for every Tracker verdict — the gate is advisory and never
// blocks a source change or a lifecycle boundary — and 2 only for a host-mode
// usage error. Fail-open on missing input, a missing git work tree, or any
// parse/lookup/git error, like the shell host.
//
// The legacy gate_mode hint comes from the per-project sync snapshot
// (projectcache.RuntimeSnapshot.TrackerGateMode), never from ai-specs.toml at
// run time (S2).

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/tracker"
	"ai-specs.dev/worktree-gate/runner"
	shared "ai-specs.dev/worktree-gate/shared"
)

const (
	// trackerGateName is the program label of every message. The verb replaced
	// the script of the same name, so the prefix is unchanged.
	trackerGateName = "tracker-card-gate"
	// trackerGateProdDirsEnv overrides the graded production directories.
	trackerGateProdDirsEnv = "TRACKER_CARD_GATE_PATHS"
	// trackerGateDefaultProdDirs is the host-only default of that env.
	trackerGateDefaultProdDirs = "lib catalog bin src"
	// trackerGateNoTerminal answers an ask checkpoint when /dev/tty is absent.
	trackerGateNoTerminal = "%s: %s needs a decision but no terminal is available; " +
		"no opt-out was recorded — Tracker advisory, continuing without blocking.\n"
)

// trackerCardGateUsage is the verb's help text, on stdout for --help/-h.
const trackerCardGateUsage = `Usage: ai-specs tracker-card-gate
       ai-specs tracker-card-gate --root <project-root>
                                  [--checkpoint pre-merge|archive-close
                                   | --stage pre-merge|pre-archive] [slug]

Grade one Tracker ledger checkpoint. With no arguments it is a pre-tool-use
hook: it reads one hook event on stdin and grades a production path write at
apply-start and ` + "`gh pr create`" + ` at pr-review. Every verdict is advisory —
a block or ask is reported on stderr and the exit code stays 0.

Options:
  --root <path>       Repository to grade. Required for the direct host mode.
  --checkpoint <name> pre-merge or archive-close (default: pre-merge)
  --stage <name>      Retained alias: pre-merge or pre-archive
  --help, -h          Show this help

Arguments:
  [slug]              Change slug the evidence is scoped to (default: the single
                      active change under <root>/openspec/changes)
`

// trackerCardGateHostUsage is the host mode's own usage line (stderr).
const trackerCardGateHostUsage = "usage: ai-specs tracker-card-gate --root <project-root> " +
	"[--checkpoint pre-merge|archive-close | --stage pre-merge|pre-archive] [slug]\n"

// trackerGateVerdict is the part of the ledger verdict JSON the host reads.
type trackerGateVerdict struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Active   bool   `json:"active"`
	Prompt   *struct {
		Evidence struct {
			Local  string `json:"local"`
			Remote string `json:"remote"`
			Code   string `json:"code"`
			Git    string `json:"git"`
		} `json:"evidence"`
		Choices []string `json:"choices"`
	} `json:"prompt"`
}

// runTrackerCardGate dispatches the two argv-selected modes.
func runTrackerCardGate(home string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if trackerGateHostRequest(args) {
		return runTrackerCardGateHost(home, args, stdout, stderr)
	}
	return runTrackerCardGateHook(home, stdin, stderr)
}

// trackerGateHostRequest reports whether argv selects the direct host mode. The
// script keyed on the checkpoint flags alone; --help joins them so the verb can
// document itself instead of silently waiting on stdin.
func trackerGateHostRequest(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--root", "--checkpoint", "--stage", "--help", "-h":
			return true
		}
	}
	return false
}

// --- Direct host mode --------------------------------------------------------

// runTrackerCardGateHost parses the host grammar (verbatim messages and exit 2
// on a usage error) and grades the requested checkpoint.
func runTrackerCardGateHost(home string, args []string, stdout, stderr io.Writer) int {
	usageError := func(message string) int {
		fmt.Fprintf(stderr, "%s: %s\n", trackerGateName, message)
		fmt.Fprint(stderr, trackerCardGateHostUsage)
		return 2
	}
	root, checkpoint, stage, slug := "", "", "", ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		next := func() string {
			if i+1 >= len(args) {
				return ""
			}
			i++
			return args[i]
		}
		switch arg {
		case "--root":
			root = next()
		case "--checkpoint":
			checkpoint = next()
		case "--stage":
			stage = next()
		case "--help", "-h":
			fmt.Fprint(stdout, trackerCardGateUsage)
			return 0
		default:
			switch {
			case strings.HasPrefix(arg, "-"):
				return usageError("unknown option '" + arg + "'")
			case slug == "":
				slug = arg
			default:
				return usageError("unexpected argument '" + arg + "'")
			}
		}
	}
	if root == "" {
		return usageError("--root is required for the direct host mode")
	}
	fromStage := ""
	switch stage {
	case "":
	case "pre-archive":
		fromStage = "archive-close"
	case "pre-merge":
		fromStage = "pre-merge"
	default:
		return usageError("invalid --stage '" + stage + "'")
	}
	switch checkpoint {
	case "", "pre-merge", "archive-close":
	default:
		return usageError("invalid --checkpoint '" + checkpoint + "'")
	}
	if checkpoint != "" && fromStage != "" && checkpoint != fromStage {
		return usageError(fmt.Sprintf("--checkpoint %s conflicts with --stage %s", checkpoint, stage))
	}
	if checkpoint == "" {
		checkpoint = fromStage
		if checkpoint == "" {
			checkpoint = "pre-merge"
		}
	}
	repoRoot, ok := trackerGateResolveRepo("", root)
	if !ok {
		fmt.Fprintf(stderr, "%s: no git work tree at '%s'; failing open\n", trackerGateName, root)
		return 0
	}
	trackerGateGrade(home, checkpoint, repoRoot, slug, stderr)
	return 0
}

// --- Hook mode ---------------------------------------------------------------

// runTrackerCardGateHook grades one pre-tool-use event. Every step fails open:
// an unclassifiable event, a missing work tree, or a non-production target
// simply exits 0 without grading.
func runTrackerCardGateHook(home string, stdin io.Reader, stderr io.Writer) int {
	event, err := io.ReadAll(stdin)
	if err != nil {
		return 0
	}
	parsed, ok := tracker.ClassifyEvent(event)
	if !ok {
		return 0
	}
	lines := strings.Split(strings.TrimSuffix(string(parsed), "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return 0
	}
	kind, cwd := trackerGateKindLine(lines[0])
	switch kind {
	case "path":
		if len(lines) < 2 || lines[1] == "" {
			return 0
		}
		filePath := lines[1]
		if !filepath.IsAbs(filePath) {
			base := cwd
			if base == "" {
				base, _ = os.Getwd()
			}
			filePath = filepath.Join(base, filePath)
		}
		repoRoot, ok := trackerGateResolveRepo(cwd, filePath)
		if !ok {
			return 0
		}
		target := shared.RealPath(filePath)
		rel, err := filepath.Rel(shared.RealPath(repoRoot), target)
		if err != nil || rel == "." {
			return 0
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") {
			return 0
		}
		if trackerGateUngradedPath(rel) {
			return 0
		}
		if !trackerGateProductionPath(rel) {
			return 0
		}
		trackerGateGrade(home, "apply-start", repoRoot, "", stderr)
		return 0
	case "shell":
		repoRoot, ok := trackerGateResolveRepo(cwd, "")
		if !ok {
			return 0
		}
		for _, line := range lines[1:] {
			action, _, _ := strings.Cut(line, "\t")
			if action == "pr_create" {
				trackerGateGrade(home, "pr-review", repoRoot, "", stderr)
			}
		}
		return 0
	}
	return 0
}

// trackerGateKindLine splits the classifier's first line into its kind and cwd
// hint (the tool name in between is not used by the gate).
func trackerGateKindLine(line string) (kind, cwd string) {
	kind, rest, _ := strings.Cut(line, "\t")
	_, cwd, _ = strings.Cut(rest, "\t")
	return kind, cwd
}

// trackerGateUngradedPath reports the targets the shell host never graded: the
// planning tree and the agent's own config.
func trackerGateUngradedPath(rel string) bool {
	if rel == "openspec" || strings.HasPrefix(rel, "openspec/") {
		return true
	}
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

// trackerGateProductionPath reports whether the first segment of rel is one of
// the configured production directories.
func trackerGateProductionPath(rel string) bool {
	dirs := os.Getenv(trackerGateProdDirsEnv)
	if strings.TrimSpace(dirs) == "" {
		dirs = trackerGateDefaultProdDirs
	}
	first, _, _ := strings.Cut(rel, "/")
	for _, dir := range strings.Fields(dirs) {
		if first == dir {
			return true
		}
	}
	return false
}

// trackerGateResolveRepo resolves the owning repository root from a cwd and a
// path hint: the hint's directory (when it names a file), the nearest existing
// ancestor, and then git's work-tree toplevel. ok is false when there is none.
func trackerGateResolveRepo(cwd, hint string) (string, bool) {
	dir := hint
	if dir == "" {
		dir = cwd
	}
	if dir == "" {
		dir, _ = os.Getwd()
	} else if !filepath.IsAbs(dir) {
		base := cwd
		if base == "" {
			base, _ = os.Getwd()
		}
		dir = filepath.Join(base, dir)
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	for dir != "" && dir != "/" && dir != "." {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			break
		}
		dir = filepath.Dir(dir)
	}
	if dir == "" {
		return "", false
	}
	if worktreeGateGit(dir, "rev-parse", "--is-inside-work-tree") != "true" {
		return "", false
	}
	toplevel := worktreeGateGit(dir, "rev-parse", "--show-toplevel")
	if toplevel == "" {
		return "", false
	}
	return shared.RealPath(toplevel), true
}

// --- The ledger bridge -------------------------------------------------------

// trackerGateGrade runs one checkpoint through the Go predicate and reports the
// verdict. It never fails: the Tracker host is advisory, so a block or ask is
// printed and the caller continues.
func trackerGateGrade(home, checkpoint, root, slug string, stderr io.Writer) {
	var extra []string
	evidence, done := trackerGateEvidence(root, slug)
	if evidence != "" {
		extra = append(extra, "--evidence", evidence)
		defer done()
	}
	out, rc := trackerGateLedger(home, checkpoint, root, extra...)
	if out == "" {
		return
	}
	var verdict trackerGateVerdict
	if err := json.Unmarshal([]byte(out), &verdict); err != nil || verdict.Decision == "" {
		return
	}
	if rc == 2 || verdict.Decision == "block" {
		fmt.Fprintf(stderr, "%s: blocked at %s — %s (Tracker advisory: reported, not enforced).\n",
			trackerGateName, checkpoint, trackerGateOr(verdict.Reason, "missing tracked item"))
		return
	}
	if verdict.Decision == "ask" && verdict.Reason == "identity_unavailable" {
		// No identity means no durable key for a checkpoint-scoped answer, so ask
		// cannot collect a recordable decision: report and proceed.
		fmt.Fprintf(stderr, "%s: %s cannot be keyed (identity_unavailable); "+
			"reporting and proceeding without recording a decision.\n", trackerGateName, checkpoint)
		return
	}
	if verdict.Decision == "ask" {
		trackerGateAsk(home, checkpoint, root, verdict, stderr)
		return
	}
	if verdict.Active && verdict.Decision != "allow" {
		fmt.Fprintf(stderr, "%s: %s at %s — %s (Tracker advisory).\n",
			trackerGateName, verdict.Decision, checkpoint, trackerGateOr(verdict.Reason, "no primary item"))
	}
}

// trackerGateLedger runs `worktree-gate --ledger` in process and returns its
// verdict JSON and exit code. The predicate's own stderr is discarded, exactly
// as the shell host discarded it.
func trackerGateLedger(home, checkpoint, root string, extra ...string) (string, int) {
	args := []string{
		"--ledger",
		"--checkpoint", checkpoint,
		"--ledger-gate-mode", trackerGateMode(home, root),
		"--project-root", root,
	}
	args = append(args, extra...)
	var out bytes.Buffer
	rc := runner.Run(args, strings.NewReader(""), &out, io.Discard, "unknown")
	return out.String(), rc
}

// trackerGateMode reads the legacy gate_mode hint from the sync snapshot (S2).
// The graded repository comes first — it owns the ledger being read — then the
// project that installed the hook, so a linked worktree still finds the policy
// sync wrote in its main checkout. No snapshot leaves the hint empty, so the
// resolver falls through to env, the manifest and its warn default.
func trackerGateMode(home, root string) string {
	snapshot, found := worktreeGateSnapshot(home, append([]string{root}, worktreeGateRoots()...))
	if !found {
		return ""
	}
	return snapshot.TrackerGateMode
}

// trackerGateEvidence writes the ledger evidence file for one slug and returns
// its path plus its cleanup. An empty path means no evidence: the slug is
// ambiguous or the change is unreadable, which the predicate handles.
func trackerGateEvidence(root, slug string) (string, func()) {
	if slug == "" {
		slug = tracker.ChangeSlug(root)
	}
	payload, err := tracker.EvidencePayload(root, slug)
	if err != nil {
		return "", func() {}
	}
	file, err := os.CreateTemp("", "tracker-ledger-evidence.*")
	if err != nil {
		return "", func() {}
	}
	name := file.Name()
	if _, err := file.Write(payload.JSON()); err != nil {
		file.Close()
		return "", func() {}
	}
	if err := file.Close(); err != nil {
		return "", func() {}
	}
	return name, func() { _ = os.Remove(name) }
}

// trackerGateAsk offers the human opt-out for an ask verdict: it prints the
// evidence sides and the legal choices, then reads one answer from the
// terminal. Without a terminal it reports and proceeds. Every outcome is
// advisory: the caller always continues.
func trackerGateAsk(home, checkpoint, root string, verdict trackerGateVerdict, stderr io.Writer) {
	if verdict.Prompt != nil {
		evidence := verdict.Prompt.Evidence
		for _, side := range []struct{ name, value string }{
			{"local", evidence.Local},
			{"remote", evidence.Remote},
			{"code", evidence.Code},
			{"git", evidence.Git},
		} {
			fmt.Fprintf(stderr, "  %s: %s\n", side.name, trackerGateOr(side.value, "(unavailable)"))
		}
		fmt.Fprintf(stderr, "  choices: %s\n", strings.Join(verdict.Prompt.Choices, ", "))
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintf(stderr, trackerGateNoTerminal, trackerGateName, checkpoint)
		return
	}
	defer tty.Close()
	fmt.Fprintf(stderr, "%s: opt out of the %s checkpoint? [y/N] ", trackerGateName, checkpoint)
	answer, _ := bufio.NewReader(tty).ReadString('\n')
	switch strings.TrimSpace(answer) {
	case "y", "Y", "yes", "YES", "Yes":
		decide := fmt.Sprintf(`{"checkpoint":%q,"kind":"opt-out","choice":"continue"}`, checkpoint)
		if rc := trackerGateDecide(home, checkpoint, root, decide); rc == 0 {
			fmt.Fprintf(stderr, "%s: opt-out recorded for %s; proceeding.\n", trackerGateName, checkpoint)
			return
		}
		fmt.Fprintf(stderr, "%s: failed to record the %s opt-out — Tracker advisory, continuing without blocking.\n",
			trackerGateName, checkpoint)
		return
	}
	fmt.Fprintf(stderr, "%s: no opt-out recorded for %s — Tracker advisory, continuing without blocking.\n",
		trackerGateName, checkpoint)
}

// trackerGateDecide persists the human answer through the predicate's own
// --decide vehicle, forwarding the raw stamped hint so Go re-resolves the mode
// it graded with.
func trackerGateDecide(home, checkpoint, root, decide string) int {
	return runner.Run([]string{
		"--ledger",
		"--checkpoint", checkpoint,
		"--ledger-gate-mode", trackerGateMode(home, root),
		"--project-root", root,
		"--decide", decide,
	}, strings.NewReader(""), io.Discard, io.Discard, "unknown")
}

// trackerGateOr returns value, or fallback when it is empty.
func trackerGateOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
