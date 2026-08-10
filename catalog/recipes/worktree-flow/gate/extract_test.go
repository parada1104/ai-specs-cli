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
