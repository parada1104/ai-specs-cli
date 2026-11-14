package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/projectcache"
)

// Full-payload oracle: the legacy skill-resolution.py collect_skills output
// over the fabricated four-tier tree was captured once into
// testdata/skill_resolution_golden.json. The test never execs python3 or reads
// lib/. The golden is a snapshot: regenerating it needs a human decision.

const skillResolutionGoldenPath = "testdata/skill_resolution_golden.json"

// Tokens replace the host-dependent absolute directories in a frozen payload
// and in a frozen input manifest.
const (
	rootToken  = "<ROOT>"
	homeToken  = "<HOME>"
	cacheToken = "<CACHE>"
)

type pathToken struct{ path, token string }

// frozenTokens lists the host-dependent directories of one case, longest-lived
// prefix first: the cache root lives under the CLI home. Both the raw and the
// resolved form are tokenized because Python and the port differ on symlinked
// prefixes (/var vs /private/var) and the resolved form contains the raw one.
func frozenTokens(root, home string) []pathToken {
	var tokens []pathToken
	add := func(path, token string) {
		if path != "" {
			tokens = append(tokens, pathToken{path, token})
		}
	}
	cache := projectcache.CacheRoot(root, home)
	add(projectcache.ResolvePath(cache), cacheToken)
	add(cache, cacheToken)
	add(projectcache.ResolvePath(root), rootToken)
	add(root, rootToken)
	add(projectcache.ResolvePath(home), homeToken)
	add(home, homeToken)
	return tokens
}

func applyTokens(s string, tokens []pathToken) string {
	for _, tk := range tokens {
		s = strings.ReplaceAll(s, tk.path, tk.token)
	}
	return s
}

// treeManifest describes the trees a case builds as "sha256  <tokenized path>"
// lines sorted by path, so a frozen golden pins exactly the inputs its oracle
// was captured from. Symlinks are pinned by target, not followed.
func treeManifest(t *testing.T, dirs ...string) []string {
	t.Helper()
	if len(dirs) != 2 {
		t.Fatalf("treeManifest wants root and home, got %d dirs", len(dirs))
	}
	tokens := frozenTokens(dirs[0], dirs[1])
	type entry struct{ path, line string }
	var entries []entry
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			tokenized := applyTokens(projectcache.ResolvePath(path), tokens)
			if d.Type()&os.ModeSymlink != 0 {
				target, err := os.Readlink(path)
				if err != nil {
					return err
				}
				entries = append(entries, entry{tokenized,
					"link " + applyTokens(projectcache.ResolvePath(target), tokens) + "  " + tokenized})
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			entries = append(entries, entry{tokenized, hex.EncodeToString(sum[:]) + "  " + tokenized})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	lines := make([]string, len(entries))
	for i, e := range entries {
		lines[i] = e.line
	}
	return lines
}

// expectedSkillResolutionGoldenCases is the independent inventory of the frozen
// bundle: deleting, duplicating or reordering a golden case fails before any
// comparison runs.
var expectedSkillResolutionGoldenCases = []string{
	"four-tiers-duplicates",
}

type frozenResolvedSkill struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

type frozenSkillResolutionCase struct {
	Name   string                         `json:"name"`
	Tree   []string                       `json:"tree"`
	Skills map[string]frozenResolvedSkill `json:"skills"`
}

type frozenSkillResolution struct {
	Oracle map[string]any              `json:"oracle"`
	Cases  []frozenSkillResolutionCase `json:"cases"`
}

type skillResolutionCase struct {
	name  string
	build func(t *testing.T, root, home string)
}

func skillResolutionCases() []skillResolutionCase {
	return []skillResolutionCase{
		{
			// The four tiers plus duplicates inside a tier; the winning tier,
			// the winning directory and the recipe/dep duplicate warnings are
			// the legacy payload.
			name: "four-tiers-duplicates",
			build: func(t *testing.T, root, home string) {
				buildSkillTreeAt(t, root, home)
			},
		},
	}
}

func loadSkillResolutionGolden(t *testing.T) frozenSkillResolution {
	t.Helper()
	raw, err := os.ReadFile(skillResolutionGoldenPath)
	if err != nil {
		t.Fatalf("read frozen oracle %s: %v", skillResolutionGoldenPath, err)
	}
	var golden frozenSkillResolution
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode frozen oracle %s: %v", skillResolutionGoldenPath, err)
	}
	return golden
}

// TestDifferentialCollectSkills compares the native resolver with the frozen
// legacy oracle: the winning source tier and directory of every skill id.
func TestDifferentialCollectSkills(t *testing.T) {
	golden := loadSkillResolutionGolden(t)
	corpus := skillResolutionCases()

	if len(corpus) != len(expectedSkillResolutionGoldenCases) {
		t.Fatalf("in-test corpus has %d cases, inventory %d", len(corpus), len(expectedSkillResolutionGoldenCases))
	}
	if len(golden.Cases) != len(expectedSkillResolutionGoldenCases) {
		t.Fatalf("frozen oracle has %d cases, inventory %d", len(golden.Cases), len(expectedSkillResolutionGoldenCases))
	}
	seen := map[string]bool{}
	for i, want := range expectedSkillResolutionGoldenCases {
		if seen[want] {
			t.Fatalf("inventory lists %q twice", want)
		}
		seen[want] = true
		if golden.Cases[i].Name != want || corpus[i].name != want {
			t.Fatalf("case %d: golden %q, corpus %q, inventory %q", i, golden.Cases[i].Name, corpus[i].name, want)
		}
	}

	for i, tc := range corpus {
		t.Run(tc.name, func(t *testing.T) {
			frozen := golden.Cases[i]
			root := t.TempDir()
			home := t.TempDir()
			tc.build(t, root, home)

			gotTree := treeManifest(t, root, home)
			if strings.Join(gotTree, "\n") != strings.Join(frozen.Tree, "\n") {
				t.Fatalf("%s: frozen inputs differ from the corpus tree:\n--- frozen ---\n%s\n--- corpus ---\n%s",
					tc.name, strings.Join(frozen.Tree, "\n"), strings.Join(gotTree, "\n"))
			}

			tokens := frozenTokens(root, home)
			got := CollectSkills(root, home)
			if len(got) != len(frozen.Skills) {
				t.Fatalf("skill count = %d, legacy %d\ngot    %#v\nlegacy %#v", len(got), len(frozen.Skills), got, frozen.Skills)
			}
			for id, want := range frozen.Skills {
				if got[id].Source != want.Source || applyTokens(got[id].Path, tokens) != want.Path {
					t.Errorf("%s:\n port   %+v\n legacy %+v", id, got[id], want)
				}
			}
		})
	}
}
