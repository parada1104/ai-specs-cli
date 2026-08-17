package main

import (
	"encoding/json"
	"os/exec"
	"testing"
)

func TestSplitPOSIXRepresentativePythonParity(t *testing.T) {
	cases := []struct {
		input string
		want  []string
		ok    bool
	}{
		{"echo x", []string{"echo", "x"}, true},
		{"printf '%s' \"hello world\"", []string{"printf", "%s", "hello world"}, true},
		{"echo hello\\ world", []string{"echo", "hello world"}, true},
		{"echo x # comment", []string{"echo", "x", "#", "comment"}, true},
		{"echo \"unterminated", nil, false},
	}
	for _, tc := range cases {
		got, ok := splitPOSIX(tc.input)
		if ok != tc.ok || !equalStrings(got, tc.want) {
			t.Fatalf("input %q: got %#v,%v want %#v,%v", tc.input, got, ok, tc.want, tc.ok)
		}
	}
}

func TestSplitPOSIXPythonOracleAvailable(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	input := "echo 'hello world'"
	cmd := exec.Command("python3", "-c", "import json, shlex, sys; print(json.dumps(shlex.split(sys.argv[1])))", input)
	var out []byte
	var err error
	if out, err = cmd.Output(); err != nil {
		t.Fatal(err)
	}
	var want []string
	if err := json.Unmarshal(out, &want); err != nil {
		t.Fatal(err)
	}
	got, ok := splitPOSIX(input)
	if !ok || !equalStrings(got, want) {
		t.Fatalf("got %#v,%v want %#v", got, ok, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
