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
	"github.com/axsmak/aim/internal/loadout"
	"github.com/axsmak/aim/internal/localconfig"
	"github.com/axsmak/aim/internal/mcp"
	"github.com/axsmak/aim/internal/skill"
)

// Declarative reconciliation engine (ADR-0004, decisions 3-8).
//
// BuildReconcilePlan computes the full A/M/D plan for bringing every
// admissible environment exactly to the set of a loadout, within the
// AIM-managed namespace derived from the inventory at execution time
// (no state file, ADR-0004 decision 5). ExecuteReconcilePlan performs
// the plan through the adapters. The API is deliberately free of any
// Cobra context so the pinned-loadout sync path (ADR-0006) can reuse
// it verbatim.
//
// Category D exists ONLY on this loadout path. The ordinary apply path
// (computeApplyDelta) stays additive per ADR-0003 5.1.

// ReconcileEnv is one detected AI environment the engine reconciles.
type ReconcileEnv struct {
	Adapter adapter.Adapter
	BaseDir string
}

// DetectReconcileEnvs returns every configured adapter that detects its
// environment under homeDir, paired with the detected base directory.
func DetectReconcileEnvs(cfg localconfig.Config, homeDir string) []ReconcileEnv {
	var envs []ReconcileEnv
	for _, a := range adapter.DefaultAdapters(cfg) {
		baseDir, found := a.Detect(homeDir)
		if !found {
			continue
		}
		envs = append(envs, ReconcileEnv{Adapter: a, BaseDir: baseDir})
	}
	return envs
}

// ReconcileInventory is the inventory snapshot the plan is computed from.
//
// The namespaces define the AIM-managed boundary (ADR-0004 decision 3):
// only names present there may ever be planned for deletion. SkillNamespace
// contains every skills/<name> that exists in the inventory — flat or
// folder format, valid or not — because the namespace is about existence,
// not validity. MCPNamespace contains the names of parsed mcp/<name>.yaml
// items; an unparseable MCP file has no knowable server key, so it stays
// outside the effective namespace and is never removed.
type ReconcileInventory struct {
	Skills         []skill.Skill // valid inventory skills
	SkillNamespace map[string]bool
	MCPs           []mcp.MCP // valid inventory MCP items
	MCPNamespace   map[string]bool
}

// LoadReconcileInventory reads skills and MCP items from the inventory
// working tree and computes the AIM-managed namespaces. Warnings (invalid
// skills, unparseable MCP files) are returned as printable strings; only
// an unreadable skills directory is a hard error, mirroring runApply.
func LoadReconcileInventory(skillsDir, mcpDir string) (ReconcileInventory, []string, error) {
	inv := ReconcileInventory{
		SkillNamespace: make(map[string]bool),
		MCPNamespace:   make(map[string]bool),
	}

	valid, invalid, err := skill.ReadAll(skillsDir)
	if err != nil {
		return inv, nil, fmt.Errorf("cannot read skills: %w", err)
	}
	inv.Skills = valid

	var warnings []string
	for _, ve := range invalid {
		warnings = append(warnings, ve.Error())
	}

	// Namespace = everything existing as skills/<name> (flat or folder),
	// regardless of validity (ADR-0004 decision 5).
	flat, err := filepath.Glob(filepath.Join(skillsDir, "*.md"))
	if err != nil {
		return inv, warnings, fmt.Errorf("cannot list skills: %w", err)
	}
	for _, path := range flat {
		inv.SkillNamespace[strings.TrimSuffix(filepath.Base(path), ".md")] = true
	}
	folder, err := filepath.Glob(filepath.Join(skillsDir, "*", "SKILL.md"))
	if err != nil {
		return inv, warnings, fmt.Errorf("cannot list skills: %w", err)
	}
	for _, path := range folder {
		inv.SkillNamespace[filepath.Base(filepath.Dir(path))] = true
	}

	mcpItems, mcpErrs := mcp.ParseDir(mcpDir)
	for _, e := range mcpErrs {
		warnings = append(warnings, e.Error())
	}
	inv.MCPs = mcpItems
	for _, m := range mcpItems {
		inv.MCPNamespace[m.Name] = true
	}

	return inv, warnings, nil
}

