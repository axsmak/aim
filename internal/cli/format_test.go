package cli

import (
	"bytes"
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

func TestPrintDeltaBlock(t *testing.T) {
	makeLines := func(n int) []string {
		lines := make([]string, n)
		for i := range lines {
			lines[i] = fmt.Sprintf("A skills/skill-%02d.md", i+1)
		}
		return lines
	}

	t.Run("empty", func(t *testing.T) {
		var w bytes.Buffer
		PrintDeltaBlock(&w, []string{})
		if w.String() != "" {
			t.Errorf("empty slice: want no output, got %q", w.String())
		}
	})

	t.Run("below_threshold", func(t *testing.T) {
		var w bytes.Buffer
		lines := makeLines(3)
		PrintDeltaBlock(&w, lines)
		got := w.String()
		wantLines := []string{
			"  A skills/skill-01.md",
			"  A skills/skill-02.md",
			"  A skills/skill-03.md",
		}
		want := strings.Join(wantLines, "\n") + "\n"
		if got != want {
			t.Errorf("below_threshold:\n  got:  %q\n  want: %q", got, want)
		}
		if strings.Contains(got, "more") {
			t.Errorf("below_threshold: unexpected truncation line")
		}
	})

	t.Run("at_threshold", func(t *testing.T) {
		var w bytes.Buffer
		lines := makeLines(20)
		PrintDeltaBlock(&w, lines)
		got := w.String()
		outLines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		if len(outLines) != 20 {
			t.Errorf("at_threshold: want 20 lines, got %d", len(outLines))
		}
		if strings.Contains(got, "more") {
			t.Errorf("at_threshold: unexpected truncation line")
		}
		for i, line := range outLines {
			if !strings.HasPrefix(line, "  ") {
				t.Errorf("at_threshold: line %d missing 2-space indent: %q", i, line)
			}
		}
	})

	t.Run("above_threshold", func(t *testing.T) {
		var w bytes.Buffer
		lines := makeLines(23)
		PrintDeltaBlock(&w, lines)
		got := w.String()
		outLines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		if len(outLines) != 21 {
			t.Errorf("above_threshold: want 21 lines (20 + truncation), got %d", len(outLines))
		}
		wantLast := "  … and 3 more"
		if outLines[20] != wantLast {
			t.Errorf("above_threshold: last line:\n  got:  %q\n  want: %q", outLines[20], wantLast)
		}
		for i := 0; i < 20; i++ {
			if !strings.HasPrefix(outLines[i], "  ") {
				t.Errorf("above_threshold: line %d missing 2-space indent: %q", i, outLines[i])
			}
		}
	})

	t.Run("with_qualifier", func(t *testing.T) {
		var w bytes.Buffer
		lines := []string{`M skills/x.md  (updated in staging)`}
		PrintDeltaBlock(&w, lines)
		got := w.String()
		want := "  M skills/x.md  (updated in staging)\n"
		if got != want {
			t.Errorf("with_qualifier:\n  got:  %q\n  want: %q", got, want)
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
