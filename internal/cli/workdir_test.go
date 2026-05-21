package cli

import (
	"testing"

	"github.com/axsmak/aim/internal/globalconfig"
)

func TestResolveWorkDirNoConfig(t *testing.T) {
	homeDir := t.TempDir()
	got := resolveWorkDir(homeDir)
	if got != "." {
		t.Errorf("expected '.', got %q", got)
	}
}

func TestResolveWorkDirWithConfig(t *testing.T) {
	homeDir := t.TempDir()
	want := "/some/repo/path"
	if err := globalconfig.Save(homeDir, globalconfig.Config{Repo: want}); err != nil {
		t.Fatal(err)
	}
	got := resolveWorkDir(homeDir)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}
