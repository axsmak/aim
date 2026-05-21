package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/axsmak/aim/internal/adapter"
	"github.com/axsmak/aim/internal/errs"
	"github.com/axsmak/aim/internal/gitops"
	"github.com/axsmak/aim/internal/globalconfig"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/axsmak/aim/internal/repoconfig"
	"github.com/spf13/cobra"
)

type repoClass int

const (
	repoAdoptable   repoClass = iota
	repoExistingAIM repoClass = iota
)

func newInitCmd() *cobra.Command {
	var destPath string
	cmd := &cobra.Command{
		Use:   "init <repo-url>",
		Short: "Clone a skill library repository and initialise local config",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				errs.Fatal("repo-url is required. Usage: aiman init <repo-url>")
			}
			url := args[0]

			homeDir, err := os.UserHomeDir()
			if err != nil {
				errs.Fatalf("cannot determine home directory: %v", err)
			}

			// Resolve destination directory
			var resolvedDest string
			if destPath != "" {
				// Expand ~ if needed
				if strings.HasPrefix(destPath, "~/") {
					resolvedDest = filepath.Join(homeDir, destPath[2:])
				} else {
					resolvedDest = destPath
				}
			} else {
				// Interactive prompt
				defaultDir := filepath.Join(homeDir, repoNameFromURL(url))
				fmt.Fprintf(cmd.OutOrStdout(), "Where should the repository be cloned? [%s]: ", defaultDir)
				scanner := bufio.NewScanner(cmd.InOrStdin())
				scanner.Scan()
				input := strings.TrimSpace(scanner.Text())
				if input == "" {
					resolvedDest = defaultDir
				} else {
					if strings.HasPrefix(input, "~/") {
						resolvedDest = filepath.Join(homeDir, input[2:])
					} else {
						resolvedDest = input
					}
				}
			}

			absDestDir, err := filepath.Abs(resolvedDest)
			if err != nil {
				return fmt.Errorf("cannot resolve destination path: %w", err)
			}

			return runInit(url, absDestDir, homeDir, gitops.New(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&destPath, "path", "", "destination directory for the cloned repository (skips interactive prompt)")
	return cmd
}

func runInit(url, destDir, homeDir string, git gitops.Ops, in io.Reader, out io.Writer) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("repo-url cannot be empty")
	}

	// Guard: check if global config already exists
	gcfg, err := globalconfig.Load(homeDir)
	if err == nil && gcfg.Repo != "" {
		return fmt.Errorf("AIM is already configured. Use 'aim switch <path>' to change repos.")
	}

	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("cannot create destination directory: %w", err)
	}

	// Clone repository
	if err := git.Clone(url, destDir); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	// Classify the cloned repo and handle each state
	class, classErr := classifyRepo(destDir)
	if classErr != nil {
		return classErr
	}

	var nextStep string
	switch class {
	case repoAdoptable:
		if err := createScaffold(destDir); err != nil {
			return err
		}
		nextStep = "Library scaffold created. Add skills or run aiman push to publish the scaffold."
	case repoExistingAIM:
		if err := validateExistingRepo(destDir, out); err != nil {
			return err
		}
		nextStep = "Library found. Run aiman sync to apply, or aiman push if you have local changes."
	}

	// Detect available AI environments
	cfg := localconfig.Config{Repo: url}
	if class == repoExistingAIM {
		if headHash, err := git.HeadHash(destDir); err == nil {
			cfg.SyncedHash = headHash
		}
	}

	type envResult struct {
		name  string
		found bool
		dir   string
	}
	var results []envResult

	for _, a := range adapter.Registry() {
		baseDir, found := a.Detect(homeDir)
		results = append(results, envResult{name: a.Name(), found: found, dir: baseDir})
		if !found {
			continue
		}
		switch a.Name() {
		case "claude-code":
			cfg.Adapters.ClaudeCode.BaseDir = baseDir
		case "cursor":
			cfg.Adapters.Cursor.BaseDir = baseDir
		case "codex":
			cfg.Adapters.Codex.BaseDir = baseDir
		}
	}

	// Save aim.local.yaml
	if err := localconfig.Save(destDir, cfg); err != nil {
		return fmt.Errorf("cannot write aim.local.yaml: %w", err)
	}

	// Write global config
	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		absDestDir = destDir
	}
	if err := globalconfig.Save(homeDir, globalconfig.Config{Repo: absDestDir}); err != nil {
		return fmt.Errorf("cannot write global config: %w", err)
	}

	// Print result
	switch class {
	case repoAdoptable:
		fmt.Fprintf(out, "initialized: %s (new library)\n", url)
	case repoExistingAIM:
		fmt.Fprintf(out, "initialized: %s (existing library)\n", url)
	}
	if nextStep != "" {
		fmt.Fprintln(out, nextStep)
	}
	fmt.Fprintln(out, "Environments:")
	for _, r := range results {
		if r.found {
			fmt.Fprintf(out, "  ✓ %s: %s\n", r.name, r.dir)
		} else {
			fmt.Fprintf(out, "  - %s: not found\n", r.name)
		}
	}
	return nil
}

