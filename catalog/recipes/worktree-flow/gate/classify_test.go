package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-specs.dev/worktree-gate/shared"
)

func classifyCLI(t *testing.T, in shared.ClassifyInput) (int, string, string) {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	return runPlanCLI(t, string(raw), "--plan-classify")
}

// TestPlanClassifyCLIManagedStaleEnvelope pins the whole stdin/stdout contract:
// a regular destination file, a tracking entry, and rendered bytes that differ
// from disk classify as managed_stale and echo every input hash.
func TestPlanClassifyCLIManagedStaleEnvelope(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "card.md")
	if err := os.WriteFile(dest, []byte("catalog"), 0o600); err != nil {
		t.Fatal(err)
	}
	diskSHA := shared.Sha256Bytes([]byte("catalog"))
	wouldWrite := "evolved"
	code, stdout, stderr := classifyCLI(t, shared.ClassifyInput{
		Dest:         dest,
		ManagedEntry: &shared.ClassifyManagedEntry{SHA256: diskSHA},
		WouldWrite:   &wouldWrite,
	})
	if code != 0 {
		t.Fatalf("--plan-classify exit = %d, want 0; stderr: %s", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("--plan-classify stderr = %q, want empty", stderr)
	}
	if strings.Contains(stdout, "null") {
		t.Fatalf("--plan-classify emitted null: %q", stdout)
	}
	var got shared.ClassifyResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("--plan-classify stdout is not JSON: %v (%q)", err, stdout)
	}
	want := shared.ClassifyResult{
		State:            shared.ClassifyManagedStale,
		Dest:             dest,
		DiskSHA256:       diskSHA,
		ManagedSHA256:    diskSHA,
		WouldWriteSHA256: shared.Sha256Bytes([]byte("evolved")),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("--plan-classify = %#v, want %#v", got, want)
	}
}

// TestPlanClassifyCLIMissingAndNonRegular pins the first branch: an absent path
// and a directory are both "missing", with no disk hash and no error.
func TestPlanClassifyCLIMissingAndNonRegular(t *testing.T) {
	dir := t.TempDir()
	for _, dest := range []string{filepath.Join(dir, "absent.md"), dir} {
		code, stdout, stderr := classifyCLI(t, shared.ClassifyInput{Dest: dest})
		if code != 0 {
			t.Fatalf("dest %q exit = %d, want 0; stderr: %s", dest, code, stderr)
		}
		var got shared.ClassifyResult
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("dest %q stdout is not JSON: %v", dest, err)
		}
		if got.State != shared.ClassifyMissing || got.DiskSHA256 != "" {
			t.Fatalf("dest %q = %#v, want missing with no disk hash", dest, got)
		}
	}
}

// TestPlanClassifyCLIUntracked pins a present file with no lock entry: untracked
// echoes the disk hash but no managed hash.
func TestPlanClassifyCLIUntracked(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "card.md")
	if err := os.WriteFile(dest, []byte("custom"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := classifyCLI(t, shared.ClassifyInput{Dest: dest})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	var got shared.ClassifyResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if got.State != shared.ClassifyUntracked || got.DiskSHA256 != shared.Sha256Bytes([]byte("custom")) {
		t.Fatalf("untracked = %#v", got)
	}
}

// TestPlanClassifyCLIRejectsMalformedInput pins the fail-closed exit 2 with no
// stdout for an empty/malformed stdin or a missing destination.
func TestPlanClassifyCLIRejectsMalformedInput(t *testing.T) {
	for _, tt := range []struct {
		name  string
		stdin string
	}{
		{name: "empty stdin", stdin: ""},
		{name: "malformed JSON", stdin: "{not json"},
		{name: "missing dest", stdin: `{"managed_entry":null}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runPlanCLI(t, tt.stdin, "--plan-classify")
			if code != 2 {
				t.Fatalf("exit = %d, want 2; stderr: %s", code, stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, "plan-classify") {
				t.Fatalf("stderr = %q, want a plan-classify diagnostic", stderr)
			}
		})
	}
}
