package gates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPassesWhenArchivedWithCanonicalDatedName(t *testing.T) {
	root := newRepo(t)
	slug := "dated-change"
	archived := archiveWith(t, root, slug, "Depth: standard\n", archiveOpts{
		proposal: true, spec: true, report: repoString(standardReport),
		archiveName: "2026-08-16-" + slug,
	})
	result := CheckPremerge(root, slug, tierOpt("standard"))
	if !result.OK {
		t.Fatalf("result = %+v, want OK", result)
	}
	if result.ArchivePath != archived {
		t.Fatalf("archive path = %q, want %q", result.ArchivePath, archived)
	}
}

func TestUndatedArchiveRemainsLegacyCompatible(t *testing.T) {
	root := newRepo(t)
	archived := archiveWith(t, root, "legacy-change", "Depth: light\n", archiveOpts{proposal: true})
	result := CheckPremerge(root, "legacy-change", tierOpt("light"))
	if !result.OK || result.ArchivePath != archived {
		t.Fatalf("result = %+v, want OK at %q", result, archived)
	}
}

func TestMultipleDatedArchivesBlockAsAmbiguous(t *testing.T) {
	root := newRepo(t)
	slug := "ambiguous-change"
	for _, name := range []string{"2026-08-15-" + slug, "2026-08-16-" + slug} {
		archiveWith(t, root, slug, "Depth: light\n", archiveOpts{proposal: true, archiveName: name})
	}
	result := CheckPremerge(root, slug, tierOpt("light"))
	if result.OK {
		t.Fatal("two dated archives must block")
	}
	assertBlockerContains(t, result, "ambiguous", "2026-08-15-"+slug, "2026-08-16-"+slug)
}

func TestDatedAndUndatedArchivesBlockAsAmbiguous(t *testing.T) {
	root := newRepo(t)
	slug := "mixed-change"
	archiveWith(t, root, slug, "Depth: light\n", archiveOpts{proposal: true})
	archiveWith(t, root, slug, "Depth: light\n", archiveOpts{proposal: true, archiveName: "2026-08-16-" + slug})
	result := CheckPremerge(root, slug, tierOpt("light"))
	if result.OK {
		t.Fatal("dated plus undated must block")
	}
	assertBlockerContains(t, result, "ambiguous")
}

func TestDatedArchiveRequiresValidCalendarDate(t *testing.T) {
	root := newRepo(t)
	slug := "invalid-date-change"
	archiveWith(t, root, slug, "Depth: light\n", archiveOpts{
		proposal: true, archiveName: "2026-02-30-" + slug,
	})
	result := CheckPremerge(root, slug, tierOpt("light"))
	if result.OK {
		t.Fatal("an impossible date prefix must block")
	}
	assertBlockerContains(t, result, "archive", "invalid dated archive candidate", "valid iso calendar date")
}

func TestDatedNearMatchIsRejected(t *testing.T) {
	root := newRepo(t)
	slug := "near-match-change"
	candidate := archiveWith(t, root, slug, "Depth: light\n", archiveOpts{
		proposal: true, archiveName: "2026-08-16-" + slug + "-extra",
	})
	result := CheckPremerge(root, slug, tierOpt("light"))
	if result.OK {
		t.Fatal("a near-match name must block")
	}
	assertBlockerContains(t, result, "near-match", filepath.Base(candidate))
}

func TestBlocksWhenDatedArchiveIsSymlinkToExternalDir(t *testing.T) {
	root := newRepo(t)
	slug := "symlink-change"
	external := filepath.Join(root, "outside-planning-tree")
	mkdirAll(t, external)
	writeFile(t, filepath.Join(external, "tasks.md"), "Depth: light\n")
	writeFile(t, filepath.Join(external, "proposal.md"), "# proposal\n")
	if err := os.Symlink(external, filepath.Join(root, "openspec", "changes", "archive", "2026-08-16-"+slug)); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	result := CheckPremerge(root, slug, tierOpt("light"))
	if result.OK {
		t.Fatal("a dated archive symlink must block")
	}
	assertBlockerContains(t, result, "symlink", slug)
}

func TestBlocksWhenLegacyArchiveIsSymlinkToExternalDir(t *testing.T) {
	root := newRepo(t)
	slug := "legacy-symlink-change"
	external := filepath.Join(root, "outside-planning-tree")
	mkdirAll(t, external)
	writeFile(t, filepath.Join(external, "tasks.md"), "Depth: light\n")
	writeFile(t, filepath.Join(external, "proposal.md"), "# proposal\n")
	if err := os.Symlink(external, filepath.Join(root, "openspec", "changes", "archive", slug)); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	result := CheckPremerge(root, slug, tierOpt("light"))
	if result.OK {
		t.Fatal("a legacy archive symlink must block")
	}
	assertBlockerContains(t, result, "symlink")
}

func TestUnrelatedDatedSymlinkDoesNotPoisonRequestedSlug(t *testing.T) {
	root := newRepo(t)
	slug := "target-change"
	archived := archiveWith(t, root, slug, "Depth: light\n", archiveOpts{
		proposal: true, archiveName: "2026-08-16-" + slug,
	})
	external := filepath.Join(root, "outside-planning-tree")
	mkdirAll(t, external)
	writeFile(t, filepath.Join(external, "tasks.md"), "Depth: light\n")
	writeFile(t, filepath.Join(external, "proposal.md"), "# proposal\n")
	if err := os.Symlink(external, filepath.Join(root, "openspec", "changes", "archive", "2026-08-16-other-change")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	result := CheckPremerge(root, slug, tierOpt("light"))
	if !result.OK || result.ArchivePath != archived {
		t.Fatalf("result = %+v, want OK at %q", result, archived)
	}
}

func TestBlocksWhenArchiveRootIsASymlink(t *testing.T) {
	root := newRepo(t)
	outside := filepath.Join(root, "outside-archive")
	mkdirAll(t, filepath.Join(outside, "done"))
	writeFile(t, filepath.Join(outside, "done", "tasks.md"), "Depth: light\n")
	writeFile(t, filepath.Join(outside, "done", "proposal.md"), "# proposal\n")
	archiveRoot := filepath.Join(root, "openspec", "changes", "archive")
	if err := os.Remove(archiveRoot); err != nil {
		t.Fatalf("remove archive root: %v", err)
	}
	if err := os.Symlink(outside, archiveRoot); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	result := CheckPremerge(root, "done", tierOpt("light"))
	if result.OK {
		t.Fatal("a symlinked archive root must block")
	}
	assertBlockerContains(t, result, "archive root", "symlink")
}

func TestMissingArchiveNamesBothAcceptedForms(t *testing.T) {
	root := newRepo(t)
	result := CheckPremerge(root, "nowhere", tierOpt("light"))
	if result.OK {
		t.Fatal("expected a missing-archive blocker")
	}
	assertBlockerContains(t, result,
		"missing archive at openspec/changes/archive/nowhere/ or a valid dated archive named YYYY-MM-DD-nowhere/")
}
