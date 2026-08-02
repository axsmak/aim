package gitops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ManagedPaths are the AIM-owned paths in the inventory repo checked for dirty state.
var ManagedPaths = []string{"aim.yaml", ".gitignore", "skills/", "mcp/", "loadouts/"}

// Ops defines git operations used by aiman commands.
type Ops interface {
	Clone(url, dir string) error
	Fetch(dir string) error
	ResetHard(dir, ref string) error
	IsFastForward(dir, ref string) (bool, error)
	HasLocalChanges(dir, sinceHash string) (bool, error)
	HeadHash(dir string) (string, error)
	RemoteHash(dir, ref string) (string, error)
	LsRemote(dir, ref string) (string, error)
	Commit(dir, msg string) error
	Push(dir string) error
	ResetSoft(dir string) error
	IsFileStaged(workDir, path string) (bool, error)
	// IsAncestor reports whether ancestor is reachable from descendant in the git graph.
	IsAncestor(dir, ancestor, descendant string) (bool, error)
	HasDirtyWorktree(dir string) (bool, error)
	HasUntrackedInPaths(dir string, paths []string) (bool, error)
	// UntrackedConflictsWithRef returns untracked files in paths that also exist in ref.
	// These files would be silently overwritten by git reset --hard.
	UntrackedConflictsWithRef(dir, ref string, paths []string) ([]string, error)
	CountAheadBehind(dir, base, ref string) (ahead, behind int, err error)
	// DiffNameStatus returns "status\tpath" lines for managed paths between ref and working tree.
	DiffNameStatus(dir, ref string, paths []string) ([]string, error)
	// ListUntrackedInPaths returns untracked file paths under the given paths.
	ListUntrackedInPaths(dir string, paths []string) ([]string, error)
	// DiffSyncDelta returns formatted "X path" lines for skills/ and mcp/ between HEAD and
	// origin/main. Must be called before ResetHard — the diff is gone after reset.
	// ADR-0003 5.1: one function shared by dry-run (4.1) and result reporting (5.1).
	DiffSyncDelta(workDir string) ([]string, error)
	// CleanUntracked removes untracked files in paths via git clean -fd.
	// ADR-0003 5.3 (Н-5): caller must pass the same paths slice used for reporting.
	CleanUntracked(workDir string, paths []string) error
}

// ExecOps implements Ops by calling git as an external command.
type ExecOps struct{}

// New returns a new ExecOps.
func New() *ExecOps { return &ExecOps{} }

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func (e *ExecOps) Clone(url, dir string) error {
	_, err := run("", "clone", url, dir)
	return err
}

func (e *ExecOps) Fetch(dir string) error {
	_, err := run(dir, "fetch", "origin")
	return err
}

func (e *ExecOps) ResetHard(dir, ref string) error {
	_, err := run(dir, "reset", "--hard", ref)
	return err
}

func (e *ExecOps) IsFastForward(dir, ref string) (bool, error) {
	cmd := exec.Command("git", "-C", dir, "merge-base", "--is-ancestor", "HEAD", ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("git merge-base --is-ancestor: %w\n%s", err, out)
	}
	return true, nil
}

func (e *ExecOps) HasLocalChanges(dir, sinceHash string) (bool, error) {
	cmd := exec.Command("git", "-C", dir, "diff", "--quiet", sinceHash, "--", "skills/")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("git diff --quiet: %w\n%s", err, out)
	}
	return false, nil
}

func (e *ExecOps) HeadHash(dir string) (string, error) {
	return run(dir, "rev-parse", "HEAD")
}

func (e *ExecOps) RemoteHash(dir, ref string) (string, error) {
	return run(dir, "rev-parse", ref)
}

