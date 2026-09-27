package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// runShim execs the legacy implementation for an unported verb — the
// strangler mechanism. It mirrors bin/ai-specs:
//
//	bash "$LIB_DIR/<script>" "$@"   # args after the verb shift
//
// with AI_SPECS_HOME exported, AI_SPECS_INVOKED_AS set for add-dep, and the
// child's exit code propagated verbatim (128+signal when killed by a signal).
// When the provided writers are *os.File, os/exec passes the real fds so TTY
// detection inside the legacy scripts keeps working; non-file writers (tests)
// get pipes/buffers.
func runShim(r route, args []string, home string, stdin io.Reader, stdout, stderr io.Writer) int {
	// args[0] is the verb; the shim receives the post-verb shift.
	argv := append([]string{filepath.Join(home, "lib", r.script)}, args[1:]...)
	cmd := exec.Command("bash", argv...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	cmd.Env = childEnv(home, r.invokedAs)

	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			if code := ee.ExitCode(); code >= 0 {
				return code
			}
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				return 128 + int(ws.Signal())
			}
		}
		fmt.Fprintf(stderr, "ai-specs: %v\n", err)
		return 1
	}
	return 0
}

// childEnv builds the child environment: the parent environment with
// AI_SPECS_HOME pinned to the resolved home (the legacy launcher exports it
// after its own resolution) and, for add-dep, AI_SPECS_INVOKED_AS naming the
// alias the user typed — the legacy error hints depend on it.
func childEnv(home, invokedAs string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		if len(kv) >= len("AI_SPECS_HOME=") && kv[:len("AI_SPECS_HOME=")] == "AI_SPECS_HOME=" {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "AI_SPECS_HOME="+home)
	if invokedAs != "" {
		env = append(env, "AI_SPECS_INVOKED_AS="+invokedAs)
	}
	return env
}