// repoNameFromURL extracts the repository name from a Git URL.
// Examples:
//
//	https://github.com/user/my-loadout.git → my-loadout
//	git@github.com:user/my-loadout.git     → my-loadout
//	Fallback: loadout
func repoNameFromURL(url string) string {
	url = strings.TrimSpace(url)
	if url == "" {
		return "loadout"
	}
	// Strip trailing .git
	url = strings.TrimSuffix(url, ".git")
	// Get last path segment (works for both https and ssh URLs)
	// For ssh: git@github.com:user/repo → split on "/" gives [..., "repo"]
	// For https: https://github.com/user/repo → split on "/" gives [..., "repo"]
	// Handle ssh colon separator: git@github.com:user/repo
	if idx := strings.LastIndex(url, "/"); idx >= 0 {
		name := url[idx+1:]
		if name != "" {
			return name
		}
	}
	// Try colon separator for ssh without slash after colon
	if idx := strings.LastIndex(url, ":"); idx >= 0 {
		name := url[idx+1:]
		if name != "" {
			return name
		}
	}
	return "loadout"
}

// classifyRepo determines the repository state after cloning.
// Returns a non-nil error for structural conflicts (skills/mcp exist as files).
func classifyRepo(workDir string) (repoClass, error) {
	skillsPath := filepath.Join(workDir, "skills")
	mcpPath := filepath.Join(workDir, "mcp")

	if info, err := os.Stat(skillsPath); err == nil && !info.IsDir() {
		return 0, fmt.Errorf("skills exists as a file, expected a directory")
	}
	if info, err := os.Stat(mcpPath); err == nil && !info.IsDir() {
		return 0, fmt.Errorf("mcp exists as a file, expected a directory")
	}

	if _, err := os.Stat(filepath.Join(workDir, "aim.yaml")); err == nil {
		return repoExistingAIM, nil
	}
	return repoAdoptable, nil
}

// createScaffold creates missing AIM library structure. Existing files are not overwritten.
func createScaffold(workDir string) error {
	aimYAML := filepath.Join(workDir, "aim.yaml")
	if _, err := os.Stat(aimYAML); os.IsNotExist(err) {
		if err := os.WriteFile(aimYAML, []byte("skill_paths: {}\n"), 0644); err != nil {
			return fmt.Errorf("cannot create aim.yaml: %w", err)
		}
	}

	gitignore := filepath.Join(workDir, ".gitignore")
	if _, err := os.Stat(gitignore); os.IsNotExist(err) {
		if err := os.WriteFile(gitignore, []byte("aim.local.yaml\n"), 0644); err != nil {
			return fmt.Errorf("cannot create .gitignore: %w", err)
		}
	} else if !isInGitignore(workDir, "aim.local.yaml") {
		f, err := os.OpenFile(gitignore, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("cannot update .gitignore: %w", err)
		}
		defer f.Close()
		if _, err := fmt.Fprintln(f, "aim.local.yaml"); err != nil {
			return fmt.Errorf("cannot update .gitignore: %w", err)
		}
	}

	skillsDir := filepath.Join(workDir, "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(skillsDir, 0755); err != nil {
			return fmt.Errorf("cannot create skills/: %w", err)
		}
		if err := os.WriteFile(filepath.Join(skillsDir, ".gitkeep"), nil, 0644); err != nil {
			return fmt.Errorf("cannot create skills/.gitkeep: %w", err)
		}
	}

	mcpDir := filepath.Join(workDir, "mcp")
	if _, err := os.Stat(mcpDir); os.IsNotExist(err) {
		if err := os.MkdirAll(mcpDir, 0755); err != nil {
			return fmt.Errorf("cannot create mcp/: %w", err)
		}
		if err := os.WriteFile(filepath.Join(mcpDir, ".gitkeep"), nil, 0644); err != nil {
			return fmt.Errorf("cannot create mcp/.gitkeep: %w", err)
		}
	}

	return nil
}

// validateExistingRepo checks the integrity of a repo that already has aim.yaml.
func validateExistingRepo(workDir string, out io.Writer) error {
	if _, err := repoconfig.Load(workDir); err != nil {
		return fmt.Errorf("aim.yaml is not valid YAML: %w", err)
	}

	skillsPath := filepath.Join(workDir, "skills")
	skillsInfo, err := os.Stat(skillsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("malformed repository: aim.yaml exists but skills/ is missing")
		}
		return fmt.Errorf("cannot check skills/: %w", err)
	}
	if !skillsInfo.IsDir() {
		return fmt.Errorf("skills exists as a file, expected a directory")
	}

	mcpPath := filepath.Join(workDir, "mcp")
	if info, err := os.Stat(mcpPath); err == nil && !info.IsDir() {
		return fmt.Errorf("mcp exists as a file, expected a directory")
	}

	if !isInGitignore(workDir, "aim.local.yaml") {
		fmt.Fprintln(out, "warning: aim.local.yaml not in .gitignore — local config may be committed accidentally")
	}

	return nil
}
