package main

import "testing"

// TestIsInternalURI pins the twelve-scheme allowlist semantics
// (worktree-gate-legacy.sh:339-352, task 1.12): genuine internal URIs bypass
// classification in PATH mode only; traversal-masked and absolute-path-masked
// variants are filesystem paths and stay gated; SHELL mode never allowlists.
func TestIsInternalURI(t *testing.T) {
	cases := []struct {
		name, candidate, mode string
		want                  bool
	}{
		// All twelve schemes in PATH mode bypass.
		{"xd", "xd://resolve", "path", true},
		{"skill", "skill://testing/init", "path", true},
		{"rule", "rule://worktree-gate", "path", true},
		{"agent", "agent://abc123", "path", true},
		{"history", "history://abc123", "path", true},
		{"artifact", "artifact://abc123", "path", true},
		{"local", "local://plan.md", "path", true},
		{"vault", "vault://hermes-vault/doc.md", "path", true},
		{"mcp", "mcp://trello/get_health", "path", true},
		{"issue", "issue://42", "path", true},
		{"pr", "pr://42", "path", true},
		{"omp", "omp://", "path", true},

		// SHELL mode never allowlists: every candidate is a literal write target.
		{"shell xd literal", "xd://out.txt", "shell", false},
		{"shell xd absolute", "xd:///abs/path", "shell", false},

		// Unknown schemes are gated normally.
		{"https", "https://example.com/src.py", "path", false},
		{"file", "file:///etc/hosts", "path", false},
		{"custom", "custom://thing", "path", false},

		// Traversal-masked paths resolve into the repository: classify.
		{"traversal", "xd://repo/../repo/src/app.py", "path", false},
		{"traversal suffix", "xd://repo/..", "path", false},

		// Absolute-path-masked: a filesystem path wearing a URI prefix.
		{"absolute masked", "xd:///abs/repo/src.py", "path", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsInternalURI(tc.candidate, tc.mode); got != tc.want {
				t.Fatalf("IsInternalURI(%q, %q) = %v, want %v",
					tc.candidate, tc.mode, got, tc.want)
			}
		})
	}
}
