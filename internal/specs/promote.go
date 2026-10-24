package specs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
func activeDomainCollisions(root, slug string, domains map[string]bool) []string {
	if len(domains) == 0 {
		return nil
	}
	changes := filepath.Join(root, "openspec", "changes")
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

// planDomains composes every delta of one change folder without writing.
func planDomains(root, folder string, allowRemoved bool) ([]DomainPlan, []string) {
	deltas, blockers := deltaSpecs(folder)
	if len(deltas) == 0 {
		return nil, blockers
	}
	specsBase := filepath.Join(root, "openspec", "specs")
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
	planning, err := planningRoot(root)
	if err != nil {
		return PromotionReport{Root: root, Slug: slug, Blockers: []string{err.Error()}}
	}
	if err := validateSlug(slug); err != nil {
		return PromotionReport{Root: root, Slug: slug, Blockers: []string{err.Error()}}
	}

	folder := filepath.Join(planning, "openspec", "changes", slug)
	report := PromotionReport{Root: planning, Slug: slug, Folder: folder}
	if isSymlink(folder) || !isDir(folder) {
		report.Blockers = append(report.Blockers,
			fmt.Sprintf("active change folder missing at openspec/changes/%s/ — the promoter runs "+
				"before archive-tail while the change folder is still active", slug))
		return report
	}

	plans, blockers := planDomains(planning, folder, allowRemoved)
	report.Plans = plans
	domains := make(map[string]bool, len(plans))
	for _, plan := range plans {
		domains[plan.Domain] = true
	}
	report.Blockers = append(activeDomainCollisions(planning, slug, domains), blockers...)
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

// CheckFolderParity is the read-only half the Plan Build guardian uses: blockers
// when a delta is unpromoted or cannot be composed. A change with no specs/
// deltas is a no-op.
func CheckFolderParity(root, folder string) []string {
	plans, blockers := planDomains(root, folder, false)
	for _, plan := range plans {
		if len(plan.Composition.Pending) == 0 {
			continue
		}
		blockers = append(blockers, fmt.Sprintf(
			"specs/%s/spec.md is not promoted into openspec/specs/%s/spec.md (%s); run the "+
				"canonical spec promoter (lib/_internal/spec_promotion.py) before archive",
			plan.Domain, plan.Domain, strings.Join(plan.Composition.Pending, ", ")))
	}
	return blockers
}
