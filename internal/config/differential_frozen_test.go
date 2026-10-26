package config

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Frozen oracle bundles for the toml-read differential. Each slice extends
// them; S1 covers the seven diff_* inputs and S2 the nine write_* inputs. The
// live Python differential (TestDifferentialReadSections) still compares all
// 31 inputs until S6 retires it.
const (
	tomlReadFrozenGoldenPath = "testdata/config_toml_read_golden.json"
	tomlReadFrozenCorpusPath = "testdata/config_toml_read_corpus.json"
)

// tomlReadFrozenS1 lists the inputs of slice S1 (42 byte-compared pairs).
var tomlReadFrozenS1 = []string{
	"internal_config_testdata_diff_bindings_deps.toml",
	"internal_config_testdata_diff_empty.toml",
	"internal_config_testdata_diff_mcp.toml",
	"internal_config_testdata_diff_missing.toml",
	"internal_config_testdata_diff_mixed.toml",
	"internal_config_testdata_diff_nonascii.toml",
	"internal_config_testdata_diff_recipes.toml",
}

// tomlReadFrozenS2 lists the inputs of slice S2: seven succeed (42 byte-compared
// pairs) and the two invalid_base inputs are refused by the oracle (12 both-fail pairs).
var tomlReadFrozenS2 = []string{
	"internal_config_testdata_write_dep_crlf.toml",
	"internal_config_testdata_write_dep_guard.toml",
	"internal_config_testdata_write_dep_invalid_base.toml",
	"internal_config_testdata_write_deps_two.toml",
	"internal_config_testdata_write_recipe_basic.toml",
	"internal_config_testdata_write_recipe_crlf.toml",
	"internal_config_testdata_write_recipe_crlf_guard.toml",
	"internal_config_testdata_write_recipe_guard.toml",
	"internal_config_testdata_write_recipe_invalid_base.toml",
}

// tomlReadFrozenTotal is the input count the bundles must hold after every slice so far.
var tomlReadFrozenTotal = len(tomlReadFrozenS1) + len(tomlReadFrozenS2)

var tomlReadFrozenSections = []string{"project", "agents", "deps", "mcp", "recipes", "bindings"}

// tomlReadFrozenRecord is one frozen oracle invocation: exit code, stdout, stderr.
type tomlReadFrozenRecord struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// tomlReadFrozenEntry is one frozen corpus input; base64 marks non-UTF-8 bytes.
type tomlReadFrozenEntry struct {
	Base64  bool   `json:"base64"`
	Content string `json:"content"`
}

func tomlReadFrozenBundle(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read frozen bundle %s: %v", path, err)
	}
	var records map[string]json.RawMessage
	if err := json.Unmarshal(raw, &records); err != nil {
		t.Fatalf("decode frozen bundle %s: %v", path, err)
	}
	return records
}

func tomlReadFrozenDecode(t *testing.T, bundle map[string]json.RawMessage, key string, v any) {
	t.Helper()
	raw, ok := bundle[key]
	if !ok {
		t.Fatalf("frozen bundle has no %s record", key)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("decode frozen record %s: %v", key, err)
	}
}

// tomlReadFrozenCheck replays the given frozen inputs through Go LoadManifest and
// ReadSectionJSON. It never runs Python and never reads the repository tree. A
// non-zero oracle record is accepted only for invalid_base inputs, where Go must
// refuse too (both-fail parity, as in the live differential). It returns the
// byte-compared pair count and the both-fail pair count.
func tomlReadFrozenCheck(t *testing.T, inputs []string) (compared, refused int) {
	t.Helper()
	corpus := tomlReadFrozenBundle(t, tomlReadFrozenCorpusPath)
	golden := tomlReadFrozenBundle(t, tomlReadFrozenGoldenPath)
	if len(corpus) != tomlReadFrozenTotal || len(golden) != tomlReadFrozenTotal*len(tomlReadFrozenSections) {
		t.Fatalf("frozen bundles hold %d inputs and %d records; the slices so far expect exactly %d and %d",
			len(corpus), len(golden), tomlReadFrozenTotal, tomlReadFrozenTotal*len(tomlReadFrozenSections))
	}

	dir := t.TempDir()
	for _, name := range inputs {
		var entry tomlReadFrozenEntry
		tomlReadFrozenDecode(t, corpus, name, &entry)
		data := []byte(entry.Content)
		if entry.Base64 {
			decoded, err := base64.StdEncoding.DecodeString(entry.Content)
			if err != nil {
				t.Fatalf("decode base64 corpus entry %s: %v", name, err)
			}
			data = decoded
		}
		manifestPath := filepath.Join(dir, name)
		if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
			t.Fatalf("materialize frozen corpus entry %s: %v", name, err)
		}
		for _, section := range tomlReadFrozenSections {
			var rec tomlReadFrozenRecord
			tomlReadFrozenDecode(t, golden, name+"."+section, &rec)
			if rec.ExitCode != 0 {
				if !strings.Contains(name, "invalid_base") || rec.Stdout != "" || rec.Stderr == "" {
					t.Fatalf("%s [%s]: frozen record exit %d stderr %q; only invalid_base inputs may be refused",
						name, section, rec.ExitCode, rec.Stderr)
				}
				if _, err := LoadManifest(manifestPath); err == nil {
					t.Errorf("%s [%s]: oracle refused the input but Go LoadManifest accepted it", name, section)
				}
				refused++
				continue
			}
			if rec.Stderr != "" {
				t.Fatalf("%s [%s]: frozen record exit 0 with stderr %q", name, section, rec.Stderr)
			}
			manifest, err := LoadManifest(manifestPath)
			if err != nil {
				t.Errorf("%s: Go LoadManifest failed where the oracle succeeded: %v", name, err)
				continue
			}
			got, err := ReadSectionJSON(manifest, section)
			if err != nil {
				t.Errorf("%s [%s]: Go ReadSectionJSON failed: %v", name, section, err)
				continue
			}
			want := strings.TrimSuffix(rec.Stdout, "\n")
			if got != want {
				t.Errorf("byte mismatch %s [%s]:\n  go:     %s\n  oracle: %s", name, section, got, want)
			}
			compared++
		}
	}
	return compared, refused
}

// TestFrozenDiffReadSectionsS1 compares Go LoadManifest + ReadSectionJSON with the
// frozen oracle records for the S1 inputs. Every S1 input must succeed silently.
func TestFrozenDiffReadSectionsS1(t *testing.T) {
	compared, refused := tomlReadFrozenCheck(t, tomlReadFrozenS1)
	t.Logf("frozen differential S1: compared %d (input, section) pairs", compared)
	if compared != len(tomlReadFrozenS1)*len(tomlReadFrozenSections) || refused != 0 {
		t.Fatalf("compared %d and refused %d pairs; S1 requires %d compared and 0 refused",
			compared, refused, len(tomlReadFrozenS1)*len(tomlReadFrozenSections))
	}
}

// TestFrozenDiffReadSectionsS2 covers the nine write_* inputs: 42 byte-compared
// pairs plus 12 both-fail pairs for the two invalid_base inputs (54 in total).
func TestFrozenDiffReadSectionsS2(t *testing.T) {
	compared, refused := tomlReadFrozenCheck(t, tomlReadFrozenS2)
	t.Logf("frozen differential S2: compared %d pairs; %d both-fail invalid_base pairs", compared, refused)
	if compared != 7*len(tomlReadFrozenSections) || refused != 2*len(tomlReadFrozenSections) {
		t.Fatalf("compared %d and refused %d pairs; S2 requires 42 compared and 12 refused",
			compared, refused)
	}
}
