package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/axsmak/aim/internal/errs"
	"github.com/axsmak/aim/internal/gitops"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/spf13/cobra"
)

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
			return runStatus(resolveWorkDir(homeDir), gitops.New(), cmd.OutOrStdout())
		},
	}
}

func runStatus(workDir string, git gitops.Ops, out io.Writer) error {
	cfg, err := localconfig.Load(workDir)
	if err != nil {
		errs.Fatal(err.Error())
	}

	if cfg.Repo == "" {
		fmt.Fprintln(out, "Repository not initialized. Run: aiman init <url>")
		return nil
	}

	headHash, _ := git.HeadHash(workDir)

	fmt.Fprintf(out, "Repository:   %s\n", cfg.Repo)
	fmt.Fprintf(out, "Position:     %s\n", statusPosition(workDir, git))
	fmt.Fprintf(out, "Environments: %s\n", statusEnvStatus(workDir, git, cfg, headHash))

	delta := statusDelta(workDir, git)

	fmt.Fprintln(out)
	if len(delta) == 0 {
		fmt.Fprintln(out, "Working tree matches origin/main · nothing to publish")
		return nil
	}

	fmt.Fprintln(out, "Changes not yet published (origin/main → working tree):")
	for _, d := range delta {
		fmt.Fprintln(out, d)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  run aiman push to publish")
	return nil
}

func statusPosition(workDir string, git gitops.Ops) string {
	ahead, behind, err := git.CountAheadBehind(workDir, "origin/main", "HEAD")
	if err != nil {
		return "unreachable"
	}
	switch {
	case ahead > 0 && behind == 0:
		return fmt.Sprintf("ahead %d commit(s)", ahead)
	case behind > 0 && ahead == 0:
		return fmt.Sprintf("behind %d commit(s)", behind)
	case ahead > 0 && behind > 0:
		return "diverged"
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
	return fmt.Sprintf("needs sync (%d commit(s) not applied)", aheadFromSynced)
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
