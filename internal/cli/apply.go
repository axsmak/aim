package cli

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/axsmak/aim/internal/adapter"
	"github.com/axsmak/aim/internal/errs"
	"github.com/axsmak/aim/internal/loadout"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
	"github.com/spf13/cobra"
)

func newApplyCmd() *cobra.Command {
	var dryRun bool
	var loadoutName string
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply local inventory working tree to AI environments without publishing",
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				errs.Fatalf("cannot determine home directory: %v", err)
			}
			workDir := resolveWorkDir(homeDir)
			if loadoutName != "" {
				return runApplyLoadout(loadoutName, dryRun, homeDir, workDir, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			}
			return runApply(dryRun, homeDir, workDir, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be applied without making changes")
	cmd.Flags().StringVar(&loadoutName, "loadout", "", "apply the named loadout declaratively: environments are reconciled exactly to its set within the AIM-managed namespace")
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

	// Resolve detected adapters before installation so computeApplyDelta can be
	// called with the same adapter set used by installSkills.
	var detectedAdapters []adapter.Adapter
	var detectedBaseDirs []string
	for _, a := range adapter.DefaultAdapters(cfg) {
		baseDir, found := a.Detect(homeDir)
		if !found {
			continue
		}
		detectedAdapters = append(detectedAdapters, a)
		detectedBaseDirs = append(detectedBaseDirs, baseDir)
	}

	// Read valid skills and compute delta BEFORE installation (ADR-0003 5.1).
	// Warnings for invalid skills are printed by installSkills; suppress them here
	// to avoid double-printing.
	valid, _, skillReadErr := skill.ReadAll(skillsDir)
	if skillReadErr != nil {
		return fmt.Errorf("cannot read skills: %w", skillReadErr)
	}
	deltaLines := computeApplyDelta(valid, detectedAdapters, detectedBaseDirs, false)

	skillCount, envCount, installErr := installSkills(skillsDir, cfg, homeDir, out, errOut)
	mcpCount, mcpEnvNames, mcpErr := installMCPs(mcpDirFull, &cfg, homeDir, in, out, errOut)

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
	fmt.Fprintln(out, FormatSuccess("applied", "", skillCount, mcpCount, envCount, mcpEnvNames))
	// Print delta block only when something changed (ADR-0003 5.1).
	PrintDeltaBlock(out, deltaLines)
	return nil
}

// skillDelta holds the change category and list of envs where the skill differs.
type skillDelta struct {
	category string // "A" = new, "M" = modified
	envNames []string
}

// computeApplyDelta returns formatted delta lines for skills that differ from the
// installed state in the detected adapter environments.
//
// Each line is formatted as "A skills/x.md   (new in all environments)" or
// "M skills/y.md   (updated in claude-code, cursor)" for real runs, and
// "M skills/y.md   (differs in claude-code, cursor)" for dry-runs.
//
// The function is called BEFORE installation so that the snapshot reflects
// the pre-install state. It is reused by both runApply and runApplyDryRun.
//
// ADR-0003 5.1: category D (deletion from env) stays out of scope HERE — the
// ordinary apply path is additive (A/M only). ADR-0004 (decision 7) revises
// that record exclusively for the loadout path: apply --loadout gets D through
// the reconciliation engine (BuildReconcilePlan in reconcile.go), which shares
// the sha256 comparison below via skillEnvCategory.
func computeApplyDelta(validSkills []skill.Skill, adapters []adapter.Adapter, baseDirs []string, dryRun bool) []string {
	if len(adapters) == 0 {
		return nil
	}
	totalEnvs := len(adapters)

	deltaMap := make(map[string]*skillDelta)
	for _, s := range validSkills {
		// Note: for folder-format skills, only SKILL.md is hashed; reference files
		// are not tracked by this delta. A changed reference file will not be detected
		// here. See issue #138 for a planned follow-up.
		inventoryHash := sha256.Sum256(s.Raw)
		for i, a := range adapters {
			switch skillEnvCategory(baseDirs[i], s.Name, inventoryHash) {
			case "A":
				// Skill not present in this env. An entry that already exists
				// keeps its category (first-seen wins, as before the refactor).
				d := deltaMap[s.Name]
				if d == nil {
					d = &skillDelta{category: "A"}
					deltaMap[s.Name] = d
				}
				d.envNames = append(d.envNames, a.Name())
			case "M":
				d := deltaMap[s.Name]
				if d == nil {
					d = &skillDelta{category: "M"}
					deltaMap[s.Name] = d
				}
				d.envNames = append(d.envNames, a.Name())
			}
		}
	}

	if len(deltaMap) == 0 {
		return nil
	}

	var skillNames []string
	for name := range deltaMap {
		skillNames = append(skillNames, name)
	}
	sort.Strings(skillNames)

	var lines []string
	for _, name := range skillNames {
		d := deltaMap[name]
		var envDesc string
		if len(d.envNames) == totalEnvs {
			envDesc = "all environments"
		} else {
			envDesc = strings.Join(d.envNames, ", ")
		}
		switch d.category {
		case "A":
			lines = append(lines, fmt.Sprintf("A skills/%s.md   (new in %s)", name, envDesc))
		case "M":
			qualifier := "updated in"
			if dryRun {
				qualifier = "differs in"
			}
			lines = append(lines, fmt.Sprintf("M skills/%s.md   (%s %s)", name, qualifier, envDesc))
		}
	}
	return lines
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

	// Reuse shared delta computation (dry-run qualifier: "differs in").
	skillLines := computeApplyDelta(valid, detectedAdapters, detectedBaseDirs, true)

	// Build MCP lines (5.5: list target envs, no unverified status claim).
	type mcpLine struct {
		name   string
		envs   string // comma-separated targeted env names
		envTag string // optional [missing env: ...] suffix
	}
	var mcpLines []mcpLine
	for _, m := range mcpItems {
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
		mcpLines = append(mcpLines, mcpLine{name: m.Name, envs: strings.Join(targetedEnvs, ", "), envTag: envTag})
	}

	// If no changes and no MCPs to report, output "nothing to apply".
	if len(skillLines) == 0 && len(mcpLines) == 0 {
		fmt.Fprintln(out, "[dry-run] nothing to apply — environments match local inventory")
		return nil
	}

	// Build summary.
	totalEnvCount := len(detectedNames)
	envList := strings.Join(detectedNames, ", ")

	if len(skillLines) > 0 {
		changeCount := len(skillLines)
		// ADR-0003 5.6 (4.7): use Plural helper instead of (s) suffix.
		fmt.Fprintf(out, "[dry-run] would apply %s to %s (%s):\n",
			Plural(changeCount, "change"),
			Plural(totalEnvCount, "environment"),
			envList,
		)
		PrintDeltaBlock(out, skillLines)
	}

	// 5.5: print "[dry-run] MCP <name> → <env1>, <env2>" without status claim.
	for _, ml := range mcpLines {
		if ml.envTag != "" {
			fmt.Fprintf(out, "[dry-run] MCP %s → %s  %s\n", ml.name, ml.envs, ml.envTag)
		} else {
			fmt.Fprintf(out, "[dry-run] MCP %s → %s\n", ml.name, ml.envs)
		}
	}

	return nil
}

// runApplyLoadout is the loadout path of apply (ADR-0004): every admissible
// environment is brought exactly to the loadout set within the AIM-managed
// namespace, via the reconciliation engine in reconcile.go. This is the only
// apply path where category D (removal from env) exists; plain apply stays
// additive (ADR-0003 5.1).
func runApplyLoadout(name string, dryRun bool, homeDir, workDir string, in io.Reader, out, errOut io.Writer) error {
	cfg, err := localconfig.Load(workDir)
	if err != nil {
		return fmt.Errorf("cannot parse aim.local.yaml: %w", err)
	}

	loadoutsDir := filepath.Join(workDir, "loadouts")
	lo, valErrs, warns, err := loadout.Resolve(loadoutsDir, name)
	if err != nil {
		var nf loadout.NotFoundError
		if errors.As(err, &nf) {
			// A missing loadouts/ directory resolves to the same not-found
			// class: Resolve finds no candidate files there.
			printAvailableLoadoutsHint(loadoutsDir, errOut)
			// nf.Error() is `loadout "X" not found in loadouts/`; main
			// prefixes "error: " and exits 1 (errs.Fatal format, US-L05).
			return nf
		}
		return fmt.Errorf("cannot read loadout %q: %w", name, err)
	}
	for _, w := range warns {
		fmt.Fprintf(errOut, "warning: %s\n", w)
	}
	if len(valErrs) > 0 {
		// Fail fast on the first finding (apply contract, US-L05); push is
		// the path that reports every finding at once (US-L04).
		ve := valErrs[0]
		return fmt.Errorf("loadout %q: %s: %s", name, ve.Field, ve.Message)
	}

	inv, invWarnings, err := LoadReconcileInventory(filepath.Join(workDir, "skills"), filepath.Join(workDir, "mcp"))
	if err != nil {
		return err
	}
	for _, w := range invWarnings {
		fmt.Fprintf(errOut, "warning: %s\n", w)
	}

	envs := DetectReconcileEnvs(cfg, homeDir)
	plan, err := BuildReconcilePlan(lo, inv, envs)
	if err != nil {
		return err
	}
	// Loadout entries without a valid inventory element behind them (deleted,
	// renamed, or currently invalid) are neither installed nor removed; they
	// must not be dropped silently.
	for _, ref := range plan.MissingRefs {
		fmt.Fprintf(errOut, "warning: loadout %q: no valid inventory item for %s (skipped)\n", lo.Name, ref)
	}

	if dryRun {
		printLoadoutDryRun(lo, plan, out)
		return nil
	}

	res, execErr := ExecuteReconcilePlan(plan, &cfg, in, out, errOut)
	// Save resolved MCP env values, mirroring runApply.
	// IMPORTANT: synced_hash and published_hash are never touched here.
	if err := localconfig.Save(workDir, cfg); err != nil {
		fmt.Fprintf(errOut, "warning: cannot save mcp_env: %v\n", err)
	}
	if execErr != nil {
		return execErr
	}

	// Success line counters = operation volume; composition lives in the
	// delta block (ADR-0003 5.2). An empty plan prints the line alone.
	verb := fmt.Sprintf("applied loadout %q", lo.Name)
	fmt.Fprintln(out, FormatSuccess(verb, "", res.SkillCount, res.MCPCount, res.EnvCount, res.MCPEnvNames))
	PrintDeltaBlock(out, res.DeltaLines)
	return nil
}

// printLoadoutDryRun renders the full A/M/D plan without touching anything:
// building the plan performed no writes, and the caller saves no config on
// this path (ADR-0004 decision 8 — dry-run instead of interactive prompts).
func printLoadoutDryRun(lo loadout.Loadout, plan *ReconcilePlan, out io.Writer) {
	if plan.Empty() {
		fmt.Fprintf(out, "[dry-run] nothing to apply — environments match loadout %q\n", lo.Name)
		return
	}
	envNames := plan.EnvNames()
	fmt.Fprintf(out, "[dry-run] would apply loadout %q — %s to %s (%s):\n",
		lo.Name,
		Plural(len(plan.Actions), "change"),
		Plural(len(envNames), "environment"),
		strings.Join(envNames, ", "),
	)
	PrintDeltaBlock(out, plan.DeltaLines(true))
}

// printAvailableLoadoutsHint lists the valid loadouts when there are five or
// fewer (US-L05). With none — e.g. loadouts/ missing or empty — there is
// nothing useful to suggest, so no hint is printed.
func printAvailableLoadoutsHint(loadoutsDir string, errOut io.Writer) {
	valid, _, _, err := loadout.List(loadoutsDir)
	if err != nil || len(valid) == 0 || len(valid) > 5 {
		return
	}
	names := make([]string, 0, len(valid))
	for _, l := range valid {
		names = append(names, l.Name)
	}
	fmt.Fprintf(errOut, "hint: available loadouts: %s\n", strings.Join(names, ", "))
}
