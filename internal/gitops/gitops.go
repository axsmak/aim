package gitops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ManagedPaths are the AIM-owned paths in the inventory repo checked for dirty state.
var ManagedPaths = []string{"aim.yaml", ".gitignore", "skills/", "mcp/"}

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
	CountAheadBehind(dir, base, ref string) (ahead, behind int, err error)
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
	// Stage mcp/ only if the directory exists
	if info, statErr := os.Stat(filepath.Join(dir, "mcp")); statErr == nil && info.IsDir() {
		if _, err := run(dir, "add", "mcp/"); err != nil {
			return err
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

// ManagedStatus returns git status --porcelain lines for managed paths.
// Returns nil, nil when the working tree is clean for those paths.
func ManagedStatus(workDir string) ([]string, error) {
	cmd := exec.Command("git", "-C", workDir,
		"status", "--porcelain", "--untracked-files=all",
		"--", "aim.yaml", ".gitignore", "skills/", "mcp/")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w", err)
	}
	raw := strings.TrimRight(string(out), "\n")
	if raw == "" {
		return nil, nil
	}
	return strings.Split(raw, "\n"), nil
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
