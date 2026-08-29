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

// syncCleanPaths are the paths used for both git clean and the --force report.
// ADR-0003 5.3 (Н-5): single constant — report and clean always use identical paths.
var syncCleanPaths = []string{"skills/", "mcp/", "loadouts/"}

func runSync(dryRun, force bool, homeDir, skillsDir, mcpDir, workDir string, in io.Reader, out, errOut io.Writer) error {
	if _, err := os.Stat(filepath.Join(workDir, ".git")); err == nil {
		return runGitSync(dryRun, force, homeDir, mcpDir, workDir, gitops.New(), in, out, errOut)
	}
	return runLocalSync(dryRun, homeDir, skillsDir, mcpDir, workDir, in, out, errOut)
}

func runGitSync(dryRun, force bool, homeDir, mcpDir, workDir string, git gitops.Ops, in io.Reader, out, errOut io.Writer) error {
	cfg, err := localconfig.Load(workDir)
	if err != nil {
		return fmt.Errorf("cannot parse aim.local.yaml: %w", err)
	}
	// ADR-0006: read-only, no side effects — resolving/validating the pin
	// itself happens later, after fetch (and after ResetHard for a real run),
	// never before (see materializeSync doc comment).
	gcfg, _ := globalconfig.Load(homeDir)

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
	// ADR-0003 5.3: snapshot before clean, report after — same paths (Н-5).
	var forceDiscarded []string
	if dirtyUntracked && force {
		forceDiscarded, err = git.ListUntrackedInPaths(workDir, syncCleanPaths)
		if err != nil {
			return fmt.Errorf("cannot list untracked files: %w", err)
		}
		if err := git.CleanUntracked(workDir, syncCleanPaths); err != nil {
			return fmt.Errorf("git clean failed: %w", err)
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
			fmt.Fprintln(out, "[dry-run] nothing to sync — environments up to date with origin/main")
		} else {
			// ADR-0003 5.6 / 4.1: dry-run shows delta HEAD→origin/main, not full inventory.
			deltaLines, deltaErr := git.DiffSyncDelta(workDir)
			if deltaErr != nil {
				return fmt.Errorf("cannot compute sync delta: %w", deltaErr)
			}
			envs := countEnvs(homeDir, cfg)
			fmt.Fprintf(out, "[dry-run] would sync %s from origin/main → %s:\n",
				Plural(len(deltaLines), "change"),
				Plural(envs, "environment"))
			PrintDeltaBlock(out, deltaLines)
		}
		if gcfg.Loadout == "" {
			return nil
		}
		// ADR-0006 decision 5: pinned dry-run additionally shows the A/M/D
		// application plan — a second, separate block from the git transport
		// delta above. Computed from the inventory as it sits on disk right
		// now (dry-run never resets), against the current fetch state.
		res, err := materializeSync(true, gcfg, &cfg, homeDir, skillsDir, mcpDirFull, filepath.Join(workDir, "loadouts"), nil, in, out, errOut)
		if err != nil {
			return err
		}
		printPinnedDryRunPlan(res, out)
		return nil
	}

	// ADR-0003 5.1: snapshot delta before ResetHard — the diff is gone after reset.
	var syncDeltaLines []string
	if needsReset {
		syncDeltaLines, err = git.DiffSyncDelta(workDir)
		if err != nil {
			return fmt.Errorf("cannot compute sync delta: %w", err)
		}
	}

	if needsReset {
		if err := git.ResetHard(workDir, "origin/main"); err != nil {
			return fmt.Errorf("git reset --hard failed: %w", err)
		}
	}

	// ADR-0006: the pin is validated here, after ResetHard already ran, so a
	// loadout deleted/renamed in this very remote update is correctly seen as
	// missing rather than resolved against stale pre-reset state.
	res, materializeErr := materializeSync(false, gcfg, &cfg, homeDir, skillsDir, mcpDirFull, filepath.Join(workDir, "loadouts"), nil, in, out, errOut)

	hash, err := git.HeadHash(workDir)
	if err != nil {
		return fmt.Errorf("cannot get HEAD hash: %w", err)
	}

	combinedErr := materializeErr

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

	// ADR-0003 5.3: print --force report before success line.
	if len(forceDiscarded) > 0 {
		fmt.Fprintln(out, "discarded untracked files (--force):")
		PrintDeltaBlock(out, forceDiscarded)
	}

	fmt.Fprintln(out, FormatSuccess("synced", shortHash, res.SkillCount, res.MCPCount, res.EnvCount, res.MCPEnvNames))

	// ADR-0003 5.1: print delta block after success line (empty = no block).
	PrintDeltaBlock(out, syncDeltaLines)

	// ADR-0006 decision 5: pinned mode's D-report is a second, separate block
	// — the plan's own composition (application), never conflated with the
	// git transport delta printed above.
	if res.Pinned {
		PrintDeltaBlock(out, res.DeltaLines)
	}

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

	// ADR-0006: read-only, no side effects.
	gcfg, _ := globalconfig.Load(homeDir)

	if gcfg.Loadout != "" {
		return runLocalSyncPinned(dryRun, gcfg, cfg, homeDir, workDir, in, out, errOut)
	}

	// --- no pin: byte-for-byte the pre-ADR-0006 additive behavior ---

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
		// ADR-0003 5.6 / 4.4: consistent "nothing to …" phrasing.
		fmt.Fprintln(out, "nothing to sync")
		return nil
	}

	if dryRun {
		// ADR-0003 5.6 / 4.3: single [dry-run] line, not per-environment listing.
		envCount := countEnvs(homeDir, cfg)
		fmt.Fprintf(out, "[dry-run] would sync %s, %s → %s\n",
			Plural(len(valid), "skill"),
			Plural(len(mcpItems), "MCP server"),
			Plural(envCount, "environment"))
		return nil
	}

	// Reuse the skills/mcp already parsed above instead of having
	// materializeSync re-read the directory and re-print every warning.
	preRead := &inventoryPreRead{skills: valid, mcps: mcpItems}
	res, err := materializeSync(false, gcfg, &cfg, homeDir, skillsDir, mcpDir, "", preRead, in, out, errOut)
	if err != nil {
		return err
	}

	// ADR-0003 5.6 / 4.2: local-mode uses "synced:" not "applied:". Printed
	// unconditionally past the "nothing to sync" gate above, mirroring
	// runGitSync/runLocalSyncPinned/runApply: EnvCount==0 is a valid outcome
	// (e.g. every skill's item-level targets excluded every discovered
	// environment, ADR-0007 decision 6) and FormatSuccess already omits the
	// "→ N environments" segment in that case rather than needing a gate here.
	fmt.Fprintln(out, FormatSuccess("synced", "", res.SkillCount, res.MCPCount, res.EnvCount, res.MCPEnvNames))

	if res.CfgChanged {
		if err := localconfig.Save(workDir, cfg); err != nil {
			fmt.Fprintf(errOut, "warning: cannot save mcp_env: %v\n", err)
		}
	}

	return nil
}

