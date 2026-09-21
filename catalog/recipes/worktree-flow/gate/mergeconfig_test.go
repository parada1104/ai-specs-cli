package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// runMergeConfigCLI feeds one stdin envelope through the full CLI surface and
// returns the exit code, the parsed stdout envelope and the raw stdout text.
func runMergeConfigCLI(t *testing.T, input string) (int, mergeConfigResult, string) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := run([]string{"--plan-merge-config"}, strings.NewReader(input), &stdout, &stderr)
	var got mergeConfigResult
	if err := json.Unmarshal([]byte(stdout.String()), &got); err != nil && code == 0 {
		t.Fatalf("stdout is not a valid result envelope: %v\nstdout: %q stderr: %q", err, stdout.String(), stderr.String())
	}
	return code, got, stdout.String()
}

// assertConfigEqual compares the emitted config object against want
// value-by-value (ordering has its own explicit test below).
func assertConfigEqual(t *testing.T, rawOut, want string) {
	t.Helper()
	var envelope struct {
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal([]byte(rawOut), &envelope); err != nil {
		t.Fatalf("stdout has no config object: %v\nstdout: %q", err, rawOut)
	}
	var gotAny, wantAny any
	if err := json.Unmarshal(envelope.Config, &gotAny); err != nil {
		t.Fatalf("config is not valid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &wantAny); err != nil {
		t.Fatalf("test want config is not valid JSON: %v", err)
	}
	if !reflect.DeepEqual(gotAny, wantAny) {
		t.Fatalf("config mismatch:\n got: %s\nwant: %s", envelope.Config, want)
	}
}

// assertKeyOrder asserts that the raw config JSON lists the given keys in the
// given order (insertion order parity with the Python dict).
func assertKeyOrder(t *testing.T, rawOut string, wantKeys []string) {
	t.Helper()
	var envelope struct {
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal([]byte(rawOut), &envelope); err != nil {
		t.Fatalf("stdout has no config object: %v", err)
	}
	prev := -1
	for _, key := range wantKeys {
		token := fmt.Sprintf("%q:", key)
		idx := strings.Index(string(envelope.Config), token)
		if idx < 0 {
			t.Fatalf("config output missing key %q: %s", key, envelope.Config)
		}
		if idx < prev {
			t.Fatalf("config key %q out of order:\n%s", key, envelope.Config)
		}
		prev = idx
	}
}

func fieldJSON(key string, required bool, hasDefault bool, def string, enum []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `{"key":%q,"required":%t,"has_default":%t`, key, required, hasDefault)
	if hasDefault {
		fmt.Fprintf(&b, `,"default":%s`, def)
	}
	if enum != nil {
		fmt.Fprintf(&b, `,"enum":[`)
		for i, e := range enum {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "%q", e)
		}
		b.WriteString(`]`)
	}
	b.WriteString("}")
	return b.String()
}

func manifestJSON(key, value string) string {
	return fmt.Sprintf(`{"key":%q,"value":%s}`, key, value)
}

const reconcileShape = `{"scope_field":"string","max_age_seconds":"integer","expectations":[{"event":"string","property":"string","config_field":"string","config_field_when_set":"string"}]}`

func mergeInput(recipeName string, fields []string, tables []string, manifest []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `{"recipe_name":%q`, recipeName)
	b.WriteString(`,"fields":[`)
	for i, f := range fields {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(f)
	}
	b.WriteString(`],"tables":[`)
	for i, tb := range tables {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(tb)
	}
	b.WriteString(`],"manifest":[`)
	for i, m := range manifest {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(m)
	}
	b.WriteString(`]}`)
	return b.String()
}

func TestMergeConfigDefaultsAndOverrides(t *testing.T) {
	input := mergeInput("worktree-flow",
		[]string{
			fieldJSON("gate_scope", false, true, `"auto"`, []string{"auto", "ask", "off"}),
			fieldJSON("board_id", true, false, "", nil),
			fieldJSON("flag_false", false, true, `false`, nil),
			fieldJSON("count_zero", false, true, `0`, nil),
			fieldJSON("label_empty", false, true, `""`, nil),
		},
		[]string{},
		[]string{
			manifestJSON("gate_scope", `"ask"`),
			manifestJSON("board_id", `"69ec0a2099ea20956e371d62"`),
		},
	)
	code, got, raw := runMergeConfigCLI(t, input)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr diagnostic is allowed but exit stays 0)", code)
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want empty", got.Error)
	}
	assertConfigEqual(t, raw, `{
		"gate_scope": "ask",
		"flag_false": false,
		"count_zero": 0,
		"label_empty": "",
		"board_id": "69ec0a2099ea20956e371d62"
	}`)
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", got.Warnings)
	}
	// Insertion-order parity: defaults in field order, then manifest-new keys.
	assertKeyOrder(t, raw, []string{"gate_scope", "flag_false", "count_zero", "label_empty", "board_id"})
}

