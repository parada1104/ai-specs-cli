package cli

// The native `init` verb: the classic, non-interactive bootstrap of
// odd/tasks/go-20-u4-init.md (slice T4a). It is a port of lib/init.sh's classic
// path — argument contract, banner, directories, template manifest, baseline
// AGENTS.md, both .gitignore files, the bundled-assets seam and the next-steps
// text — and it delegates the WHOLE verb to the legacy script when the
// interactive wizard is due, so the wizard keeps its behaviour until the native
// one lands.
//
// The messages, their order and the exit codes are the legacy script's
// contract (S3): 0 success, 1 unusable target or a failed step, 2 usage error.

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"ai-specs.dev/ai-specs"
	"ai-specs.dev/ai-specs/internal/sync"
)

// initUsage is lib/init.sh's usage() heredoc, verbatim.
const initUsage = `Usage: ai-specs init [path] [--name <project-name>] [--force] [--adopt-brief] [--tui|--no-tui]

Bootstrap the ai-specs standard in a project (idempotent by default).

Arguments:
  path              Project root (default: current directory)

Flags:
  --name <name>     Project name in ai-specs.toml (default: basename of path)
  --force           Refresh the managed agent-block in the root .gitignore
                    (AGENTS.md handling is identical with and without it)
  --adopt-brief     Explicitly adopt the current AGENTS.md as the managed
                    runtime-brief baseline
  --tui             Force interactive onboarding (Rich prompts)
  --no-tui          Skip interactive onboarding (scriptable / CI)
  -h, --help        Show this help

Interactive onboarding (TTY):
  When stdin/stdout are a TTY, no --name/--force/--no-tui is passed, and
  ai-specs.toml does not yet exist, init launches a short wizard to choose
  project name, agents, and recipes. Use --no-tui to keep the classic path.

Examples:
  ai-specs init                        # TTY → wizard; else classic template
  ai-specs init --no-tui               # classic non-interactive bootstrap
  ai-specs init --tui --name my-app    # wizard with name prefilled
  ai-specs init ~/code/my-app          # initialize specific path
  ai-specs init --force                # refresh the root .gitignore agent-block
`

// initOptions is the legacy argument contract.
type initOptions struct {
	target       string
	name         string
	nameExplicit bool
	force        bool
	adoptBrief   bool
	tuiMode      string // auto | on | off
}

const errInitUnknownFlag = "ERROR: unknown flag: %s\nRun 'ai-specs init --help' for usage.\n"

// errInitMissingNameValue is the port's message for a trailing `--name`: the
// shell aborts with its own `shift: 2: shift count out of range` at exit 1, and
// that text is an accident of bash, not a designed message (S12).
const errInitMissingNameValue = "ERROR: --name requires a value\nRun 'ai-specs init --help' for usage.\n"

// runInit is the native entry. args[0] is the verb.
func runInit(args []string, home string, stdin io.Reader, stdout, stderr io.Writer, selfPath string) int {
	opts, rc, done := parseInitArgs(args, stdout, stderr)
	if done {
		return rc
	}

	target, rc := resolveInitTarget(opts.target, stderr)
	if rc != 0 {
		return rc
	}
	if opts.name == "" {
		opts.name = filepath.Base(target)
	}

	aiSpecsDir := filepath.Join(target, "ai-specs")
	tomlPath := filepath.Join(aiSpecsDir, "ai-specs.toml")
	agentsPath := filepath.Join(target, "AGENTS.md")
	rootGitignore := filepath.Join(target, ".gitignore")
	aiGitignore := filepath.Join(aiSpecsDir, ".gitignore")

	// S6: the wizard is still the legacy script's, so the whole verb goes there.
	// The predicate is init.sh's: --tui, or auto with both streams on a TTY, no
	// explicit --name, no --force and no manifest yet.
	if shouldInitTUI(opts, isTTY(stdin, stdout), tomlPath) {
		return runShim(route{kind: routeShim, script: "init.sh"}, args, home, stdin, stdout, stderr, selfPath)
	}

	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "ai-specs init")
	fmt.Fprintf(stdout, "  target:  %s\n", target)
	fmt.Fprintf(stdout, "  name:    %s\n", opts.name)
	fmt.Fprintf(stdout, "  force:   %s\n", yesNo(opts.force))
	fmt.Fprintln(stdout, "  tui:     no")
	fmt.Fprintln(stdout)

	if rc := ensureInitDirs(aiSpecsDir, stdout, stderr); rc != 0 {
		return rc
	}
	if rc := writeInitManifest(opts.name, tomlPath, stdout, stderr); rc != 0 {
		return rc
	}
	if rc := renderInitBrief(home, target, tomlPath, agentsPath, opts.adoptBrief, stdout, stderr); rc != 0 {
		return rc
	}
	if rc := applyInitGitignores(tomlPath, rootGitignore, aiGitignore, opts.force, stdout, stderr); rc != 0 {
		return rc
	}
	if rc := refreshBundledAtInit(home, target, stdout, stderr); rc != 0 {
		return rc
	}
	writeInitNextSteps(target, aiSpecsDir, stdout)
	return 0
}

