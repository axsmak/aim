package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/axsmak/aim/internal/adapter"
	"github.com/axsmak/aim/internal/errs"
	"github.com/axsmak/aim/internal/gitops"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose AI environments, skills, and MCP env variables",
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				errs.Fatalf("cannot determine home directory: %v", err)
			}
			workDir := resolveWorkDir(homeDir)
			cfg, err := localconfig.Load(workDir)
			if err != nil {
				errs.Fatalf("cannot parse aim.local.yaml: %v", err)
			}
			adapters := adapter.DefaultAdapters(cfg)
			skillsDir := workDir + "/skills/"
			mcpDir := workDir + "/mcp/"
			return runDoctor(homeDir, workDir, skillsDir, mcpDir, cfg, adapters, cmd.OutOrStdout())
		},
	}
}

func runDoctor(homeDir, workDir, skillsDir, mcpDir string, cfg localconfig.Config, adapters []adapter.Adapter, out io.Writer) error {
	// Print Active Repo at top
	gcfgPath := filepath.Join(homeDir, ".config", "aim", "config.yaml")
	if workDir == "." {
		fmt.Fprintf(out, "Active Repo: . (current directory — no global config set)\n\n")
	} else {
		fmt.Fprintf(out, "Active Repo: %s (from %s)\n\n", workDir, gcfgPath)
	}

	type envResult struct {
		name  string
		path  string
		found bool
	}

	results := make([]envResult, 0, len(adapters))
	for _, a := range adapters {
		path, found := a.Detect(homeDir)
		if !found {
			path = adapterDefaultPath(a.Name(), homeDir)
		}
		results = append(results, envResult{name: a.Name(), path: path, found: found})
	}

	valid, invalid, _ := skill.ReadAll(skillsDir)

	fmt.Fprintln(out, "=== AI Environments ===")
	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	for _, r := range results {
		mark := "✓"
		status := "found"
		if !r.found {
			mark = "✗"
			status = "not found"
		}
		fmt.Fprintf(w, "%s %s\t%s\t%s\n", mark, r.name, r.path, status)
	}
	w.Flush()

	fmt.Fprintln(out)
	fmt.Fprintln(out, "=== Skills ===")
	fmt.Fprintf(out, "Found: %d valid, %d invalid\n", len(valid), len(invalid))

	fmt.Fprintln(out)
	fmt.Fprintln(out, "=== Sync State ===")
	if cfg.Repo == "" {
		fmt.Fprintln(out, "repository not initialized")
	} else {
		syncedDisplay := "not set"
		if cfg.SyncedHash != "" {
			syncedDisplay = cfg.SyncedHash[:min7(cfg.SyncedHash)]
		}
		publishedDisplay := "not set"
		if cfg.PublishedHash != "" {
			publishedDisplay = cfg.PublishedHash[:min7(cfg.PublishedHash)]
		}
		remoteDisplay := "unreachable"
		syncStatus := "unknown"
		if remoteHash, err := gitops.New().RemoteHash(workDir, "origin/main"); err == nil {
			remoteDisplay = remoteHash[:min7(remoteHash)]
			if cfg.SyncedHash == "" {
				syncStatus = "not yet synced"
			} else if cfg.SyncedHash == remoteHash {
				syncStatus = "up-to-date"
			} else {
				syncStatus = "out of date"
			}
		}
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "synced_hash:\t%s\n", syncedDisplay)
		fmt.Fprintf(tw, "published_hash:\t%s\n", publishedDisplay)
		fmt.Fprintf(tw, "remote HEAD:\t%s\n", remoteDisplay)
		fmt.Fprintf(tw, "status:\t%s\n", syncStatus)
		tw.Flush()
	}

	var issues []string
	for _, r := range results {
		if !r.found {
			issues = append(issues, fmt.Sprintf("• %s: not installed or not found at %s", r.name, r.path))
		}
	}
	for _, ve := range invalid {
		issues = append(issues, fmt.Sprintf("• %s: invalid: %s", filepath.Base(ve.FilePath), formatValidationReason(ve)))
	}

	// MCP env section
	mcpItems, mcpErrs := mcp.ParseDir(mcpDir)
	for _, e := range mcpErrs {
		issues = append(issues, fmt.Sprintf("• %s", e))
	}
	if len(mcpItems) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "=== MCP Environment Variables ===")
		tw := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
		for _, item := range mcpItems {
			existing := cfg.GetMCPEnvForServer(item.Name)
			for _, ev := range item.Env {
				if !ev.Required {
					continue
				}
				mark := "✓"
				status := "set"
				if existing[ev.Name] == "" {
					mark = "✗"
					status = "missing (required)"
					issues = append(issues, fmt.Sprintf("• %s › %s — missing (required)", item.Name, ev.Name))
				}
				fmt.Fprintf(tw, "%s %s › %s\t— %s\n", mark, item.Name, ev.Name, status)
			}
		}
		tw.Flush()
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "=== Issues ===")
	if len(issues) == 0 {
		fmt.Fprintln(out, "No issues found.")
	} else {
		for _, issue := range issues {
			fmt.Fprintln(out, issue)
		}
	}

	return nil
}

func adapterDefaultPath(name, homeDir string) string {
	dotDir := "." + name
	if name == "claude-code" {
		dotDir = ".claude"
	}
	return filepath.Join(homeDir, dotDir)
}

func min7(s string) int {
	if len(s) < 7 {
		return len(s)
	}
	return 7
}
