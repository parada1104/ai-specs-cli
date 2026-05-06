Apply and verify the SDD change "fortalecer-boardid-validation" in the ai-specs-cli project.

You are in the worktree at: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/card78-boardid-validation

The change has been fully designed and tasks are defined at:
openspec/changes/fortalecer-boardid-validation/tasks.md

Your job:
1. Read the full change artifacts: proposal.md, specs/*/spec.md, design.md, tasks.md
2. Run `openspec apply --change "fortalecer-boardid-validation"` to execute implementation
3. Implement ALL tasks in order: schema → validation logic → AGENTS.md renderer → recipe assets
4. After implementation, run `./tests/run.sh` to verify
5. Create a PR via `gh pr create --base development --title "feat: fortalecer validación de board_id en trello-mcp-workflow (#78)" --body "Implementa validación de formato, detección de shortLink, y documentación clara para board_id en trello-mcp-workflow."`

RULES:
- Work ONLY inside the worktree at /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/card78-boardid-validation/
- Do NOT push to development
- Do NOT merge
- Create the PR when implementation and tests pass
- Commit atomically per task group
