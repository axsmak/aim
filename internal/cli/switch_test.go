package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/globalconfig"
)

func TestSwitchURLRejected(t *testing.T) {
	homeDir := t.TempDir()
	var out bytes.Buffer

	err := runSwitch("https://github.com/user/repo", homeDir, &out)
	if err == nil {
		t.Fatal("expected error for HTTPS URL, got nil")
	}
	if !strings.Contains(err.Error(), "aiman init") {
		t.Errorf("error should mention 'aiman init', got: %s", err.Error())
	}
}

func TestSwitchGitAtURLRejected(t *testing.T) {
	homeDir := t.TempDir()
	var out bytes.Buffer

	err := runSwitch("git@github.com:user/repo.git", homeDir, &out)
	if err == nil {
		t.Fatal("expected error for git@ URL, got nil")
	}
	if !strings.Contains(err.Error(), "aiman init") {
		t.Errorf("error should mention 'aiman init', got: %s", err.Error())
	}
}

func TestSwitchPathDoesNotExist(t *testing.T) {
	homeDir := t.TempDir()
	var out bytes.Buffer

	err := runSwitch("/nonexistent/path/that/does/not/exist", homeDir, &out)
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should mention 'does not exist', got: %s", err.Error())
	}
}

func TestSwitchNotAIMRepo(t *testing.T) {
	homeDir := t.TempDir()
	emptyDir := t.TempDir()
	var out bytes.Buffer

	err := runSwitch(emptyDir, homeDir, &out)
	if err == nil {
		t.Fatal("expected error for non-AIM directory, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid AIM repository") {
		t.Errorf("error should mention 'not a valid AIM repository', got: %s", err.Error())
	}
}

func TestSwitchValidRepoWithGitAndSkills(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()
	var out bytes.Buffer

	// Create .git directory and skills/ directory
	if err := os.Mkdir(filepath.Join(repoDir, ".git"), 0755); err != nil {
		t.Fatalf("failed to create .git: %v", err)
	}
	if err := os.Mkdir(filepath.Join(repoDir, "skills"), 0755); err != nil {
		t.Fatalf("failed to create skills/: %v", err)
	}

	err := runSwitch(repoDir, homeDir, &out)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	absPath, _ := filepath.Abs(repoDir)
	if !strings.Contains(out.String(), "switched to "+absPath) {
		t.Errorf("expected output 'switched to %s', got: %s", absPath, out.String())
	}

	// Verify global config updated
	cfg, err := globalconfig.Load(homeDir)
	if err != nil {
		t.Fatalf("failed to load global config: %v", err)
	}
	if cfg.Repo != absPath {
		t.Errorf("expected repo %s in config, got: %s", absPath, cfg.Repo)
	}
}

func TestSwitchValidRepoWithAimLocalYaml(t *testing.T) {
	homeDir := t.TempDir()
	repoDir := t.TempDir()
	var out bytes.Buffer

	// Create aim.local.yaml
	if err := os.WriteFile(filepath.Join(repoDir, "aim.local.yaml"), []byte("repo: .\n"), 0644); err != nil {
		t.Fatalf("failed to create aim.local.yaml: %v", err)
	}

	err := runSwitch(repoDir, homeDir, &out)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	absPath, _ := filepath.Abs(repoDir)
	if !strings.Contains(out.String(), "switched to "+absPath) {
		t.Errorf("expected output 'switched to %s', got: %s", absPath, out.String())
	}

	// Verify global config updated
	cfg, err := globalconfig.Load(homeDir)
	if err != nil {
		t.Fatalf("failed to load global config: %v", err)
	}
	if cfg.Repo != absPath {
		t.Errorf("expected repo %s in config, got: %s", absPath, cfg.Repo)
	}
}

func TestSwitchTildeExpansion(t *testing.T) {
	homeDir := t.TempDir()
	var out bytes.Buffer

	// Create a subdirectory inside homeDir simulating ~/some/path
	subDir := filepath.Join(homeDir, "some", "path")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdirectory: %v", err)
	}
	// Make it a valid AIM repo (aim.local.yaml)
	if err := os.WriteFile(filepath.Join(subDir, "aim.local.yaml"), []byte("repo: .\n"), 0644); err != nil {
		t.Fatalf("failed to create aim.local.yaml: %v", err)
	}

	// Pass ~/some/path which should expand to homeDir/some/path
	err := runSwitch("~/some/path", homeDir, &out)
	if err != nil {
		t.Fatalf("expected success with tilde expansion, got error: %v", err)
	}

	expectedPath := subDir
	if !strings.Contains(out.String(), "switched to "+expectedPath) {
		t.Errorf("expected output 'switched to %s', got: %s", expectedPath, out.String())
	}
}

func TestSwitchCommandNoArgs(t *testing.T) {
	// Test the cobra command integration: no args should call errs.Fatal
	// We test this by verifying the command's RunE logic via root command
	rootCmd := NewRootCmd("test")
	rootCmd.SetArgs([]string{"switch"})
	var stderr bytes.Buffer
	rootCmd.SetErr(&stderr)

	// errs.Fatal calls os.Exit(1), so we can't test it directly without subprocess.
	// Instead, verify the command is registered and has correct Use string.
	switchCmd, _, err := rootCmd.Find([]string{"switch"})
	if err != nil {
		t.Fatalf("switch command not found: %v", err)
	}
	if switchCmd.Use != "switch <path>" {
		t.Errorf("unexpected Use: %s", switchCmd.Use)
	}
}
