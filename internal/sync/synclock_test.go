//go:build unix

package sync

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSyncLockTimeoutAndRelease(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, spineManifest)
	root := spinePlan(t, project).Root

	previousWait := syncLockWait
	syncLockWait = 20 * time.Millisecond
	defer func() { syncLockWait = previousWait }()

	unlock, err := acquireSyncLock(root, home)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if rc := Run([]string{"sync", project}, home, strings.NewReader(""), &stdout, &stderr); rc != 1 {
		t.Fatalf("sync with held lock rc = %d, want 1\nstderr: %s", rc, stderr.String())
	}
	want := "ERROR: another ai-specs sync is still running for " + root + " (waited 20ms); try again.\n"
	if stderr.String() != want {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}

	unlock()
	reacquire, err := acquireSyncLock(root, home)
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	reacquire()
}

func TestSyncConcurrentRunsSerializeSharedCache(t *testing.T) {
	home, _ := spineEnv(t)
	project := spineProject(t, spineManifest)

	const runs = 4
	type result struct {
		rc     int
		stdout string
		stderr string
	}
	results := make([]result, runs)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var stdout, stderr bytes.Buffer
			results[i].rc = Run([]string{"sync", project}, home, strings.NewReader(""), &stdout, &stderr)
			results[i].stdout = stdout.String()
			results[i].stderr = stderr.String()
		}(i)
	}
	wg.Wait()

	for i, got := range results {
		if got.rc != 0 {
			t.Errorf("run %d rc = %d\nstdout:\n%s\nstderr:\n%s", i, got.rc, got.stdout, got.stderr)
		}
		if strings.Contains(got.stderr, "file exists") || strings.Contains(got.stderr, "directory not empty") {
			t.Errorf("run %d exposed shared-cache filesystem race: %s", i, got.stderr)
		}
	}
}