// parseInitArgs mirrors the shell's while/case loop. done reports that the
// caller must return rc without running the verb (help, or a usage error).
func parseInitArgs(args []string, stdout, stderr io.Writer) (initOptions, int, bool) {
	opts := initOptions{tuiMode: "auto"}
	rest := args
	if len(rest) > 0 {
		rest = rest[1:]
	}
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch {
		case arg == "--name":
			// A trailing `--name` with no value is NOT an empty name: the shell
			// runs `shift 2` with one argument left, which fails under
			// `set -euo pipefail` and aborts the whole verb with exit 1 before
			// anything is written (JD-B-001). The port keeps the exit code and
			// replaces the shell's own "shift count out of range" text with a
			// message that names the flag.
			if i+1 >= len(rest) {
				fmt.Fprint(stderr, errInitMissingNameValue)
				return opts, 1, true
			}
			opts.name = rest[i+1]
			opts.nameExplicit = true
			i++
		case strings.HasPrefix(arg, "--name="):
			opts.name = strings.TrimPrefix(arg, "--name=")
			opts.nameExplicit = true
		case arg == "--force":
			opts.force = true
		case arg == "--adopt-brief":
			opts.adoptBrief = true
		case arg == "--tui":
			opts.tuiMode = "on"
		case arg == "--no-tui":
			opts.tuiMode = "off"
		case arg == "-h" || arg == "--help":
			fmt.Fprint(stdout, initUsage)
			return opts, 0, true
		case arg == "--":
			// The shell `break`s here: whatever follows -- is ignored.
			return opts, 0, false
		case strings.HasPrefix(arg, "-"):
			// `-` alone is an unknown flag on the shell route too: `-*` matches
			// it before the positional branch (JD-A-002).
			fmt.Fprintf(stderr, errInitUnknownFlag, arg)
			return opts, 2, true
		default:
			if opts.target != "" {
				fmt.Fprintf(stderr, "ERROR: unexpected positional argument: %s\n", arg)
				return opts, 2, true
			}
			opts.target = arg
		}
	}
	return opts, 0, false
}

// resolveInitTarget applies init.sh's target rules: cwd default, an existing
// directory, then the absolute path the shell gets from `cd <path> && pwd`.
func resolveInitTarget(raw string, stderr io.Writer) (string, int) {
	if raw == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: target path does not exist or is not a directory: %s\n", raw)
			return "", 1
		}
		raw = cwd
	}
	if fi, err := os.Stat(raw); err != nil || !fi.IsDir() {
		fmt.Fprintf(stderr, "ERROR: target path does not exist or is not a directory: %s\n", raw)
		return "", 1
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: target path does not exist or is not a directory: %s\n", raw)
		return "", 1
	}
	return abs, 0
}

