package cli

import (
	"fmt"
	"io"
	"strings"
)

const deltaTruncateThreshold = 20

// TruncateDelta caps artifact lines at maxLines, appending "  … and N more" when exceeded.
func TruncateDelta(lines []string, maxLines int) []string {
	if len(lines) <= maxLines {
		return lines
	}
	out := make([]string, maxLines+1)
	copy(out, lines[:maxLines])
	out[maxLines] = fmt.Sprintf("  … and %d more", len(lines)-maxLines)
	return out
}

// ADR-0003 5.2: success line counters = operation volume (how many total applied),
// delta block = change composition (what is new/modified/deleted).
// Do not conflate: fixing "21 skills" to show only changed count would break
// the "sync with no changes" case.

// PrintDeltaBlock writes delta lines to w with 2-space indent and truncation.
// Lines are already formatted by the caller (e.g. "A skills/x.md").
// Empty lines slice produces no output.
// See ADR-0003 5.2: success line = operation volume; delta block = change composition.
func PrintDeltaBlock(w io.Writer, lines []string) {
	if len(lines) == 0 {
		return
	}
	truncated := TruncateDelta(lines, deltaTruncateThreshold)
	truncationAdded := len(truncated) == deltaTruncateThreshold+1
	for i, line := range truncated {
		// TruncateDelta appends "  … and N more" with its own 2-space prefix when
		// the input exceeds deltaTruncateThreshold; print it as-is to avoid double-indent.
		if truncationAdded && i == deltaTruncateThreshold {
			fmt.Fprintf(w, "%s\n", line)
		} else {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}
}

// FormatSuccess builds the canonical success line.
//
// Canonical template (ADR 4.1):
//
//	<verb>: [<hash> · ] <N skill(s)>[, <M MCP server(s)>][ → <K environment(s)>]
//
// Skills install into every detected environment, but MCP servers install only
// into the subset selected by their `targets`. When that subset is smaller than
// the skill environment count (issue #120), a single shared "→ K environments"
// suffix would misattribute the skill count to the MCP segment. In that case the
// line splits into two arrow segments instead, naming the MCP servers' actual
// environments:
//
//	<verb>: <N skill(s)> → <K environment(s)>, <M MCP server(s)> → <env1, env2, ...>
//
// hash: commit short-hash (pass "" if not commit-bound, e.g. apply).
// skills, mcpServers: artifact counts. mcpServers=0 omits the MCP segment.
// skillEnvs: environment count skills were installed into (0 if op doesn't touch environments, e.g. push).
// mcpEnvs: names of environments the MCP servers were installed into (nil if mcpServers=0).
func FormatSuccess(verb, hash string, skills, mcpServers, skillEnvs int, mcpEnvs []string) string {
	var sb strings.Builder
	sb.WriteString(verb)
	sb.WriteString(": ")

	if hash != "" {
		sb.WriteString(hash)
		sb.WriteString(" · ")
	}

	// Require a non-empty mcpEnvs to call it divergent — an empty mcpEnvs alongside
	// mcpServers>0 means targets matched no detected environment at all, which is a
	// separate, pre-existing edge case; fall back to the shared-arrow rendering rather
	// than printing a dangling "→ " with no environment names.
	divergent := mcpServers > 0 && skillEnvs > 0 && len(mcpEnvs) > 0 && len(mcpEnvs) < skillEnvs

	sb.WriteString(Plural(skills, "skill"))

	if mcpServers > 0 && !divergent {
		sb.WriteString(", ")
		sb.WriteString(Plural(mcpServers, "MCP server"))
	}

	if skillEnvs > 0 {
		sb.WriteString(" → ")
		sb.WriteString(Plural(skillEnvs, "environment"))
	}

	if divergent {
		sb.WriteString(", ")
		sb.WriteString(Plural(mcpServers, "MCP server"))
		sb.WriteString(" → ")
		sb.WriteString(strings.Join(mcpEnvs, ", "))
	}

	return sb.String()
}

// Plural returns "N <word>" with correct English plural form.
// Handles: "skill", "MCP server", "environment".
func Plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	switch word {
	case "MCP server":
		return fmt.Sprintf("%d MCP servers", n)
	default:
		return fmt.Sprintf("%d %ss", n, word)
	}
}
