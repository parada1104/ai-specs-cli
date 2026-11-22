package doctor

import (
	"path/filepath"
	"testing"

	"ai-specs.dev/ai-specs/internal/lock"
)

func TestClassifyManagedOverride(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "f")
	writeFile(t, dest, "disk\n")
	diskSHA := lock.Sha256Bytes([]byte("disk\n"))

	if got := classifyManagedOverride(filepath.Join(dir, "absent"), nil, nil, false); got != "missing" {
		t.Errorf("absent = %q, want missing", got)
	}
	if got := classifyManagedOverride(dest, nil, nil, false); got != "untracked" {
		t.Errorf("no entry = %q, want untracked", got)
	}
	if got := classifyManagedOverride(dest, map[string]any{"sha256": ""}, nil, false); got != "untracked" {
		t.Errorf("empty sha = %q, want untracked", got)
	}
	if got := classifyManagedOverride(dest, map[string]any{"sha256": lock.Sha256Bytes([]byte("other"))}, nil, false); got != "user_modified" {
		t.Errorf("changed disk = %q, want user_modified", got)
	}
	if got := classifyManagedOverride(dest, map[string]any{"sha256": diskSHA}, nil, false); got != "managed_current" {
		t.Errorf("no candidate = %q, want managed_current", got)
	}
	if got := classifyManagedOverride(dest, map[string]any{"sha256": diskSHA}, []byte("disk\n"), true); got != "managed_current" {
		t.Errorf("matching candidate = %q, want managed_current", got)
	}
	if got := classifyManagedOverride(dest, map[string]any{"sha256": diskSHA}, []byte("elsewhere\n"), true); got != "managed_stale" {
		t.Errorf("stale candidate = %q, want managed_stale", got)
	}
	if got := classifyManagedOverride(dest, map[string]any{"sha256": diskSHA}, []byte{}, true); got != "managed_stale" {
		t.Errorf("empty candidate = %q, want managed_stale", got)
	}
}

func TestPyStrOr(t *testing.T) {
	cases := []struct {
		value any
		want  string
	}{
		{nil, "auto"}, {"go", "go"}, {"", "auto"}, {true, "True"},
		{false, "auto"}, {int64(0), "auto"}, {int64(7), "7"},
	}
	for _, tc := range cases {
		if got := pyStrOr(tc.value, "auto"); got != tc.want {
			t.Errorf("pyStrOr(%v) = %q, want %q", tc.value, got, tc.want)
		}
	}
}