// shouldInitTUI is init.sh's SHOULD_TUI decision (S6).
func shouldInitTUI(opts initOptions, terminal bool, tomlPath string) bool {
	switch opts.tuiMode {
	case "on":
		return true
	case "off":
		return false
	}
	if !terminal {
		return false
	}
	if opts.nameExplicit || opts.force {
		return false
	}
	_, err := os.Stat(tomlPath)
	return os.IsNotExist(err)
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// ensureInitDirs is `mkdir -p skills/ commands/` and its two status lines. The
// shell creates BOTH directories in one command and prints no status line when
// either fails, and mkdir's own diagnostic goes to stderr (JD-B-005).
func ensureInitDirs(aiSpecsDir string, stdout, stderr io.Writer) int {
	for _, dir := range []string{"skills", "commands"} {
		if err := os.MkdirAll(filepath.Join(aiSpecsDir, dir), 0o755); err != nil {
			fmt.Fprintf(stderr, "ERROR: cannot create %s: %v\n", filepath.Join(aiSpecsDir, dir), err)
			return 1
		}
	}
	for _, dir := range []string{"skills", "commands"} {
		fmt.Fprintf(stdout, "  ✓ ensure %s/%s/\n", aiSpecsDir, dir)
	}
	return 0
}

// tomlBasicStringBody escapes value for the body of a TOML basic string
// (`"..."`): backslash, quote and the control characters TOML gives a short
// escape to, and \uXXXX for the rest.
func tomlBasicStringBody(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u%04X`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// writeInitManifest renders the embedded template when the manifest is missing
// and NEVER overwrites an existing one: the manifest is user-owned (S4).
func writeInitManifest(name, tomlPath string, stdout, stderr io.Writer) int {
	if fi, err := os.Stat(tomlPath); err == nil && fi.Mode().IsRegular() {
		fmt.Fprintln(stdout, "  ✓ keep   ai-specs/ai-specs.toml")
		return 0
	}
	afs, err := assets.FS("templates")
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: cannot read the bundled manifest template: %v\n", err)
		return 1
	}
	tmpl, err := fs.ReadFile(afs, "ai-specs.toml.tmpl")
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: cannot read the bundled manifest template: %v\n", err)
		return 1
	}
	// S8: literal substitution, ESCAPED for the TOML basic string the template
	// puts it in (`name = "{{PROJECT_NAME}}"`). The shell's
	// `sed s/{{PROJECT_NAME}}/<name>/g` mangles & and /, and a name carrying a
	// backslash, a quote or a control character would produce a manifest that
	// does not parse — which the never-overwrite rule would then make permanent
	// (JD-A-003).
	body := strings.ReplaceAll(string(tmpl), "{{PROJECT_NAME}}", tomlBasicStringBody(name))
	if err := os.WriteFile(tomlPath, []byte(body), 0o644); err != nil {
		fmt.Fprintf(stderr, "ERROR: cannot write %s: %v\n", tomlPath, err)
		return 1
	}
	fmt.Fprintln(stdout, "  ✓ wrote  ai-specs/ai-specs.toml")
	return 0
}

// renderInitBrief is init.sh's 3b block: the policy gate, the baseline render
// chain in process, and the placeholder fallback — same messages, same exit 0
// in every branch.
func renderInitBrief(home, target, tomlPath, agentsPath string, adoptBrief bool, stdout, stderr io.Writer) int {
	enabled, err := sync.EvaluateBriefRender(tomlPath)
	if err != nil {
		// The seam prints `error: <…>` and answers with empty stdout, which the
		// shell's [[ ]] comparison reads as "not true": the disabled branch.
		fmt.Fprintf(stderr, "error: %v\n", err)
		enabled = false
	}
	if enabled {
		tmp, tmpErr := os.CreateTemp("", "ai-specs-resolved-config-*.json")
		if tmpErr != nil {
			return briefFallback(agentsPath, stderr)
		}
		resolvedOut := tmp.Name()
		tmp.Close()
		defer os.Remove(resolvedOut)

		rc := sync.RenderBaselineBrief(sync.BaselineBriefRequest{
			ProjectRoot:       target,
			AISpecsHome:       home,
			ResolvedConfigOut: resolvedOut,
			AdoptBrief:        adoptBrief,
			Stdout:            stdout,
			Stderr:            stderr,
		})
		if rc != 0 {
			return briefFallback(agentsPath, stderr)
		}
		fmt.Fprintln(stdout, "  ✓ render AGENTS.md (baseline brief)")
		return 0
	}
	if _, err := os.Stat(agentsPath); err == nil {
		fmt.Fprintln(stdout, "  · skipped AGENTS.md (brief.render = false)")
		return 0
	}
	if err := os.WriteFile(agentsPath, []byte("# AGENTS.md - Runtime context\n"), 0o644); err != nil {
		return 1
	}
	fmt.Fprintln(stderr, "  · skipped AGENTS.md render (brief.render = false)")
	fmt.Fprintln(stderr, "  ! created placeholder — replace with your manual brief")
	return 0
}

// briefFallback is the render-failure branch: a one-line placeholder when
// nothing was written, the same warning either way, and exit 0.
func briefFallback(agentsPath string, stderr io.Writer) int {
	if _, err := os.Stat(agentsPath); err != nil {
		if err := os.WriteFile(agentsPath, []byte("# AGENTS.md - Runtime context\n"), 0o644); err != nil {
			return 1
		}
	}
	fmt.Fprintln(stderr, "  ! render skipped — fallback placeholder written")
	return 0
}

// applyInitGitignores is init.sh's steps 5 and 6.
func applyInitGitignores(tomlPath, rootGitignore, aiGitignore string, force bool, stdout, stderr io.Writer) int {
	afs, err := assets.FS("templates")
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: cannot read the bundled templates: %v\n", err)
		return 1
	}
	template, err := fs.ReadFile(afs, "gitignore-root.tmpl")
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: cannot read the bundled templates: %v\n", err)
		return 1
	}
	action, err := applyRootGitignoreBlock(rootGitignore, template, force)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	switch action {
	case gitignoreRefreshed:
		fmt.Fprintln(stdout, "  ✓ refresh .gitignore (agent block)")
	case gitignoreKept:
		fmt.Fprintln(stdout, "  ✓ keep   .gitignore (agent block present)")
	default:
		fmt.Fprintln(stdout, "  ✓ append .gitignore (agent block)")
	}
	// Always derived from the manifest. The legacy step runs gitignore-render.py
	// directly (not through the seam), so its status line IS part of init's
	// stdout: the printing wrapper is the faithful call.
	if rc := sync.RenderAiSpecsGitignore(tomlPath, aiGitignore, stdout); rc != 0 {
		// The shell routes this step through gitignore-render.py, whose traceback
		// names the manifest; the port prints one line with the same information
		// (S12). Without it the user sees only the brief-policy error and a bare
		// exit 1 (JD-A-004 / JD-B-004).
		fmt.Fprintf(stderr, "ERROR: cannot render %s from %s: the manifest could not be read or parsed\n", aiGitignore, tomlPath)
		return rc
	}
	return 0
}

type gitignoreAction int

const (
	gitignoreAppended gitignoreAction = iota
	gitignoreRefreshed
	gitignoreKept
)

const (
	initGitignoreMarkerBegin = "# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---"
	initGitignoreMarkerEnd   = "# --- end ai-specs ---"
)

// applyRootGitignoreBlock is init.sh's append_block/strip_block pair, which is
// NOT the same algorithm as gitignore-root-refresh.py: it appends with at most
// one separating newline and, on --force, strips the marked region and appends
// again.
func applyRootGitignoreBlock(path string, template []byte, force bool) (gitignoreAction, error) {
	raw, readErr := os.ReadFile(path)
	exists := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return gitignoreAppended, readErr
	}
	text := string(raw)

	if exists && hasExactLine(text, initGitignoreMarkerBegin) {
		if !force {
			return gitignoreKept, nil
		}
		text = stripMarkedBlock(text)
		if err := appendGitignoreBlock(path, text, template); err != nil {
			return gitignoreRefreshed, err
		}
		return gitignoreRefreshed, nil
	}
	if err := appendGitignoreBlock(path, text, template); err != nil {
		return gitignoreAppended, err
	}
	return gitignoreAppended, nil
}

// appendGitignoreBlock is append_block: ensure the file ends with a newline
// when it has content, then append the template bytes.
//
// The bytes are the shell's, the WRITE is not: the shell appends with `>>` and
// its strip path goes through a temp file and `mv`, so a failure partway never
// truncates content outside the managed block. os.WriteFile truncates first, so
// an ENOSPC or a kill mid-write would lose the user's own lines (JD-B-003). The
// port keeps the shell's bytes and adopts the shell's other tactic: write the
// whole result beside the target and rename it into place.
func appendGitignoreBlock(path, text string, template []byte) error {
	out := text
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if !strings.HasSuffix(string(template), "\n") {
		template = append(template, '\n')
	}
	return replaceFileAtomically(path, append([]byte(out), template...))
}

// replaceFileAtomically writes body to a sibling temp file and renames it over
// path, so a failed or interrupted write leaves the previous bytes intact. The
// temp file inherits the target's mode when it exists (0o644 otherwise).
func replaceFileAtomically(path string, body []byte) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".ai-specs-gitignore-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// stripMarkedBlock is strip_block's awk program: drop every line from the begin
// marker through the end marker, inclusive, keeping the rest as it was. awk
// re-emits each surviving record with a newline, so the result always ends with
// one.
func stripMarkedBlock(text string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if strings.HasSuffix(text, "\n") {
		lines = lines[:len(lines)-1]
	}
	kept := make([]string, 0, len(lines))
	inBlock := false
	for _, line := range lines {
		if !inBlock && line == initGitignoreMarkerBegin {
			inBlock = true
			continue
		}
		if inBlock {
			if line == initGitignoreMarkerEnd {
				inBlock = false
			}
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, "\n") + "\n"
}

// hasExactLine is `grep -qxF <marker> <file>`.
func hasExactLine(text, marker string) bool {
	if text == "" {
		return false
	}
	for _, line := range strings.Split(text, "\n") {
		if line == marker {
			return true
		}
	}
	return false
}

// refreshBundledAtInit is init.sh's step 7. Under the single binary the bundled
// assets are embedded and the flattened cache is a compatibility surface, but
// the same script also stamps the lock's [meta] provenance, which the rest of
// the system reads; until the native stamp lands (T4c) the step keeps the
// legacy entry, exactly once (S9).
func refreshBundledAtInit(home, target string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stdout, "▸ refresh-bundled --init")
	cmd := exec.Command("python3", filepath.Join(home, "lib", "_internal", "refresh-bundled.py"), target, home, "--init")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			// The shell's `set -e` reports 128+signal for a killed child, and so
			// does runShim; ExitCode() alone would answer -1 and os.Exit would
			// turn a SIGTERM into 255 (JD-B-006).
			if code := ee.ExitCode(); code >= 0 {
				return code
			}
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				return 128 + int(ws.Signal())
			}
		}
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	return 0
}

// writeInitNextSteps is init.sh's closing heredoc, byte for byte.
func writeInitNextSteps(target, aiSpecsDir string, stdout io.Writer) {
	fmt.Fprintf(stdout, "\n✓ ai-specs initialized at %s\n", target)
	fmt.Fprint(stdout, `
Next steps:
  1. Edit  `+aiSpecsDir+`/ai-specs.toml
       - set [agents].enabled       (claude, cursor, opencode, codex, copilot, gemini)
       - add [[deps]]               (vendored skills from git)
       - add [mcp.*] sections       (MCP servers)
  2. Optional: add recommended policy skills via [[deps]] (see catalog/README.md in the ai-specs-cli repo)
  3. Run   ai-specs sync             (vendor deps + regenerate AGENTS.md + fan-out per agent)
  4. Commit:
       - ai-specs/ai-specs.toml
       - ai-specs/skills/ (your LOCAL skills only — CLI-bundled skills resolve from the cache)
       - ai-specs/recipes/*/overrides/ (declared recipe overrides only)
       - ai-specs/commands/<your-local-commands>.md
       - AGENTS.md   .gitignore   ai-specs/.ai-specs.lock
     (ai-specs/.deps/ and the rest of ai-specs/recipes/ are gitignored.)

Generated agent files (.claude/, .cursor/, opencode.json, .mcp.json, CLAUDE.md, ...) are
gitignored — they are regenerated by 'ai-specs sync-agent' on every clone.
`)
}