// runLocalSyncPinned is the pin path of local-mode sync (ADR-0006). Local
// mode has no fetch/reset step, so "the pin is validated after the inventory
// update" collapses to "validated against whatever is on disk right now" —
// there is no earlier state to race against.
func runLocalSyncPinned(dryRun bool, gcfg globalconfig.Config, cfg localconfig.Config, homeDir, workDir string, in io.Reader, out, errOut io.Writer) error {
	skillsDirFull := filepath.Join(workDir, "skills")
	mcpDirFull := filepath.Join(workDir, "mcp")
	loadoutsDir := filepath.Join(workDir, "loadouts")

	res, err := materializeSync(dryRun, gcfg, &cfg, homeDir, skillsDirFull, mcpDirFull, loadoutsDir, nil, in, out, errOut)
	if err != nil {
		// Nothing has been written to environments or aim.local.yaml at this
		// point: a failed resolve/build returns before ExecuteReconcilePlan,
		// and an ExecuteReconcilePlan error is returned here before any Save
		// below — sync's partial-failure state is never persisted silently.
		return err
	}

	if dryRun {
		printPinnedDryRunPlan(res, out)
		return nil
	}

	// Success line = operation volume; delta block = composition (ADR-0003
	// 5.2). An empty plan still prints the line alone, mirroring
	// runApplyLoadout.
	fmt.Fprintln(out, FormatSuccess("synced", "", res.SkillCount, res.MCPCount, res.EnvCount, res.MCPEnvNames))
	PrintDeltaBlock(out, res.DeltaLines)

	// Persist resolved MCP env values, mirroring runApplyLoadout — only
	// reached on success (see the err != nil guard above).
	if err := localconfig.Save(workDir, cfg); err != nil {
		fmt.Fprintf(errOut, "warning: cannot save mcp_env: %v\n", err)
	}
	return nil
}

