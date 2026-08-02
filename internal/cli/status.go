package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/axsmak/aim/internal/errs"
	"github.com/axsmak/aim/internal/gitops"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/spf13/cobra"
)

const fetchTimeout = 5 * time.Second

var statusManagedPaths = []string{"skills/", "mcp/", "loadouts/"}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show repository sync status",
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				errs.Fatalf("cannot determine home directory: %v", err)
			}
			return runStatus(resolveWorkDir(homeDir), gitops.New(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func runStatus(workDir string, git gitops.Ops, out, errOut io.Writer) error {
	cfg, err := localconfig.Load(workDir)
	if err != nil {
		errs.Fatal(err.Error())
	}

	if cfg.Repo == "" {
		fmt.Fprintln(out, "Repository not initialized. Run: aiman init <url>")
		return nil
	}

	headHash, _ := git.HeadHash(workDir)

	// Fetch from remote with a hard timeout so the interactive command never hangs.
	fetchErr := fetchWithTimeout(git, workDir, fetchTimeout)
	remoteUnreachable := fetchErr != nil

	posText, ahead, behind, unreachable := statusPosition(workDir, git, remoteUnreachable, errOut)
	envText, needsSync := statusEnvStatus(workDir, git, cfg, headHash)

	fmt.Fprintf(out, "Repository:   %s\n", cfg.Repo)
	fmt.Fprintf(out, "Position:     %s\n", posText)
	fmt.Fprintf(out, "Environments: %s\n", envText)

	delta := statusDelta(workDir, git)
	hasDelta := len(delta) > 0

	fmt.Fprintln(out)
	if hasDelta {
		fmt.Fprintln(out, "Changes not yet published (origin/main → working tree):")
		for _, d := range TruncateDelta(delta, deltaTruncateThreshold) {
			fmt.Fprintln(out, d)
		}
	} else {
		fmt.Fprintln(out, "Working tree matches origin/main · nothing to publish")
	}

	syncHint := statusSyncHint(unreachable, ahead, behind, needsSync)
	if !hasDelta && syncHint == "" {
		return nil
	}

	fmt.Fprintln(out)
	if hasDelta {
		fmt.Fprintln(out, "  run aiman push to publish")
	}
	if syncHint != "" {
		fmt.Fprintln(out, "  "+syncHint)
	}
	return nil
}

// statusSyncHint reports the action needed to bring environments up to date,
// per ADR-0003 4.10 / 02-message-design.md 3.1(d,f). Behind/diverged take
// priority over a generic needs-sync state since they name the actual cause.
// No hint is printed when the remote is unreachable: aiman sync needs the
// same remote and would fail with the warning status just reported.
func statusSyncHint(unreachable bool, ahead, behind int, needsSync bool) string {
	switch {
	case unreachable:
		return ""
	case ahead > 0 && behind > 0:
		return "history diverged — resolve with git, then run aiman sync"
	case behind > 0:
		return "run aiman sync to apply remote changes"
	case needsSync:
		return "run aiman sync to apply"
	default:
		return ""
	}
}

// fetchWithTimeout calls git.Fetch in a goroutine and returns an error if it
// fails or does not complete within the given timeout.
func fetchWithTimeout(git gitops.Ops, workDir string, timeout time.Duration) error {
	ch := make(chan error, 1)
	go func() {
		ch <- git.Fetch(workDir)
	}()
	select {
	case err := <-ch:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("fetch timed out after %s", timeout)
	}
}

// statusPosition reports the working tree's position relative to origin/main,
// plus the raw ahead/behind counts and unreachable flag for hint selection.
func statusPosition(workDir string, git gitops.Ops, remoteUnreachable bool, errOut io.Writer) (text string, ahead, behind int, unreachable bool) {
	if remoteUnreachable {
		fmt.Fprintln(errOut, "warning: cannot reach remote repository")
		return "unknown (remote unreachable)", 0, 0, true
	}
	ahead, behind, err := git.CountAheadBehind(workDir, "origin/main", "HEAD")
	if err != nil {
		fmt.Fprintln(errOut, "warning: cannot reach remote repository")
		return "unknown (remote unreachable)", 0, 0, true
	}
	switch {
	case ahead > 0 && behind == 0:
		return fmt.Sprintf("%s ahead of origin/main", Plural(ahead, "commit")), ahead, behind, false
	case behind > 0 && ahead == 0:
		return fmt.Sprintf("%s behind of origin/main", Plural(behind, "commit")), ahead, behind, false
	case ahead > 0 && behind > 0:
		return fmt.Sprintf("diverged from origin/main (%s ahead, %s behind)",
			Plural(ahead, "commit"), Plural(behind, "commit")), ahead, behind, false
	default:
		return "up-to-date with origin/main", ahead, behind, false
	}
}

// statusEnvStatus reports whether environments match the synced hash, plus
// a needsSync flag for hint selection.
func statusEnvStatus(workDir string, git gitops.Ops, cfg localconfig.Config, headHash string) (text string, needsSync bool) {
	if cfg.SyncedHash == "" {
		return "unknown", false
	}
	if cfg.SyncedHash == headHash {
		return fmt.Sprintf("applied (synced %s)", shortHash(cfg.SyncedHash)), false
	}
	aheadFromSynced, _, err := git.CountAheadBehind(workDir, cfg.SyncedHash, headHash)
	if err != nil {
		return "needs sync", true
	}
	return fmt.Sprintf("needs sync (%s not applied)", Plural(aheadFromSynced, "commit")), true
}

func statusDelta(workDir string, git gitops.Ops) []string {
	diffLines, err := git.DiffNameStatus(workDir, "origin/main", statusManagedPaths)
	if err != nil {
		return nil
	}
	untracked, _ := git.ListUntrackedInPaths(workDir, statusManagedPaths)

	var delta []string
	for _, line := range diffLines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || len(parts[0]) == 0 {
			continue
		}
		delta = append(delta, fmt.Sprintf("  %c %s", parts[0][0], parts[1]))
	}
	for _, f := range untracked {
		delta = append(delta, fmt.Sprintf("  A %s", f))
	}
	return delta
}

func shortHash(h string) string {
	if len(h) >= 7 {
		return h[:7]
	}
	return h
}
