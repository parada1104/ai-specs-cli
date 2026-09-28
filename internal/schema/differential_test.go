package schema

// Differential tests against the legacy Python implementation
// (lib/_internal/recipe_schema.py), whose validation error strings are a
// FROZEN surface per docs/go-migration-parity-contract.md.
//
// The Python side runs via testdata/driver.py; both sides emit the same
// canonical projection of the parsed Recipe (or the exact error string) and
// the test requires byte-for-byte equality.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/toml"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func runDriver(t *testing.T, root string, args ...string) string {
	t.Helper()
	driver := filepath.Join(root, "internal", "schema", "testdata", "driver.py")
	cmd := exec.Command("python3", append([]string{driver}, args...)...)
	cmd.Dir = root
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("python driver failed: %v\nstderr: %s", err, errb.String())
	}
	return strings.TrimSpace(out.String())
}

func decodeDriver(t *testing.T, line string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	var payload map[string]any
	if err := dec.Decode(&payload); err != nil {
		t.Fatalf("driver emitted non-JSON output: %q: %v", line, err)
	}
	return payload
}

// --- canonical JSON (mirrors python json.dumps(sort_keys=True,
// ensure_ascii=False, separators=(',', ':')) on already-canonicalized values).

func canonicalJSON(v any) string {
	var sb strings.Builder
	writeCanonical(&sb, v)
	return sb.String()
}

func writeCanonical(sb *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		sb.WriteString("null")
	case bool:
		if x {
			sb.WriteString("true")
		} else {
			sb.WriteString("false")
		}
	case string:
		writeJSONString(sb, x)
	case int:
		sb.WriteString(strconv.Itoa(x))
	case int64:
		sb.WriteString(strconv.FormatInt(x, 10))
	case json.Number:
		sb.WriteString(x.String())
	case []string:
		sb.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				sb.WriteByte(',')
			}
			writeJSONString(sb, e)
		}
		sb.WriteByte(']')
	case []any:
		sb.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				sb.WriteByte(',')
			}
			writeCanonical(sb, e)
		}
		sb.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sb.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				sb.WriteByte(',')
			}
			writeJSONString(sb, k)
			sb.WriteByte(':')
			writeCanonical(sb, x[k])
		}
		sb.WriteByte('}')
	default:
		panic(fmt.Sprintf("canonicalJSON: unsupported type %T", v))
	}
}

func writeJSONString(sb *strings.Builder, s string) {
	sb.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			sb.WriteString(`\"`)
		case '\\':
			sb.WriteString(`\\`)
		case '\n':
			sb.WriteString(`\n`)
		case '\r':
			sb.WriteString(`\r`)
		case '\t':
			sb.WriteString(`\t`)
		case '\b':
			sb.WriteString(`\b`)
		case '\f':
			sb.WriteString(`\f`)
		default:
			if r < 0x20 {
				sb.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				sb.WriteRune(r)
			}
		}
	}
	sb.WriteByte('"')
}

// --- canonicalization of raw TOML/Recipe values for projection comparison.

