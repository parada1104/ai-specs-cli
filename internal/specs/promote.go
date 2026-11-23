package specs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ai-specs.dev/ai-specs/internal/planbuild"
)

type domainSource struct {
	domain string
	source string
}

// deltaSpecs returns the (domain, delta path) pairs under a change folder and
// any ambiguity blockers.
func deltaSpecs(folder string) ([]domainSource, []string) {
	specs := filepath.Join(folder, "specs")
	if !isDir(specs) {
		return nil, nil
	}
	found := map[string]string{}
	var blockers []string
	// WalkDir visits each directory lexically, the same order as Python's
	// sorted(specs.rglob("spec.md")), and never follows directory symlinks.
	_ = filepath.WalkDir(specs, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Name() != "spec.md" {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			return nil // skips symlinks and non-regular files
		}
		domain := filepath.Base(filepath.Dir(path))
		if previous, ok := found[domain]; ok {
			blockers = append(blockers, fmt.Sprintf(
				"ambiguous delta domain '%s': %s and %s both declare a domain spec",
				domain, previous, path))
			return nil
		}
		found[domain] = path
		return nil
	})
	domains := make([]string, 0, len(found))
	for domain := range found {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	pairs := make([]domainSource, 0, len(domains))
	for _, domain := range domains {
		pairs = append(pairs, domainSource{domain: domain, source: found[domain]})
	}
	return pairs, blockers
}

// activeDomainCollisions names the other active changes that declare a delta for
// the same domains. Pure discovery: the caller applies the result before any
// write, so a collision aborts the whole promotion.
func activeDomainCollisions(changesBase, slug string, domains map[string]bool) []string {
	if len(domains) == 0 {
		return nil
	}
	changes := changesBase
	entries, err := os.ReadDir(changes)
	if err != nil {
		return nil // ReadDir sorts by name, like the Python sorted(iterdir())
	}
	var blockers []string
	for _, entry := range entries {
		name := entry.Name()
		if name == "archive" || name == slug || !entry.IsDir() {
			continue
		}
		shared, _ := deltaSpecs(filepath.Join(changes, name))
		for _, pair := range shared {
			if !domains[pair.domain] {
				continue
			}
			blockers = append(blockers, fmt.Sprintf(
				`active change "%s" also declares spec delta specs/%s/spec.md — the collision is `+
					"unresolved; choose an explicit sync/archive order or narrow one change before promoting",
				name, pair.domain))
		}
	}
	return blockers
}

// planDomains composes every delta of one change folder without writing. The
// canonical tree is the caller's, so the same composer serves both modes.
func planDomains(folder, specsBase string, allowRemoved bool) ([]DomainPlan, []string) {
	deltas, blockers := deltaSpecs(folder)
	if len(deltas) == 0 {
		return nil, blockers
	}
	var plans []DomainPlan
	for _, pair := range deltas {
		canonical, err := canonicalTarget(specsBase, pair.domain)
		if err != nil {
			blockers = append(blockers, err.Error())
			continue
		}
		var canonicalText *string
		if info, statErr := os.Stat(canonical); statErr == nil {
			if !info.Mode().IsRegular() {
				blockers = append(blockers, fmt.Sprintf(
					"canonical spec path %s exists and is not a regular file", canonical))
				continue
			}
			text, readErr := readMarkdown(canonical)
			if readErr != nil {
				blockers = append(blockers, fmt.Sprintf("canonical spec path %s could not be read: %v",
					canonical, readErr))
				continue
			}
			canonicalText = &text
		}
		deltaText, readErr := readMarkdown(pair.source)
		if readErr != nil {
			blockers = append(blockers, fmt.Sprintf("specs/%s/spec.md could not be read: %v",
				pair.domain, readErr))
			continue
		}
		composition := Compose(canonicalText, deltaText, pair.domain, allowRemoved)
		for _, blocker := range composition.Blockers {
			blockers = append(blockers, "specs/"+pair.domain+"/spec.md: "+blocker)
		}
		plans = append(plans, DomainPlan{
			Domain:      pair.domain,
			Source:      pair.source,
			Canonical:   canonical,
			Composition: composition,
		})
	}
	return plans, blockers
}

// PromoteChange composes the active change's deltas into canonical specs, or
// reports blockers. Nothing is written when any domain is blocked or when
// another active change declares a delta for the same domain, so a blocked
// promotion leaves the canonical tree byte-identical.
func PromoteChange(root, slug string, allowRemoved bool) PromotionReport {
	return PromoteChangeForMode(root, slug, planbuild.ModeOpenspec, allowRemoved)
}

