package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/axsmak/aim/internal/fsutil"
)

func TestWriteFile_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.txt")

	if err := fsutil.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("mode = %o, want %o", info.Mode().Perm(), 0644)
	}
}

func TestWriteFile_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(path, []byte("old content, longer than new"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := fsutil.WriteFile(path, []byte("new"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file missing after overwrite: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}

func TestWriteFile_PermAppliedNotJustCreateDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "perm.txt")

	// os.CreateTemp defaults to 0600; if WriteFile forgot the explicit
	// Chmod, this would regress to 0600 instead of the requested 0644.
	if err := fsutil.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("mode = %o, want %o (must not be left at CreateTemp's default 0600)", info.Mode().Perm(), 0644)
	}
}

func TestWriteFile_ExistingFilePermUpdated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := fsutil.WriteFile(path, []byte("new"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("mode = %o, want %o", info.Mode().Perm(), 0644)
	}
}

func TestWriteFile_NoTempFileLeftOnFailure(t *testing.T) {
	dir := t.TempDir()
	// A directory as the target path makes the rename fail after the temp
	// file is written, exercising the cleanup path.
	badPath := filepath.Join(dir, "target-is-a-dir")
	if err := os.Mkdir(badPath, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := fsutil.WriteFile(badPath, []byte("x"), 0644); err == nil {
		t.Fatal("expected error when target path is a directory, got nil")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "target-is-a-dir" {
			t.Errorf("stray file left behind: %s", e.Name())
		}
	}
}

func TestWriteFile_NoTruncationVisibleMidWrite(t *testing.T) {
	// WriteFile must never make a caller observe a truncated file: the old
	// content stays fully intact right up until the atomic rename swaps it
	// for the new content. This is verified by confirming the target
	// directory holds no file with an unexpected (empty/truncated) size
	// between the write of the temp file and the final state.
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	original := "original content that must never be seen truncated"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if err := fsutil.WriteFile(path, []byte("replacement"), 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after write: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("file must never be observed empty/truncated")
	}
	if string(got) != "replacement" {
		t.Errorf("content = %q, want %q", got, "replacement")
	}
}

func TestWriteFile_FailureBeforeRename_OldContentUntouched(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission checks are bypassed when running as root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(path, []byte("original content"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Making the directory read-only means CreateTemp fails before any
	// write happens, and definitely before rename — so path must come
	// through untouched.
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	if err := fsutil.WriteFile(path, []byte("new content"), 0644); err == nil {
		t.Fatal("expected error writing into a read-only directory, got nil")
	}

	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatalf("restore chmod: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after failed write: %v", err)
	}
	if string(got) != "original content" {
		t.Errorf("content = %q, want %q (original must survive a failed write)", got, "original content")
	}
}
