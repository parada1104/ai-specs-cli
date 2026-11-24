package gates

// The commit recorded per task (plan-build-openspec-odd-align P3): the plan's
// Commit cells are the evidence per task, so every value that is not a
// "pendiente" marker must be a commit that landed on the branch under review.
//
// Two declared limits, repeated in SKILL §7.3: the check holds only on the
// review branch, because a squash-merge rewrites the shas and the guardian never
// runs on the base branch; and it grades the recorded sha, never whether the
// slice was a reasonable size.

import (
	"fmt"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/planbuild"
)

// pendingCommits are the Commit cells that record "not committed yet".
var pendingCommits = map[string]bool{
	"": true, "pendiente": true, "pending": true, "—": true, "-": true,
}

// checkTaskCommits grades the Commit cell of every task row. A tasks.md that does
// not parse as a plan with task rows is not the plan source in use, so it is
// graded elsewhere and never here.
func checkTaskCommits(folder, repoRoot, location string) []string {
	plan, _ := ParsePlan(readText(filepath.Join(folder, "tasks.md")))
	if len(plan.Tasks) == 0 {
		return nil
	}
	var blockers []string
	for _, task := range plan.Tasks {
		value := strings.Trim(strings.TrimSpace(task.Commit), "`")
		if pendingCommits[strings.ToLower(value)] {
			continue
		}
		if planbuild.CommitIsAncestor(repoRoot, value) {
			continue
		}
		blockers = append(blockers, fmt.Sprintf(
			"%s/tasks.md: task %s cites commit %s which is not an ancestor of HEAD on this branch: "+
				"commit the task or write \"pendiente\"", location, task.ID, value))
	}
	return blockers
}
