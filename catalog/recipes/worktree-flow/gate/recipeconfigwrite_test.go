package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// requireRecipeConfigSeam skips the end-to-end parity tests when the standard
// TOML parser subprocess is unavailable, mirroring the acquisition tests in
// recipe_toml_test.go.
func requireRecipeConfigSeam(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("recipe config write runs the standard TOML parser subprocess")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available for the TOML seam")
	}
}

// writeRecipeConfigFixture writes a manifest fixture and returns its path.
func writeRecipeConfigFixture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ai-specs.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// runRecipeConfigWriteCLI drives the command the way the Python bridge will:
// one JSON envelope on stdin, one JSON envelope on stdout, exit 0/2. The
// envelope is built with encoding/json (matching the production bridge's
// json.dumps), not Go %q quoting: %q emits Go escape syntax (\xNN) that is
// not valid JSON for control characters.
func runRecipeConfigWriteCLI(t *testing.T, manifestPath, recipeID, valuesJSON string) (int, string, string) {
	t.Helper()
	envelope, err := json.Marshal(struct {
		ManifestPath string          `json:"manifest_path"`
		RecipeID     string          `json:"recipe_id"`
		Values       json.RawMessage `json:"values"`
	}{ManifestPath: manifestPath, RecipeID: recipeID, Values: json.RawMessage(valuesJSON)})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	var stdout, stderr bytes.Buffer
	code := runWriteRecipeConfig(bytes.NewReader(envelope), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

type writeRecipeConfigEnvelope struct {
	Applied bool    `json:"applied"`
	Error   *string `json:"error"`
}

func decodeWriteRecipeConfigEnvelope(t *testing.T, out string) writeRecipeConfigEnvelope {
	t.Helper()
	var envelope writeRecipeConfigEnvelope
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %q: %v", out, err)
	}
	return envelope
}

func assertFileBytes(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if string(got) != want {
		t.Fatalf("manifest bytes differ\n--- got ---\n%q\n--- want ---\n%q", string(got), want)
	}
}

// --- end-to-end parity tests through the CLI contract ---

// TestRecipeConfigWriteFlatReplace replaces one flat key in place, preserving
// the line's indent and its TOML-aware inline comment byte for byte.
func TestRecipeConfigWriteFlatReplace(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "# project manifest\n\n[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmax_age_seconds = 30   # freshness window\nmode = \"fast\"\n"
	path := writeRecipeConfigFixture(t, original)

	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"mode": "slow"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	if envelope := decodeWriteRecipeConfigEnvelope(t, out); !envelope.Applied {
		t.Fatalf("envelope = %#v, want applied true", envelope)
	}
	want := "# project manifest\n\n[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmax_age_seconds = 30   # freshness window\nmode = \"slow\"\n"
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteIndentPreserved checks an indented config line keeps
// its indent and comment across a replacement.
func TestRecipeConfigWriteIndentPreserved(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\n  mode = \"fast\"  # operational\n"
	path := writeRecipeConfigFixture(t, original)

	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"mode": "slow"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	want := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\n  mode = \"slow\"  # operational\n"
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteMissingFlatKeysAppended appends absent flat keys in
// sorted order at the end of the config block.
func TestRecipeConfigWriteMissingFlatKeysAppended(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"fast\"\n"
	path := writeRecipeConfigFixture(t, original)

	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"mode": "slow", "zeta": 3, "alpha": true}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	want := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"slow\"\nalpha = true\nzeta = 3\n"
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteBlockCreation appends a full recipe block at EOF when
// the recipe is absent, with enabled = true and no version key, handling both
// trailing-newline shapes of the original manifest.
func TestRecipeConfigWriteBlockCreation(t *testing.T) {
	requireRecipeConfigSeam(t)
	for name, original := range map[string]string{
		"trailing newline":      "[recipes.other]\nenabled = false\n",
		"no trailing newline":   "[recipes.other]\nenabled = false",
		"empty manifest":        "",
		"newline-only manifest": "\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := writeRecipeConfigFixture(t, original)
			code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"mode": "fast", "reconcile": {"max_age_seconds": 30}}`)
			if code != 0 {
				t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
			}
			want := original
			if !strings.HasSuffix(want, "\n") && want != "" {
				want += "\n"
			}
			want += "\n[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"fast\"\nreconcile = { max_age_seconds = 30 }\n"
			assertFileBytes(t, path, want)
		})
	}
}

// TestRecipeConfigWriteInsertConfigTable inserts the config table at the end
// of an existing recipe's region.
func TestRecipeConfigWriteInsertConfigTable(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n"
	path := writeRecipeConfigFixture(t, original)

	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"mode": "fast"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	want := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"fast\"\n"
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteNoOpByteIdentity leaves the manifest untouched when
// every value already matches, reporting applied=false with no write.
func TestRecipeConfigWriteNoOpByteIdentity(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmax_age_seconds = 30\nmode = \"fast\"\nreconcile = { max_age_seconds = 30, mode = \"fast\" }\n"
	path := writeRecipeConfigFixture(t, original)

	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"max_age_seconds": 30, "mode": "fast", "reconcile": {"max_age_seconds": 30, "mode": "fast"}}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	if envelope := decodeWriteRecipeConfigEnvelope(t, out); envelope.Applied {
		t.Fatalf("envelope = %#v, want applied false", envelope)
	}
	assertFileBytes(t, path, original)
}

// TestRecipeConfigWriteEmptyValuesNoOp is a no-op before the manifest is even
// read: an empty or missing values object succeeds without touching files.
func TestRecipeConfigWriteEmptyValuesNoOp(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")
	for name, values := range map[string]string{
		"empty object": "{}",
		"null":         "null",
	} {
		t.Run(name, func(t *testing.T) {
			code, out, stderr := runRecipeConfigWriteCLI(t, missing, "reconcile", values)
			if code != 0 {
				t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
			}
			if envelope := decodeWriteRecipeConfigEnvelope(t, out); envelope.Applied {
				t.Fatalf("envelope = %#v, want applied false", envelope)
			}
			if _, err := os.Stat(missing); !os.IsNotExist(err) {
				t.Fatalf("manifest created by a no-op run: %v", err)
			}
		})
	}
}

// TestRecipeConfigWriteQuotedRecipeID uses json.dumps quoting for recipe ids
// that are not bare keys.
func TestRecipeConfigWriteQuotedRecipeID(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := ""
	path := writeRecipeConfigFixture(t, original)
	code, out, stderr := runRecipeConfigWriteCLI(t, path, "my.recipe", `{"mode": "fast"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	want := "\n[recipes.\"my.recipe\"]\nenabled = true\n\n[recipes.\"my.recipe\".config]\nmode = \"fast\"\n"
	assertFileBytes(t, path, want)

	// A second run targeting the same quoted recipe finds its block: no-op.
	code, out, stderr = runRecipeConfigWriteCLI(t, path, "my.recipe", `{"mode": "fast"}`)
	if code != 0 {
		t.Fatalf("second run exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	if envelope := decodeWriteRecipeConfigEnvelope(t, out); envelope.Applied {
		t.Fatalf("second run = %#v, want applied false", envelope)
	}
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteHashInsideStrings keeps '#' characters inside string
// values intact and the real inline comment after them.
func TestRecipeConfigWriteHashInsideStrings(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"a # b\"  # tail\n"
	path := writeRecipeConfigFixture(t, original)

	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"mode": "x # y"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	want := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"x # y\"  # tail\n"
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteDottedInlineUpdate updates an inline-table root in
// place, preserving unrelated keys in document order and the line comment.
func TestRecipeConfigWriteDottedInlineUpdate(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nreconcile = { max_age_seconds = 30, mode = \"fast\" }   # inline table\nother = 1\n"
	path := writeRecipeConfigFixture(t, original)

	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"reconcile.max_age_seconds": 60}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	want := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nreconcile = { max_age_seconds = 60, mode = \"fast\" }   # inline table\nother = 1\n"
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteDottedHeaderLeafReplace replaces a leaf inside a
// header-table root.
func TestRecipeConfigWriteDottedHeaderLeafReplace(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\n\n[recipes.reconcile.config.reconcile]\nmax_age_seconds = 30\nmode = \"fast\"\n"
	path := writeRecipeConfigFixture(t, original)

	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"reconcile.mode": "slow"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	want := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\n\n[recipes.reconcile.config.reconcile]\nmax_age_seconds = 30\nmode = \"slow\"\n"
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteDottedHeaderLeafAppend appends a missing leaf at the
// end of the header-table root's block.
func TestRecipeConfigWriteDottedHeaderLeafAppend(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\n\n[recipes.reconcile.config.reconcile]\nmax_age_seconds = 30\n"
	path := writeRecipeConfigFixture(t, original)

	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"reconcile.mode": "fast"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	want := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\n\n[recipes.reconcile.config.reconcile]\nmax_age_seconds = 30\nmode = \"fast\"\n"
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteDottedCreateRoot creates a new inline-table root at
// the end of the config block when the root exists nowhere.
func TestRecipeConfigWriteDottedCreateRoot(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"fast\"\n"
	path := writeRecipeConfigFixture(t, original)

	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"reconcile.max_age_seconds": 30}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	want := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"fast\"\nreconcile = { max_age_seconds = 30 }\n"
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteConflictRefusal refuses whole-table and dotted updates
// to the same root with the exact reference string, exit 2, and no write.
func TestRecipeConfigWriteConflictRefusal(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile.config]\nmode = \"fast\"\n"
	cases := []struct {
		values string
		want   string
	}{
		{`{"reconcile": "x", "reconcile.mode": "y"}`, "cannot combine a whole-table and a dotted update for key(s): reconcile"},
		{`{"c": 3, "a.b": 2, "a": 1, "c.d": 4}`, "cannot combine a whole-table and a dotted update for key(s): a, c"},
	}
	for _, tc := range cases {
		path := writeRecipeConfigFixture(t, original)
		code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", tc.values)
		if code != 2 {
			t.Fatalf("exit = %d (want 2), stderr %q, stdout %q", code, stderr, out)
		}
		envelope := decodeWriteRecipeConfigEnvelope(t, out)
		if envelope.Error == nil || *envelope.Error != tc.want {
			t.Fatalf("error = %#v, want %q", envelope.Error, tc.want)
		}
		assertFileBytes(t, path, original)
	}
}

// TestRecipeConfigWriteMultilineRefusals refuses to replace multiline values,
// for flat keys and inline-table roots alike.
func TestRecipeConfigWriteMultilineRefusals(t *testing.T) {
	requireRecipeConfigSeam(t)
	t.Run("flat", func(t *testing.T) {
		original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"\"\"\nopen\n\"\"\"\n"
		path := writeRecipeConfigFixture(t, original)
		code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"mode": "x"}`)
		if code != 2 {
			t.Fatalf("exit = %d (want 2), stderr %q, stdout %q", code, stderr, out)
		}
		envelope := decodeWriteRecipeConfigEnvelope(t, out)
		if envelope.Error == nil || *envelope.Error != "cannot replace multiline value for key 'mode'" {
			t.Fatalf("error = %#v, want the multiline refusal", envelope.Error)
		}
		assertFileBytes(t, path, original)
	})
	t.Run("inline root", func(t *testing.T) {
		original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nreconcile = { a = \"unterminated\n"
		path := writeRecipeConfigFixture(t, original)
		code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"reconcile.a": 1}`)
		if code != 2 {
			t.Fatalf("exit = %d (want 2), stderr %q, stdout %q", code, stderr, out)
		}
		envelope := decodeWriteRecipeConfigEnvelope(t, out)
		if envelope.Error == nil || *envelope.Error != "cannot replace multiline value for key 'reconcile'" {
			t.Fatalf("error = %#v, want the multiline refusal", envelope.Error)
		}
		assertFileBytes(t, path, original)
	})
}

// TestRecipeConfigWriteDottedTableNotFound refuses a deeper dotted update
// whose intermediate header table does not exist, with the exact reference
// string. A root that exists nowhere becomes a new inline table instead.
func TestRecipeConfigWriteDottedTableNotFound(t *testing.T) {
	requireRecipeConfigSeam(t)
	t.Run("missing intermediate header", func(t *testing.T) {
		original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\n\n[recipes.reconcile.config.opts]\nmode = \"fast\"\n"
		path := writeRecipeConfigFixture(t, original)
		code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"opts.deep.key": 1}`)
		if code != 2 {
			t.Fatalf("exit = %d (want 2), stderr %q, stdout %q", code, stderr, out)
		}
		envelope := decodeWriteRecipeConfigEnvelope(t, out)
		want := "cannot update 'opts.deep.key': table [recipes.reconcile.config.opts.deep] not found"
		if envelope.Error == nil || *envelope.Error != want {
			t.Fatalf("error = %#v, want %q", envelope.Error, want)
		}
		assertFileBytes(t, path, original)
	})
	t.Run("root nowhere becomes a new inline table", func(t *testing.T) {
		original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"fast\"\n"
		path := writeRecipeConfigFixture(t, original)
		code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"opts.deep.key": 1}`)
		if code != 0 {
			t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
		}
		want := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"fast\"\nopts = { deep = { key = 1 } }\n"
		assertFileBytes(t, path, want)
	})
}

// TestRecipeConfigWriteRootNotInlineTable refuses a dotted update against a
// scalar root line.
func TestRecipeConfigWriteRootNotInlineTable(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nreconcile = \"hello\"\n"
	path := writeRecipeConfigFixture(t, original)
	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"reconcile.a": 1}`)
	if code != 2 {
		t.Fatalf("exit = %d (want 2), stderr %q, stdout %q", code, stderr, out)
	}
	envelope := decodeWriteRecipeConfigEnvelope(t, out)
	want := "cannot update 'reconcile': its value is not an inline table"
	if envelope.Error == nil || *envelope.Error != want {
		t.Fatalf("error = %#v, want %q", envelope.Error, want)
	}
	assertFileBytes(t, path, original)
}

// TestRecipeConfigWriteValidateAndRestore forces a write that would produce
// invalid TOML (a [recipes.new] header while `recipes` is already an inline
// table): the exact invalid-TOML refusal fires and the original bytes are
// preserved on disk.
func TestRecipeConfigWriteValidateAndRestore(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "recipes = { other = {} }\n"
	path := writeRecipeConfigFixture(t, original)
	code, out, stderr := runRecipeConfigWriteCLI(t, path, "new", `{"mode": "fast"}`)
	if code != 2 {
		t.Fatalf("exit = %d (want 2), stderr %q, stdout %q", code, stderr, out)
	}
	envelope := decodeWriteRecipeConfigEnvelope(t, out)
	if envelope.Error == nil || !strings.HasPrefix(*envelope.Error, "invalid TOML after config write: ") {
		t.Fatalf("error = %#v, want the invalid-TOML refusal prefix", envelope.Error)
	}
	assertFileBytes(t, path, original)
}

// TestRecipeConfigWriteExoticStrings exercises the full pipeline with keys
// and string values that need json.dumps escaping (quotes, backslashes,
// control characters, non-ASCII) against a pre-existing quoted-key line.
func TestRecipeConfigWriteExoticStrings(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\n\"my key\" = \"old # value\"  # keep\n"
	path := writeRecipeConfigFixture(t, original)
	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"my key": "new \"quoted\" # value"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	want := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\n\"my key\" = \"new \\\"quoted\\\" # value\"  # keep\n"
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteControlCharRecipeID drives the CLI helper with a
// recipe id containing a control character: the envelope must be valid JSON
// (the bridge's json.dumps produces \u0001), and the block must be created
// under the quoted key.
func TestRecipeConfigWriteControlCharRecipeID(t *testing.T) {
	requireRecipeConfigSeam(t)
	path := writeRecipeConfigFixture(t, "")
	code, out, stderr := runRecipeConfigWriteCLI(t, path, "rec\x01ipe", `{"mode": "fast"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	want := "\n[recipes.\"rec\\u0001ipe\"]\nenabled = true\n\n[recipes.\"rec\\u0001ipe\".config]\nmode = \"fast\"\n"
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteMalformedEnvelope rejects malformed requests with
// exit 2 and a diagnostic on stderr. Empty values are a no-op success even
// without manifest path or recipe id, matching the reference contract.
func TestRecipeConfigWriteMalformedEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	for _, in := range []string{
		`{"values": {}}`,
		`{"manifest_path": "/x", "values": {}}`,
	} {
		stdout.Reset()
		stderr.Reset()
		if code := runWriteRecipeConfig(strings.NewReader(in), &stdout, &stderr); code != 0 {
			t.Errorf("envelope %q: exit = %d, want 0 (stdout %q stderr %q)", in, code, stdout.String(), stderr.String())
		}
		if envelope := decodeWriteRecipeConfigEnvelope(t, stdout.String()); envelope.Applied {
			t.Errorf("envelope %q: = %#v, want applied false", in, envelope)
		}
	}
	for _, in := range []string{
		`not json`,
		`{"manifest_path": "/x", "recipe_id": "r", "values": [1, 2]}`,
	} {
		stdout.Reset()
		stderr.Reset()
		code := runWriteRecipeConfig(strings.NewReader(in), &stdout, &stderr)
		if code != 2 {
			t.Errorf("envelope %q: exit = %d, want 2 (stdout %q stderr %q)", in, code, stdout.String(), stderr.String())
		}
		if strings.TrimSpace(stderr.String()) == "" {
			t.Errorf("envelope %q: expected a stderr diagnostic", in)
		}
	}
}

// TestRecipeConfigWriteInlineQuotedKeyRoundTrip replaces an inline table whose
// inner keys are not bare TOML keys: the written manifest must be valid TOML
// (the post-write validation gate re-parses it) and the quoted keys must
// survive byte for byte.
func TestRecipeConfigWriteInlineQuotedKeyRoundTrip(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nopts = { \"my key\" = 1, mode = \"fast\" }\n"
	path := writeRecipeConfigFixture(t, original)

	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"opts": {"my key": 2, "mode": "slow"}}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	if envelope := decodeWriteRecipeConfigEnvelope(t, out); !envelope.Applied {
		t.Fatalf("envelope = %#v, want applied true", envelope)
	}
	want := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nopts = { \"my key\" = 2, mode = \"slow\" }\n"
	assertFileBytes(t, path, want)
}

// TestRecipeConfigWriteAtomicReplaceSmoke pins the atomic-replace contract:
// a successful write lands the expected bytes and leaves no temp files in the
// manifest directory, and a validation refusal leaves the original bytes and
// directory untouched.
func TestRecipeConfigWriteAtomicReplaceSmoke(t *testing.T) {
	requireRecipeConfigSeam(t)
	original := "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"fast\"\n"
	path := writeRecipeConfigFixture(t, original)

	code, out, stderr := runRecipeConfigWriteCLI(t, path, "reconcile", `{"mode": "slow"}`)
	if code != 0 {
		t.Fatalf("exit = %d, stderr %q, stdout %q", code, stderr, out)
	}
	if envelope := decodeWriteRecipeConfigEnvelope(t, out); !envelope.Applied {
		t.Fatalf("envelope = %#v, want applied true", envelope)
	}
	assertFileBytes(t, path, "[recipes.reconcile]\nenabled = true\n\n[recipes.reconcile.config]\nmode = \"slow\"\n")
	assertNoTempFiles(t, filepath.Dir(path))

	// Refusal path: original bytes preserved and no temp files left behind.
	bad := writeRecipeConfigFixture(t, "recipes = { other = {} }\n")
	code, out, _ = runRecipeConfigWriteCLI(t, bad, "new", `{"mode": "fast"}`)
	if code != 2 {
		t.Fatalf("refusal exit = %d, want 2", code)
	}
	assertFileBytes(t, bad, "recipes = { other = {} }\n")
	assertNoTempFiles(t, filepath.Dir(bad))
}

func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

// TestRecipeConfigWriteMissingManifest reports an infrastructure error with
// exit 2 when the values are non-empty but the manifest does not exist.
func TestRecipeConfigWriteMissingManifest(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.toml")
	code, out, stderrOut := runRecipeConfigWriteCLI(t, missing, "reconcile", `{"mode": "fast"}`)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (stdout %q)", code, out)
	}
	if strings.TrimSpace(stderrOut) == "" {
		t.Fatalf("expected a stderr diagnostic, got %q", stderrOut)
	}
}
