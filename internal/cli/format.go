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
// hash: commit short-hash (pass "" if not commit-bound, e.g. apply).
// skills, mcpServers: artifact counts. mcpServers=0 omits the MCP segment.
// envs: environment count (pass 0 if op doesn't touch environments, e.g. push).
func FormatSuccess(verb, hash string, skills, mcpServers, envs int) string {
	var sb strings.Builder
	sb.WriteString(verb)
	sb.WriteString(": ")

	if hash != "" {
		sb.WriteString(hash)
		sb.WriteString(" · ")
	}

	sb.WriteString(Plural(skills, "skill"))

	if mcpServers > 0 {
		sb.WriteString(", ")
		sb.WriteString(Plural(mcpServers, "MCP server"))
	}

	if envs > 0 {
		sb.WriteString(" → ")
		sb.WriteString(Plural(envs, "environment"))
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
