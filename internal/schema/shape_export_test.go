package schema

// Focused test for the additive, order-preserving ConfigTable.ShapeJSON
// accessor the native merge-config adapter needs (GO-07 S13 PR2b).
//
// The merge-config request envelope carries each structured config section's
// declarative shape as JSON. The shape is stored as an unexported *shapeTable,
// so encoding/json.Marshal yields {} and the recipe_schema check order (which
// the shared planner reproduces) depends on declaration order. ShapeJSON is the
// minimum surface that solves both at once.

import (
	"encoding/json"
	"strings"
	"testing"
)

// reconcileShapeJSON is exactly what Python's json.dumps renders for
// recipe_schema.STRUCTURED_CONFIG_SHAPES["reconcile"]: keys in declaration
// order, nested expectation keys in declaration order.
const reconcileShapeJSON = `{"scope_field":"string","max_age_seconds":"integer","expectations":[{"event":"string","property":"string","config_field":"string","config_field_when_set":"string"}]}`

func TestConfigTableShapeJSONPreservesDeclarationOrder(t *testing.T) {
	table := &ConfigTable{Shape: StructuredConfigShapes["reconcile"]}
	got, err := table.ShapeJSON()
	if err != nil {
		t.Fatalf("ShapeJSON: %v", err)
	}
	if string(got) != reconcileShapeJSON {
		t.Fatalf("ShapeJSON =\n %s\nwant\n %s", got, reconcileShapeJSON)
	}
	// The shape is a valid JSON object, not the {} encoding/json.Marshal emits
	// for the unexported shape table.
	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("ShapeJSON is not valid JSON: %v", err)
	}
	if len(decoded) != 3 {
		t.Fatalf("ShapeJSON lost keys: %v", decoded)
	}
}

func TestConfigTableShapeJSONNestedOrderIsStable(t *testing.T) {
	table := &ConfigTable{Shape: StructuredConfigShapes["reconcile"]}
	got, err := table.ShapeJSON()
	if err != nil {
		t.Fatalf("ShapeJSON: %v", err)
	}
	// Every nested key must follow its declared predecessor.
	order := []string{`"scope_field"`, `"max_age_seconds"`, `"expectations"`, `"event"`, `"property"`, `"config_field"`, `"config_field_when_set"`}
	prev := -1
	for _, token := range order {
		idx := strings.Index(string(got), token)
		if idx < 0 {
			t.Fatalf("ShapeJSON missing %s: %s", token, got)
		}
		if idx < prev {
			t.Fatalf("ShapeJSON key %s out of declaration order: %s", token, got)
		}
		prev = idx
	}
}

func TestConfigTableShapeJSONRejectsUnknownShape(t *testing.T) {
	table := &ConfigTable{Shape: 42}
	if _, err := table.ShapeJSON(); err == nil {
		t.Fatal("ShapeJSON accepted an invalid shape declaration")
	}
}
