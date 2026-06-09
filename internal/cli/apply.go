package cli

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
	fmt.Fprintln(out, FormatSuccess("applied", "", skillCount, mcpCount, max(envCount, mcpEnvCount)))
	return nil
}

// skillDelta holds the change category and list of envs where the skill differs.
type skillDelta struct {
	category string // "A" = new, "M" = modified
	envNames []string
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

	// Collect detected adapters only.
	var detectedAdapters []adapter.Adapter
	var detectedBaseDirs []string
	var detectedNames []string
	for _, a := range adapter.DefaultAdapters(cfg) {
		baseDir, found := a.Detect(homeDir)
		if !found {
			continue
		}
		detectedAdapters = append(detectedAdapters, a)
		detectedBaseDirs = append(detectedBaseDirs, baseDir)
		detectedNames = append(detectedNames, a.Name())
	}

	// No environments detected — output nothing.
	if len(detectedAdapters) == 0 {
		return nil
	}

	// Build skill delta: map skillName → {category, []envNames}.
	// category A = file missing in env; category M = content differs.
	deltaMap := make(map[string]*skillDelta)
	for _, s := range valid {
		inventoryHash := sha256.Sum256(s.Raw)
		for i, a := range detectedAdapters {
			installedPath := filepath.Join(detectedBaseDirs[i], "skills", s.Name, "SKILL.md")
			raw, readErr := os.ReadFile(installedPath)
			if readErr != nil {
				// File not present in this env.
				d := deltaMap[s.Name]
				if d == nil {
					d = &skillDelta{category: "A"}
					deltaMap[s.Name] = d
				}
				d.envNames = append(d.envNames, a.Name())
				continue
			}
			installedHash := sha256.Sum256(raw)
			if installedHash != inventoryHash {
				d := deltaMap[s.Name]
				if d == nil {
					d = &skillDelta{category: "M"}
					deltaMap[s.Name] = d
				} else if d.category == "A" {
					// If already categorized as A (missing in other envs), keep A.
				} else {
					d.category = "M"
				}
				d.envNames = append(d.envNames, a.Name())
			}
		}
	}

	// Build MCP lines.
	type mcpLine struct {
		name   string
		status string // "up to date in all environments" or specific env status
		envTag string // optional [missing env: ...] suffix
	}
	var mcpLines []mcpLine
	for _, m := range mcpItems {
		// Only include MCPs that target at least one detected adapter.
		var targetedEnvs []string
		for _, a := range detectedAdapters {
			if containsTarget(m.Targets, a.Name()) {
				targetedEnvs = append(targetedEnvs, a.Name())
			}
		}
		if len(targetedEnvs) == 0 {
			continue
		}
		envTag := mcpEnvStatus(m, cfg)
		var status string
		if len(targetedEnvs) == len(detectedAdapters) {
			status = "up to date in all environments"
		} else {
			status = "up to date in " + strings.Join(targetedEnvs, ", ")
		}
		mcpLines = append(mcpLines, mcpLine{name: m.Name, status: status, envTag: envTag})
	}

	// If no changes and no MCPs to report, output "nothing to apply".
	if len(deltaMap) == 0 && len(mcpLines) == 0 {
		fmt.Fprintln(out, "[dry-run] nothing to apply — environments match local inventory")
		return nil
	}

	// Build summary.
	totalEnvCount := len(detectedNames)
	envList := strings.Join(detectedNames, ", ")

	if len(deltaMap) > 0 {
		// Count total unique change entries.
		changeCount := len(deltaMap)
		fmt.Fprintf(out, "[dry-run] would apply %d change(s) to %d environment(s) (%s):\n", changeCount, totalEnvCount, envList)

		// Sort for deterministic output.
		var skillNames []string
		for name := range deltaMap {
			skillNames = append(skillNames, name)
		}
		sort.Strings(skillNames)

		var skillLines []string
		for _, name := range skillNames {
			d := deltaMap[name]
			var envDesc string
			if len(d.envNames) == totalEnvCount {
				envDesc = "all environments"
			} else {
				envDesc = strings.Join(d.envNames, ", ")
			}
			if d.category == "A" {
				skillLines = append(skillLines, fmt.Sprintf("  A skills/%s.md   (new in %s)", name, envDesc))
			} else {
				skillLines = append(skillLines, fmt.Sprintf("  M skills/%s.md   (differs in %s)", name, envDesc))
			}
		}
		for _, line := range TruncateDelta(skillLines, deltaTruncateThreshold) {
			fmt.Fprintln(out, line)
		}
	}

	for _, ml := range mcpLines {
		if ml.envTag != "" {
			fmt.Fprintf(out, "[dry-run] MCP %s → %s  %s\n", ml.name, ml.status, ml.envTag)
		} else {
			fmt.Fprintf(out, "[dry-run] MCP %s → %s\n", ml.name, ml.status)
		}
	}

	return nil
}
