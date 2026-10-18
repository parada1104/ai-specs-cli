package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// strPtr is the smallest helper to express a "the caller supplied bytes" vs
// "the caller supplied nothing" (nil) would_write, mirroring the Python
// authority's None sentinel.
func strPtr(value string) *string { return &value }

// TestClassifyManagedOverrideSemantics pins the state order of the Python
// authority util.classify_managed_override. Every branch is exercised directly
// on the pure core, with no filesystem.
func TestClassifyManagedOverrideSemantics(t *testing.T) {
	managed := &classifyManagedEntry{SHA256: sha256Bytes([]byte("catalog"))}
	tests := []struct {
		name       string
		present    bool
		disk       []byte
		managed    *classifyManagedEntry
		wouldWrite *string
		want       string
	}{
		{
			name:    "non-regular destination is missing",
			present: false,
			disk:    nil,
			managed: managed,
			want:    classifyMissing,
		},
		{
			name:    "nil managed entry is untracked",
			present: true,
			disk:    []byte("catalog"),
			managed: nil,
			want:    classifyUntracked,
		},
		{
			name:    "empty managed sha256 is untracked",
			present: true,
			disk:    []byte("catalog"),
			managed: &classifyManagedEntry{SHA256: ""},
			want:    classifyUntracked,
		},
		{
			name:    "disk sha mismatch is user_modified",
			present: true,
			disk:    []byte("user edit"),
			managed: managed,
			want:    classifyUserModified,
		},
		{
			name:       "nil would_write on a tracked match is managed_current",
			present:    true,
			disk:       []byte("catalog"),
			managed:    managed,
			wouldWrite: nil,
			want:       classifyManagedCurrent,
		},
		{
			name:       "matching would_write is managed_current",
			present:    true,
			disk:       []byte("catalog"),
			managed:    managed,
			wouldWrite: strPtr("catalog"),
			want:       classifyManagedCurrent,
		},
		{
			name:       "diverged would_write is managed_stale",
			present:    true,
			disk:       []byte("catalog"),
			managed:    managed,
			wouldWrite: strPtr("evolved"),
			want:       classifyManagedStale,
		},
		{
			name:       "crlf would_write normalizes to the lf disk hash",
			present:    true,
			disk:       []byte("catalog\n"),
			managed:    &classifyManagedEntry{SHA256: sha256Bytes([]byte("catalog\n"))},
			wouldWrite: strPtr("catalog\r\n"),
			want:       classifyManagedCurrent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyManagedOverride(tt.present, tt.disk, tt.managed, tt.wouldWrite)
			if got.State != tt.want {
				t.Fatalf("state = %q, want %q", got.State, tt.want)
			}
		})
	}
}

// TestClassifyManagedOverrideEchoesSHAs pins the hashes the Python bridge reads
// back instead of re-deriving them. A state with no hash for a slot leaves it
// empty.
func TestClassifyManagedOverrideEchoesSHAs(t *testing.T) {
	disk := []byte("catalog")
	diskSHA := sha256Bytes(disk)
	managed := &classifyManagedEntry{SHA256: diskSHA}

	got := classifyManagedOverride(true, disk, managed, strPtr("evolved"))
	if got.DiskSHA256 != diskSHA {
		t.Fatalf("disk_sha256 = %q, want %q", got.DiskSHA256, diskSHA)
	}
	if got.ManagedSHA256 != diskSHA {
		t.Fatalf("managed_sha256 = %q, want %q", got.ManagedSHA256, diskSHA)
	}
	if want := sha256Bytes([]byte("evolved")); got.WouldWriteSHA256 != want {
		t.Fatalf("would_write_sha256 = %q, want %q", got.WouldWriteSHA256, want)
	}

	missing := classifyManagedOverride(false, nil, managed, nil)
	if missing.DiskSHA256 != "" || missing.ManagedSHA256 != "" || missing.WouldWriteSHA256 != "" {
		t.Fatalf("missing result carries hashes: %#v", missing)
	}

	untracked := classifyManagedOverride(true, disk, nil, strPtr("catalog"))
	if untracked.DiskSHA256 != diskSHA || untracked.ManagedSHA256 != "" || untracked.WouldWriteSHA256 != "" {
		t.Fatalf("untracked result = %#v, want disk only", untracked)
	}
}

func classifyCLI(t *testing.T, in classifyInput) (int, string, string) {
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
	diskSHA := sha256Bytes([]byte("catalog"))
	code, stdout, stderr := classifyCLI(t, classifyInput{
		Dest:         dest,
		ManagedEntry: &classifyManagedEntry{SHA256: diskSHA},
		WouldWrite:   strPtr("evolved"),
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
	var got classifyResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("--plan-classify stdout is not JSON: %v (%q)", err, stdout)
	}
	want := classifyResult{
		State:            classifyManagedStale,
		Dest:             dest,
		DiskSHA256:       diskSHA,
		ManagedSHA256:    diskSHA,
		WouldWriteSHA256: sha256Bytes([]byte("evolved")),
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
		code, stdout, stderr := classifyCLI(t, classifyInput{Dest: dest})
		if code != 0 {
			t.Fatalf("dest %q exit = %d, want 0; stderr: %s", dest, code, stderr)
		}
		var got classifyResult
		if err := json.Unmarshal([]byte(stdout), &got); err != nil {
			t.Fatalf("dest %q stdout is not JSON: %v", dest, err)
		}
		if got.State != classifyMissing || got.DiskSHA256 != "" {
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
	code, stdout, stderr := classifyCLI(t, classifyInput{Dest: dest})
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr: %s", code, stderr)
	}
	var got classifyResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if got.State != classifyUntracked || got.DiskSHA256 != sha256Bytes([]byte("custom")) {
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