// ReconcileAction is one aggregated plan entry, ready for the delta block.
type ReconcileAction struct {
	Category string           // "A" (install), "M" (update), "D" (delete)
	Kind     loadout.ItemKind // skill | mcp
	Name     string
	Envs     []string // adapter names the action applies to
}

// envPlan holds the concrete per-environment operations Execute performs.
type envPlan struct {
	env          ReconcileEnv
	skills       []skill.Skill // desired skills to install/overwrite
	mcps         []mcp.MCP     // desired MCP items to install/overwrite
	removeSkills []string
	removeMCPs   []string
}

// ReconcilePlan is the computed A/M/D plan. Building it performs no writes,
// so it doubles as the --dry-run preview (ADR-0004 decision 8).
type ReconcilePlan struct {
	// Actions is the aggregated change composition, sorted skills-first
	// then by name. Unchanged desired items produce no action.
	Actions []ReconcileAction
	// MissingRefs are loadout items with no valid inventory element behind
	// them (deleted, renamed, or currently invalid). They are neither
	// installed nor removed; the caller decides how to report them.
	MissingRefs []loadout.ItemRef

	envs              []envPlan
	desiredSkillCount int
	desiredMCPCount   int
}

// Empty reports whether the plan contains no changes (A/M/D). A second run
// of the same loadout against an already reconciled environment is Empty.
func (p *ReconcilePlan) Empty() bool { return len(p.Actions) == 0 }

// EnvNames returns the adapter names of the environments the plan covers
// (after the loadout-level targets filter).
func (p *ReconcilePlan) EnvNames() []string {
	names := make([]string, 0, len(p.envs))
	for _, ep := range p.envs {
		names = append(names, ep.env.Adapter.Name())
	}
	return names
}

// DeltaLines renders the plan in the ADR-0003 delta-block line format,
// consumable by PrintDeltaBlock. Real runs use action qualifiers
// ("updated in", "removed from"); dry-runs use statement-of-fact
// qualifiers ("differs in", "would remove from"), mirroring
// computeApplyDelta.
func (p *ReconcilePlan) DeltaLines(dryRun bool) []string {
	totalEnvs := len(p.envs)
	var lines []string
	for _, a := range p.Actions {
		var envDesc string
		if len(a.Envs) == totalEnvs {
			envDesc = "all environments"
		} else {
			envDesc = strings.Join(a.Envs, ", ")
		}

		var path string
		if a.Kind == loadout.KindMCP {
			path = "mcp/" + a.Name + ".yaml"
		} else {
			path = "skills/" + a.Name + ".md"
		}

		var qualifier string
		switch a.Category {
		case "A":
			qualifier = "new in"
		case "M":
			qualifier = "updated in"
			if dryRun {
				qualifier = "differs in"
			}
		case "D":
			qualifier = "removed from"
			if dryRun {
				qualifier = "would remove from"
			}
		}
		lines = append(lines, fmt.Sprintf("%s %s   (%s %s)", a.Category, path, qualifier, envDesc))
	}
	return lines
}

