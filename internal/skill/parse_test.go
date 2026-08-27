package skill_test

import (
	"testing"

	"github.com/axsmak/aim/internal/skill"
)

func TestParseRaw_WithTargets(t *testing.T) {
	raw := []byte("---\nname: hello\ndescription: Says hello\ntargets:\n  - claude-code\n  - cursor\n---\n\n# Role\nSay hello.\n")

	s, ve, err := skill.ParseRaw(raw, "<stdin>")
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve != nil {
		t.Fatalf("unexpected validation error: %v", ve)
	}
	want := []string{"claude-code", "cursor"}
	if len(s.Targets) != len(want) {
		t.Fatalf("Targets = %v, want %v", s.Targets, want)
	}
	for i, w := range want {
		if s.Targets[i] != w {
			t.Errorf("Targets[%d] = %q, want %q", i, s.Targets[i], w)
		}
	}
	if string(s.Raw) != string(raw) {
		t.Error("Raw must be preserved byte-for-byte; targets must not be stripped")
	}
}

func TestParseRaw_WithoutTargets(t *testing.T) {
	raw := []byte("---\nname: hello\ndescription: Says hello\n---\n\n# Role\nSay hello.\n")

	s, ve, err := skill.ParseRaw(raw, "<stdin>")
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve != nil {
		t.Fatalf("unexpected validation error: %v", ve)
	}
	if s.Targets != nil {
		t.Errorf("Targets = %v, want nil", s.Targets)
	}
}

func TestParseRaw_WithEmptyTargets(t *testing.T) {
	raw := []byte("---\nname: hello\ndescription: Says hello\ntargets: []\n---\n\n# Role\nSay hello.\n")

	s, ve, err := skill.ParseRaw(raw, "<stdin>")
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve != nil {
		t.Fatalf("unexpected validation error (empty targets list is valid): %v", ve)
	}
	if len(s.Targets) != 0 {
		t.Errorf("Targets = %v, want empty", s.Targets)
	}
}

func TestParseRaw_WithUnknownTarget(t *testing.T) {
	// Unknown environment names are not validated at parse time (ADR-0007, decision 7).
	raw := []byte("---\nname: hello\ndescription: Says hello\ntargets:\n  - claud-code\n---\n\n# Role\nSay hello.\n")

	s, ve, err := skill.ParseRaw(raw, "<stdin>")
	if err != nil {
		t.Fatalf("unexpected system error: %v", err)
	}
	if ve != nil {
		t.Fatalf("unexpected validation error: %v", ve)
	}
	if len(s.Targets) != 1 || s.Targets[0] != "claud-code" {
		t.Errorf("Targets = %v, want [claud-code]", s.Targets)
	}
}