// PromoteChangeForMode is PromoteChange with the roots of the resolved execution
// mode (S7): the composer, its blockers, its collision rule and its idempotence
// are mode-independent, and only the source and canonical trees move.
func PromoteChangeForMode(root, slug, mode string, allowRemoved bool) PromotionReport {
	planning, err := planningRootFor(root, mode)
	if err != nil {
		return PromotionReport{Root: root, Slug: slug, Blockers: []string{err.Error()}}
	}
	if err := validateSlug(slug); err != nil {
		return PromotionReport{Root: root, Slug: slug, Blockers: []string{err.Error()}}
	}

	changesBase := planbuild.ChangesBase(planning, mode)
	folder := filepath.Join(changesBase, slug)
	report := PromotionReport{Root: planning, Slug: slug, Folder: folder}
	if mode == planbuild.ModeOdd && !isDir(folder) && oddTaskOnly(planning, slug) {
		// A task-only ODD change carries no delta and no change folder, so
		// promotion has nothing to do and raises nothing.
		return report
	}
	if isSymlink(folder) || !isDir(folder) {
		report.Blockers = append(report.Blockers,
			fmt.Sprintf("active change folder missing at %s/%s/ — the promoter runs "+
				"before archive-tail while the change folder is still active",
				changesLabel(mode), slug))
		return report
	}
	if mode == planbuild.ModeOdd && !SourceContained(folder, changesBase) {
		// A symlinked odd/changes would feed the composer from the other tree.
		report.Blockers = append(report.Blockers, fmt.Sprintf(
			"refusing to read change folder %s: it resolves outside %s", folder, changesBase))
		return report
	}

	plans, blockers := planDomains(folder, planbuild.SpecsBase(planning, mode), allowRemoved)
	report.Plans = plans
	domains := make(map[string]bool, len(plans))
	for _, plan := range plans {
		domains[plan.Domain] = true
	}
	report.Blockers = append(activeDomainCollisions(changesBase, slug, domains), blockers...)
	if len(report.Blockers) > 0 {
		return report
	}
	for _, plan := range report.Plans {
		if !plan.Composition.Changed {
			continue
		}
		if writeErr := atomicWrite(plan.Canonical, plan.Composition.Content); writeErr != nil {
			report.Blockers = append(report.Blockers, fmt.Sprintf(
				"canonical spec path %s could not be written: %v", plan.Canonical, writeErr))
		}
	}
	return report
}

// canonicalLabel is the repository-relative canonical tree the mode writes.
func canonicalLabel(mode string) string {
	if mode == planbuild.ModeOdd {
		return "odd/specs"
	}
	return "openspec/specs"
}

// changesLabel is the repository-relative change tree the mode reads, used in
// messages so a blocker names the tree the reader actually has.
func changesLabel(mode string) string {
	if mode == planbuild.ModeOdd {
		return "odd/changes"
	}
	return "openspec/changes"
}

// modeStep is the tail of a parity blocker: odd has no archive step.
func modeStep(mode string) string {
	if mode == planbuild.ModeOdd {
		return "merge"
	}
	return "archive"
}

// SourceContained reports whether the change folder is reached through real
// directories: a symlinked odd/ or odd/changes/ would feed the gate or the
// composer from the other planning tree.
func SourceContained(folder, changesBase string) bool {
	for _, path := range []string{filepath.Dir(changesBase), changesBase, folder} {
		if isSymlink(path) {
			return false
		}
	}
	return true
}

// oddTaskOnly reports the paired ODD plan of a change that kept no change folder.
func oddTaskOnly(root, change string) bool {
	return planbuild.PlanIsValid(root, change)
}

// CheckFolderParity is the read-only half the Plan Build guardian uses: blockers
// when a delta is unpromoted or cannot be composed. A change with no specs/
// deltas is a no-op. The canonical root follows the resolved mode, so odd deltas
// are compared against odd/specs and never against the OpenSpec tree.
// CheckFolderParity is the OpenSpec guardian's entry point: it always compares
// against openspec/specs, so an explicit pre-archive keeps its own tree even in
// an odd-mode repository.
func CheckFolderParity(root, folder string) []string {
	return CheckFolderParityForMode(root, folder, planbuild.ModeOpenspec)
}

// CheckFolderParityForMode is the parity check against a mode the caller already
// resolved, so a caller that knows the mode (the pre-merge gate) reads one tree
// instead of resolving again.
func CheckFolderParityForMode(root, folder, mode string) []string {
	plans, blockers := planDomains(folder, planbuild.SpecsBase(root, mode), false)
	for _, plan := range plans {
		if len(plan.Composition.Pending) == 0 {
			continue
		}
		blockers = append(blockers, fmt.Sprintf(
			"specs/%s/spec.md is not promoted into %s/%s/spec.md (%s); run "+
				"`ai-specs specs promote <slug> --root <root>` before %s",
			plan.Domain, canonicalLabel(mode), plan.Domain,
			strings.Join(plan.Composition.Pending, ", "), modeStep(mode)))
	}
	return blockers
}
