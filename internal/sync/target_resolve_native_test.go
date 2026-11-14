package sync

// GO-07 S17: both Go target-resolution call sites run the native port in
// process instead of exec'ing lib/_internal/target-resolve.py.
//
// The discrimination is exact and needs no network: a PATH-stubbed python3
// records every invocation and FAILS, so any call site still delegating the
// resolver plan dies, while a native call site is unaffected. The plan bytes
// the callers receive must stay exactly the bytes internal/target emits.

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/target"
)

// nativePlanFixture builds a project root whose manifest declares two existing
// subrepos, so the resolved plan carries the root row plus both subrepo rows.
func nativePlanFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ai-specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "[project]\nname = \"t\"\nsubrepos = [\"one\", \"two\"]\n"
	if err := os.WriteFile(filepath.Join(root, "ai-specs", "ai-specs.toml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// failingPythonStub puts a python3 that logs its argv and exits 1 first on
// PATH, and returns the log path.
func failingPythonStub(t *testing.T) string {
	t.Helper()
	stubDir := t.TempDir()
	logPath := filepath.Join(stubDir, "invocations.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + logPath + "\nexit 1\n"
	if err := os.WriteFile(filepath.Join(stubDir, "python3"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stubDir+":"+os.Getenv("PATH"))
	return logPath
}

// assertNoPythonInvocation fails when the stub recorded any invocation at all.
func assertNoPythonInvocation(t *testing.T, logPath string) {
	t.Helper()
	logged, err := os.ReadFile(logPath)
	if err == nil {
		t.Fatalf("python3 was invoked: %s", logged)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stub log: %v", err)
	}
}

// TestResolvePlanRunsNatively pins the spine's call site: the plan it returns
// is byte-identical to the native authority's stdout, and python3 is never
// consulted even when it is present on PATH and broken.
func TestResolvePlanRunsNatively(t *testing.T) {
	logPath := failingPythonStub(t)
	root := nativePlanFixture(t)

	var want bytes.Buffer
	if rc := target.Run([]string{root}, &want, io.Discard); rc != 0 {
		t.Fatalf("target.Run rc = %d", rc)
	}

	r := &runner{stderr: io.Discard}
	got, ok := r.resolvePlan(root)
	if !ok {
		t.Fatal("resolvePlan failed; the native resolver must not depend on python3")
	}
	if !bytes.Equal(got, want.Bytes()) {
		t.Errorf("plan bytes differ from the native authority\n got: %s\nwant: %s", got, want.Bytes())
	}
	assertNoPythonInvocation(t, logPath)
}

// TestResolvePlanReportsFailureNatively pins the failure contract: a root that
// cannot be resolved reports failure (the caller prints its own framing) and
// still never reaches python3.
func TestResolvePlanReportsFailureNatively(t *testing.T) {
	logPath := failingPythonStub(t)

	r := &runner{stderr: io.Discard}
	// A root without a manifest is a genuine resolution failure: the native
	// path reports it without python3 and without inventing a plan.
	if _, ok := r.resolvePlan(t.TempDir()); ok {
		t.Error("resolvePlan succeeded for a root with no manifest")
	}
	assertNoPythonInvocation(t, logPath)
}
