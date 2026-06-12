package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/axsmak/aim/internal/errs"
	"github.com/axsmak/aim/internal/globalconfig"
	"github.com/spf13/cobra"
)

func newSwitchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <path>",
		Short: "Switch active AIM repository without cloning",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				errs.Fatal("path is required. Usage: aiman switch <path>")
			}
			homeDir, err := os.UserHomeDir()
			if err != nil {
				errs.Fatalf("cannot determine home directory: %v", err)
			}
			return runSwitch(args[0], homeDir, cmd.OutOrStdout())
		},
	}
}

func runSwitch(path, homeDir string, out io.Writer) error {
	// 1. Reject URLs
	if looksLikeURL(path) {
		return fmt.Errorf("aiman switch accepts a local path, not a URL — use aiman init for cloning")
	}
	// 2. Expand ~ if present
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(homeDir, path[2:])
	}
	// 3. Check path exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	}
	// 4. Validate it's an AIM repo
	if !isValidAIMRepo(path) {
		return fmt.Errorf("not a valid AIM repository: %s", path)
	}
	// 5. Make absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("cannot resolve path: %w", err)
	}
	// 6. Update global config
	if err := globalconfig.Save(homeDir, globalconfig.Config{Repo: absPath}); err != nil {
		return fmt.Errorf("cannot update global config: %w", err)
	}
	fmt.Fprintf(out, "switched: %s\n", absPath)
	return nil
}

func looksLikeURL(s string) bool {
	return strings.Contains(s, "://") || strings.HasPrefix(s, "git@")
}

func isValidAIMRepo(path string) bool {
	// Option 1: has aim.local.yaml
	if _, err := os.Stat(filepath.Join(path, "aim.local.yaml")); err == nil {
		return true
	}
	// Option 2: has .git AND skills/
	if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
		if info, err := os.Stat(filepath.Join(path, "skills")); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}