func canonAny(v any) any {
	switch x := v.(type) {
	case nil, bool, string, int64:
		return x
	case int:
		return int64(x)
	case float64:
		return pyFloatStr(x)
	case json.Number:
		return x.String()
	case *toml.Table:
		keys := x.Keys()
		m := make(map[string]any, len(keys))
		for _, k := range keys {
			raw, _ := x.Get(k)
			m[k] = canonAny(raw)
		}
		return m
	case []*toml.Table:
		arr := make([]any, len(x))
		for i, e := range x {
			arr[i] = canonAny(e)
		}
		return arr
	case []any:
		arr := make([]any, len(x))
		for i, e := range x {
			arr[i] = canonAny(e)
		}
		return arr
	case map[string]any:
		return canonAnyMap(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func canonAnyMap(m map[string]any) any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = canonAny(v)
	}
	return out
}

// --- projection (mirrors proj() in testdata/driver.py byte-for-byte).

func projRecipe(r *Recipe) map[string]any {
	// Python's ConfigField.enum is Optional[list]: absent -> None (null),
	// unlike the list-typed fields whose absence is an empty array.
	enumProj := func(e []string) any {
		if e == nil {
			return nil
		}
		arr := make([]any, len(e))
		for i, s := range e {
			arr[i] = s
		}
		return arr
	}
	var brief any
	if r.BriefFragments != nil {
		bm := map[string]any{}
		addBrief := func(name string, fs []BriefFragment) {
			if fs == nil {
				return
			}
			arr := make([]any, 0, len(fs))
			for _, f := range fs {
				arr = append(arr, map[string]any{
					"text": canonAny(f.Text),
					"key":  canonAny(f.Key),
				})
			}
			bm[name] = arr
		}
		addBrief("runtime_flow", r.BriefFragments.RuntimeFlow)
		addBrief("context_sources", r.BriefFragments.ContextSources)
		addBrief("conflict_policy", r.BriefFragments.ConflictPolicy)
		addBrief("workflow_rules", r.BriefFragments.WorkflowRules)
		addBrief("useful_commands", r.BriefFragments.UsefulCommands)
		addBrief("mcp_descriptions", r.BriefFragments.McpDescriptions)
		brief = bm
	}

	var init any
	if r.Init != nil {
		init = map[string]any{
			"prompt":         r.Init.Prompt,
			"description":    r.Init.Description,
			"needs_manifest": r.Init.NeedsManifest,
			"needs_mcp":      r.Init.NeedsMCP,
		}
	}

	fields := map[string]any{}
	for k, f := range r.ConfigSchema.Fields {
		fields[k] = map[string]any{
			"type":       f.Type,
			"default":    canonAny(f.Default),
			"enum":       enumProj(f.Enum),
			"help_text":  f.HelpText,
			"validation": canonAnyMap(f.Validation),
		}
	}
	tables := map[string]any{}
	for k, t := range r.ConfigSchema.Tables {
		tables[k] = map[string]any{
			"shape":  canonShape(t.Shape),
			"values": canonAnyMap(t.Values),
		}
	}

	skills := make([]any, 0, len(r.Skills))
	for _, s := range r.Skills {
		skills = append(skills, map[string]any{"id": s.ID, "source": s.Source, "url": s.URL, "path": s.Path})
	}
	commands := make([]any, 0, len(r.Commands))
	for _, c := range r.Commands {
		commands = append(commands, map[string]any{"id": c.ID, "path": c.Path})
	}
	mcp := make([]any, 0, len(r.MCP))
	for _, m := range r.MCP {
		mcp = append(mcp, map[string]any{"id": m.ID, "config": canonAnyMap(m.Config)})
	}
	templates := make([]any, 0, len(r.Templates))
	for _, t := range r.Templates {
		templates = append(templates, map[string]any{
			"source": t.Source, "target": t.Target,
			"condition": t.Condition, "update_policy": t.UpdatePolicy,
		})
	}
	docs := make([]any, 0, len(r.Docs))
	for _, d := range r.Docs {
		docs = append(docs, map[string]any{"source": d.Source, "target": d.Target})
	}
	caps := make([]any, 0, len(r.Capabilities))
	for _, c := range r.Capabilities {
		caps = append(caps, c.ID)
	}
	hooks := make([]any, 0, len(r.Hooks))
	for _, h := range r.Hooks {
		hooks = append(hooks, map[string]any{"event": h.Event, "action": h.Action})
	}
	rhooks := make([]any, 0, len(r.RuntimeHooks))
	for _, h := range r.RuntimeHooks {
		rhooks = append(rhooks, map[string]any{
			"id": h.ID, "event": h.Event, "script": h.Script,
			"matcher": h.Matcher, "blocking": h.Blocking, "description": h.Description,
		})
	}
	deps := make([]any, 0, len(r.CliDeps))
	for _, d := range r.CliDeps {
		deps = append(deps, map[string]any{
			"binary": d.Binary, "purpose": d.Purpose, "required": d.Required,
			"install_url": d.InstallURL, "version_check": d.VersionCheck,
			"min_version": d.MinVersion, "installer": d.Installer,
			"repository": d.Repository, "release_policy": d.ReleasePolicy,
		})
	}

	return map[string]any{
		"id":             r.ID,
		"name":           r.Name,
		"description":    r.Description,
		"version":        r.Version,
		"author":         r.Author,
		"license":        r.License,
		"tags":           r.Tags,
		"conflicts_with": r.ConflictsWith,
		"skills":         skills,
		"commands":       commands,
		"mcp":            mcp,
		"templates":      templates,
		"docs":           docs,
		"capabilities":   caps,
		"hooks":          hooks,
		"runtime_hooks":  rhooks,
		"config": map[string]any{
			"fields": fields,
			"extra":  canonAnyMap(r.ConfigSchema.Extra),
			"tables": tables,
		},
		"cli_deps": deps,
		"init":     init,
		"brief":    brief,
	}
}

func canonShape(shape any) any {
	switch x := shape.(type) {
	case *shapeTable:
		m := make(map[string]any, len(x.order))
		for _, k := range x.order {
			m[k] = canonShape(x.entries[k])
		}
		return m
	case string:
		return x
	case []any:
		arr := make([]any, len(x))
		for i, e := range x {
			arr[i] = canonShape(e)
		}
		return arr
	default:
		panic(fmt.Sprintf("canonShape: unsupported %T", shape))
	}
}

// --- tests ---

func TestDifferentialAgainstPython(t *testing.T) {
	root := repoRoot(t)
	cleanCount, okCount, errCount := 0, 0, 0

	// Clean corpus: every real recipe.toml, loaded end to end via
	// load_recipe_toml (Parse + full FS-resolved validation) on both sides.
	var corpus []string
	for _, pat := range []string{"catalog/recipes/*/recipe.toml", "tests/fixtures/recipes/*/recipe.toml"} {
		matches, err := filepath.Glob(filepath.Join(root, pat))
		if err != nil {
			t.Fatal(err)
		}
		corpus = append(corpus, matches...)
	}
	if len(corpus) < 20 {
		t.Fatalf("clean corpus unexpectedly small: %d", len(corpus))
	}
	for _, path := range corpus {
		line := runDriver(t, root, "load", path)
		payload := decodeDriver(t, line)
		if ok, _ := payload["ok"].(bool); !ok {
			t.Errorf("%s: python rejected clean recipe: %v", path, payload["error"])
			continue
		}
		goRec, err := LoadRecipeToml(path)
		if err != nil {
			t.Errorf("%s: go LoadRecipeToml failed: %v", path, err)
			continue
		}
		if got, want := canonicalJSON(projRecipe(goRec)), canonicalJSON(payload["recipe"]); got != want {
			t.Errorf("%s: projection mismatch\n  go: %s\n  py: %s", path, got, want)
			continue
		}
		cleanCount++
	}

	// Synthetic fixtures: byte-identical error strings, or byte-identical
	// projections for the ok-cases that exercise coercions and silent paths.
	// Fixtures listed here run with a fresh temp recipe dir (FS-resolved
	// checks); everything else runs with recipe_dir=None.
	tempNames := map[string]bool{
		"ok_hooks_full.toml":                  true,
		"ok_hooks_dotted_path.toml":           true,
		"ok_init_full.toml":                   true,
		"err_hooks_escape_dir.toml":           true,
		"err_hooks_escape_symlink.toml":       true,
		"ok_hooks_symlink_inside.toml":        true,
		"err_init_prompt_not_found.toml":      true,
		"err_init_prompt_is_dir.toml":         true,
		"err_init_prompt_absolute.toml":       true,
		"err_init_prompt_escape.toml":         true,
		"err_init_prompt_symlink_escape.toml": true,
		"ok_init_prompt_symlink_inside.toml":  true,
	}
	setup := map[string]func(t *testing.T, dir string) error{
		"ok_init_full.toml": func(t *testing.T, dir string) error {
			return os.WriteFile(filepath.Join(dir, "init.md"), []byte("# init\n"), 0o644)
		},
		"err_init_prompt_is_dir.toml": func(t *testing.T, dir string) error {
			return os.Mkdir(filepath.Join(dir, "subdir"), 0o755)
		},
		// err_hooks_escape_symlink: hooks is a symlinked INTERMEDIATE component
		// pointing outside the recipe dir; Path.resolve() must follow it.
		"err_hooks_escape_symlink.toml": func(t *testing.T, dir string) error {
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "x.sh"), []byte("#!/bin/sh\n"), 0o644); err != nil {
				return err
			}
			return os.Symlink(outside, filepath.Join(dir, "hooks"))
		},
		// ok_hooks_symlink_inside: the symlink stays INSIDE the recipe dir and
		// must remain legal.
		"ok_hooks_symlink_inside.toml": func(t *testing.T, dir string) error {
			real := filepath.Join(dir, "real")
			if err := os.MkdirAll(real, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(real, "x.sh"), []byte("#!/bin/sh\n"), 0o644); err != nil {
				return err
			}
			return os.Symlink(real, filepath.Join(dir, "hooks"))
		},
		"err_init_prompt_symlink_escape.toml": func(t *testing.T, dir string) error {
			outside := t.TempDir()
			if err := os.WriteFile(filepath.Join(outside, "p.md"), []byte("# p\n"), 0o644); err != nil {
				return err
			}
			return os.Symlink(outside, filepath.Join(dir, "prompts"))
		},
		"ok_init_prompt_symlink_inside.toml": func(t *testing.T, dir string) error {
			real := filepath.Join(dir, "prompts-real")
			if err := os.MkdirAll(real, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(real, "p.md"), []byte("# p\n"), 0o644); err != nil {
				return err
			}
			return os.Symlink(real, filepath.Join(dir, "prompts"))
		},
	}
	fixtures, err := filepath.Glob(filepath.Join(root, "internal", "schema", "testdata", "fixtures", "*.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) < 25 {
		t.Fatalf("fixture corpus unexpectedly small: %d", len(fixtures))
	}
	for _, fx := range fixtures {
		fx := fx
		name := filepath.Base(fx)
		t.Run(name, func(t *testing.T) {
			dirArg := "-"
			if tempNames[name] {
				dir := t.TempDir()
				if fn := setup[name]; fn != nil {
					if err := fn(t, dir); err != nil {
						t.Fatal(err)
					}
				}
				dirArg = dir
			}
			line := runDriver(t, root, "validate", fx, dirArg)
			payload := decodeDriver(t, line)

			data, err := os.ReadFile(fx)
			if err != nil {
				t.Fatal(err)
			}
			tbl, err := toml.Parse(data)
			if err != nil {
				t.Fatalf("go toml parse failed: %v", err)
			}
			goDir := ""
			if dirArg != "-" {
				goDir = dirArg
			}
			goRec, goErr := ValidateRecipeToml(tbl, goDir)

			if ok, _ := payload["ok"].(bool); !ok {
				pyErr, _ := payload["error"].(string)
				if goErr == nil {
					t.Errorf("python errored but go succeeded\n  py: %s", pyErr)
					return
				}
				if goErr.Error() != pyErr {
					t.Errorf("error mismatch\n  go: %q\n  py: %q", goErr.Error(), pyErr)
					return
				}
				var ve *ValidationError
				if !errors.As(goErr, &ve) {
					t.Errorf("expected *ValidationError, got %T", goErr)
					return
				}
				errCount++
				return
			}
			if goErr != nil {
				t.Errorf("go errored but python succeeded: %v", goErr)
				return
			}
			if got, want := canonicalJSON(projRecipe(goRec)), canonicalJSON(payload["recipe"]); got != want {
				t.Errorf("projection mismatch\n  go: %s\n  py: %s", got, want)
				return
			}
			okCount++
		})
	}

	t.Logf("differential: clean corpus %d recipes ok both sides; fixtures %d byte-identical (%d ok, %d error)",
		cleanCount, okCount+errCount, okCount, errCount)
	if cleanCount < 20 || okCount+errCount < 25 {
		t.Fatalf("differential coverage unexpectedly small: clean=%d fixtures=%d", cleanCount, okCount+errCount)
	}
}

func TestLoadRecipeTomlMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recipe.toml")
	_, err := LoadRecipeToml(path)
	if err == nil {
		t.Fatal("expected error for missing recipe.toml")
	}
	if want := "recipe.toml not found: " + path; err.Error() != want {
		t.Errorf("error mismatch\n got: %q\nwant: %q", err.Error(), want)
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected *ValidationError, got %T", err)
	}
}

// TestPyReprDepthCapGoOnly pins the recursion guard on the pyRepr surface:
// a TOML document whose update_policy nests beyond pyReprMaxDepth produces a
// descriptive error (Python raises RecursionError inside repr() there
// instead of emitting any validation message), and moderate nesting keeps
// rendering.
func TestPyReprDepthCapGoOnly(t *testing.T) {
	// pyReprMaxDepth+2 nested arrays: the TOML form reaches the cap one
	// level earlier than a programmatic chain (the innermost array is empty).
	depth := pyReprMaxDepth + 2
	src := "[recipe]\nid = \"x\"\nname = \"N\"\ndescription = \"D\"\nversion = \"1.0\"\n\n[[provides.templates]]\nsource = \"a\"\ntarget = \"b\"\nupdate_policy = " +
		strings.Repeat("[", depth) + strings.Repeat("]", depth) + "\n"
	tbl, err := toml.Parse([]byte(src))
	if err != nil {
		t.Fatalf("toml parse: %v", err)
	}
	_, err = ValidateRecipeToml(tbl, "")
	if err == nil {
		t.Fatal("ValidateRecipeToml beyond pyReprMaxDepth must error")
	}
	if !strings.Contains(err.Error(), "recursion depth") {
		t.Errorf("depth-cap error = %.120q, want a recursion-depth message", err.Error())
	}
}
