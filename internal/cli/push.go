package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/axsmak/aim/internal/errs"
	"github.com/axsmak/aim/internal/gitops"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
	"github.com/spf13/cobra"
)

func newPushCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Publish local inventory changes to the remote repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				errs.Fatalf("cannot determine home directory: %v", err)
			}
			return runPush(dryRun, resolveWorkDir(homeDir), gitops.New(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be published without making changes")
	return cmd
}

func runPush(dryRun bool, workDir string, git gitops.Ops, out, errOut io.Writer) error {
	if _, err := os.Stat(filepath.Join(workDir, ".git")); err != nil {
		return fmt.Errorf("repository not initialized — run: aiman init <url>")
	}

	cfg, err := localconfig.Load(workDir)
	if err != nil {
		return fmt.Errorf("cannot parse aim.local.yaml: %w", err)
	}

	remoteHash, err := git.LsRemote(workDir, "refs/heads/main")
	if err != nil {
		return fmt.Errorf("cannot reach remote repository: %w", err)
	}
	if remoteHash != "" {
		// Fetch downloads remote objects into the local store so IsAncestor can
		// resolve the remote hash — LsRemote only returns the hash, not the objects.
		if err := git.Fetch(workDir); err != nil {
			return fmt.Errorf("cannot reach remote repository: %w", err)
		}
		isAncestor, err := git.IsAncestor(workDir, remoteHash, "HEAD")
		if err != nil {
			return fmt.Errorf("cannot verify remote state: %w", err)
		}
		if !isAncestor {
			return fmt.Errorf("remote is ahead of local — run: aiman sync")
		}
	}

	if _, err := os.Stat(filepath.Join(workDir, "skills")); os.IsNotExist(err) {
		return fmt.Errorf("malformed repository: skills/ directory is missing — run aiman init or restore scaffold")
	}

	valid, invalid, err := skill.ReadAll(filepath.Join(workDir, "skills"))
	if err != nil {
		return err
	}
	hasErrors := false
	for _, ve := range invalid {
		fmt.Fprintf(errOut, "error: %s\n", ve)
		hasErrors = true
	}
	for _, s := range valid {
		if s.Body == "" {
			fmt.Fprintf(errOut, "error: skill %s has empty body\n", s.Name)
			hasErrors = true
		}
		if s.Description == "" {
			fmt.Fprintf(errOut, "warning: skill %s has no description\n", s.Name)
		}
	}
	if hasErrors {
		return fmt.Errorf("validation failed — fix errors before publishing")
	}

	// Validate MCP items
	mcpItems, mcpInvalid := mcp.ParseDir(filepath.Join(workDir, "mcp"))
	for _, e := range mcpInvalid {
		fmt.Fprintf(errOut, "error: %s\n", e)
		hasErrors = true
	}
	if hasErrors {
		return fmt.Errorf("validation failed — fix errors before publishing")
	}

	// Protect against accidental staging of aim.local.yaml
	staged, err := git.IsFileStaged(workDir, "aim.local.yaml")
	if err == nil && staged {
		return fmt.Errorf("aim.local.yaml is staged for commit — remove it: git reset HEAD aim.local.yaml")
	}

	if dryRun {
		lines, err := gitops.ManagedStatus(workDir)
		if err != nil {
			return fmt.Errorf("dry-run: could not read managed paths status: %w", err)
		}
		if len(lines) == 0 {
			fmt.Fprintln(out, "[dry-run] aiman push: nothing to publish — working tree is clean")
		} else {
			fmt.Fprintln(out, "[dry-run] aiman push: would publish managed changes")
			for _, l := range lines {
				fmt.Fprintf(out, "  %s\n", l)
			}
		}
		invLine := fmt.Sprintf("Validated inventory: %d skills", len(valid))
		if len(mcpItems) > 0 {
			invLine += fmt.Sprintf(", %d MCP servers", len(mcpItems))
		}
		fmt.Fprintln(out, invLine)
		return nil
	}

	msg := "aim: publish " + time.Now().UTC().Format(time.RFC3339)
	if err := git.Commit(workDir, msg); err != nil {
		if strings.Contains(err.Error(), "nothing to commit") {
			return fmt.Errorf("nothing to publish: working tree has no staged changes in managed paths")
		}
		return err
	}

	if err := git.Push(workDir); err != nil {
		git.ResetSoft(workDir) //nolint:errcheck
		return fmt.Errorf("push failed — changes not published: %w", err)
	}

	hash, err := git.HeadHash(workDir)
	if err != nil {
		return err
	}
	cfg.PublishedHash = hash
	if saveErr := localconfig.Save(workDir, cfg); saveErr != nil {
		fmt.Fprintf(errOut, "warning: failed to save published_hash: %v\n", saveErr)
	}

	shortHash := hash
	if len(shortHash) > 7 {
		shortHash = shortHash[:7]
	}
	fmt.Fprintln(out, FormatSuccess("published", shortHash, len(valid), len(mcpItems), 0))
	return nil
}
