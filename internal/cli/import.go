package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/axsmak/aim/internal/adapter"
	"github.com/axsmak/aim/internal/importer"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/spf13/cobra"
)

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <type>",
		Short: "Import a skill or MCP item from an AI environment",
	}
	cmd.AddCommand(newImportSkillCmd())
	cmd.AddCommand(newImportMCPCmd())
	return cmd
}

func newImportSkillCmd() *cobra.Command {
	var from string
	var printOnly bool
	var overwrite bool
	var targets string

	cmd := &cobra.Command{
		Use:   "skill <name>",
		Short: "Import a skill from an AI environment into the local inventory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}
			workDir, err := requireWorkDir(homeDir)
			if err != nil {
				return err
			}
			if err := importer.ValidateItemName(name); err != nil {
				return err
			}

			var scanner adapter.SkillScanner
			for _, a := range adapter.Registry() {
				if s, ok := a.(adapter.SkillScanner); ok && s.Name() == from {
					scanner = s
					break
				}
			}
			if scanner == nil {
				return fmt.Errorf("unknown environment: %s; available: claude-code, cursor, codex", from)
			}

			discovered, err := scanner.ScanSkills("")
			if err != nil {
				return err
			}

			var matches []adapter.DiscoveredSkill
			for _, d := range discovered {
				if d.Name == name {
					matches = append(matches, d)
				}
			}

			if len(matches) == 0 {
				return fmt.Errorf("skill %q not found in %s", name, from)
			}
			if len(matches) > 1 {
				sources := make([]string, len(matches))
				for i, m := range matches {
					sources[i] = m.Source
				}
				ae := importer.AmbiguousError{Name: name, Sources: sources}
				return fmt.Errorf("%s: found in multiple sources %v; rename or remove duplicates, or import the file directly with aiman add skill <file>", ae.Name, ae.Sources)
			}

			found := matches[0]

			if _, err := importer.NormalizeSkill(found); err != nil {
				return err
			}

			if printOnly {
				_, err := cmd.OutOrStdout().Write(found.Raw)
				return err
			}

			destPath := filepath.Join(workDir, "skills", found.Name+".md")

			// If ConflictError message in conflict.go changes, update this import-specific hint too.
			if err := importer.CheckConflict(destPath, found.Raw, overwrite); err != nil {
				if errors.Is(err, importer.ErrIdentical) {
					fmt.Fprintf(cmd.OutOrStdout(), "up to date: skill %s · already identical\n", found.Name)
					return nil
				}
				var ce importer.ConflictError
				if errors.As(err, &ce) {
					return fmt.Errorf("%s already exists with different content; use --overwrite to replace", ce.Path)
				}
				return err
			}

			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(destPath, found.Raw, 0644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "imported: skill %s · from %s\n", found.Name, from)
			return nil
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "source AI environment (claude-code, cursor, codex)")
	_ = cmd.MarkFlagRequired("from")
	cmd.Flags().BoolVar(&printOnly, "print", false, "print skill content without writing to disk")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace existing skill if content differs")
	cmd.Flags().StringVar(&targets, "targets", "", "target environments (ignored for skills)")

	return cmd
}

var allAdapterNames = []string{"claude-code", "cursor", "codex"}

func newImportMCPCmd() *cobra.Command {
	var from string
	var printOnly bool
	var overwrite bool
	var targets string

	cmd := &cobra.Command{
		Use:   "mcp <name>",
		Short: "Import an MCP server from an AI environment into the local inventory",
		Long:  "Import an MCP server from an AI environment.\nEnv values are stored in aim.local.yaml; mcp/<name>.yaml contains descriptors only.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}
			workDir, err := requireWorkDir(homeDir)
			if err != nil {
				return err
			}
			if err := importer.ValidateItemName(name); err != nil {
				return err
			}

			var scanner adapter.MCPScanner
			for _, a := range adapter.Registry() {
				if s, ok := a.(adapter.MCPScanner); ok && s.Name() == from {
					scanner = s
					break
				}
			}
			if scanner == nil {
				return fmt.Errorf("unknown environment: %s; available: claude-code, cursor, codex", from)
			}

			discovered, err := scanner.ScanMCP("")
			if err != nil {
				return err
			}

			var matches []adapter.DiscoveredMCP
			for _, d := range discovered {
				if d.ServerName == name {
					matches = append(matches, d)
				}
			}

			if len(matches) == 0 {
				return fmt.Errorf("MCP server %q not found in %s", name, from)
			}

			found := deduplicateMCP(matches)
			if found == nil {
				sources := make([]string, len(matches))
				for i, m := range matches {
					sources[i] = m.Source
				}
				return importer.AmbiguousError{Name: name, Sources: sources}
			}

			var resolvedTargets []string
			if targets == "all" {
				resolvedTargets = allAdapterNames
			} else {
				resolvedTargets = []string{from}
			}

			normalized, secrets, err := importer.NormalizeMCP(*found, resolvedTargets)
			if err != nil {
				return err
			}

			yamlBytes, err := yaml.Marshal(normalized)
			if err != nil {
				return err
			}

			destPath := filepath.Join(workDir, "mcp", name+".yaml")

			// If ConflictError message in conflict.go changes, update this import-specific hint too.
			if err := importer.CheckConflict(destPath, yamlBytes, overwrite); err != nil {
				if errors.Is(err, importer.ErrIdentical) {
					fmt.Fprintf(cmd.OutOrStdout(), "up to date: mcp %s · already identical\n", name)
					return nil
				}
				var ce importer.ConflictError
				if errors.As(err, &ce) {
					return fmt.Errorf("%s already exists with different content; use --overwrite to replace", ce.Path)
				}
				return err
			}

			if printOnly {
				_, err := cmd.OutOrStdout().Write(yamlBytes)
				return err
			}

			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(destPath, yamlBytes, 0644); err != nil {
				return err
			}

			if len(secrets) > 0 {
				localCfg, err := localconfig.Load(workDir)
				if err != nil {
					return fmt.Errorf("cannot load local config: %w", err)
				}
				for varName, value := range secrets {
					localCfg.SetMCPEnv(name, varName, value)
				}
				if err := localconfig.Save(workDir, localCfg); err != nil {
					return fmt.Errorf("cannot save local config: %w", err)
				}
			}

			if len(secrets) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "imported: mcp %s · from %s · secrets stored in aim.local.yaml\n", name, from)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "imported: mcp %s · from %s\n", name, from)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&from, "from", "", "source AI environment (claude-code, cursor, codex)")
	_ = cmd.MarkFlagRequired("from")
	cmd.Flags().BoolVar(&printOnly, "print", false, "print MCP config without writing to disk")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace existing MCP config if content differs")
	cmd.Flags().StringVar(&targets, "targets", "", `target environments; use "all" for all adapters`)

	return cmd
}

// deduplicateMCP returns the single result when all matches have the same command+args.
// Returns nil when matches differ (AmbiguousError should be raised by the caller).
func deduplicateMCP(matches []adapter.DiscoveredMCP) *adapter.DiscoveredMCP {
	if len(matches) == 0 {
		return nil
	}
	ref := matches[0]
	for _, m := range matches[1:] {
		if m.Command != ref.Command || !stringSliceEqual(m.Args, ref.Args) {
			return nil
		}
	}
	return &ref
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
