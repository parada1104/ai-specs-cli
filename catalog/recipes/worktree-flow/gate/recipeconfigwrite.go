package main

import (
	"encoding/json"
	"fmt"
	"io"

	"ai-specs.dev/worktree-gate/shared"
)

// Recipe-config TOML writer command surface.
//
// The line-surgery decision (update_recipe_config, ported from
// lib/_internal/recipe-config-write.py) lives in the shared package
// (ai-specs.dev/worktree-gate/shared.ApplyRecipeConfigWrite) so the gate binary
// and the root native materializer share one grader. GO-07 S14a promoted it
// out of package main; this file keeps only the envelope/CLI boundary.

// recipeConfigWriteRequest is the stdin envelope. Values are decoded with the
// ordered JSON decoder so dict key order matches the Python bridge's
// json.loads (insertion order, last value wins on duplicate keys).
type recipeConfigWriteRequest struct {
	ManifestPath string          `json:"manifest_path"`
	RecipeID     string          `json:"recipe_id"`
	Values       json.RawMessage `json:"values"`
}

// runWriteRecipeConfig is the --write-recipe-config command: one JSON envelope
// on stdin, one JSON envelope on stdout. Exit 0 on applied/no-op, exit 2 on a
// refusal (the exact reference string in the error envelope) or an
// infrastructure failure (diagnostic on stderr).
func runWriteRecipeConfig(stdin io.Reader, stdout, stderr io.Writer) int {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --write-recipe-config: read stdin: %v\n", err)
		return 2
	}
	var req recipeConfigWriteRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --write-recipe-config: invalid input JSON: %v\n", err)
		return 2
	}
	var values *shared.OrderedMap
	if len(req.Values) > 0 {
		decoded, err := decodeOrdered(req.Values)
		if err != nil {
			fmt.Fprintf(stderr, "worktree-gate: --write-recipe-config: invalid values: %v\n", err)
			return 2
		}
		if decoded != nil {
			m, ok := decoded.(*shared.OrderedMap)
			if !ok {
				fmt.Fprintf(stderr, "worktree-gate: --write-recipe-config: values must be a JSON object\n")
				return 2
			}
			values = m
		}
	}
	applied, refusal, err := shared.ApplyRecipeConfigWrite(req.ManifestPath, req.RecipeID, values)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --write-recipe-config: %v\n", err)
		return 2
	}
	if refusal != "" {
		fmt.Fprintln(stdout, `{"error": `+shared.PyJSONString(refusal)+`}`)
		return 2
	}
	appliedText := "false"
	if applied {
		appliedText = "true"
	}
	fmt.Fprintln(stdout, `{"applied": `+appliedText+`}`)
	return 0
}
