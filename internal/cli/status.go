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

var statusManagedPaths = []string{"skills/", "mcp/"}

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

	fmt.Fprintf(out, "Repository:   %s\n", cfg.Repo)
	fmt.Fprintf(out, "Position:     %s\n", statusPosition(workDir, git, remoteUnreachable, errOut))
	fmt.Fprintf(out, "Environments: %s\n", statusEnvStatus(workDir, git, cfg, headHash))

	delta := statusDelta(workDir, git)

	fmt.Fprintln(out)
	if len(delta) == 0 {
		fmt.Fprintln(out, "Working tree matches origin/main · nothing to publish")
		return nil
	}

	fmt.Fprintln(out, "Changes not yet published (origin/main → working tree):")
	for _, d := range TruncateDelta(delta, deltaTruncateThreshold) {
		fmt.Fprintln(out, d)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  run aiman push to publish")
	return nil
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

func statusPosition(workDir string, git gitops.Ops, remoteUnreachable bool, errOut io.Writer) string {
	if remoteUnreachable {
		fmt.Fprintln(errOut, "warning: cannot reach remote repository")
		return "unknown (remote unreachable)"
	}
	ahead, behind, err := git.CountAheadBehind(workDir, "origin/main", "HEAD")
	if err != nil {
		fmt.Fprintln(errOut, "warning: cannot reach remote repository")
		return "unknown (remote unreachable)"
	}
	switch {
	case ahead > 0 && behind == 0:
		return fmt.Sprintf("%s ahead of origin/main", Plural(ahead, "commit"))
	case behind > 0 && ahead == 0:
		return fmt.Sprintf("%s behind of origin/main", Plural(behind, "commit"))
	case ahead > 0 && behind > 0:
		return fmt.Sprintf("diverged from origin/main (%s ahead, %s behind)",
			Plural(ahead, "commit"), Plural(behind, "commit"))
	default:
		return "up-to-date with origin/main"
	}
}

func statusEnvStatus(workDir string, git gitops.Ops, cfg localconfig.Config, headHash string) string {
	if cfg.SyncedHash == "" {
		return "unknown"
	}
	if cfg.SyncedHash == headHash {
		return fmt.Sprintf("applied (synced %s)", shortHash(cfg.SyncedHash))
	}
	aheadFromSynced, _, err := git.CountAheadBehind(workDir, cfg.SyncedHash, headHash)
	if err != nil {
		return "needs sync"
	}
	return fmt.Sprintf("needs sync (%s not applied)", Plural(aheadFromSynced, "commit"))
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
