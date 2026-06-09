package cli

import (
	"fmt"
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
