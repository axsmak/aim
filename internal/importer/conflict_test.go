package importer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckConflict_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.yaml")

	err := CheckConflict(path, []byte("content"), false)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCheckConflict_SameContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.yaml")
	content := []byte("name: foo\nversion: 1\n")

	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	err := CheckConflict(path, content, false)
	if !errors.Is(err, ErrIdentical) {
		t.Fatalf("expected ErrIdentical for identical content, got %v", err)
	}
}

func TestCheckConflict_DifferentContent_NoOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.yaml")

	if err := os.WriteFile(path, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := CheckConflict(path, []byte("new content"), false)
	if err == nil {
		t.Fatal("expected ConflictError, got nil")
	}
	ce, ok := err.(ConflictError)
	if !ok {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
	if ce.Path != path {
		t.Errorf("expected path %q in error, got %q", path, ce.Path)
	}
}

func TestCheckConflict_DifferentContent_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.yaml")

	if err := os.WriteFile(path, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := CheckConflict(path, []byte("new content"), true)
	if err != nil {
		t.Fatalf("expected nil with overwrite=true, got %v", err)
	}
}

func TestConflictError_Message(t *testing.T) {
	e := ConflictError{Path: "/some/skill.yaml"}
	msg := e.Error()

	if !strings.Contains(msg, "/some/skill.yaml") {
		t.Errorf("error message does not contain path: %q", msg)
	}
	if !strings.Contains(msg, "--overwrite") {
		t.Errorf("error message does not contain --overwrite hint: %q", msg)
	}
}

func TestAmbiguousError_Message(t *testing.T) {
	e := AmbiguousError{Name: "my-skill", Sources: []string{"registry", "local"}}
	msg := e.Error()

	if !strings.Contains(msg, "my-skill") {
		t.Errorf("error message does not contain name: %q", msg)
	}
	if !strings.Contains(msg, "--name") {
		t.Errorf("error message does not contain --name hint: %q", msg)
	}
}
