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
// them; S1 covers the seven diff_* inputs, S2 the nine write_* inputs, S3
// the repo manifest plus the two target-resolve fixtures, S4 the four
// large catalog recipes and S5 the eight small catalog recipes. S6 retired the
// live Python differential (TestDifferentialReadSections); the bundles hold all
// 31 inputs and TestFrozenDiffReadSectionsComplete requires 31 × 6.
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

// tomlReadFrozenS3 lists the inputs of slice S3: the repo manifest and the two
// target-resolve fixtures (18 byte-compared pairs, no refusal).
var tomlReadFrozenS3 = []string{
	"ai-specs_ai-specs.toml",
	"tests_fixtures_target-resolve_multi-target_ai-specs_ai-specs.toml",
	"tests_fixtures_target-resolve_root-only_ai-specs_ai-specs.toml",
}

// tomlReadFrozenS4 lists the inputs of slice S4: the four large catalog recipes
// (24 byte-compared pairs, no refusal).
var tomlReadFrozenS4 = []string{
	"catalog_recipes_plan-build-flow_recipe.toml",
	"catalog_recipes_trello-mcp-workflow_recipe.toml",
	"catalog_recipes_vault-canonical-store_recipe.toml",
	"catalog_recipes_worktree-flow_recipe.toml",
}

// tomlReadFrozenS5 lists the inputs of slice S5: the eight small catalog recipes
// (48 byte-compared pairs, no refusal).
var tomlReadFrozenS5 = []string{
	"catalog_recipes_bitbucket-pr-flow_recipe.toml",
	"catalog_recipes_git-pr-flow_recipe.toml",
	"catalog_recipes_gitlab-mr-flow_recipe.toml",
	"catalog_recipes_jinna-mcp-recipe_recipe.toml",
	"catalog_recipes_playwright-mcp_recipe.toml",
	"catalog_recipes_playwright-ui-flow_recipe.toml",
	"catalog_recipes_session-context_recipe.toml",
	"catalog_recipes_tdd-flow_recipe.toml",
}

// tomlReadFrozenTotal is the input count the bundles must hold after every slice so far.
var tomlReadFrozenTotal = len(tomlReadFrozenS1) + len(tomlReadFrozenS2) + len(tomlReadFrozenS3) + len(tomlReadFrozenS4) + len(tomlReadFrozenS5)

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

// TestFrozenDiffReadSectionsS3 covers the repo manifest and the two
// target-resolve fixtures: 18 byte-compared pairs and no refusal.
func TestFrozenDiffReadSectionsS3(t *testing.T) {
	compared, refused := tomlReadFrozenCheck(t, tomlReadFrozenS3)
	t.Logf("frozen differential S3: compared %d pairs; %d refused", compared, refused)
	if compared != 3*len(tomlReadFrozenSections) || refused != 0 {
		t.Fatalf("compared %d and refused %d pairs; S3 requires 18 compared and 0 refused",
			compared, refused)
	}
}

// TestFrozenDiffReadSectionsS5 covers the eight small catalog recipes:
// 48 byte-compared pairs and no refusal.
func TestFrozenDiffReadSectionsS5(t *testing.T) {
	compared, refused := tomlReadFrozenCheck(t, tomlReadFrozenS5)
	t.Logf("frozen differential S5: compared %d pairs; %d refused", compared, refused)
	if compared != 8*len(tomlReadFrozenSections) || refused != 0 {
		t.Fatalf("compared %d and refused %d pairs; S5 requires 48 compared and 0 refused",
			compared, refused)
	}
}

// TestFrozenDiffReadSectionsS4 covers the four large catalog recipes:
// 24 byte-compared pairs and no refusal.
func TestFrozenDiffReadSectionsS4(t *testing.T) {
	compared, refused := tomlReadFrozenCheck(t, tomlReadFrozenS4)
	t.Logf("frozen differential S4: compared %d pairs; %d refused", compared, refused)
	if compared != 4*len(tomlReadFrozenSections) || refused != 0 {
		t.Fatalf("compared %d and refused %d pairs; S4 requires 24 compared and 0 refused",
			compared, refused)
	}
}

// TestFrozenDiffReadSectionsComplete requires the frozen bundles to cover all 31
// inputs × 6 sections without the live Python differential: the slice lists
// partition the corpus exactly (no duplicate, no missing, no extra input), the
// golden holds exactly one record per input and section, and replaying every
// input gives 174 byte-compared pairs plus 12 both-fail invalid_base pairs (186).
func TestFrozenDiffReadSectionsComplete(t *testing.T) {
	corpus := tomlReadFrozenBundle(t, tomlReadFrozenCorpusPath)
	golden := tomlReadFrozenBundle(t, tomlReadFrozenGoldenPath)

	slices := [][]string{tomlReadFrozenS1, tomlReadFrozenS2, tomlReadFrozenS3, tomlReadFrozenS4, tomlReadFrozenS5}
	seen := map[string]int{}
	for _, s := range slices {
		for _, name := range s {
			seen[name]++
		}
	}
	for name, n := range seen {
		if n != 1 {
			t.Errorf("input %s listed in %d slices; each input belongs to exactly one", name, n)
		}
		if _, ok := corpus[name]; !ok {
			t.Errorf("slice input %s is absent from the frozen corpus", name)
		}
	}
	for name := range corpus {
		if seen[name] == 0 {
			t.Errorf("frozen corpus input %s is not covered by any slice", name)
		}
	}
	if len(corpus) != 31 || len(seen) != 31 {
		t.Fatalf("corpus holds %d inputs and slices cover %d; completeness requires 31", len(corpus), len(seen))
	}
	if len(golden) != 31*len(tomlReadFrozenSections) {
		t.Fatalf("golden holds %d records; completeness requires 31 × 6 = 186", len(golden))
	}
	for name := range corpus {
		for _, section := range tomlReadFrozenSections {
			if _, ok := golden[name+"."+section]; !ok {
				t.Errorf("golden has no record for %s [%s]", name, section)
			}
		}
	}

	all := make([]string, 0, len(seen))
	for _, s := range slices {
		all = append(all, s...)
	}
	compared, refused := tomlReadFrozenCheck(t, all)
	t.Logf("frozen completeness: compared %d pairs; %d both-fail invalid_base pairs", compared, refused)
	if compared != 174 || refused != 12 {
		t.Fatalf("compared %d and refused %d pairs; completeness requires 174 compared and 12 refused", compared, refused)
	}
}
