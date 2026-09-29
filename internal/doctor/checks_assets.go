package doctor

import (
	"fmt"
	"path/filepath"
	"strings"
)

// checkBundledAssets is doctor._check_bundled_assets: CLI-bundled skills and
// commands resolve from the cache ({cache}/.bundled/...), never the project
// surface, because sync/refresh-bundled flatten them there. An empty
// hand-authored ai-specs/skills/ or ai-specs/commands/ is healthy on its own
// and is not checked here.
func (d *Doctor) checkBundledAssets() {
	skillsRoot := filepath.Join(d.bundledSkillsRoot(), "skills")
	for _, skill := range bundledSkillNames() {
		if isDir(filepath.Join(skillsRoot, skill)) {
			d.add(OK, "bundled-skill", "cache .bundled/skills/"+skill+" present")
			continue
		}
		d.add(ERROR, "bundled-skill", "cache .bundled/skills/"+skill+" missing",
			"ai-specs sync (flattens CLI-bundled skills into the cache)")
	}
	commandsRoot := d.bundledCommandsRoot()
	for _, command := range bundledCommandNames() {
		if isFile(filepath.Join(commandsRoot, command+".md")) {
			d.add(OK, "bundled-commands", "cache .bundled/commands/"+command+".md present")
			continue
		}
		d.add(ERROR, "bundled-commands", "cache .bundled/commands/"+command+".md missing",
			"ai-specs sync (flattens CLI-bundled commands into the cache)")
	}
}

// checkTrackedBundledLeftovers is doctor._check_tracked_bundled_leftovers: WARN
// when git still tracks CLI-bundled assets removed from disk. It never runs
// `git rm`; it only guides the developer.
func (d *Doctor) checkTrackedBundledLeftovers() {
	if skillIDs := trackedBundledLeftovers(d.Root, bundledSkillIDs(), "ai-specs/skills/{name}"); len(skillIDs) > 0 {
		paths := make([]string, 0, len(skillIDs))
		for _, id := range skillIDs {
			paths = append(paths, "ai-specs/skills/"+id)
		}
		d.add(WARN, "tracked-bundled-leftover",
			fmt.Sprintf("%d removed CLI-bundled skill(s) still tracked in git", len(skillIDs)),
			"git rm -r --cached "+strings.Join(paths, " ")+
				"  # then commit; ai-specs never modifies the index")
	}
	if commandIDs := trackedBundledLeftovers(d.Root, bundledCommandNames(), "ai-specs/commands/{name}.md"); len(commandIDs) > 0 {
		paths := make([]string, 0, len(commandIDs))
		for _, id := range commandIDs {
			paths = append(paths, "ai-specs/commands/"+id+".md")
		}
		d.add(WARN, "tracked-bundled-leftover",
			fmt.Sprintf("%d removed CLI-bundled command(s) still tracked in git", len(commandIDs)),
			"git rm --cached "+strings.Join(paths, " ")+
				"  # then commit; ai-specs never modifies the index")
	}
}
