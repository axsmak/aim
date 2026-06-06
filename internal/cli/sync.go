package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/axsmak/aim/internal/adapter"
	"github.com/axsmak/aim/internal/errs"
	"github.com/axsmak/aim/internal/gitops"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
	"github.com/spf13/cobra"
)

func newSyncCmd() *cobra.Command {
	var dryRun bool
	var force bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Apply skills and MCP servers from the library to AI environments",
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				errs.Fatalf("cannot determine home directory: %v", err)
			}
			workDir := resolveWorkDir(homeDir)
			return runSync(dryRun, force, homeDir, "skills/", "mcp/", workDir, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be installed without making changes")
	cmd.Flags().BoolVar(&force, "force", false, "remove untracked files in managed paths before syncing (tracked or staged changes always block)")
	return cmd
}

func runSync(dryRun, force bool, homeDir, skillsDir, mcpDir, workDir string, in io.Reader, out, errOut io.Writer) error {
	if _, err := os.Stat(filepath.Join(workDir, ".git")); err == nil {
		return runGitSync(dryRun, force, homeDir, mcpDir, workDir, in, out, errOut)
	}
	return runLocalSync(dryRun, homeDir, skillsDir, mcpDir, workDir, in, out, errOut)
}

func runGitSync(dryRun, force bool, homeDir, mcpDir, workDir string, in io.Reader, out, errOut io.Writer) error {
	cfg, err := localconfig.Load(workDir)
	if err != nil {
		return fmt.Errorf("cannot parse aim.local.yaml: %w", err)
	}

	git := gitops.New()

	dirtyTracked, err := git.HasDirtyWorktree(workDir)
	if err != nil {
		return fmt.Errorf("cannot check dirty state: %w", err)
	}
	dirtyUntracked, err := git.HasUntrackedInPaths(workDir, gitops.ManagedPaths)
	if err != nil {
		return fmt.Errorf("cannot check untracked files: %w", err)
	}
	if dirtyTracked {
		return fmt.Errorf("tracked or staged changes detected — commit, stash, or push before sync")
	}
	if dirtyUntracked && force {
		cmd := exec.Command("git", "-C", workDir, "clean", "-fd", "--", "skills/", "mcp/")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git clean failed: %w\n%s", err, out)
		}
		dirtyUntracked = false
	}

	if err := git.Fetch(workDir); err != nil {
		return fmt.Errorf("cannot reach remote repository: %w", err)
	}

	remoteHash, err := git.LsRemote(workDir, "refs/heads/main")
	if err != nil {
		return fmt.Errorf("cannot reach remote repository: %w", err)
	}
	if remoteHash == "" {
		return fmt.Errorf("remote has no published state yet — run aiman push first")
	}

	headHash, err := git.HeadHash(workDir)
	if err != nil {
		return fmt.Errorf("cannot get HEAD: %w", err)
	}
	originHash, err := git.RemoteHash(workDir, "origin/main")
	if err != nil {
		return fmt.Errorf("cannot get origin/main: %w", err)
	}

	headIsAncestor, err := git.IsAncestor(workDir, headHash, originHash)
	if err != nil {
		return fmt.Errorf("cannot determine history relationship: %w", err)
	}
	originIsAncestor, err := git.IsAncestor(workDir, originHash, headHash)
	if err != nil {
		return fmt.Errorf("cannot determine history relationship: %w", err)
	}

	needsReset := false
	switch {
	case headHash == originHash:
		// HEAD == origin/main: already up-to-date, apply without reset
	case headIsAncestor:
		// HEAD is ancestor of origin/main: fast-forward
		needsReset = true
	case originIsAncestor:
		return fmt.Errorf("local commits are not published — run: aiman push, or reset manually")
	default:
		return fmt.Errorf("history diverged — resolve with git manually, then run: aiman sync")
	}

	// git reset --hard preserves untracked files unless they exist in the incoming ref.
	// Only check for conflicts when a reset is actually needed.
	if needsReset && dirtyUntracked {
		conflicts, conflictErr := git.UntrackedConflictsWithRef(workDir, "origin/main", gitops.ManagedPaths)
		if conflictErr != nil {
			return fmt.Errorf("cannot check untracked conflicts: %w", conflictErr)
		}
		if len(conflicts) > 0 {
			return fmt.Errorf("untracked files would be overwritten by remote: %s\n"+
				"rename them or use --force to discard local copies", strings.Join(conflicts, ", "))
		}
	}

	skillsDir := filepath.Join(workDir, "skills")
	mcpDirFull := filepath.Join(workDir, mcpDir)

	if dryRun {
		if headHash == originHash {
			fmt.Fprintln(out, "[dry-run] aiman sync: already up to date — nothing to apply")
			return nil
		}
		shortHash := originHash
		if len(shortHash) > 7 {
			shortHash = shortHash[:7]
		}
		fmt.Fprintf(out, "[dry-run] would apply: %s\n", shortHash)
		fmt.Fprintln(out, "Current local inventory (before reset):")
		skills, _, err := skill.ReadAll(skillsDir)
		if err == nil && len(skills) > 0 {
			fmt.Fprintf(out, "Skills (%d):\n", len(skills))
			for _, s := range skills {
				fmt.Fprintf(out, "  - %s\n", s.Name)
			}
		}
		mcpItems, _ := mcp.ParseDir(mcpDirFull)
		if len(mcpItems) > 0 {
			fmt.Fprintf(out, "MCP servers (%d):\n", len(mcpItems))
			for _, m := range mcpItems {
				envStatus := mcpEnvStatus(m, cfg)
				fmt.Fprintf(out, "  - %s → %s  %s\n", m.Name, strings.Join(m.Targets, ", "), envStatus)
			}
		}
		return nil
	}

	if needsReset {
		if err := git.ResetHard(workDir, "origin/main"); err != nil {
			return fmt.Errorf("git reset --hard failed: %w", err)
		}
	}

	skillCount, envCount, installErr := installSkills(skillsDir, cfg, homeDir, out, errOut)

	mcpCount, mcpEnvCount, mcpErr := installMCPs(mcpDirFull, &cfg, homeDir, in, out, errOut)

	hash, err := git.HeadHash(workDir)
	if err != nil {
		return fmt.Errorf("cannot get HEAD hash: %w", err)
	}

	combinedErr := installErr
	if combinedErr == nil {
		combinedErr = mcpErr
	}

	if combinedErr == nil {
		// synced_hash is written only after ResetHard succeeds, so it reliably
		// reflects the local HEAD. runPush uses this to allow push after sync
		// without requiring a prior publish from this machine (see issue #65).
		cfg.SyncedHash = hash
		if err := localconfig.Save(workDir, cfg); err != nil {
			fmt.Fprintf(errOut, "warning: cannot save synced_hash: %v\n", err)
		}
	}

	shortHash := hash
	if len(shortHash) > 7 {
		shortHash = shortHash[:7]
	}
	if combinedErr != nil {
		return combinedErr
	}
	fmt.Fprintf(out, "Synced: %s — %d skills, %d MCP servers in %d environments\n", shortHash, skillCount, mcpCount, max(envCount, mcpEnvCount))
	return nil
}