// BuildReconcilePlan computes the declarative plan for applying lo to envs.
// It performs no writes (plan-only; the --dry-run path stops here).
//
// Per environment (BFT section 6):
//   - environments excluded by loadout-level targets are not in the plan
//     at all, so they are never touched;
//   - the desired set is the loadout items admissible in that environment
//     by the INTERSECTION of loadout-level and item-level targets (an empty
//     item-level targets list on a skill means every discovered environment,
//     ADR-0007 decision 2; MCP items require a non-empty targets list);
//   - desired item absent in the environment → A; present with different
//     content → M (sha256, like computeApplyDelta); identical → no action;
//   - a namespace item outside the loadout that is present in the
//     environment → D;
//   - anything not in the inventory namespace is invisible to the plan.
//
// MCP presence is checked through the adapter's MCP scanner. Present desired
// MCP items are reinstalled on execute but reported neither as A nor M: their
// materialized form (resolved env values, shared config) has no reliable
// content identity, and ADR-0003 5.5 forbids unverified status claims.
func BuildReconcilePlan(lo loadout.Loadout, inv ReconcileInventory, envs []ReconcileEnv) (*ReconcilePlan, error) {
	p := &ReconcilePlan{}

	for _, e := range envs {
		if len(lo.Targets) > 0 && !containsTarget(lo.Targets, e.Adapter.Name()) {
			continue
		}
		p.envs = append(p.envs, envPlan{env: e})
	}

	skillByName := make(map[string]skill.Skill, len(inv.Skills))
	for _, s := range inv.Skills {
		skillByName[s.Name] = s
	}
	mcpByName := make(map[string]mcp.MCP, len(inv.MCPs))
	for _, m := range inv.MCPs {
		mcpByName[m.Name] = m
	}

	// loadoutSkillNames guards deletion: a name the loadout asks for is never
	// planned for removal, even when its inventory file is currently invalid
	// (installing it is impossible, but deleting what the user requested
	// would be worse).
	loadoutSkillNames := make(map[string]bool)
	var desiredSkills []skill.Skill
	var desiredMCPs []mcp.MCP
	seenRef := make(map[loadout.ItemRef]bool)
	for _, ref := range lo.Refs {
		if seenRef[ref] {
			continue
		}
		seenRef[ref] = true
		switch ref.Kind {
		case loadout.KindSkill:
			loadoutSkillNames[ref.Name] = true
			if s, ok := skillByName[ref.Name]; ok {
				desiredSkills = append(desiredSkills, s)
			} else {
				p.MissingRefs = append(p.MissingRefs, ref)
			}
		case loadout.KindMCP:
			if m, ok := mcpByName[ref.Name]; ok {
				desiredMCPs = append(desiredMCPs, m)
			} else {
				p.MissingRefs = append(p.MissingRefs, ref)
			}
		}
	}
	p.desiredSkillCount = len(desiredSkills)
	p.desiredMCPCount = len(desiredMCPs)

	// Install actions (A/M) for one item collapse into a single entry with
	// A-precedence; D aggregates separately — the same MCP name can be
	// desired in one environment and deleted from another when item-level
	// targets differ per environment.
	type aggKey struct {
		kind   loadout.ItemKind
		name   string
		delete bool
	}
	aggMap := make(map[aggKey]*ReconcileAction)
	var aggOrder []aggKey
	addAction := func(kind loadout.ItemKind, name, category, envName string) {
		key := aggKey{kind: kind, name: name, delete: category == "D"}
		a := aggMap[key]
		if a == nil {
			a = &ReconcileAction{Category: category, Kind: kind, Name: name}
			aggMap[key] = a
			aggOrder = append(aggOrder, key)
		} else if a.Category == "M" && category == "A" {
			// Missing in some environment takes precedence over differing
			// in another, regardless of environment order.
			a.Category = "A"
		}
		a.Envs = append(a.Envs, envName)
	}

	for i := range p.envs {
		ep := &p.envs[i]
		envName := ep.env.Adapter.Name()

		// Skills: desired set = loadout skills whose item-level targets admit
		// this environment (empty targets = admit every discovered
		// environment, ADR-0007 decision 2; intersection with loadout-level
		// targets, which already filtered p.envs).
		skillDesiredHere := make(map[string]bool)
		for _, s := range desiredSkills {
			if len(s.Targets) > 0 && !containsTarget(s.Targets, envName) {
				continue
			}
			ep.skills = append(ep.skills, s)
			skillDesiredHere[s.Name] = true
		}
		for _, s := range ep.skills {
			inventoryHash := sha256.Sum256(s.Raw)
			if cat := skillEnvCategory(ep.env.BaseDir, s.Name, inventoryHash); cat != "" {
				addAction(loadout.KindSkill, s.Name, cat, envName)
			}
		}

		// Namespace skills outside the loadout, materialized in this
		// environment → D (branch 2). Anything else on disk is outside the
		// namespace and invisible (branch 3).
		var removeCandidates []string
		for name := range inv.SkillNamespace {
			if loadoutSkillNames[name] {
				continue
			}
			removeCandidates = append(removeCandidates, name)
		}
		// Loadout skills not admitted into this environment by their
		// item-level targets → D here too (ADR-0007 decision 4), symmetric
		// with the MCP intersection below. desiredSkills only ever holds
		// names resolved to a valid inventory file (loadoutSkillNames guards
		// invalid files out of it and out of the namespace branch above), so
		// this never plans deletion from an unknown targets list (ADR-0007
		// decision 5).
		for _, s := range desiredSkills {
			if skillDesiredHere[s.Name] {
				continue
			}
			removeCandidates = append(removeCandidates, s.Name)
		}
		sort.Strings(removeCandidates)
		for _, name := range removeCandidates {
			installed := filepath.Join(ep.env.BaseDir, "skills", name, "SKILL.md")
			if _, err := os.Stat(installed); err != nil {
				continue
			}
			ep.removeSkills = append(ep.removeSkills, name)
			addAction(loadout.KindSkill, name, "D", envName)
		}

		// MCP: desired set = loadout items whose item-level targets admit
		// this environment (intersection with loadout-level targets, which
		// already filtered p.envs).
		desiredHere := make(map[string]bool)
		for _, m := range desiredMCPs {
			if !containsTarget(m.Targets, envName) {
				continue
			}
			ep.mcps = append(ep.mcps, m)
			desiredHere[m.Name] = true
		}

		present, err := scanMCPKeys(ep.env)
		if err != nil {
			return nil, fmt.Errorf("cannot scan MCP servers in %s: %w", envName, err)
		}
		for _, m := range ep.mcps {
			if !present[m.Name] {
				addAction(loadout.KindMCP, m.Name, "A", envName)
			}
		}
		var mcpRemoveCandidates []string
		for name := range inv.MCPNamespace {
			if desiredHere[name] || !present[name] {
				continue
			}
			mcpRemoveCandidates = append(mcpRemoveCandidates, name)
		}
		sort.Strings(mcpRemoveCandidates)
		for _, name := range mcpRemoveCandidates {
			ep.removeMCPs = append(ep.removeMCPs, name)
			addAction(loadout.KindMCP, name, "D", envName)
		}
	}

	sort.SliceStable(aggOrder, func(i, j int) bool {
		if aggOrder[i].kind != aggOrder[j].kind {
			return aggOrder[i].kind == loadout.KindSkill
		}
		return aggOrder[i].name < aggOrder[j].name
	})
	for _, key := range aggOrder {
		p.Actions = append(p.Actions, *aggMap[key])
	}

	return p, nil
}