func TestMergeConfigDefaultExcludedOnlyWhenHasDefaultFalse(t *testing.T) {
	input := mergeInput("worktree-flow",
		[]string{fieldJSON("board_id", true, false, "", nil)},
		[]string{},
		[]string{},
	)
	code, got, _ := runMergeConfigCLI(t, input)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	wantErr := "recipe 'worktree-flow': missing required config field 'board_id'"
	if got.Error != wantErr {
		t.Fatalf("error = %q, want %q", got.Error, wantErr)
	}
}

func TestMergeConfigRequiredPrecedesEnum(t *testing.T) {
	input := mergeInput("worktree-flow",
		[]string{
			fieldJSON("board_id", true, false, "", nil),
			fieldJSON("gate_scope", false, false, "", []string{"auto", "ask", "off"}),
		},
		[]string{},
		[]string{manifestJSON("gate_scope", `"bogus"`)},
	)
	_, got, _ := runMergeConfigCLI(t, input)
	wantErr := "recipe 'worktree-flow': missing required config field 'board_id'"
	if got.Error != wantErr {
		t.Fatalf("error = %q, want required error %q (required loop runs before enum loop)", got.Error, wantErr)
	}
}

func TestMergeConfigEnumInvalidMessage(t *testing.T) {
	input := mergeInput("worktree-flow",
		[]string{fieldJSON("gate_scope", false, false, "", []string{"auto", "ask", "off"})},
		[]string{},
		[]string{manifestJSON("gate_scope", `"bogus"`)},
	)
	code, got, _ := runMergeConfigCLI(t, input)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 for semantic error", code)
	}
	wantErr := "recipe 'worktree-flow': config field 'gate_scope' value 'bogus' is invalid; allowed: auto | ask | off"
	if got.Error != wantErr {
		t.Fatalf("error = %q, want %q", got.Error, wantErr)
	}
}

func TestMergeConfigGateImplSpecialMessage(t *testing.T) {
	input := mergeInput("worktree-flow",
		[]string{fieldJSON("gate_impl", false, false, "", []string{"auto", "go"})},
		[]string{},
		[]string{manifestJSON("gate_impl", `"bash"`)},
	)
	_, got, _ := runMergeConfigCLI(t, input)
	wantErr := "invalid gate_impl 'bash'; bash has been removed; allowed: auto | go"
	if got.Error != wantErr {
		t.Fatalf("error = %q, want %q", got.Error, wantErr)
	}
}

func TestMergeConfigGateScopeFallback(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		want     string
	}{
		{"absent stays auto", "", "auto"},
		{"empty string becomes auto", `""`, "auto"},
		{"whitespace becomes auto", `"   "`, "auto"},
		{"false becomes auto", `false`, "auto"},
		{"zero becomes auto", `0`, "auto"},
		{"non-blank override kept", `"ask"`, "ask"},
	}
	gateScopeField := fieldJSON("gate_scope", false, false, "", []string{"auto", "ask", "off"})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var manifest []string
			if tc.manifest != "" {
				manifest = []string{manifestJSON("gate_scope", tc.manifest)}
			}
			input := mergeInput("worktree-flow", []string{gateScopeField}, []string{}, manifest)
			code, got, raw := runMergeConfigCLI(t, input)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			if got.Error != "" {
				t.Fatalf("error = %q, want empty", got.Error)
			}
			assertConfigEqual(t, raw, `{"gate_scope": "`+tc.want+`"}`)
		})
	}
}

func TestMergeConfigGateScopeFallbackOnlyWhenDeclared(t *testing.T) {
	input := mergeInput("worktree-flow",
		[]string{},
		[]string{},
		[]string{manifestJSON("gate_scope", `"   "`)},
	)
	_, got, raw := runMergeConfigCLI(t, input)
	if got.Error != "" {
		t.Fatalf("error = %q, want empty", got.Error)
	}
	assertConfigEqual(t, raw, `{}`)
	wantWarn := "recipe 'worktree-flow': unknown config key 'gate_scope' in manifest (ignored)"
	if len(got.Warnings) != 1 || got.Warnings[0] != wantWarn {
		t.Fatalf("warnings = %v, want [%q]", got.Warnings, wantWarn)
	}
}

