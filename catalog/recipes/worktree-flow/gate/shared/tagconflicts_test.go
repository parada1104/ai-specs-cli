package shared

import (
	"encoding/json"
	"reflect"
	"testing"
)

// tagConflictOf builds one expected graded tag conflict matching the JSON
// contract emitted on stdout.
func tagConflictOf(tag, severity string, recipes ...string) TagConflict {
	return TagConflict{Type: "tag_conflict", Tag: tag, Recipes: recipes, Severity: severity}
}

// TestCheckTagConflicts mirrors recipe-conflicts.check_tag_conflicts: group the
// enabled recipes by tag in first-seen order, skip any tag held by fewer than two
// recipes or fewer than two distinct ids, and grade the overlap fatal only when a
// sharing recipe lists another sharing recipe in conflicts_with.
func TestCheckTagConflicts(t *testing.T) {
	cases := []struct {
		name    string
		recipes []RecipeTagMetadata
		want    []TagConflict
	}{
		{
			name:    "shared tag without conflicts_with warns",
			recipes: []RecipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}}, {ID: "beta", Tags: []string{"vcs"}}},
			want:    []TagConflict{tagConflictOf("vcs", "warning", "alpha", "beta")},
		},
		{
			name:    "one-sided conflicts_with is fatal",
			recipes: []RecipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}, ConflictsWith: []string{"beta"}}, {ID: "beta", Tags: []string{"vcs"}}},
			want:    []TagConflict{tagConflictOf("vcs", "fatal", "alpha", "beta")},
		},
		{
			name:    "conflicts_with is symmetric across the pair",
			recipes: []RecipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}}, {ID: "beta", Tags: []string{"vcs"}, ConflictsWith: []string{"alpha"}}},
			want:    []TagConflict{tagConflictOf("vcs", "fatal", "alpha", "beta")},
		},
		{
			name:    "conflicts_with naming a recipe outside the tag group stays a warning",
			recipes: []RecipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}, ConflictsWith: []string{"ghost"}}, {ID: "beta", Tags: []string{"vcs"}}},
			want:    []TagConflict{tagConflictOf("vcs", "warning", "alpha", "beta")},
		},
		{
			name:    "recipes sharing no tag produce no conflict",
			recipes: []RecipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}}, {ID: "beta", Tags: []string{"tracker"}}},
			want:    []TagConflict{},
		},
		{
			name:    "a single recipe holding a tag is not a conflict",
			recipes: []RecipeTagMetadata{{ID: "alpha", Tags: []string{"vcs"}}},
			want:    []TagConflict{},
		},
		{
			name:    "one recipe listing the same tag twice is not a conflict",
			recipes: []RecipeTagMetadata{{ID: "alpha", Tags: []string{"vcs", "vcs"}}},
			want:    []TagConflict{},
		},
		{
			name:    "three recipes sharing a tag report every id sorted",
			recipes: []RecipeTagMetadata{{ID: "gamma", Tags: []string{"infra"}}, {ID: "alpha", Tags: []string{"infra"}}, {ID: "beta", Tags: []string{"infra"}}},
			want:    []TagConflict{tagConflictOf("infra", "warning", "alpha", "beta", "gamma")},
		},
		{
			name:    "empty input is an empty conflict list",
			recipes: nil,
			want:    []TagConflict{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckTagConflicts(tc.recipes)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("CheckTagConflicts() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// TestCheckTagConflictsTagOrderIsFirstSeen checks that conflict output follows
// the first-seen tag order rather than Go map iteration order.
func TestCheckTagConflictsTagOrderIsFirstSeen(t *testing.T) {
	got := CheckTagConflicts([]RecipeTagMetadata{
		{ID: "alpha", Tags: []string{"z-tag", "a-tag"}},
		{ID: "beta", Tags: []string{"z-tag", "a-tag"}},
	})
	wantTags := []string{"z-tag", "a-tag"}
	if len(got) != len(wantTags) {
		t.Fatalf("conflicts = %#v, want %d entries", got, len(wantTags))
	}
	for i, tag := range wantTags {
		if got[i].Tag != tag {
			t.Fatalf("conflict[%d].Tag = %q, want %q", i, got[i].Tag, tag)
		}
	}
}

// TestTagConflictPlanJSONShape pins the exact stdout contract the later Python
// bridge consumes: one JSON object with a conflicts list.
func TestTagConflictPlanJSONShape(t *testing.T) {
	plan := TagConflictPlan{Conflicts: CheckTagConflicts([]RecipeTagMetadata{
		{ID: "alpha", Tags: []string{"vcs"}},
		{ID: "beta", Tags: []string{"vcs"}},
	})}
	payload, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	want := `{"conflicts":[{"type":"tag_conflict","tag":"vcs","recipes":["alpha","beta"],"severity":"warning"}]}`
	if string(payload) != want {
		t.Fatalf("payload = %s, want %s", payload, want)
	}
}