// installSkills reads skillsDir and installs every valid skill into every
// detected AI environment.
func installSkills(skillsDir string, cfg localconfig.Config, homeDir string, out, errOut io.Writer) (skillCount, envCount int, err error) {
	valid, invalid, readErr := skill.ReadAll(skillsDir)
	if readErr != nil {
		return 0, 0, fmt.Errorf("cannot read skills: %w", readErr)
	}
	for _, ve := range invalid {
		fmt.Fprintf(errOut, "warning: %s\n", ve)
	}
	return installSkillsInto(valid, cfg, homeDir, errOut)
}

// installSkillsInto installs an already-parsed skill list into every detected
// AI environment. Factored out of installSkills (ADR-0006) so a caller that
// already holds a parsed inventory — runLocalSync's pre-read for the
// "nothing to sync" gate — can install from it directly, instead of
// materializeSync re-reading skillsDir and re-printing every "invalid skill"
// warning a second time.
func installSkillsInto(valid []skill.Skill, cfg localconfig.Config, homeDir string, errOut io.Writer) (skillCount, envCount int, err error) {
	for _, a := range adapter.DefaultAdapters(cfg) {
		baseDir, found := a.Detect(homeDir)
		if !found {
			fmt.Fprintf(errOut, "warning: %s not found\n", a.Name())
			continue
		}
		installed := 0
		for _, s := range valid {
			// Item-level targets (ADR-0007 decision 2): empty/nil means "all
			// discovered environments" (unlike MCP, where targets is required).
			if len(s.Targets) > 0 && !containsTarget(s.Targets, a.Name()) {
				continue
			}
			if installErr := a.InstallSkill(s, baseDir); installErr != nil {
				return skillCount, envCount, fmt.Errorf("failed to install %s in %s: %w", s.Name, a.Name(), installErr)
			}
			installed++
		}
		if installed > 0 {
			envCount++
		}
	}
	return len(valid), envCount, nil
}

// installMCPs reads mcpDir and applies each MCP item to all matching
// adapters. Returns MCP item count, the names of environments actually
// installed into, and a non-nil error if any install failed. synced_hash must
// not be updated on error. Signature predates ADR-0006 and is kept as-is:
// runApply (apply.go) calls it directly and does not need the cfgChanged
// value installMCPsInto also tracks (see materializeAdditive, which calls
// installMCPsInto directly to get it).
func installMCPs(mcpDir string, cfg *localconfig.Config, homeDir string, in io.Reader, out, errOut io.Writer) (mcpCount int, envNames []string, err error) {
	items, parseErrs := mcp.ParseDir(mcpDir)
	for _, e := range parseErrs {
		fmt.Fprintf(errOut, "warning: %v\n", e)
	}
	mcpCount, envNames, _, err = installMCPsInto(items, cfg, homeDir, in, out, errOut)
	return mcpCount, envNames, err
}

// installMCPsInto installs an already-parsed MCP item list, mirroring
// installSkillsInto's split from installSkills for the same reason (ADR-0006).
func installMCPsInto(items []mcp.MCP, cfg *localconfig.Config, homeDir string, in io.Reader, out, errOut io.Writer) (mcpCount int, envNames []string, cfgChanged bool, err error) {
	if len(items) == 0 {
		return 0, nil, false, nil
	}
	mcpCount = len(items)

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
				cfgChanged = true
			}
			if installErr := a.InstallMCP(item, baseDir, resolved); installErr != nil {
				fmt.Fprintf(errOut, "warning: failed to install MCP %s in %s: %v\n", item.Name, a.Name(), installErr)
				hadInstallError = true
				continue
			}
			installed++
		}
		if installed > 0 {
			envNames = append(envNames, a.Name())
		}
	}
	if hadInstallError {
		return mcpCount, envNames, cfgChanged, fmt.Errorf("one or more MCP servers failed to install; synced_hash not updated")
	}
	return mcpCount, envNames, cfgChanged, nil
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

// countEnvs returns the number of detected AI environments for the given config.
// Used by dry-run paths to show "→ K environments" without actually installing.
func countEnvs(homeDir string, cfg localconfig.Config) int {
	count := 0
	for _, a := range adapter.DefaultAdapters(cfg) {
		if _, found := a.Detect(homeDir); found {
			count++
		}
	}
	return count
}
