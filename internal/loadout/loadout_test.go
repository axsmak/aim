package loadout_test

import (
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/loadout"
)

var validYAML = []byte(`
name: Documentation Work
description: Skills and MCP for documentation tasks
targets:
  - claude-code
items:
  - skill:create-spec
  - skill:wpage
  - mcp:context7
`)

func TestParse_Valid(t *testing.T) {
	l, errs, warns := loadout.Parse(validYAML, "loadouts/documentation-work.yaml")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if l.Name != "Documentation Work" {
		t.Errorf("Name = %q, want %q", l.Name, "Documentation Work")
	}
	if l.Description == "" {
		t.Error("Description should not be empty")
	}
	if len(l.Targets) != 1 || l.Targets[0] != "claude-code" {
		t.Errorf("Targets = %v, want [claude-code]", l.Targets)
	}
	if len(l.Items) != 3 {
		t.Fatalf("Items len = %d, want 3", len(l.Items))
	}
	wantRefs := []loadout.ItemRef{
		{Kind: loadout.KindSkill, Name: "create-spec"},
		{Kind: loadout.KindSkill, Name: "wpage"},
		{Kind: loadout.KindMCP, Name: "context7"},
	}
	if len(l.Refs) != len(wantRefs) {
		t.Fatalf("Refs len = %d, want %d", len(l.Refs), len(wantRefs))
	}
	for i, want := range wantRefs {
		if l.Refs[i] != want {
			t.Errorf("Refs[%d] = %v, want %v", i, l.Refs[i], want)
		}
	}
}

func TestParse_MissingName(t *testing.T) {
	data := []byte(`
description: no name here
items:
  - skill:create-spec
`)
	_, errs, _ := loadout.Parse(data, "loadouts/x.yaml")
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want exactly 1", errs)
	}
	if errs[0].Field != "name" {
		t.Errorf("Field = %q, want %q", errs[0].Field, "name")
	}
}

func TestParse_EmptyItems(t *testing.T) {
	data := []byte(`
name: empty
description: d
items: []
`)
	_, errs, _ := loadout.Parse(data, "loadouts/empty.yaml")
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want exactly 1", errs)
	}
	if errs[0].Field != "items" || errs[0].Message != "cannot be empty" {
		t.Errorf("got %s: %s, want items: cannot be empty", errs[0].Field, errs[0].Message)
	}
}

func TestParse_MissingItems(t *testing.T) {
	data := []byte(`
name: no-items
description: d
`)
	_, errs, _ := loadout.Parse(data, "loadouts/no-items.yaml")
	if len(errs) != 1 || errs[0].Field != "items" {
		t.Fatalf("errors = %v, want single items error", errs)
	}
}

func TestParse_BadItemRefs(t *testing.T) {
	tests := []struct {
		name    string
		item    string
		wantSub string
	}{
		{"unknown prefix", "tool:hammer", "unknown prefix"},
		{"missing prefix", "create-spec", "missing prefix"},
		{"empty item name", "skill:", "empty item name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte("name: x\ndescription: d\nitems:\n  - \"" + tt.item + "\"\n")
			_, errs, _ := loadout.Parse(data, "loadouts/x.yaml")
			if len(errs) != 1 {
				t.Fatalf("errors = %v, want exactly 1", errs)
			}
			if errs[0].Field != "items[0]" {
				t.Errorf("Field = %q, want %q", errs[0].Field, "items[0]")
			}
			if !strings.Contains(errs[0].Message, tt.wantSub) {
				t.Errorf("Message = %q, want substring %q", errs[0].Message, tt.wantSub)
			}
		})
	}
}

// Push must see every finding, not just the first (US-L04).
func TestParse_CollectsAllErrors(t *testing.T) {
	data := []byte(`
items:
  - tool:hammer
  - "mcp:"
`)
	l, errs, warns := loadout.Parse(data, "loadouts/broken.yaml")
	if len(errs) != 3 {
		t.Fatalf("errors = %v, want 3 (name + two item refs)", errs)
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want 1 (missing description)", warns)
	}
	if len(l.Refs) != 0 {
		t.Errorf("Refs = %v, want empty", l.Refs)
	}
}

func TestParse_MissingDescription_Warning(t *testing.T) {
	data := []byte(`
name: no-desc
items:
  - skill:create-spec
`)
	_, errs, warns := loadout.Parse(data, "loadouts/no-desc.yaml")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(warns) != 1 || warns[0].Field != "description" {
		t.Fatalf("warnings = %v, want single description warning", warns)
	}
}

func TestParse_FilenameMismatch_Warning(t *testing.T) {
	l, errs, warns := loadout.Parse(validYAML, "loadouts/other-name.yaml")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(warns) != 1 || warns[0].Field != "name" {
		t.Fatalf("warnings = %v, want single file-name warning", warns)
	}
	if !strings.Contains(warns[0].Message, "documentation-work") {
		t.Errorf("Message = %q, want normalized name mentioned", warns[0].Message)
	}
	// Warning must not block: the loadout itself is usable.
	if l.Name != "Documentation Work" {
		t.Errorf("Name = %q, want %q", l.Name, "Documentation Work")
	}
}

func TestParse_NoPath_SkipsFilenameCheck(t *testing.T) {
	_, errs, warns := loadout.Parse(validYAML, "")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	data := []byte("name: [unclosed")
	_, errs, _ := loadout.Parse(data, "loadouts/bad.yaml")
	if len(errs) != 1 || errs[0].Field != "yaml" {
		t.Fatalf("errors = %v, want single yaml error", errs)
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Documentation Work", "documentation-work"},
		{"documentation-work", "documentation-work"},
		{"ARCH Work", "arch-work"},
		{"one two three", "one-two-three"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := loadout.Normalize(tt.in); got != tt.want {
			t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidationError_Error(t *testing.T) {
	e := loadout.ValidationError{FilePath: "loadouts/x.yaml", Field: "items", Message: "cannot be empty"}
	want := "loadouts/x.yaml: items: cannot be empty"
	if e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}