// skillEnvCategory classifies the installed state of one skill in one
// environment against its inventory content: "A" when not materialized at
// baseDir/skills/<name>/SKILL.md, "M" when the installed SKILL.md hash
// differs from inventoryHash, "" when identical. Shared by the additive
// apply delta (computeApplyDelta) and the reconciliation engine so the
// sha256 comparison exists exactly once.
func skillEnvCategory(baseDir, name string, inventoryHash [sha256.Size]byte) string {
	raw, err := os.ReadFile(filepath.Join(baseDir, "skills", name, "SKILL.md"))
	if err != nil {
		return "A"
	}
	if sha256.Sum256(raw) != inventoryHash {
		return "M"
	}
	return ""
}

// scanMCPKeys returns the set of MCP server keys currently present in the
// environment's config, via the adapter's scanner. An adapter without a
// scanner reports no keys: nothing can be classified as present, so nothing
// is deleted (fail-safe direction).
func scanMCPKeys(env ReconcileEnv) (map[string]bool, error) {
	scanner, ok := env.Adapter.(adapter.MCPScanner)
	if !ok {
		return nil, nil
	}
	discovered, err := scanner.ScanMCP(env.BaseDir)
	if err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(discovered))
	for _, d := range discovered {
		keys[d.ServerName] = true
	}
	return keys, nil
}