func TestMergeConfigUnknownKeyWarningsInManifestOrder(t *testing.T) {
	input := mergeInput("worktree-flow",
		[]string{fieldJSON("board_id", false, false, "", nil)},
		[]string{},
		[]string{
			manifestJSON("zzz_bogus", `1`),
			manifestJSON("board_id", `"x"`),
			manifestJSON("aaa_bogus", `2`),
		},
	)
	code, got, raw := runMergeConfigCLI(t, input)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want empty", got.Error)
	}
	wantWarnings := []string{
		"recipe 'worktree-flow': unknown config key 'zzz_bogus' in manifest (ignored)",
		"recipe 'worktree-flow': unknown config key 'aaa_bogus' in manifest (ignored)",
	}
	if !reflect.DeepEqual(got.Warnings, wantWarnings) {
		t.Fatalf("warnings = %v, want %v", got.Warnings, wantWarnings)
	}
	assertConfigEqual(t, raw, `{"board_id": "x"}`)
}

func TestMergeConfigReconcileValid(t *testing.T) {
	value := `{"scope_field":"gate_scope","max_age_seconds":3600,"expectations":[{"event":"pre-tool-use","property":"gate_mode","config_field":"gate_mode","config_field_when_set":"override_mode"}]}`
	input := mergeInput("worktree-flow",
		[]string{},
		[]string{fmt.Sprintf(`{"key":"reconcile","shape":%s}`, reconcileShape)},
		[]string{manifestJSON("reconcile", value)},
	)
	code, got, raw := runMergeConfigCLI(t, input)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want empty", got.Error)
	}
	assertConfigEqual(t, raw, `{"reconcile": `+value+`}`)
}

func TestMergeConfigReconcilePartialValid(t *testing.T) {
	input := mergeInput("worktree-flow",
		[]string{},
		[]string{fmt.Sprintf(`{"key":"reconcile","shape":%s}`, reconcileShape)},
		[]string{manifestJSON("reconcile", `{"scope_field":"gate_scope"}`)},
	)
	_, got, raw := runMergeConfigCLI(t, input)
	if got.Error != "" {
		t.Fatalf("error = %q, want empty", got.Error)
	}
	assertConfigEqual(t, raw, `{"reconcile": {"scope_field":"gate_scope"}}`)
}

func TestMergeConfigReconcileInvalidCases(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantErr string
	}{
		{
			"wrong scalar type",
			`{"scope_field":5}`,
			"recipe 'worktree-flow': invalid config field 'reconcile': [config.reconcile].scope_field: expected string, got int",
		},
		{
			"float where integer",
			`{"max_age_seconds":5.5}`,
			"recipe 'worktree-flow': invalid config field 'reconcile': [config.reconcile].max_age_seconds: expected integer, got float",
		},
		{
			"bool where integer",
			`{"max_age_seconds":true}`,
			"recipe 'worktree-flow': invalid config field 'reconcile': [config.reconcile].max_age_seconds: expected integer, got bool",
		},
		{
			"string where integer",
			`{"max_age_seconds":"5"}`,
			"recipe 'worktree-flow': invalid config field 'reconcile': [config.reconcile].max_age_seconds: expected integer, got str",
		},
		{
			"unknown key",
			`{"bogus":1}`,
			"recipe 'worktree-flow': invalid config field 'reconcile': [config.reconcile]: unknown key 'bogus'",
		},
		{
			"unknown key first-error follows value order",
			`{"zzz":1,"aaa":2}`,
			"recipe 'worktree-flow': invalid config field 'reconcile': [config.reconcile]: unknown key 'zzz'",
		},
		{
			"not a table",
			`[]`,
			"recipe 'worktree-flow': invalid config field 'reconcile': [config.reconcile]: expected table, got list",
		},
		{
			"nested index context",
			`{"expectations":[{"event":"x"},{"event":false}]}`,
			"recipe 'worktree-flow': invalid config field 'reconcile': [config.reconcile].expectations[1].event: expected string, got bool",
		},
		{
			"sub-checks follow shape order",
			`{"max_age_seconds":"x","scope_field":1}`,
			"recipe 'worktree-flow': invalid config field 'reconcile': [config.reconcile].scope_field: expected string, got int",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := mergeInput("worktree-flow",
				[]string{},
				[]string{fmt.Sprintf(`{"key":"reconcile","shape":%s}`, reconcileShape)},
				[]string{manifestJSON("reconcile", tc.value)},
			)
			code, got, _ := runMergeConfigCLI(t, input)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 for semantic error", code)
			}
			if got.Error != tc.wantErr {
				t.Fatalf("error = %q, want %q", got.Error, tc.wantErr)
			}
		})
	}
}

