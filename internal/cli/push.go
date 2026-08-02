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
	"github.com/axsmak/aim/internal/loadout"
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
		if ve.Field == "body" {
			fmt.Fprintf(errOut, "error: %s\n", ve)
			hasErrors = true
		} else {
			fmt.Fprintf(errOut, "warning: %s\n", ve)
		}
	}
	for _, s := range valid {
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

	// Validate loadouts (BFT 5.2): format invariants plus reference integrity
	// against skills/ and mcp/. A repository without loadouts/ is untouched —
	// List returns empty results and this block is a no-op. All errors are
	// reported at once (US-L04), together with any MCP errors above.
	loadouts, loadoutErrs, loadoutWarns, err := loadout.List(filepath.Join(workDir, "loadouts"))
	if err != nil {
		return err
	}
	for _, w := range loadoutWarns {
		fmt.Fprintf(errOut, "warning: %s\n", w)
	}
	for _, ve := range loadoutErrs {
		fmt.Fprintf(errOut, "error: %s\n", ve)
		hasErrors = true
	}
	for _, l := range loadouts {
		broken := loadout.CheckRefs(l, filepath.Join(workDir, "skills"), filepath.Join(workDir, "mcp"))
		for _, re := range broken {
			// BFT 5.2 sample message: hint points at the file and its items field.
			fmt.Fprintf(errOut, "error: %s\n  hint: check loadouts/%s → items\n", re, filepath.Base(re.FilePath))
			hasErrors = true
		}
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
			fmt.Fprintln(out, "[dry-run] nothing to publish — working tree is clean")
		} else {
			fmt.Fprintln(out, "[dry-run] would publish managed changes:")
			PrintDeltaBlock(out, lines)
		}
		invLine := fmt.Sprintf("  validated inventory: %d skills", len(valid))
		if len(mcpItems) > 0 {
			invLine += fmt.Sprintf(", %d MCP servers", len(mcpItems))
		}
		fmt.Fprintln(out, invLine)
		return nil
	}

	// Snapshot managed-paths delta before commit — after commit the working tree is clean.
	// ADR-0003 5.1: print change composition after the success line.
	deltaLines, err := gitops.ManagedStatus(workDir)
	if err != nil {
		// Non-fatal: delta is informational; proceed with the publish.
		fmt.Fprintf(errOut, "warning: could not read managed paths status: %v\n", err)
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
	fmt.Fprintln(out, FormatSuccess("published", shortHash, len(valid), len(mcpItems), 0, nil))
	// ADR-0003 5.1: delta block after success line; omitted when nothing changed.
	PrintDeltaBlock(out, deltaLines)
	return nil
}
