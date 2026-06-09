package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestFormatSuccess(t *testing.T) {
	tests := []struct {
		name      string
		verb      string
		hash      string
		skills    int
		mcpServer int
		envs      int
		want      string
	}{
		{
			name:      "push: skills only, no MCP",
			verb:      "published",
			hash:      "96e091b",
			skills:    19,
			mcpServer: 0,
			envs:      0,
			want:      "published: 96e091b · 19 skills",
		},
		{
			name:      "push: skills + MCP",
			verb:      "published",
			hash:      "96e091b",
			skills:    19,
			mcpServer: 1,
			envs:      0,
			want:      "published: 96e091b · 19 skills, 1 MCP server",
		},
		{
			name:      "push: skills + multiple MCP",
			verb:      "published",
			hash:      "96e091b",
			skills:    19,
			mcpServer: 3,
			envs:      0,
			want:      "published: 96e091b · 19 skills, 3 MCP servers",
		},
		{
			name:      "sync: hash + skills + MCP + envs",
			verb:      "synced",
			hash:      "96e091b",
			skills:    19,
			mcpServer: 1,
			envs:      3,
			want:      "synced: 96e091b · 19 skills, 1 MCP server → 3 environments",
		},
		{
			name:      "sync: hash + skills only + envs",
			verb:      "synced",
			hash:      "96e091b",
			skills:    5,
			mcpServer: 0,
			envs:      2,
			want:      "synced: 96e091b · 5 skills → 2 environments",
		},
		{
			name:      "apply: no hash, skills + MCP + envs",
			verb:      "applied",
			hash:      "",
			skills:    19,
			mcpServer: 1,
			envs:      3,
			want:      "applied: 19 skills, 1 MCP server → 3 environments",
		},
		{
			name:      "apply: no hash, skills only",
			verb:      "applied",
			hash:      "",
			skills:    5,
			mcpServer: 0,
			envs:      2,
			want:      "applied: 5 skills → 2 environments",
		},
		{
			name:      "singular skill",
			verb:      "applied",
			hash:      "",
			skills:    1,
			mcpServer: 1,
			envs:      1,
			want:      "applied: 1 skill, 1 MCP server → 1 environment",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatSuccess(tc.verb, tc.hash, tc.skills, tc.mcpServer, tc.envs)
			if got != tc.want {
				t.Errorf("FormatSuccess(%q, %q, %d, %d, %d)\n  got:  %q\n  want: %q",
					tc.verb, tc.hash, tc.skills, tc.mcpServer, tc.envs, got, tc.want)
			}
		})
	}
}

func TestTruncateDelta(t *testing.T) {
	makeLines := func(n int) []string {
		lines := make([]string, n)
		for i := range lines {
			lines[i] = fmt.Sprintf("  A skills/skill-%02d.md", i+1)
		}
		return lines
	}

	t.Run("fewer than threshold: no truncation", func(t *testing.T) {
		in := makeLines(5)
		got := TruncateDelta(in, 20)
		if len(got) != 5 {
			t.Errorf("want 5 lines, got %d", len(got))
		}
	})

	t.Run("exactly threshold: no truncation", func(t *testing.T) {
		in := makeLines(20)
		got := TruncateDelta(in, 20)
		if len(got) != 20 {
			t.Errorf("want 20 lines, got %d", len(got))
		}
		if strings.Contains(strings.Join(got, "\n"), "more") {
			t.Errorf("unexpected truncation line at exactly threshold")
		}
	})

	t.Run("exceeds threshold: first 20 + counter", func(t *testing.T) {
		in := makeLines(57)
		got := TruncateDelta(in, 20)
		if len(got) != 21 {
			t.Errorf("want 21 lines (20 + counter), got %d", len(got))
		}
		last := got[20]
		want := "  … and 37 more"
		if last != want {
			t.Errorf("counter line: got %q, want %q", last, want)
		}
	})

	t.Run("exactly one over threshold: counter shows 1", func(t *testing.T) {
		in := makeLines(21)
		got := TruncateDelta(in, 20)
		if len(got) != 21 {
			t.Errorf("want 21 lines, got %d", len(got))
		}
		want := "  … and 1 more"
		if got[20] != want {
			t.Errorf("counter line: got %q, want %q", got[20], want)
		}
	})
}

func TestPlural(t *testing.T) {
	tests := []struct {
		n    int
		word string
		want string
	}{
		{1, "skill", "1 skill"},
		{2, "skill", "2 skills"},
		{0, "skill", "0 skills"},
		{1, "MCP server", "1 MCP server"},
		{2, "MCP server", "2 MCP servers"},
		{0, "MCP server", "0 MCP servers"},
		{1, "environment", "1 environment"},
		{3, "environment", "3 environments"},
		{0, "environment", "0 environments"},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			got := Plural(tc.n, tc.word)
			if got != tc.want {
				t.Errorf("Plural(%d, %q) = %q, want %q", tc.n, tc.word, got, tc.want)
			}
		})
	}
}
