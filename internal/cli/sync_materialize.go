package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/axsmak/aim/internal/globalconfig"
	"github.com/axsmak/aim/internal/loadout"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
)

// Shared sync materialization (ADR-0006 decision 2).
//
// Before this file, the git branch of sync (runGitSync) installed the full
// inventory additively via installSkills/installMCPs, while the local branch
// (runLocalSync) held its own separate install loop that duplicated the same
// logic a third time (alongside apply's computeApplyDelta path) and never
// computed a delta at all. Pin is a property of materialization, not of
// transport, so both branches must apply it identically — hence a single
// entry point, materializeSync, used by both.
//
// materializeSync branches on the pin exactly like the table in ADR-0006
// decision 2:
//   - Loadout == ""  -> byte-for-byte the pre-existing additive logic (A/M,
//     no D), via installSkills/installMCPs (or their *Into variants when the
//     caller already parsed the inventory — see inventoryPreRead).
//   - Loadout != ""  -> loadout.Resolve -> LoadReconcileInventory ->
//     DetectReconcileEnvs -> BuildReconcilePlan -> ExecuteReconcilePlan,
//     mirroring runApplyLoadout (apply.go) call for call.
//
// The caller is responsible for calling materializeSync at the right point
// in its own flow: AFTER the inventory has been brought up to date (git:
// after fetch, since ls-remote/ancestor checks already needed it, and after
// ResetHard for a real run; local: there is no fetch, so "current on-disk
// state" already satisfies the ordering requirement) — never before, so a
// loadout deleted in the very same remote update is correctly detected as
// missing (ADR-0006, "Порядок валидации").

// inventoryPreRead lets a caller that already parsed the inventory (e.g.
// runLocalSync's "nothing to sync" / dry-run gate) hand the parsed lists to
// the additive path directly, instead of materializeSync re-reading the
// directory and re-printing every diagnostic a second time. Pass nil when no
// such read exists (runGitSync never pre-reads).
type inventoryPreRead struct {
	skills []skill.Skill
	mcps   []mcp.MCP
}

// materializeResult carries everything a caller needs to render output,
// covering both the additive and pinned shapes.
type materializeResult struct {
	// Operation volume, populated by both paths the way FormatSuccess wants
	// it (ADR-0003 5.2): how many total, not what changed.
	SkillCount  int
	MCPCount    int
	EnvCount    int
	MCPEnvNames []string
	// CfgChanged reports whether ResolveEnv produced new mcp_env values that
	// the caller must persist. Additive-path only; the pinned path's caller
	// always persists cfg after a successful execute (mirroring
	// runApplyLoadout), so it does not need this flag.
	CfgChanged bool

	// Pinned-mode-only fields (Pinned is false for the additive path).
	Pinned       bool
	LoadoutName  string
	PlanEnvNames []string // environments the plan covers (for the dry-run message)
	Empty        bool     // plan.Empty(): no A/M/D changes at all
	// DeltaLines is the plan's own A/M/D composition (plan.DeltaLines), i.e.
	// the composition of the *application* — never git.DiffSyncDelta, which
	// is the composition of the *transport*. The two must never be merged
	// into one block (ADR-0006 decision 5).
	DeltaLines []string
}

// materializeSync is the single entry point used by both runGitSync and
// runLocalSync.
func materializeSync(
	dryRun bool,
	gcfg globalconfig.Config,
	cfg *localconfig.Config,
	homeDir, skillsDir, mcpDir, loadoutsDir string,
	preRead *inventoryPreRead,
	in io.Reader, out, errOut io.Writer,
) (materializeResult, error) {
	if gcfg.Loadout == "" {
		return materializeAdditive(cfg, homeDir, skillsDir, mcpDir, preRead, in, out, errOut)
	}
	return materializePinned(dryRun, gcfg.Loadout, cfg, homeDir, skillsDir, mcpDir, loadoutsDir, in, out, errOut)
}

// materializeAdditive is the no-pin path: install the full inventory into
// every detected environment (A/M only, no D). Both installs are attempted
// unconditionally, mirroring the pre-ADR-0006 git branch exactly (a skill
// install failure does not skip the MCP install attempt); the first non-nil
// error takes precedence, also mirroring the pre-existing combinedErr order.
func materializeAdditive(cfg *localconfig.Config, homeDir, skillsDir, mcpDir string, preRead *inventoryPreRead, in io.Reader, out, errOut io.Writer) (materializeResult, error) {
	var skillCount, envCount int
	var skillErr error
	if preRead != nil {
		skillCount, envCount, skillErr = installSkillsInto(preRead.skills, *cfg, homeDir, errOut)
	} else {
		skillCount, envCount, skillErr = installSkills(skillsDir, *cfg, homeDir, out, errOut)
	}

	// installMCPsInto (not installMCPs) so cfgChanged is available here too —
	// installMCPs keeps its pre-ADR-0006 3-value signature because runApply
	// (apply.go) calls it directly and does not need this flag.
	var mcpItems []mcp.MCP
	if preRead != nil {
		mcpItems = preRead.mcps
	} else {
		var parseErrs []error
		mcpItems, parseErrs = mcp.ParseDir(mcpDir)
		for _, e := range parseErrs {
			fmt.Fprintf(errOut, "warning: %v\n", e)
		}
	}
	mcpCount, mcpEnvNames, cfgChanged, mcpErr := installMCPsInto(mcpItems, cfg, homeDir, in, out, errOut)

	res := materializeResult{
		SkillCount:  skillCount,
		MCPCount:    mcpCount,
		EnvCount:    envCount,
		MCPEnvNames: mcpEnvNames,
		CfgChanged:  cfgChanged,
	}

	if skillErr != nil {
		return res, skillErr
	}
	return res, mcpErr
}

