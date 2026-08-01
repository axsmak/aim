package loadout_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/axsmak/aim/internal/loadout"
)

func writeLoadout(t *testing.T, dir, fileName, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

const docsWorkYAML = `name: Documentation Work
description: docs context
items:
  - skill:create-spec
  - mcp:context7
`

const archWorkYAML = `name: architecture-work
description: arch context
items:
  - skill:create-arch
`

func TestResolve_ExactFileName(t *testing.T) {
	dir := t.TempDir()
	writeLoadout(t, dir, "documentation-work.yaml", docsWorkYAML)

	l, errs, warns, err := loadout.Resolve(dir, "documentation-work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 || len(warns) != 0 {
		t.Fatalf("unexpected findings: errs=%v warns=%v", errs, warns)
	}
	if l.Name != "Documentation Work" {
		t.Errorf("Name = %q, want %q", l.Name, "Documentation Work")
	}
}

func TestResolve_NormalizedArgument(t *testing.T) {
	dir := t.TempDir()
	writeLoadout(t, dir, "documentation-work.yaml", docsWorkYAML)

	l, _, _, err := loadout.Resolve(dir, "Documentation Work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l.Name != "Documentation Work" {
		t.Errorf("Name = %q, want %q", l.Name, "Documentation Work")
	}
}

// A file whose name does not match its normalized `name` field is still
// resolvable through the name field (with the mismatch warning attached).
func TestResolve_ByNameField(t *testing.T) {
	dir := t.TempDir()
	writeLoadout(t, dir, "legacy-file.yaml", docsWorkYAML)

	l, errs, warns, err := loadout.Resolve(dir, "Documentation Work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if l.Name != "Documentation Work" {
		t.Errorf("Name = %q, want %q", l.Name, "Documentation Work")
	}
	if len(warns) != 1 || warns[0].Field != "name" {
		t.Errorf("warnings = %v, want file-name mismatch warning", warns)
	}
}

func TestResolve_NotFound(t *testing.T) {
	dir := t.TempDir()
	writeLoadout(t, dir, "architecture-work.yaml", archWorkYAML)

	_, _, _, err := loadout.Resolve(dir, "NonExisting")
	var nf loadout.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error = %v, want NotFoundError", err)
	}
	if nf.Name != "NonExisting" {
		t.Errorf("NotFoundError.Name = %q, want %q", nf.Name, "NonExisting")
	}
}

func TestResolve_MissingDir(t *testing.T) {
	_, _, _, err := loadout.Resolve(filepath.Join(t.TempDir(), "loadouts"), "anything")
	var nf loadout.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error = %v, want NotFoundError", err)
	}
}

// A found-but-invalid loadout is not "not found": Resolve returns it with
// its validation errors so apply can fail with the actual cause.
func TestResolve_FoundButInvalid(t *testing.T) {
	dir := t.TempDir()
	writeLoadout(t, dir, "broken.yaml", "name: broken\ndescription: d\nitems: []\n")

	l, errs, _, err := loadout.Resolve(dir, "broken")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if l.Name != "broken" {
		t.Errorf("Name = %q, want %q", l.Name, "broken")
	}
	if len(errs) != 1 || errs[0].Field != "items" {
		t.Fatalf("errors = %v, want single items error", errs)
	}
}

func TestResolve_PathSeparatorInName(t *testing.T) {
	dir := t.TempDir()
	writeLoadout(t, dir, "documentation-work.yaml", docsWorkYAML)

	_, _, _, err := loadout.Resolve(dir, "../documentation-work")
	var nf loadout.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error = %v, want NotFoundError", err)
	}
}

func TestList_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writeLoadout(t, dir, "documentation-work.yaml", docsWorkYAML)
	writeLoadout(t, dir, "architecture-work.yaml", archWorkYAML)

	valid, errs, warns, err := loadout.List(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 || len(warns) != 0 {
		t.Fatalf("unexpected findings: errs=%v warns=%v", errs, warns)
	}
	if len(valid) != 2 {
		t.Fatalf("valid len = %d, want 2", len(valid))
	}
	// filepath.Glob returns sorted paths: architecture-work before documentation-work.
	if valid[0].Name != "architecture-work" || valid[1].Name != "Documentation Work" {
		t.Errorf("order = [%q, %q], want file-name order", valid[0].Name, valid[1].Name)
	}
}

func TestList_MissingDir(t *testing.T) {
	valid, errs, warns, err := loadout.List(filepath.Join(t.TempDir(), "loadouts"))
	if err != nil {
		t.Fatalf("missing dir must not be an error, got: %v", err)
	}
	if len(valid) != 0 || len(errs) != 0 || len(warns) != 0 {
		t.Errorf("want empty result, got valid=%v errs=%v warns=%v", valid, errs, warns)
	}
}

func TestList_EmptyDir(t *testing.T) {
	valid, errs, _, err := loadout.List(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(valid) != 0 || len(errs) != 0 {
		t.Errorf("want empty result, got valid=%v errs=%v", valid, errs)
	}
}

func TestList_PartialSuccess(t *testing.T) {
	dir := t.TempDir()
	writeLoadout(t, dir, "documentation-work.yaml", docsWorkYAML)
	writeLoadout(t, dir, "broken.yaml", "description: no name, empty items\nitems: []\n")
	writeLoadout(t, dir, "no-desc.yaml", "name: no-desc\nitems:\n  - skill:wpage\n")

	valid, errs, warns, err := loadout.List(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(valid) != 2 {
		t.Errorf("valid len = %d, want 2", len(valid))
	}
	// broken.yaml: missing name + empty items = 2 errors.
	if len(errs) != 2 {
		t.Errorf("errors = %v, want 2", errs)
	}
	// no-desc.yaml misses description.
	if len(warns) != 1 {
		t.Errorf("warnings = %v, want 1", warns)
	}
}
