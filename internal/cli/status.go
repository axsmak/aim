package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/axsmak/aim/internal/errs"
	"github.com/axsmak/aim/internal/gitops"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show repository sync status",
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				errs.Fatalf("cannot determine home directory: %v", err)
			}
			return runStatus(resolveWorkDir(homeDir))
		},
	}
}

func runStatus(workDir string) error {
	cfg, err := localconfig.Load(workDir)
	if err != nil {
		errs.Fatal(err.Error())
	}

	if cfg.Repo == "" {
		fmt.Println("Repository not initialized. Run: aiman init <url>")
		return nil
	}

	ops := gitops.New()

	// --- Repository axis ---

	headHash, err := ops.HeadHash(workDir)
	if err != nil {
		errs.Fatal("cannot read HEAD: " + err.Error())
	}

	dirty, err := ops.HasDirtyWorktree(workDir)
	if err != nil {
		errs.Fatal("cannot check worktree state: " + err.Error())
	}
	untracked, err := ops.HasUntrackedInPaths(workDir, gitops.ManagedPaths)
	if err != nil {
		errs.Fatal("cannot check untracked files: " + err.Error())
	}

	repoStatus := "clean"
	if dirty || untracked {
		repoStatus = "dirty"
	}

	ahead, behind, err := ops.CountAheadBehind(workDir, "origin/main", "HEAD")
	if err != nil {
		errs.Fatal("cannot compute ahead/behind: " + err.Error())
	}

	var position string
	switch {
	case ahead > 0 && behind == 0:
		position = fmt.Sprintf("ahead %d commit(s)", ahead)
	case behind > 0 && ahead == 0:
		position = fmt.Sprintf("behind %d commit(s)", behind)
	case ahead > 0 && behind > 0:
		position = "diverged"
	default:
		position = "up-to-date"
	}

	remoteHash, err := ops.RemoteHash(workDir, "origin/main")
	if err != nil {
		errs.Fatal("cannot read remote state: " + err.Error())
	}

	fmt.Println("Repository:")
	fmt.Printf("  repo:     %s\n", cfg.Repo)
	fmt.Printf("  status:   %s\n", repoStatus)
	fmt.Printf("  position: %s\n", position)
	fmt.Printf("  HEAD:     %s\n", shortHash(headHash))
	fmt.Printf("  origin:   %s\n", shortHash(remoteHash))
	if repoStatus == "dirty" || strings.HasPrefix(position, "ahead") {
		fmt.Printf("  action:   run aiman push\n")
	}

	// --- Environment axis ---

	publishedDisplay := "not set"
	if cfg.PublishedHash != "" {
		publishedDisplay = shortHash(cfg.PublishedHash)
	}

	appliedDisplay := "not set"
	envStatus := "unknown"
	action := ""

	if cfg.SyncedHash != "" {
		appliedDisplay = shortHash(cfg.SyncedHash)
		if cfg.SyncedHash == headHash {
			envStatus = "applied"
		} else {
			aheadFromSynced, _, countErr := ops.CountAheadBehind(workDir, cfg.SyncedHash, headHash)
			if countErr != nil {
				envStatus = "needs sync (cannot compute delta)"
			} else {
				envStatus = fmt.Sprintf("needs sync (%d commit(s) not applied)", aheadFromSynced)
			}
			action = "run aiman sync"
		}
	}

	fmt.Println()
	fmt.Println("Environment:")
	fmt.Printf("  published: %s\n", publishedDisplay)
	fmt.Printf("  applied:   %s\n", appliedDisplay)
	fmt.Printf("  status:    %s\n", envStatus)
	if action != "" {
		fmt.Printf("  action:    %s\n", action)
	}

	return nil
}

func shortHash(h string) string {
	if len(h) >= 7 {
		return h[:7]
	}
	return h
}