func (e *ExecOps) LsRemote(dir, ref string) (string, error) {
	out, err := run(dir, "ls-remote", "origin", ref)
	if err != nil {
		return "", fmt.Errorf("remote unreachable: %w", err)
	}
	if out == "" {
		return "", nil // ref doesn't exist yet (empty repo or first push)
	}
	// output format: "<hash>\t<ref>"
	hash := strings.SplitN(out, "\t", 2)[0]
	if len(hash) < 40 {
		return "", fmt.Errorf("unexpected ls-remote output: %q", out)
	}
	return hash[:40], nil
}

func (e *ExecOps) Commit(dir, msg string) error {
	// Stage library-owned config files if they exist
	for _, f := range []string{"aim.yaml", ".gitignore"} {
		if _, statErr := os.Stat(filepath.Join(dir, f)); statErr == nil {
			if _, err := run(dir, "add", f); err != nil {
				return err
			}
		}
	}
	if _, err := run(dir, "add", "skills/"); err != nil {
		return err
	}
	// Stage mcp/ and loadouts/ only if the directory exists — a repository
	// created before loadouts (v0.8.0) has no loadouts/ at all.
	for _, d := range []string{"mcp", "loadouts"} {
		if info, statErr := os.Stat(filepath.Join(dir, d)); statErr == nil && info.IsDir() {
			if _, err := run(dir, "add", d+"/"); err != nil {
				return err
			}
		}
	}
	cmd := exec.Command("git", "commit", "-m", msg)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "nothing to commit") {
			return fmt.Errorf("nothing to commit")
		}
		return fmt.Errorf("git commit: %w\n%s", err, out)
	}
	return nil
}

func (e *ExecOps) Push(dir string) error {
	_, err := run(dir, "push", "origin", "main")
	return err
}

func (e *ExecOps) ResetSoft(dir string) error {
	_, err := run(dir, "reset", "--soft", "HEAD~1")
	return err
}

func (e *ExecOps) IsFileStaged(workDir, path string) (bool, error) {
	out, err := run(workDir, "status", "--porcelain", path)
	if err != nil {
		return false, err
	}
	if len(out) < 2 {
		return false, nil
	}
	return out[0] != ' ' && out[0] != '?', nil
}

func (e *ExecOps) IsAncestor(dir, ancestor, descendant string) (bool, error) {
	cmd := exec.Command("git", "-C", dir, "merge-base", "--is-ancestor", ancestor, descendant)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return false, nil // not an ancestor — valid non-error case
		}
		return false, fmt.Errorf("git merge-base --is-ancestor: %w\n%s", err, out)
	}
	return true, nil
}

func (e *ExecOps) HasDirtyWorktree(dir string) (bool, error) {
	for _, args := range [][]string{
		{"diff", "--quiet", "--"},
		{"diff", "--cached", "--quiet", "--"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
				return true, nil
			}
			return false, fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
		}
	}
	return false, nil
}

func (e *ExecOps) HasUntrackedInPaths(dir string, paths []string) (bool, error) {
	args := append([]string{"status", "--porcelain", "--"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git status --porcelain: %w\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "??") {
			return true, nil
		}
	}
	return false, nil
}

func (e *ExecOps) UntrackedConflictsWithRef(dir, ref string, paths []string) ([]string, error) {
	// --untracked-files=all expands directory entries to individual files.
	args := append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status: %w\n%s", err, out)
	}
	var untracked []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "??") {
			untracked = append(untracked, strings.TrimSpace(line[3:]))
		}
	}
	if len(untracked) == 0 {
		return nil, nil
	}
	var conflicts []string
	for _, f := range untracked {
		lsOut, lsErr := run(dir, "ls-tree", "--name-only", ref, "--", f)
		if lsErr == nil && strings.TrimSpace(lsOut) != "" {
			conflicts = append(conflicts, f)
		}
	}
	return conflicts, nil
}

func (e *ExecOps) DiffNameStatus(dir, ref string, paths []string) ([]string, error) {
	args := append([]string{"diff", "--name-status", ref, "--"}, paths...)
	out, err := run(dir, args...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func (e *ExecOps) ListUntrackedInPaths(dir string, paths []string) ([]string, error) {
	args := append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, paths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status --porcelain: %w\n%s", err, raw)
	}
	var result []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "??") {
			result = append(result, strings.TrimSpace(line[3:]))
		}
	}
	return result, nil
}

