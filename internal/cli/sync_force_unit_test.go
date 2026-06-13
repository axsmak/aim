package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/gitops"
	"github.com/axsmak/aim/internal/localconfig"
)

// forceTrackingGitOps is a gitops.Ops fake that records the paths passed to
// CleanUntracked and returns a pre-set list from ListUntrackedInPaths.
// ADR-0003 5.3 (Н-5): verifies that the --force report and CleanUntracked
// receive exactly the same paths — no silent divergence.
type forceTrackingGitOps struct {
	fakeGitOps
	listUntrackedPaths []string // set by caller: files to report as untracked
	cleanCalledWith    []string // captured paths passed to CleanUntracked
	cleanPathsSnap     []string // captured paths passed to ListUntrackedInPaths (for report)
}

func (f *forceTrackingGitOps) HasUntrackedInPaths(dir string, paths []string) (bool, error) {
	return len(f.listUntrackedPaths) > 0, nil
}
func (f *forceTrackingGitOps) ListUntrackedInPaths(dir string, paths []string) ([]string, error) {
	f.cleanPathsSnap = append([]string(nil), paths...)
	return append([]string(nil), f.listUntrackedPaths...), nil
}
func (f *forceTrackingGitOps) CleanUntracked(dir string, paths []string) error {
	f.cleanCalledWith = append([]string(nil), paths...)
	return nil
}
func (f *forceTrackingGitOps) Fetch(dir string) error { return nil }
func (f *forceTrackingGitOps) LsRemote(dir, ref string) (string, error) {
	return "aaaa1234aaaa1234aaaa1234aaaa1234aaaa1234", nil
}
func (f *forceTrackingGitOps) HeadHash(dir string) (string, error) {
	return "bbbb1234bbbb1234bbbb1234bbbb1234bbbb1234", nil
}
func (f *forceTrackingGitOps) RemoteHash(dir, ref string) (string, error) {
	return "bbbb1234bbbb1234bbbb1234bbbb1234bbbb1234", nil
}
func (f *forceTrackingGitOps) IsAncestor(dir, ancestor, descendant string) (bool, error) {
	return true, nil
}
func (f *forceTrackingGitOps) DiffSyncDelta(dir string) ([]string, error) { return nil, nil }

var _ gitops.Ops = (*forceTrackingGitOps)(nil)

// TestForceReportMatchesCleanPaths is the Н-5 unit test.
// It verifies that the paths listed in the --force report are EXACTLY the same
// as the paths passed to CleanUntracked — byte-for-byte, no divergence.
func TestForceReportMatchesCleanPaths(t *testing.T) {
	untrackedFiles := []string{"skills/draft.md", "mcp/local-only.yaml"}

	fake := &forceTrackingGitOps{
		listUntrackedPaths: untrackedFiles,
	}

	// Populate a minimal homeDir so countEnvs returns 0 (no env detection needed).
	homeDir := t.TempDir()
	workDir := t.TempDir()

	// Write a minimal aim.local.yaml so localconfig.Load doesn't fail.
	if err := localconfig.Save(workDir, localconfig.Config{}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	// Create skills/ so installSkills doesn't fail trying to read a missing dir.
	if err := os.MkdirAll(workDir+"/skills", 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}

	var outBuf bytes.Buffer
	var errBuf bytes.Buffer

	// Run git sync with --force and --dry-run=false.
	// workDir has no .git so runGitSync cannot be called via runSync —
	// we call runGitSync directly to test the git path.
	_ = runGitSync(
		false, // dryRun
		true,  // force
		homeDir,
		"mcp/",
		workDir,
		fake,
		strings.NewReader(""),
		&outBuf,
		&errBuf,
	)

	// Н-5 assertion 1: ListUntrackedInPaths and CleanUntracked received the SAME paths.
	if len(fake.cleanPathsSnap) != len(fake.cleanCalledWith) {
		t.Fatalf("path mismatch: ListUntrackedInPaths got %v, CleanUntracked got %v",
			fake.cleanPathsSnap, fake.cleanCalledWith)
	}
	for i := range fake.cleanPathsSnap {
		if fake.cleanPathsSnap[i] != fake.cleanCalledWith[i] {
			t.Errorf("path[%d]: ListUntrackedInPaths=%q, CleanUntracked=%q",
				i, fake.cleanPathsSnap[i], fake.cleanCalledWith[i])
		}
	}

	stdout := outBuf.String()

	// Н-5 assertion 2: every file from ListUntrackedInPaths appears in the report.
	for _, f := range untrackedFiles {
		if !strings.Contains(stdout, f) {
			t.Errorf("--force report missing %q; stdout:\n%s", f, stdout)
		}
	}

	// Н-5 assertion 3: the report header is present.
	if !strings.Contains(stdout, "discarded untracked files (--force):") {
		t.Errorf("expected 'discarded untracked files (--force):' in stdout; got:\n%s", stdout)
	}
}
