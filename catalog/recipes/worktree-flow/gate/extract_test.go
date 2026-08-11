package main

import "testing"

func TestExtractPass1(t *testing.T) {
	cases := []struct {
		name         string
		tokens, want []string
	}{
		{"redirect", []string{"echo", "x", ">", "out"}, []string{"out"}},
		{"tee", []string{"echo", "x", "|", "tee", "-a", "out.log"}, []string{"out.log"}},
		{"sed", []string{"sed", "-i", "s/a/b/", "cfg"}, []string{"cfg"}},
		{"cp", []string{"cp", "src", "dest"}, []string{"dest"}},
		{"fd dup", []string{"echo", "x", ">&2"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPass1(tc.tokens)
			if !equalStrings(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}

// TestScrub pins the reference dedupe() scrub step (worktree-gate-legacy.sh:90-100):
// ".", "-", "&"-prefixed tokens and the /dev/null|/dev/stdout|/dev/stderr|/dev/fd/*
// special files are never write candidates, so the gate must not block on them.
func TestScrub(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"out.txt", "out.txt"},
		{"  out.txt  ", "out.txt"},
		{"", ""},
		{"   ", ""},
		{".", ""},
		{"-", ""},
		{"&1", ""},
		{"&*", ""},
		{"&2", ""},
		{"/dev/null", ""},
		{"/dev/stdout", ""},
		{"/dev/stderr", ""},
		{"/dev/fd/1", ""},
		{"/dev/fd/3", ""},
		{"/dev/fd/anything", ""},
		{"/dev/nullx", "/dev/nullx"}, // prefix only, not a real special file
		{"sub/file.txt", "sub/file.txt"},
		{"&file", ""}, // any &-prefixed token is fd duplication, not a path
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := scrub(tc.in); got != tc.want {
				t.Fatalf("scrub(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestExtractPass1Scrub pins scrub at the pass1 level: redirect and tee
// targets that are not real write destinations never yield a candidate.
func TestExtractPass1Scrub(t *testing.T) {
	cases := []struct {
		name         string
		tokens, want []string
	}{
		{"redirect dot", []string{"echo", "x", ">", "."}, nil},
		{"redirect dash", []string{"echo", "x", ">", "-"}, nil},
		{"redirect amp-star", []string{"echo", "x", ">", "&*"}, nil},
		{"redirect amp-1", []string{"echo", "x", ">", "&1"}, nil},
		{"redirect dev-null", []string{"echo", "x", ">", "/dev/null"}, nil},
		{"redirect dev-stdout", []string{"echo", "x", ">", "/dev/stdout"}, nil},
		{"redirect dev-stderr", []string{"echo", "x", ">", "/dev/stderr"}, nil},
		{"redirect dev-fd", []string{"echo", "x", ">", "/dev/fd/1"}, nil},
		{"tee dev-null", []string{"echo", "x", "|", "tee", "/dev/null"}, nil},
		{"tee amp-star", []string{"echo", "x", "|", "tee", "&*"}, nil},
		{"tee amp-1", []string{"echo", "x", "|", "tee", "&1"}, nil},
		{"real redirect kept", []string{"echo", "x", ">", "out"}, []string{"out"}},
		{"real tee kept", []string{"echo", "x", "|", "tee", "-a", "out.log"}, []string{"out.log"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPass1(tc.tokens)
			if !equalStrings(got, tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}
