package main

import "testing"

func TestSplitPOSIX(t *testing.T) {
	cases := []struct {
		name, input string
		want        []string
		ok          bool
	}{
		{"words", "echo x", []string{"echo", "x"}, true},
		{"single", "echo 'hello world'", []string{"echo", "hello world"}, true},
		{"double", `echo "hello world"`, []string{"echo", "hello world"}, true},
		{"escape", `echo hello\ world`, []string{"echo", "hello world"}, true},
		{"hash is ordinary", "echo x # ignored", []string{"echo", "x", "#", "ignored"}, true},
		{"hash at token start", "# comment", []string{"#", "comment"}, true},
		{"unbalanced", `echo "x`, nil, false},
		{"trailing slash", `echo x\`, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := splitPOSIX(tc.input)
			if ok != tc.ok || !equalStrings(got, tc.want) {
				t.Fatalf("got %#v,%v want %#v,%v", got, ok, tc.want, tc.ok)
			}
		})
	}
}
