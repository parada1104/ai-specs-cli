package sync

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// stepFunc is one pipeline step. runStep hands it the writers that stand in
// for the step's captured stdout and stderr and returns its exit status. The
// indirection exists so the AGENTS.md step (which composes two children) is
// expressible as a step.
type stepFunc func(out, errW io.Writer) int

// runner carries the process-level context every child needs: the pinned
// environment, the stdin/stdout/stderr of the sync invocation, and the
// resolved verbose mode.
type runner struct {
	home    string
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	verbose bool
	env     []string
}

// runStep mirrors run_step from lib/sync.sh: announce the step, capture its
// stdout and stderr into two temp files, then replay each through the verbosity
// filter on its original stream. On failure the FULL unfiltered output is
// replayed before the wrapped status is returned.
//
// Cleanup (removing both temp files) happens before the status is returned and,
// because Go cleanup is a statement rather than a command, it cannot replace
// the wrapped status — the property shell `set +e` around cleanup protects.
func (r *runner) runStep(label string, step stepFunc) int {
	fmt.Fprintf(r.stdout, "  syncing %s\n", label)

	// Shell `if ! A || ! B`: when the first mktemp fails the second is never
	// attempted; when the first succeeds and the second fails the first is
	// removed. A temp created before the failure stays stranded, exactly like
	// the shell (reproduce, do not improve).
	outPath, ok := r.mktemp()
	if ok {
		errPath, ok2 := r.mktemp()
		if ok2 {
			return r.runStepCaptured(step, outPath, errPath)
		}
		os.Remove(outPath)
	}
	fmt.Fprintln(r.stderr, "  ! cannot create temporary files (check TMPDIR); running this step with unfiltered output")
	return step(r.stdout, r.stderr)
}

// runStepCaptured is the normal run_step path: capture both streams, then
// replay them raw on failure or filtered on success, and remove both temps.
func (r *runner) runStepCaptured(step stepFunc, outPath, errPath string) int {
	outF, err := os.Create(outPath)
	if err != nil {
		os.Remove(outPath)
		os.Remove(errPath)
		return 1
	}
	errF, err := os.Create(errPath)
	if err != nil {
		outF.Close()
		os.Remove(outPath)
		os.Remove(errPath)
		return 1
	}

	rc := step(outF, errF)
	outF.Close()
	errF.Close()

	if rc != 0 {
		replayRaw(r.stdout, outPath)
		replayRaw(r.stderr, errPath)
		os.Remove(outPath)
		os.Remove(errPath)
		return rc
	}
	printStepOutput(r.stdout, outPath, r.verbose)
	printStepOutput(r.stderr, errPath, r.verbose)
	os.Remove(outPath)
	os.Remove(errPath)
	return 0
}

// replayRaw writes a capture file's bytes verbatim when it is non-empty —
// the `[[ -s f ]] && cat f` failure path.
func replayRaw(w io.Writer, path string) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return
	}
	w.Write(data)
}

// printStepOutput mirrors print_step_output from lib/sync.sh. A missing or
// empty capture prints nothing. Verbose mode replays the bytes verbatim (cat
// semantics, trailing blank lines preserved). Compact mode drops blank lines
// and every line whose first non-whitespace character is one of the
// success/detail markers `✓ · ⇢ ▸`, keeping every other line (including its
// leading whitespace) intact. The leading-whitespace strip is ASCII-only,
// matching the C-locale `[:space:]` class.
func printStepOutput(w io.Writer, path string, verbose bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return
	}
	if verbose {
		w.Write(data)
		return
	}
	rest := string(data)
	for {
		var line string
		idx := strings.IndexByte(rest, '\n')
		if idx < 0 {
			// Final line without a newline: bash `read` returns non-zero but
			// `|| [[ -n "$line" ]]` keeps it, and printf appends the newline.
			line = rest
			rest = ""
		} else {
			line = rest[:idx]
			rest = rest[idx+1:]
		}
		stripped := strings.TrimLeft(line, " \t\n\v\f\r")
		if stripped != "" && !hasNoisyPrefix(stripped) {
			io.WriteString(w, line)
			io.WriteString(w, "\n")
		}
		if idx < 0 {
			break
		}
	}
}

// hasNoisyPrefix reports whether the line's first character is one of the four
// compact-mode markers.
func hasNoisyPrefix(s string) bool {
	for _, p := range [...]string{"✓", "·", "⇢", "▸"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// mktemp runs the real mktemp binary and returns its stdout with trailing
// newlines stripped (command-substitution semantics), false on any failure.
//
// It deliberately execs mktemp instead of using os.CreateTemp: on macOS mktemp
// ignores TMPDIR and `mktemp -t tmpl-XXXXXX.json` appends a random suffix
// rather than substituting XXXXXX, so a stdlib reimplementation would diverge
// from the frozen behavior this port must preserve.
func (r *runner) mktemp(args ...string) (string, bool) {
	cmd := exec.Command("mktemp", args...)
	cmd.Env = r.env
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", false
	}
	return strings.TrimRight(buf.String(), "\n"), true
}

// exec runs a child argv with the pinned environment and maps its exit status
// the way runShim does: the exact exit code, 128+signal on signal death, and
// 127 with a `command not found` line on a start failure.
func (r *runner) exec(argv []string, out, errW io.Writer) int {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = r.stdin
	cmd.Stdout = out
	cmd.Stderr = errW
	cmd.Env = r.env
	return runExitCode(cmd, errW, argv[0])
}

// execNested is exec with AI_SPECS_SYNC_NESTED=1 added to the child
// environment (the fan-out export in lib/sync.sh). Any inherited value is
// replaced so exactly one is present.
func (r *runner) execNested(argv []string, out, errW io.Writer) int {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = r.stdin
	cmd.Stdout = out
	cmd.Stderr = errW
	cmd.Env = append(dropEnv(r.env, "AI_SPECS_SYNC_NESTED"), "AI_SPECS_SYNC_NESTED=1")
	return runExitCode(cmd, errW, argv[0])
}

// runExitCode runs cmd and translates its result into a process exit code.
func runExitCode(cmd *exec.Cmd, errW io.Writer, name string) int {
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code
		}
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
	}
	if isNotFound(err) {
		fmt.Fprintf(errW, "%s: command not found\n", name)
		return 127
	}
	fmt.Fprintf(errW, "%s: %s\n", name, strerrorText(err))
	return 126
}

// isNotFound reports whether an exec start failure means "no such command":
// exec.LookPath's ErrNotFound (nothing on PATH) or ENOENT from an explicit
// path. Anything else (EACCES, ENOTDIR, EISDIR, ENOEXEC, ...) is a
// found-but-refused command, which the shell reports with status 126.
func isNotFound(err error) bool {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == syscall.ENOENT
}

// childEnv mirrors internal/cli/shim.go's childEnv: the parent environment
// with any inherited AI_SPECS_HOME dropped and AI_SPECS_HOME pinned to home.
// AI_SPECS_INVOKED_AS is intentionally not set (it is an add-dep concern).
func childEnv(home string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "AI_SPECS_HOME=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "AI_SPECS_HOME="+home)
}

// dropEnv returns env without any entry for key.
func dropEnv(env []string, key string) []string {
	prefix := key + "="
	out := env[:0:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			continue
		}
		out = append(out, kv)
	}
	return out
}

// removeIfSet removes path when non-empty; the empty case corresponds to a
// mktemp that never produced a name (the shell's `${VAR:-}` expansion).
func removeIfSet(path string) {
	if path != "" {
		os.Remove(path)
	}
}