// ManagedStatus returns "<marker> <path>" lines for managed paths, in the same
// A/M/D form as DiffSyncDelta (ADR-0003 5.1). Returns nil, nil when the working
// tree is clean for those paths.
func ManagedStatus(workDir string) ([]string, error) {
	args := append([]string{"-C", workDir,
		"status", "--porcelain", "--untracked-files=all", "--"}, ManagedPaths...)
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}
	raw := strings.TrimRight(string(out), "\n")
	if raw == "" {
		return nil, nil
	}
	split := strings.Split(raw, "\n")
	lines := make([]string, 0, len(split))
	for _, line := range split {
		if formatted := formatPorcelainLine(line); formatted != "" {
			lines = append(lines, formatted)
		}
	}
	return lines, nil
}

// formatPorcelainLine converts one `git status --porcelain` record into the
// "<marker> <path>" form. Porcelain reports the index and worktree state in two
// columns; the delta block describes the net effect on the published tree, so a
// letter in either column decides the marker.
func formatPorcelainLine(line string) string {
	if len(line) < 4 {
		return ""
	}
	code, path := line[:2], strings.TrimSpace(line[3:])
	var marker string
	switch {
	case code == "??", strings.ContainsRune(code, 'A'):
		marker = "A"
	case strings.ContainsRune(code, 'D'):
		marker = "D"
	case strings.ContainsRune(code, 'R'):
		marker = "R"
	default:
		marker = "M"
	}
	return marker + " " + path
}

func (e *ExecOps) CountAheadBehind(dir, base, ref string) (ahead, behind int, err error) {
	aheadOut, err := run(dir, "rev-list", "--count", base+".."+ref)
	if err != nil {
		return 0, 0, err
	}
	behindOut, err := run(dir, "rev-list", "--count", ref+".."+base)
	if err != nil {
		return 0, 0, err
	}
	if _, err = fmt.Sscanf(aheadOut, "%d", &ahead); err != nil {
		return 0, 0, fmt.Errorf("parse ahead count %q: %w", aheadOut, err)
	}
	if _, err = fmt.Sscanf(behindOut, "%d", &behind); err != nil {
		return 0, 0, fmt.Errorf("parse behind count %q: %w", behindOut, err)
	}
	return ahead, behind, nil
}

// syncDeltaPaths are the paths used by both DiffSyncDelta and CleanUntracked.
// ADR-0003 5.3 (Н-5): single constant — report and clean always use identical paths.
var syncDeltaPaths = []string{"skills/", "mcp/", "loadouts/"}

// DiffSyncDelta returns formatted "X path" lines for skills/ and mcp/ between HEAD and
// origin/main. Must be called before ResetHard — the diff is gone after reset.
// ADR-0003 5.1: one function shared by dry-run (4.1) and result reporting (5.1).
func (e *ExecOps) DiffSyncDelta(workDir string) ([]string, error) {
	args := append([]string{"diff", "--name-status", "HEAD", "origin/main", "--"}, syncDeltaPaths...)
	out, err := run(workDir, args...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	raw := strings.Split(out, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// git outputs: "M\tskills/foo.md" — reformat to "M skills/foo.md"
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		status := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])
		// Normalise rename similarity score (e.g. "R100") to "R"
		if len(status) > 1 && (status[0] == 'R' || status[0] == 'C') {
			status = string(status[0])
		}
		lines = append(lines, status+" "+path)
	}
	return lines, nil
}

// CleanUntracked removes untracked files under paths via git clean -fd.
// ADR-0003 5.3 (Н-5): caller must pass the same paths slice used for reporting.
func (e *ExecOps) CleanUntracked(workDir string, paths []string) error {
	args := append([]string{"clean", "-fd", "--"}, paths...)
	_, err := run(workDir, args...)
	return err
}