// materializePinned is the pin path: resolve the loadout, build the
// reconciliation plan, and (unless dryRun) execute it. Sequence mirrors
// runApplyLoadout (apply.go) call for call: resolve -> warnings -> inventory
// -> plan -> MissingRefs warnings -> (dry-run: return) -> execute.
func materializePinned(dryRun bool, name string, cfg *localconfig.Config, homeDir, skillsDir, mcpDir, loadoutsDir string, in io.Reader, out, errOut io.Writer) (materializeResult, error) {
	res := materializeResult{Pinned: true, LoadoutName: name}

	lo, err := resolvePinnedLoadout(loadoutsDir, name, errOut)
	if err != nil {
		return res, err
	}
	res.LoadoutName = lo.Name

	inv, invWarnings, err := LoadReconcileInventory(skillsDir, mcpDir)
	if err != nil {
		return res, err
	}
	for _, w := range invWarnings {
		fmt.Fprintf(errOut, "warning: %s\n", w)
	}

	envs := DetectReconcileEnvs(*cfg, homeDir)
	plan, err := BuildReconcilePlan(lo, inv, envs)
	if err != nil {
		return res, err
	}
	// Loadout entries without a valid inventory element behind them (deleted,
	// renamed, or currently invalid) are neither installed nor removed; sync
	// must not drop them silently (ADR-0006, MissingRefs handling).
	for _, ref := range plan.MissingRefs {
		fmt.Fprintf(errOut, "warning: loadout %q: no valid inventory item for %s (skipped)\n", lo.Name, ref)
	}

	res.PlanEnvNames = plan.EnvNames()
	res.Empty = plan.Empty()

	if dryRun {
		// plan-only: no writes to environments or config (ADR-0006 decision 5).
		res.DeltaLines = plan.DeltaLines(true)
		return res, nil
	}

	// ADR-0006 decision 5: observability is a hard requirement, not polish —
	// printed before any success line the caller renders.
	fmt.Fprintf(out, "applying loadout %q (pinned)\n", lo.Name)

	execRes, execErr := ExecuteReconcilePlan(plan, cfg, in, out, errOut)
	res.SkillCount = execRes.SkillCount
	res.MCPCount = execRes.MCPCount
	res.EnvCount = execRes.EnvCount
	res.MCPEnvNames = execRes.MCPEnvNames
	res.DeltaLines = execRes.DeltaLines
	return res, execErr
}

// resolvePinnedLoadout resolves and validates the pinned loadout name against
// loadoutsDir. Its error text is deliberately different from
// loadout.NotFoundError's message (`loadout "X" not found in loadouts/`, used
// by the apply --loadout path) so the two error sources stay distinguishable
// in logs and error handling (ADR-0006, Technical Details).
func resolvePinnedLoadout(loadoutsDir, name string, errOut io.Writer) (loadout.Loadout, error) {
	lo, valErrs, warns, err := loadout.Resolve(loadoutsDir, name)
	if err != nil {
		var nf loadout.NotFoundError
		if errors.As(err, &nf) {
			return loadout.Loadout{}, fmt.Errorf("pinned loadout %q not found in inventory", name)
		}
		return loadout.Loadout{}, fmt.Errorf("cannot read pinned loadout %q: %w", name, err)
	}
	for _, w := range warns {
		fmt.Fprintf(errOut, "warning: %s\n", w)
	}
	if len(valErrs) > 0 {
		ve := valErrs[0]
		return loadout.Loadout{}, fmt.Errorf("pinned loadout %q: %s: %s", name, ve.Field, ve.Message)
	}
	return lo, nil
}

// printPinnedDryRunPlan renders the A/M/D preview for `sync --dry-run` in
// pinned mode. By analogy with apply --loadout --dry-run (printLoadoutDryRun,
// apply.go), building the plan performed no writes, so this is the entire
// dry-run output for the pinned path.
func printPinnedDryRunPlan(res materializeResult, out io.Writer) {
	if res.Empty {
		fmt.Fprintf(out, "[dry-run] nothing to sync — environments match loadout %q\n", res.LoadoutName)
		return
	}
	fmt.Fprintf(out, "[dry-run] would sync loadout %q — %s to %s (%s):\n",
		res.LoadoutName,
		Plural(len(res.DeltaLines), "change"),
		Plural(len(res.PlanEnvNames), "environment"),
		strings.Join(res.PlanEnvNames, ", "),
	)
	PrintDeltaBlock(out, res.DeltaLines)
}
