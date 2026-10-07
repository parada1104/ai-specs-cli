package shared

import (
	"encoding/json"
	"reflect"
	"testing"
)

// recipePrimitivesOf builds one acquired recipe declaration for the decision
// tests; ids and names differ deliberately so name-vs-id confusion shows up.
func recipePrimitivesOf(id, name string, skills, commands, mcp []string) RecipePrimitives {
	return RecipePrimitives{ID: id, Name: name, Skills: skills, Commands: commands, MCP: mcp}
}

// primitiveConflictOf builds one expected conflict matching the JSON contract
// emitted on stdout; recipes are stored as given (the grader sorts them).
func primitiveConflictOf(primitiveType, id string, recipes ...string) PrimitiveConflict {
	return PrimitiveConflict{Type: primitiveType, ID: id, Recipes: recipes, Severity: "fatal"}
}

// TestCheckPrimitiveConflicts mirrors recipe-conflicts.check_recipe_conflicts:
// claims per recipe in skill -> command -> mcp order, a shared registry keyed
// by primitive type and id, one fatal conflict per collision, and the remaining
// claims of a colliding recipe abandoned.
func TestCheckPrimitiveConflicts(t *testing.T) {
	cases := []struct {
		name    string
		recipes []RecipePrimitives
		want    []PrimitiveConflict
	}{
		{
			name: "no overlapping claims produce no conflict",
			recipes: []RecipePrimitives{
				recipePrimitivesOf("dir-a", "Alpha", []string{"skill-a"}, []string{"cmd-a"}, []string{"mcp-a"}),
				recipePrimitivesOf("dir-b", "Beta", []string{"skill-b"}, []string{"cmd-b"}, []string{"mcp-b"}),
			},
			want: []PrimitiveConflict{},
		},
		{
			name: "shared skill id conflicts with recipe names",
			recipes: []RecipePrimitives{
				recipePrimitivesOf("dir-a", "Alpha", []string{"shared"}, nil, nil),
				recipePrimitivesOf("dir-b", "Beta", []string{"shared"}, nil, nil),
			},
			want: []PrimitiveConflict{primitiveConflictOf("skill", "shared", "Alpha", "Beta")},
		},
		{
			name: "shared command id conflicts",
			recipes: []RecipePrimitives{
				recipePrimitivesOf("dir-a", "Alpha", nil, []string{"shared"}, nil),
				recipePrimitivesOf("dir-b", "Beta", nil, []string{"shared"}, nil),
			},
			want: []PrimitiveConflict{primitiveConflictOf("command", "shared", "Alpha", "Beta")},
		},
		{
			name: "shared mcp id conflicts",
			recipes: []RecipePrimitives{
				recipePrimitivesOf("dir-a", "Alpha", nil, nil, []string{"shared"}),
				recipePrimitivesOf("dir-b", "Beta", nil, nil, []string{"shared"}),
			},
			want: []PrimitiveConflict{primitiveConflictOf("mcp", "shared", "Alpha", "Beta")},
		},
		{
			name: "the same id across different primitive types is independent",
			recipes: []RecipePrimitives{
				recipePrimitivesOf("dir-a", "Alpha", []string{"shared"}, nil, nil),
				recipePrimitivesOf("dir-b", "Beta", nil, []string{"shared"}, nil),
			},
			want: []PrimitiveConflict{},
		},
		{
			name: "a third colliding recipe pairs against the first owner",
			recipes: []RecipePrimitives{
				recipePrimitivesOf("dir-a", "Alpha", []string{"shared"}, nil, nil),
				recipePrimitivesOf("dir-b", "Beta", []string{"shared"}, nil, nil),
				recipePrimitivesOf("dir-c", "Gamma", []string{"shared"}, nil, nil),
			},
			want: []PrimitiveConflict{
				primitiveConflictOf("skill", "shared", "Alpha", "Beta"),
				primitiveConflictOf("skill", "shared", "Alpha", "Gamma"),
			},
		},
		{
			name: "first collision aborts the colliding recipe's remaining claims",
			recipes: []RecipePrimitives{
				recipePrimitivesOf("dir-a", "Alpha", []string{"x", "y"}, nil, nil),
				recipePrimitivesOf("dir-b", "Beta", []string{"x", "y"}, nil, nil),
				recipePrimitivesOf("dir-c", "Gamma", []string{"y"}, nil, nil),
			},
			want: []PrimitiveConflict{
				primitiveConflictOf("skill", "x", "Alpha", "Beta"),
				primitiveConflictOf("skill", "y", "Alpha", "Gamma"),
			},
		},
		{
			name: "one recipe claiming the same id twice conflicts with itself",
			recipes: []RecipePrimitives{
				recipePrimitivesOf("dir-a", "Alpha", []string{"x", "x"}, nil, nil),
			},
			want: []PrimitiveConflict{primitiveConflictOf("skill", "x", "Alpha")},
		},
		{
			name: "skill claims are graded before command claims",
			recipes: []RecipePrimitives{
				recipePrimitivesOf("dir-a", "Alpha", []string{"s"}, []string{"c"}, nil),
				recipePrimitivesOf("dir-b", "Beta", []string{"s"}, []string{"c"}, nil),
			},
			want: []PrimitiveConflict{primitiveConflictOf("skill", "s", "Alpha", "Beta")},
		},
		{
			name: "command claims are graded before mcp claims",
			recipes: []RecipePrimitives{
				recipePrimitivesOf("dir-a", "Alpha", nil, []string{"c"}, []string{"m"}),
				recipePrimitivesOf("dir-b", "Beta", nil, []string{"c"}, []string{"m"}),
			},
			want: []PrimitiveConflict{primitiveConflictOf("command", "c", "Alpha", "Beta")},
		},
		{
			name: "recipes are sorted and deduplicated in the conflict",
			recipes: []RecipePrimitives{
				recipePrimitivesOf("dir-z", "Zulu", []string{"shared"}, nil, nil),
				recipePrimitivesOf("dir-a", "Alpha", []string{"shared"}, nil, nil),
			},
			want: []PrimitiveConflict{primitiveConflictOf("skill", "shared", "Alpha", "Zulu")},
		},
		{
			name:    "empty input is an empty conflict list",
			recipes: nil,
			want:    []PrimitiveConflict{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckPrimitiveConflicts(tc.recipes)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("CheckPrimitiveConflicts() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestCheckPrimitiveConflictsOrderFollowsRecipeOrder checks that conflicts are
// emitted in the given recipe order (the --recipe flag order), never Go map
// iteration order: the first recipe owns claims, and each later recipe's first
// collision is appended in turn.
func TestCheckPrimitiveConflictsOrderFollowsRecipeOrder(t *testing.T) {
	got := CheckPrimitiveConflicts([]RecipePrimitives{
		recipePrimitivesOf("dir-g", "Gamma", []string{"skill-x"}, []string{"cmd-y"}, nil),
		recipePrimitivesOf("dir-a", "Alpha", []string{"skill-x"}, nil, nil),
		recipePrimitivesOf("dir-b", "Beta", nil, []string{"cmd-y"}, nil),
	})
	want := []PrimitiveConflict{
		primitiveConflictOf("skill", "skill-x", "Alpha", "Gamma"),
		primitiveConflictOf("command", "cmd-y", "Beta", "Gamma"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("conflicts = %#v, want %#v", got, want)
	}
}

// TestPrimitiveConflictPlanJSONShape pins the exact stdout contract the later
// Python bridge consumes: one JSON object with a conflicts list whose recipes
// carry [recipe].name values, never TOML ids.
func TestPrimitiveConflictPlanJSONShape(t *testing.T) {
	plan := PrimitiveConflictPlan{Conflicts: CheckPrimitiveConflicts([]RecipePrimitives{
		recipePrimitivesOf("dir-a", "Alpha", []string{"shared"}, nil, nil),
		recipePrimitivesOf("dir-b", "Beta", []string{"shared"}, nil, nil),
	})}
	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	want := `{"conflicts":[{"type":"skill","id":"shared","recipes":["Alpha","Beta"],"severity":"fatal"}]}`
	if string(payload) != want {
		t.Fatalf("payload = %s, want %s", payload, want)
	}
}