func runLocalSync(dryRun bool, homeDir, skillsDir, mcpDir, workDir string, in io.Reader, out, errOut io.Writer) error {
	cfg, err := localconfig.Load(workDir)
	if err != nil {
		return fmt.Errorf("cannot parse aim.local.yaml: %w", err)
	}

	if _, statErr := os.Stat(filepath.Join(workDir, "aim.local.yaml")); statErr == nil {
		if !isInGitignore(workDir, "aim.local.yaml") {
			fmt.Fprintln(errOut, "warning: aim.local.yaml is not in .gitignore — local config may be committed accidentally")
		}
	}

	valid, invalid, err := skill.ReadAll(skillsDir)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", skillsDir, err)
	}

	for _, ve := range invalid {
		fmt.Fprintf(errOut, "warning: %s\n", ve)
	}

	mcpItems, mcpErrs := mcp.ParseDir(mcpDir)
	for _, e := range mcpErrs {
		fmt.Fprintf(errOut, "warning: %v\n", e)
	}

	if len(valid) == 0 && len(mcpItems) == 0 {
		fmt.Fprintln(out, "no valid skills or MCP servers to install")
		return nil
	}

	cfgChanged := false

	for _, a := range adapter.DefaultAdapters(cfg) {
		baseDir, found := a.Detect(homeDir)
		if !found {
			fmt.Fprintf(errOut, "warning: %s not found\n", a.Name())
			continue
		}

		if dryRun {
			if len(valid) > 0 {
				fmt.Fprintf(out, "[dry-run] would install in %s (%d skills):\n", a.Name(), len(valid))
				for _, s := range valid {
					fmt.Fprintf(out, "  - %s\n", s.Name)
				}
			}
			if len(mcpItems) > 0 {
				for _, m := range mcpItems {
					if !containsTarget(m.Targets, a.Name()) {
						continue
					}
					envStatus := mcpEnvStatus(m, cfg)
					fmt.Fprintf(out, "[dry-run] MCP %s → %s  %s\n", m.Name, a.Name(), envStatus)
				}
			}
			continue
		}

		for _, s := range valid {
			if err := a.InstallSkill(s, baseDir); err != nil {
				return fmt.Errorf("failed to install %s in %s: %w", s.Name, a.Name(), err)
			}
		}

		for _, item := range mcpItems {
			if !containsTarget(item.Targets, a.Name()) {
				continue
			}
			existing := cfg.GetMCPEnvForServer(item.Name)
			resolved, changed, err := mcp.ResolveEnv(item, existing, in, out)
			if err != nil {
				fmt.Fprintf(errOut, "warning: env resolution for %s: %v\n", item.Name, err)
			}
			if changed {
				for k, v := range resolved {
					cfg.SetMCPEnv(item.Name, k, v)
				}
				cfgChanged = true
			}
			if err := a.InstallMCP(item, baseDir, resolved); err != nil {
				fmt.Fprintf(errOut, "warning: failed to install MCP %s in %s: %v\n", item.Name, a.Name(), err)
				continue
			}
		}
		if len(valid) > 0 {
			fmt.Fprintf(out, "applied: %d skills in %s\n", len(valid), a.Name())
		}
		if len(mcpItems) > 0 {
			fmt.Fprintf(out, "applied: %d MCP servers in %s\n", len(mcpItems), a.Name())
		}
	}

	if cfgChanged {
		if err := localconfig.Save(workDir, cfg); err != nil {
			fmt.Fprintf(errOut, "warning: cannot save mcp_env: %v\n", err)
		}
	}

	return nil
}

