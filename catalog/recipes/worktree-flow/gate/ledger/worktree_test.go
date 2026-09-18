package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestEvaluateWorktreeOutcomes is the Worktree port acceptance table. Every row
// is a normalized observation, so this test needs no repository and no
// subprocess: the evaluator decides from evidence alone.
//
// Order is part of the contract. Detached outranks dirty, dirty outranks merge
// evidence, and a merge needs either a local proof or a provider merge commit
// that acquisition proved reachable from a base candidate.
func TestEvaluateWorktreeOutcomes(t *testing.T) {
	cases := []struct {
		name string
		obs  WorktreeObservation
		want WorktreeOutcome
	}{
		{
			name: "detached outranks every other fact",
			obs: WorktreeObservation{
				Detached: true, Dirty: true, LocalMerged: true,
				PRMergeCommit: "cafe", MergeCommitInBase: true,
			},
			want: WorktreeOutcome{Reason: WorktreeReasonDetached},
		},
		{
			name: "dirty outranks merge evidence",
			obs: WorktreeObservation{
				Dirty: true, LocalMerged: true,
				PRMergeCommit: "cafe", MergeCommitInBase: true,
			},
			want: WorktreeOutcome{Reason: WorktreeReasonDirty},
		},
		{
			name: "a local proof merges",
			obs:  WorktreeObservation{LocalMerged: true},
			want: WorktreeOutcome{Merged: true, Reason: WorktreeReasonMerged},
		},
		{
			name: "a provider merge commit proven in base merges",
			obs:  WorktreeObservation{PRMergeCommit: "cafe", MergeCommitInBase: true},
			want: WorktreeOutcome{Merged: true, Reason: WorktreeReasonMerged},
		},
		{
			name: "a provider merge commit outside the base preserves",
			obs:  WorktreeObservation{PRMergeCommit: "cafe"},
			want: WorktreeOutcome{Reason: WorktreeReasonUnmerged},
		},
		{
			name: "no merge evidence preserves",
			obs:  WorktreeObservation{},
			want: WorktreeOutcome{Reason: WorktreeReasonUnmerged},
		},
		{
			name: "an in-base flag without a recorded commit is not proof",
			obs:  WorktreeObservation{MergeCommitInBase: true},
			want: WorktreeOutcome{Reason: WorktreeReasonUnmerged},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvaluateWorktree(tc.obs); got != tc.want {
				t.Fatalf("EvaluateWorktree(%+v) = %+v, want %+v", tc.obs, got, tc.want)
			}
		})
	}
}

// worktreeCorpusCase is one JSON golden fixture: a normalized observation and
// the outcome the port must return for it. The JSON keys match the Go field
// names case-insensitively, so the corpus pins the same shape the port reads in
// production.
type worktreeCorpusCase struct {
	Observation WorktreeObservation `json:"observation"`
	Want        WorktreeOutcome     `json:"want"`
}

// TestEvaluateWorktreeGoldenCorpus drives the port from the on-disk golden
// corpus under testdata. It keeps the fixtures out of Go source so the accepted
// observations read like data, while the table test above stays the fast
// in-process check.
func TestEvaluateWorktreeGoldenCorpus(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "worktree-ledger-corpus", "*.json"))
	if err != nil {
		t.Fatalf("glob worktree corpus: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("worktree corpus is empty: no golden fixtures were found")
	}
	cases := make([]worktreeCorpusCase, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read corpus fixture %s: %v", path, err)
		}
		var tc worktreeCorpusCase
		if err := json.Unmarshal(raw, &tc); err != nil {
			t.Fatalf("parse corpus fixture %s: %v", path, err)
		}
		cases = append(cases, tc)
	}
	assertWorktreeCorpusNonVacuous(t, cases)

	for i, tc := range cases {
		name := filepath.Base(paths[i])
		t.Run(name, func(t *testing.T) {
			if got := EvaluateWorktree(tc.Observation); got != tc.Want {
				t.Fatalf("EvaluateWorktree(%+v) = %+v, want %+v", tc.Observation, got, tc.Want)
			}
		})
	}
}

// assertWorktreeCorpusNonVacuous is the mutation guard. A corpus that pins only
// preserves would pass for an evaluator that always preserves, and one that
// pins only merges would pass for an evaluator that always merges: both
// polarities plus every reason in the vocabulary must be present, or the corpus
// proves nothing.
func assertWorktreeCorpusNonVacuous(t *testing.T, cases []worktreeCorpusCase) {
	t.Helper()
	var merged, preserved int
	reasons := map[string]bool{}
	for _, tc := range cases {
		if tc.Want.Merged {
			merged++
		} else {
			preserved++
		}
		reasons[tc.Want.Reason] = true
	}
	if merged == 0 {
		t.Fatal("corpus mutation guard: no expected-merged fixture, so an always-preserve evaluator would pass")
	}
	if preserved == 0 {
		t.Fatal("corpus mutation guard: no expected-preserve fixture, so an always-merged evaluator would pass")
	}
	for _, reason := range []string{
		WorktreeReasonDetached,
		WorktreeReasonDirty,
		WorktreeReasonMerged,
		WorktreeReasonUnmerged,
	} {
		if !reasons[reason] {
			t.Fatalf("corpus mutation guard: no fixture pins reason %q", reason)
		}
	}
}
