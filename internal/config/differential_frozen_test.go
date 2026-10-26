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
// them; S1 covers only the seven diff_* inputs. The live Python differential
// (TestDifferentialReadSections) still compares all 31 inputs until S6 retires it.
const (
	tomlReadFrozenGoldenPath = "testdata/config_toml_read_golden.json"
	tomlReadFrozenCorpusPath = "testdata/config_toml_read_corpus.json"
)

// tomlReadFrozenS1 lists the inputs of this slice. The bundles must hold exactly these.
var tomlReadFrozenS1 = []string{
	"internal_config_testdata_diff_bindings_deps.toml",
	"internal_config_testdata_diff_empty.toml",
	"internal_config_testdata_diff_mcp.toml",
	"internal_config_testdata_diff_missing.toml",
	"internal_config_testdata_diff_mixed.toml",
	"internal_config_testdata_diff_nonascii.toml",
	"internal_config_testdata_diff_recipes.toml",
}

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

// TestFrozenDiffReadSectionsS1 compares Go LoadManifest + ReadSectionJSON with the
// frozen oracle records for the S1 inputs. It never runs Python and never reads the repository tree.
func TestFrozenDiffReadSectionsS1(t *testing.T) {
	corpus := tomlReadFrozenBundle(t, tomlReadFrozenCorpusPath)
	golden := tomlReadFrozenBundle(t, tomlReadFrozenGoldenPath)
	if len(corpus) != len(tomlReadFrozenS1) || len(golden) != len(tomlReadFrozenS1)*len(tomlReadFrozenSections) {
		t.Fatalf("frozen bundles hold %d inputs and %d records; S1 expects exactly %d and %d",
			len(corpus), len(golden), len(tomlReadFrozenS1), len(tomlReadFrozenS1)*len(tomlReadFrozenSections))
	}

	dir := t.TempDir()
	compared := 0
	for _, name := range tomlReadFrozenS1 {
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
			if rec.ExitCode != 0 || rec.Stderr != "" {
				t.Fatalf("%s [%s]: frozen record exit %d stderr %q; every S1 input must succeed silently",
					name, section, rec.ExitCode, rec.Stderr)
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
	t.Logf("frozen differential S1: compared %d (input, section) pairs", compared)
	if compared != len(tomlReadFrozenS1)*len(tomlReadFrozenSections) {
		t.Fatalf("compared %d pairs; S1 requires %d", compared, len(tomlReadFrozenS1)*len(tomlReadFrozenSections))
	}
}