// installSkills installs all valid skills into all detected AI environments.
func installSkills(skillsDir string, cfg localconfig.Config, homeDir string, out, errOut io.Writer) (skillCount, envCount int, err error) {
	valid, invalid, readErr := skill.ReadAll(skillsDir)
	if readErr != nil {
		return 0, 0, fmt.Errorf("cannot read skills: %w", readErr)
	}
	for _, ve := range invalid {
		fmt.Fprintf(errOut, "warning: %s\n", ve)
	}

	for _, a := range adapter.DefaultAdapters(cfg) {
		baseDir, found := a.Detect(homeDir)
		if !found {
			fmt.Fprintf(errOut, "warning: %s not found\n", a.Name())
			continue
		}
		for _, s := range valid {
			if installErr := a.InstallSkill(s, baseDir); installErr != nil {
				return skillCount, envCount, fmt.Errorf("failed to install %s in %s: %w", s.Name, a.Name(), installErr)
			}
		}
		envCount++
	}
	return len(valid), envCount, nil
}

// installMCPs reads mcp/ directory and applies each MCP item to all matching adapters.
// Returns MCP item count, env count, and a non-nil error if any install failed.
// synced_hash must not be updated when this returns an error.
func installMCPs(mcpDir string, cfg *localconfig.Config, homeDir string, in io.Reader, out, errOut io.Writer) (mcpCount, envCount int, err error) {
	items, parseErrs := mcp.ParseDir(mcpDir)
	for _, e := range parseErrs {
		fmt.Fprintf(errOut, "warning: %v\n", e)
	}
	if len(items) == 0 {
		return 0, 0, nil
	}

	var hadInstallError bool
	for _, a := range adapter.DefaultAdapters(*cfg) {
		baseDir, found := a.Detect(homeDir)
		if !found {
			continue
		}
		installed := 0
		for _, item := range items {
			if !containsTarget(item.Targets, a.Name()) {
				continue
			}
			existing := cfg.GetMCPEnvForServer(item.Name)
			resolved, changed, resolveErr := mcp.ResolveEnv(item, existing, in, out)
			if resolveErr != nil {
				fmt.Fprintf(errOut, "warning: env resolution for %s: %v\n", item.Name, resolveErr)
			}
			if changed {
				for k, v := range resolved {
					cfg.SetMCPEnv(item.Name, k, v)
				}
			}
			if installErr := a.InstallMCP(item, baseDir, resolved); installErr != nil {
				fmt.Fprintf(errOut, "warning: failed to install MCP %s in %s: %v\n", item.Name, a.Name(), installErr)
				hadInstallError = true
				continue
			}
			installed++
		}
		if installed > 0 {
			envCount++
			mcpCount = len(items)
		}
	}
	if hadInstallError {
		return mcpCount, envCount, fmt.Errorf("one or more MCP servers failed to install; synced_hash not updated")
	}
	return mcpCount, envCount, nil
}

func mcpEnvStatus(m mcp.MCP, cfg localconfig.Config) string {
	existing := cfg.GetMCPEnvForServer(m.Name)
	var missing []string
	for _, ev := range m.Env {
		if ev.Required {
			if existing[ev.Name] == "" {
				missing = append(missing, ev.Name)
			}
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("[missing env: %s]", strings.Join(missing, ", "))
}

func containsTarget(targets []string, name string) bool {
	for _, t := range targets {
		if t == name {
			return true
		}
	}
	return false
}

func isInGitignore(workDir, filename string) bool {
	f, err := os.Open(filepath.Join(workDir, ".gitignore"))
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == filename {
			return true
		}
	}
	return false
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
