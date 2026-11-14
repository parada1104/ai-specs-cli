package shared

import (
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
	managed := &ClassifyManagedEntry{SHA256: Sha256Bytes([]byte("catalog"))}
	tests := []struct {
		name       string
		present    bool
		disk       []byte
		managed    *ClassifyManagedEntry
		wouldWrite *string
		want       string
	}{
		{
			name:    "non-regular destination is missing",
			present: false,
			disk:    nil,
			managed: managed,
			want:    ClassifyMissing,
		},
		{
			name:    "nil managed entry is untracked",
			present: true,
			disk:    []byte("catalog"),
			managed: nil,
			want:    ClassifyUntracked,
		},
		{
			name:    "empty managed sha256 is untracked",
			present: true,
			disk:    []byte("catalog"),
			managed: &ClassifyManagedEntry{SHA256: ""},
			want:    ClassifyUntracked,
		},
		{
			name:    "disk sha mismatch is user_modified",
			present: true,
			disk:    []byte("user edit"),
			managed: managed,
			want:    ClassifyUserModified,
		},
		{
			name:       "nil would_write on a tracked match is managed_current",
			present:    true,
			disk:       []byte("catalog"),
			managed:    managed,
			wouldWrite: nil,
			want:       ClassifyManagedCurrent,
		},
		{
			name:       "matching would_write is managed_current",
			present:    true,
			disk:       []byte("catalog"),
			managed:    managed,
			wouldWrite: strPtr("catalog"),
			want:       ClassifyManagedCurrent,
		},
		{
			name:       "diverged would_write is managed_stale",
			present:    true,
			disk:       []byte("catalog"),
			managed:    managed,
			wouldWrite: strPtr("evolved"),
			want:       ClassifyManagedStale,
		},
		{
			name:       "crlf would_write normalizes to the lf disk hash",
			present:    true,
			disk:       []byte("catalog\n"),
			managed:    &ClassifyManagedEntry{SHA256: Sha256Bytes([]byte("catalog\n"))},
			wouldWrite: strPtr("catalog\r\n"),
			want:       ClassifyManagedCurrent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyManagedOverride(tt.present, tt.disk, tt.managed, tt.wouldWrite)
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
	diskSHA := Sha256Bytes(disk)
	managed := &ClassifyManagedEntry{SHA256: diskSHA}

	got := ClassifyManagedOverride(true, disk, managed, strPtr("evolved"))
	if got.DiskSHA256 != diskSHA {
		t.Fatalf("disk_sha256 = %q, want %q", got.DiskSHA256, diskSHA)
	}
	if got.ManagedSHA256 != diskSHA {
		t.Fatalf("managed_sha256 = %q, want %q", got.ManagedSHA256, diskSHA)
	}
	if want := Sha256Bytes([]byte("evolved")); got.WouldWriteSHA256 != want {
		t.Fatalf("would_write_sha256 = %q, want %q", got.WouldWriteSHA256, want)
	}

	missing := ClassifyManagedOverride(false, nil, managed, nil)
	if missing.DiskSHA256 != "" || missing.ManagedSHA256 != "" || missing.WouldWriteSHA256 != "" {
		t.Fatalf("missing result carries hashes: %#v", missing)
	}

	untracked := ClassifyManagedOverride(true, disk, nil, strPtr("catalog"))
	if untracked.DiskSHA256 != diskSHA || untracked.ManagedSHA256 != "" || untracked.WouldWriteSHA256 != "" {
		t.Fatalf("untracked result = %#v, want disk only", untracked)
	}
}