func TestMergeConfigReconcileListCap(t *testing.T) {
	var expectations strings.Builder
	expectations.WriteString(`{"expectations":[`)
	for i := 0; i < 33; i++ {
		if i > 0 {
			expectations.WriteString(",")
		}
		fmt.Fprintf(&expectations, `{"event":"e%d"}`, i)
	}
	expectations.WriteString(`]}`)
	input := mergeInput("worktree-flow",
		[]string{},
		[]string{fmt.Sprintf(`{"key":"reconcile","shape":%s}`, reconcileShape)},
		[]string{manifestJSON("reconcile", expectations.String())},
	)
	code, got, _ := runMergeConfigCLI(t, input)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 for semantic error", code)
	}
	wantErr := "recipe 'worktree-flow': invalid config field 'reconcile': [config.reconcile].expectations: expected at most 32 entries, got 33"
	if got.Error != wantErr {
		t.Fatalf("error = %q, want %q", got.Error, wantErr)
	}
}

func TestMergeConfigEmptyEnvelope(t *testing.T) {
	code, got, raw := runMergeConfigCLI(t, `{"recipe_name":"worktree-flow"}`)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got.Error != "" {
		t.Fatalf("error = %q, want empty", got.Error)
	}
	if got.Warnings == nil || len(got.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want empty non-nil", got.Warnings)
	}
	assertConfigEqual(t, raw, `{}`)
}

func TestMergeConfigEnumUsesPythonStr(t *testing.T) {
	cases := []struct {
		name      string
		enumValue string
		manifest  string
		wantErr   string
	}{
		{"bool True matches", `["True"]`, `true`, ""},
		{"bool False matches", `["False"]`, `false`, ""},
		{"int matches", `["1"]`, `1`, ""},
		{"float matches", `["1.5"]`, `1.5`, ""},
		{"bool where int enum", `["1"]`, `true`, "recipe 'worktree-flow': config field 'mode' value 'True' is invalid; allowed: 1"},
		{"list value uses python repr", `["x"]`, `[1,"a"]`, "recipe 'worktree-flow': config field 'mode' value '[1, 'a']' is invalid; allowed: x"},
		// CPython repr() parity for strings inside containers: double-quote
		// wrap when the string holds an apostrophe and no double quote;
		// control characters escape as \xNN (and \uXXXX past 0xFF).
		{"apostrophe switches to double quotes", `["x"]`, `["a'b"]`, `recipe 'worktree-flow': config field 'mode' value '["a'b"]' is invalid; allowed: x`},
		{"both quote kinds keep single quotes", `["x"]`, `["a'b\"c"]`, `recipe 'worktree-flow': config field 'mode' value '['a\'b"c']' is invalid; allowed: x`},
		{"control char escapes as xNN", `["x"]`, `["a\u0001b"]`, `recipe 'worktree-flow': config field 'mode' value '['a\x01b']' is invalid; allowed: x`},
		{"DEL escapes as x7f", `["x"]`, `["a\u007fb"]`, `recipe 'worktree-flow': config field 'mode' value '['a\x7fb']' is invalid; allowed: x`},
		{"C1 control escapes as x85", `["x"]`, `["a\u0085b"]`, `recipe 'worktree-flow': config field 'mode' value '['a\x85b']' is invalid; allowed: x`},
		{"non-printable astral-adjacent escapes as uXXXX", `["x"]`, `["a\u2028b"]`, `recipe 'worktree-flow': config field 'mode' value '['a\u2028b']' is invalid; allowed: x`},
		{"apostrophe in dict value", `["x"]`, `{"k":"o'clock"}`, `recipe 'worktree-flow': config field 'mode' value '{'k': "o'clock"}' is invalid; allowed: x`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := mergeInput("worktree-flow",
				[]string{fieldJSON("mode", false, false, "", []string{"placeholder"})},
				[]string{},
				[]string{manifestJSON("mode", tc.manifest)},
			)
			// Swap in the case's enum list (fieldJSON cannot carry it directly).
			input = strings.Replace(input, `"enum":["placeholder"]`, `"enum":`+tc.enumValue, 1)
			code, got, _ := runMergeConfigCLI(t, input)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 for semantic error", code)
			}
			if got.Error != tc.wantErr {
				t.Fatalf("error = %q, want %q", got.Error, tc.wantErr)
			}
		})
	}
}

func TestMergeConfigMalformedInputExitsTwo(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"not json", "not json"},
		{"empty stdin", ""},
		{"missing recipe_name", `{"fields":[]}`},
		{"blank recipe_name", `{"recipe_name":"  "}`},
		{"fields wrong type", `{"recipe_name":"r","fields":{}}`},
		{"manifest entry missing key", `{"recipe_name":"r","manifest":[{"value":1}]}`},
		{"truncated json", `{"recipe_name":"r","fields":[`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			code := run([]string{"--plan-merge-config"}, strings.NewReader(tc.input), &stdout, &stderr)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2 (stderr: %q)", code, stderr.String())
			}
			if stderr.String() == "" {
				t.Fatal("expected a stderr diagnostic for the process error")
			}
		})
	}
}
