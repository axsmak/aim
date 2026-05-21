package mcp

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ResolveEnv checks which required env values are missing and prompts for them.
// existing contains already-stored values (keyed by VAR_NAME).
// Returns resolved map of all provided values and whether any new values were entered.
func ResolveEnv(m MCP, existing map[string]string, r io.Reader, w io.Writer) (resolved map[string]string, changed bool, err error) {
	resolved = make(map[string]string)
	scanner := bufio.NewScanner(r)

	for _, ev := range m.Env {
		if val, ok := existing[ev.Name]; ok && val != "" {
			resolved[ev.Name] = val
			continue
		}
		if !ev.Required {
			continue
		}
		prompt := fmt.Sprintf("  %s › %s", m.Name, ev.Name)
		if ev.Description != "" {
			prompt += fmt.Sprintf(" (%s)", ev.Description)
		}
		if ev.Example != "" {
			prompt += fmt.Sprintf(" [e.g. %s]", ev.Example)
		}
		fmt.Fprintf(w, "%s: ", prompt)

		if scanner.Scan() {
			val := strings.TrimSpace(scanner.Text())
			if val != "" {
				resolved[ev.Name] = val
				changed = true
			}
		}
		if err := scanner.Err(); err != nil {
			return resolved, changed, err
		}
	}
	return resolved, changed, nil
}
