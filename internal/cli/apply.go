package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/axsmak/aim/internal/adapter"
	"github.com/axsmak/aim/internal/errs"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
	"github.com/spf13/cobra"
)

func newApplyCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply local inventory working tree to AI environments without publishing",
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				errs.Fatalf("cannot determine home directory: %v", err)
			}
			workDir := resolveWorkDir(homeDir)
			return runApply(dryRun, homeDir, workDir, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be applied without making changes")
	return cmd
}

func runApply(dryRun bool, homeDir, workDir string, in io.Reader, out, errOut io.Writer) error {
	cfg, err := localconfig.Load(workDir)
	if err != nil {
		return fmt.Errorf("cannot parse aim.local.yaml: %w", err)
	}

	skillsDir := filepath.Join(workDir, "skills")
	mcpDirFull := filepath.Join(workDir, "mcp")

	if dryRun {
		return runApplyDryRun(skillsDir, mcpDirFull, cfg, homeDir, out, errOut)
	}

	skillCount, envCount, installErr := installSkills(skillsDir, cfg, homeDir, out, errOut)
	mcpCount, mcpEnvCount, mcpErr := installMCPs(mcpDirFull, &cfg, homeDir, in, out, errOut)

	// Save cfg if MCP env values were resolved.
	// IMPORTANT: synced_hash and published_hash are never touched here.
	if err := localconfig.Save(workDir, cfg); err != nil {
		fmt.Fprintf(errOut, "warning: cannot save mcp_env: %v\n", err)
	}

	if installErr != nil {
		return installErr
	}
	if mcpErr != nil {
		return mcpErr
	}
	fmt.Fprintf(out, "Applied: %d skills, %d MCP servers in %d environments\n",
		skillCount, mcpCount, max(envCount, mcpEnvCount))
	return nil
}

func runApplyDryRun(skillsDir, mcpDirFull string, cfg localconfig.Config, homeDir string, out, errOut io.Writer) error {
	valid, invalid, err := skill.ReadAll(skillsDir)
	if err != nil {
		return fmt.Errorf("cannot read skills: %w", err)
	}
	for _, ve := range invalid {
		fmt.Fprintf(errOut, "warning: %s\n", ve)
	}

	mcpItems, mcpErrs := mcp.ParseDir(mcpDirFull)
	for _, e := range mcpErrs {
		fmt.Fprintf(errOut, "warning: %v\n", e)
	}

	for _, a := range adapter.DefaultAdapters(cfg) {
		_, found := a.Detect(homeDir)
		if !found {
			continue
		}
		if len(valid) > 0 {
			fmt.Fprintf(out, "[dry-run] would install in %s (%d skills):\n", a.Name(), len(valid))
			for _, s := range valid {
				fmt.Fprintf(out, "  - %s\n", s.Name)
			}
		}
		for _, m := range mcpItems {
			if !containsTarget(m.Targets, a.Name()) {
				continue
			}
			envStatus := mcpEnvStatus(m, cfg)
			fmt.Fprintf(out, "[dry-run] MCP %s → %s  %s\n", m.Name, a.Name(), envStatus)
		}
	}
	return nil
}
