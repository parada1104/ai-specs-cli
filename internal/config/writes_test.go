package config

// GO-20 U6b (T2): unit tests for AppendRecipeSegments — the ONE appender of
// the [recipes.<id>] manifest surgery (S6: beside RemoveRecipeSegments, no
// second writer in the CLI package). The port mirrors recipe-add.py's write
// step line for line: the literal append at EOF of the enabled block plus the
// [recipes.<id>.config] placeholder (sorted keys; required → `= ""  #
// REQUIRED`, a default → toml_value(default), otherwise the commented
// optional line — required WINS over a default), then the parity contract's
// (docs/go-migration-parity-contract.md §3) parse-validate-and-restore step:
// the appended text is written, validated, and on failure the ORIGINAL text
// is restored — including its universal-newline normalization, exactly what
// Path.read_text + write_text(original_text) leaves behind. The guard fires
// whenever the APPENDED text does not parse, even over an already-invalid
// base (recipe-add.py validates unconditionally; unlike the remove heredocs'
// valid→invalid-only guardValidToInvalid).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppendRecipeSegmentsRendersSectionBytes pins the appended bytes: the
// block itself, the newline injected when the manifest has none, the
// read_text universal-newline normalization, and the placeholder rendering
// with Python's sorted() key order.
func TestAppendRecipeSegmentsRendersSectionBytes(t *testing.T) {
	cases := []struct {
		name     string
		manifest string
		id       string
		fields   []RecipeConfigField
		want     string
	}{
		{
			name:     "plain append",
			manifest: "[project]\nname = \"x\"\n",
			id:       "alpha",
			want:     "[project]\nname = \"x\"\n\n[recipes.alpha]\nenabled = true\n",
		},
		{
			name:     "newline injected before the block",
			manifest: "[project]\nname = \"x\"",
			id:       "alpha",
			want:     "[project]\nname = \"x\"\n\n[recipes.alpha]\nenabled = true\n",
		},
		{
			name:     "CRLF manifest is LF-normalized like read_text",
			manifest: "[project]\r\nname = \"x\"\r\n",
			id:       "alpha",
			want:     "[project]\nname = \"x\"\n\n[recipes.alpha]\nenabled = true\n",
		},
		{
			name:     "placeholders sorted, required beating a default",
			manifest: "[project]\nname = \"x\"\n",
			id:       "cfg",
			// Deliberately unsorted: the renderer owns Python's sorted().
			fields: []RecipeConfigField{
				{Key: "tags", Default: []any{"alpha", "beta"}},
				{Key: "board_id", Required: true, Default: "ignored"},
				{Key: "epic_list"},
				{Key: "default_list", Default: "In Progress"},
				{Key: "auto_go", Default: true},
				{Key: "retries", Default: int64(3)},
			},
			want: "[project]\nname = \"x\"\n" +
				"\n[recipes.cfg]\nenabled = true\n" +
				"\n[recipes.cfg.config]\n" +
				"auto_go = true\n" +
				"board_id = \"\"  # REQUIRED\n" +
				"default_list = \"In Progress\"\n" +
				"# epic_list = \"\"  # optional\n" +
				"retries = 3\n" +
				"tags = [\"alpha\", \"beta\"]\n",
		},
		{
			name:     "no config block for an empty field list",
			manifest: "[project]\nname = \"x\"\n",
			id:       "alpha",
			fields:   []RecipeConfigField{},
			want:     "[project]\nname = \"x\"\n\n[recipes.alpha]\nenabled = true\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "ai-specs.toml")
			if err := os.WriteFile(path, []byte(tc.manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := AppendRecipeSegments(path, tc.id, tc.fields); err != nil {
				t.Fatalf("AppendRecipeSegments: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("manifest bytes diverge:\n got: %q\nwant: %q", got, tc.want)
			}
			checkNoTempFiles(t, dir)
		})
	}
}

// TestAppendRecipeSegmentsGuardRestoresOriginal pins the restore-on-invalid
// path: the guard fires whenever the appended text does not parse (an
// already-invalid base, or a valid base broken by an id that is not a legal
// bare key), the frozen message frames the detail, and the file ends up
// holding the ORIGINAL text — byte-identical for LF input, its LF-normalized
// form for CRLF input (recipe-add.py restores read_text's normalized copy).
func TestAppendRecipeSegmentsGuardRestoresOriginal(t *testing.T) {
	const brokenBase = "[project]\nname = \"x\"\n\n[recipes.old.config]\nflag = True\n"
	cases := []struct {
		name         string
		manifest     string
		id           string
		wantRestored string
	}{
		{"already-invalid base refuses the append", brokenBase, "alpha", brokenBase},
		{"valid base, invalid append from a space id", "[project]\nname = \"x\"\n", "bad id",
			"[project]\nname = \"x\"\n"},
		{"CRLF original restores its normalized bytes", "a = 1\r\nb = 2\r\n", "bad id",
			"a = 1\nb = 2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "ai-specs.toml")
			if err := os.WriteFile(path, []byte(tc.manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			err := AppendRecipeSegments(path, tc.id, nil)
			if err == nil {
				t.Fatal("want the guard to refuse the append")
			}
			prefix := "Error: adding '" + tc.id + "' would produce an invalid TOML manifest ("
			if !strings.HasPrefix(err.Error(), prefix) {
				t.Errorf("error prefix = %q, want %q", err.Error(), prefix)
			}
			if !strings.HasSuffix(err.Error(), "). ai-specs.toml was not modified.") {
				t.Errorf("error suffix = %q, want it to end with %q", err.Error(),
					"). ai-specs.toml was not modified.")
			}
			got, rerr := os.ReadFile(path)
			if rerr != nil {
				t.Fatal(rerr)
			}
			if string(got) != tc.wantRestored {
				t.Errorf("restore diverges:\n got: %q\nwant: %q", got, tc.wantRestored)
			}
			checkNoTempFiles(t, dir)
		})
	}
}
