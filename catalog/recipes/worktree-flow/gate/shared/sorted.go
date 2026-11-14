package shared

import "sort"

// SortedUnique returns values as a sorted, deduplicated copy. It is the shape
// both conflict graders emit, so a set-like recipe group has one stable order.
//
// Moved verbatim from the gate main package (bindings.go) as part of slice
// SX0a: the orphans planner shares this helper with the conflict graders,
// bindings and resolved-config. Single authority; the main-package copy was
// deleted with this move.
func SortedUnique(values []string) []string {
	unique := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if unique[value] {
			continue
		}
		unique[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
