package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/cli"
)

func runDoctorCmd(t *testing.T, fakeHome, workDir string) (stdout string, err error) {
	t.Helper()
	t.Setenv("HOME", fakeHome)

	oldDir, cdErr := os.Getwd()
	if cdErr != nil {
		t.Fatalf("cannot get working directory: %v", cdErr)
	}
	t.Cleanup(func() { os.Chdir(oldDir) })
	if cdErr := os.Chdir(workDir); cdErr != nil {
		t.Fatalf("cannot chdir to %s: %v", workDir, cdErr)
	}

	outBuf := new(bytes.Buffer)
	root := cli.NewRootCmd("test")
	root.SetOut(outBuf)
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"doctor"})

	err = root.Execute()
	return outBuf.String(), err
}

func TestDoctor_AllFoundNoIssues(t *testing.T) {
	fakeHome := t.TempDir()
	for _, dir := range []string{".claude", ".cursor", ".codex"} {
		if err := os.Mkdir(filepath.Join(fakeHome, dir), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, err := runDoctorCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "No issues found.") {
		t.Errorf("expected 'No issues found.' in output, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "✓ claude-code") {
		t.Errorf("expected claude-code found, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Found: 1 valid, 0 invalid") {
		t.Errorf("expected skills count, got:\n%s", stdout)
	}
}

func TestDoctor_NoneFound(t *testing.T) {
	fakeHome := t.TempDir()
	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	stdout, err := runDoctorCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "✗ claude-code") {
		t.Errorf("expected claude-code not found, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "✗ cursor") {
		t.Errorf("expected cursor not found, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "✗ codex") {
		t.Errorf("expected codex not found, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "No issues found.") {
		t.Errorf("expected issues, but got 'No issues found.'")
	}
	// All three adapters should appear in Issues section
	issueCount := strings.Count(stdout, "not installed or not found at")
	if issueCount != 3 {
		t.Errorf("expected 3 issues, got %d in:\n%s", issueCount, stdout)
	}
}

func TestDoctor_MixedWithInvalidSkill(t *testing.T) {
	fakeHome := t.TempDir()
	if err := os.Mkdir(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatalf("write valid skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "bad-skill.md"), []byte(invalidSkillContent), 0644); err != nil {
		t.Fatalf("write invalid skill: %v", err)
	}

	stdout, err := runDoctorCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Found: 1 valid, 1 invalid") {
		t.Errorf("expected 1 valid 1 invalid, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "✓ claude-code") {
		t.Errorf("expected claude-code found, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "invalid:") {
		t.Errorf("expected invalid skill in issues, got:\n%s", stdout)
	}
}

func TestDoctor_MCPEnv_AllSet(t *testing.T) {
	fakeHome := t.TempDir()
	if err := os.Mkdir(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	mcpDir := filepath.Join(workDir, "mcp")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(mcpDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatal(err)
	}
	mcpContent := "name: my-server\ndescription: test\ncommand: npx\nargs: []\ntargets:\n  - claude-code\nenv:\n  - name: API_KEY\n    description: API key\n    required: true\n"
	if err := os.WriteFile(filepath.Join(mcpDir, "my-server.yaml"), []byte(mcpContent), 0644); err != nil {
		t.Fatal(err)
	}
	localCfg := "mcp_env:\n  my-server.API_KEY: secret\n"
	if err := os.WriteFile(filepath.Join(workDir, "aim.local.yaml"), []byte(localCfg), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, err := runDoctorCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "MCP Environment Variables") {
		t.Errorf("expected MCP Environment Variables section, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "✓ my-server › API_KEY") {
		t.Errorf("expected ✓ for API_KEY, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "missing") {
		t.Errorf("expected no 'missing' when env is set, got:\n%s", stdout)
	}
}

func TestDoctor_MCPEnv_Missing(t *testing.T) {
	fakeHome := t.TempDir()
	if err := os.Mkdir(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	mcpDir := filepath.Join(workDir, "mcp")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(mcpDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatal(err)
	}
	mcpContent := "name: my-server\ndescription: test\ncommand: npx\nargs: []\ntargets:\n  - claude-code\nenv:\n  - name: API_KEY\n    description: API key\n    required: true\n"
	if err := os.WriteFile(filepath.Join(mcpDir, "my-server.yaml"), []byte(mcpContent), 0644); err != nil {
		t.Fatal(err)
	}
	// No aim.local.yaml = no env values

	stdout, err := runDoctorCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "✗ my-server › API_KEY") {
		t.Errorf("expected ✗ for missing API_KEY, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "missing (required)") {
		t.Errorf("expected 'missing (required)' text, got:\n%s", stdout)
	}
}

func TestDoctor_NoMCPDir_NoSection(t *testing.T) {
	fakeHome := t.TempDir()
	if err := os.Mkdir(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	if err := os.Mkdir(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "test-skill.md"), []byte(validSkillContent), 0644); err != nil {
		t.Fatal(err)
	}
	// No mcp/ directory

	stdout, err := runDoctorCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// MCP section should not appear
	if strings.Contains(stdout, "MCP env") {
		t.Errorf("MCP env section should not appear when no mcp/ dir, got:\n%s", stdout)
	}
}

func TestDoctor_MissingSkillsDir(t *testing.T) {
	fakeHome := t.TempDir()
	if err := os.Mkdir(filepath.Join(fakeHome, ".claude"), 0755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	workDir := t.TempDir()

	stdout, err := runDoctorCmd(t, fakeHome, workDir)
	if err != nil {
		t.Fatalf("expected no error with missing skills dir, got: %v", err)
	}
	if !strings.Contains(stdout, "Found: 0 valid, 0 invalid") {
		t.Errorf("expected 0 valid 0 invalid, got:\n%s", stdout)
	}
}