// ReconcileResult reports what an execution did, in the shape the CLI
// success line and delta block need.
type ReconcileResult struct {
	SkillCount  int      // loadout skills installed per environment
	MCPCount    int      // loadout MCP servers in the desired set
	EnvCount    int      // environments reconciled
	MCPEnvNames []string // environments that received at least one MCP server
	DeltaLines  []string // actual delta (real-run qualifiers), for PrintDeltaBlock
}

// ExecuteReconcilePlan performs the plan: installs every desired item and
// removes every planned D entry through the adapters. Desired items are
// (re)installed even when unchanged — installation is idempotent and this
// mirrors the additive apply/sync paths; the delta block still reports only
// real changes.
//
// cfg collects resolved MCP env values via SetMCPEnv; persisting it is the
// caller's responsibility (as with installMCPs). Skill failures abort
// immediately; MCP failures degrade to warnings and a combined error, both
// mirroring the existing install paths.
func ExecuteReconcilePlan(p *ReconcilePlan, cfg *localconfig.Config, in io.Reader, out, errOut io.Writer) (ReconcileResult, error) {
	res := ReconcileResult{
		SkillCount: p.desiredSkillCount,
		MCPCount:   p.desiredMCPCount,
		EnvCount:   len(p.envs),
		// Snapshot the delta before writing, like runApply: after execution
		// the pre-install state is gone.
		DeltaLines: p.DeltaLines(false),
	}

	var hadMCPError bool
	for _, ep := range p.envs {
		a := ep.env.Adapter
		for _, s := range ep.skills {
			if err := a.InstallSkill(s, ep.env.BaseDir); err != nil {
				return res, fmt.Errorf("failed to install %s in %s: %w", s.Name, a.Name(), err)
			}
		}
		for _, name := range ep.removeSkills {
			if err := a.RemoveSkill(name, ep.env.BaseDir); err != nil {
				return res, fmt.Errorf("failed to remove %s from %s: %w", name, a.Name(), err)
			}
		}

		installed := 0
		for _, m := range ep.mcps {
			existing := cfg.GetMCPEnvForServer(m.Name)
			resolved, changed, resolveErr := mcp.ResolveEnv(m, existing, in, out)
			if resolveErr != nil {
				fmt.Fprintf(errOut, "warning: env resolution for %s: %v\n", m.Name, resolveErr)
			}
			if changed {
				for k, v := range resolved {
					cfg.SetMCPEnv(m.Name, k, v)
				}
			}
			if err := a.InstallMCP(m, ep.env.BaseDir, resolved); err != nil {
				fmt.Fprintf(errOut, "warning: failed to install MCP %s in %s: %v\n", m.Name, a.Name(), err)
				hadMCPError = true
				continue
			}
			installed++
		}
		for _, name := range ep.removeMCPs {
			if err := a.RemoveMCP(name, ep.env.BaseDir); err != nil {
				fmt.Fprintf(errOut, "warning: failed to remove MCP %s from %s: %v\n", name, a.Name(), err)
				hadMCPError = true
			}
		}
		if installed > 0 {
			res.MCPEnvNames = append(res.MCPEnvNames, a.Name())
		}
	}

	if hadMCPError {
		return res, fmt.Errorf("one or more MCP operations failed")
	}
	return res, nil
}
