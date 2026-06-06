package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/gitops"
	"github.com/axsmak/aim/internal/globalconfig"
	"github.com/axsmak/aim/internal/localconfig"
)

// noReader is an empty reader used in tests that don't exercise the interactive prompt.
var noReader = strings.NewReader("")

// fakeGitOps is a configurable gitops.Ops for testing CLI commands without real git.
type fakeGitOps struct {
	cloneErr             error
	cloned               bool
	lsRemoteResult       string
	lsRemoteErr          error
	commitErr            error
	commitCalled         bool
	pushErr              error
	pushCalled           bool
	resetSoftCalled      bool
	headHashResult       string
	isFileStagedResult   map[string]bool
	isAncestorResult     bool
	isAncestorErr        error
}

func (f *fakeGitOps) Clone(url, dir string) error                         { f.cloned = true; return f.cloneErr }
func (f *fakeGitOps) Fetch(dir string) error                              { return nil }
func (f *fakeGitOps) ResetHard(dir, ref string) error                     { return nil }
func (f *fakeGitOps) IsFastForward(dir, ref string) (bool, error)         { return true, nil }
func (f *fakeGitOps) HasLocalChanges(dir, sinceHash string) (bool, error) { return false, nil }
func (f *fakeGitOps) HeadHash(dir string) (string, error) {
	h := f.headHashResult
	if h == "" {
		h = "abc1234567890123456789012345678901234567890"
	}
	return h, nil
}
func (f *fakeGitOps) RemoteHash(dir, ref string) (string, error) { return "abc1234", nil }
func (f *fakeGitOps) LsRemote(dir, ref string) (string, error)   { return f.lsRemoteResult, f.lsRemoteErr }
func (f *fakeGitOps) Commit(dir, msg string) error               { f.commitCalled = true; return f.commitErr }
func (f *fakeGitOps) Push(dir string) error                      { f.pushCalled = true; return f.pushErr }
func (f *fakeGitOps) ResetSoft(dir string) error                 { f.resetSoftCalled = true; return nil }
func (f *fakeGitOps) IsFileStaged(workDir, path string) (bool, error) {
	return f.isFileStagedResult[path], nil
}
func (f *fakeGitOps) IsAncestor(dir, ancestor, descendant string) (bool, error) {
	return f.isAncestorResult, f.isAncestorErr
}
func (f *fakeGitOps) HasDirtyWorktree(dir string) (bool, error)                    { return false, nil }
func (f *fakeGitOps) HasUntrackedInPaths(dir string, paths []string) (bool, error) { return false, nil }
func (f *fakeGitOps) UntrackedConflictsWithRef(dir, ref string, paths []string) ([]string, error) {
	return nil, nil
}
func (f *fakeGitOps) CountAheadBehind(dir, base, ref string) (int, int, error) { return 0, 0, nil }

var _ gitops.Ops = (*fakeGitOps)(nil)

func TestRunInit_AlreadyConfigured(t *testing.T) {
	dir := t.TempDir()
	homeDir := t.TempDir()

	// Pre-write global config so AIM is already configured
	if err := os.MkdirAll(filepath.Join(homeDir, ".config", "aim"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, ".config", "aim", "config.yaml"), []byte("repo: /some/path\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := runInit("https://example.com/repo.git", dir, homeDir, &fakeGitOps{}, noReader, &out)
	if err == nil {
		t.Fatal("expected error when AIM is already configured, got nil")
	}
	if !strings.Contains(err.Error(), "already configured") {
		t.Errorf("expected 'already configured' in error, got: %q", err.Error())
	}
}

func TestRunInit_WritesAimLocalYaml(t *testing.T) {
	dir := t.TempDir()
	fakeHome := t.TempDir()

	var out bytes.Buffer
	err := runInit("https://example.com/repo.git", dir, fakeHome, &fakeGitOps{}, noReader, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// aim.local.yaml should be created after successful init
	if _, err := os.Stat(filepath.Join(dir, "aim.local.yaml")); err != nil {
		t.Errorf("aim.local.yaml not created: %v", err)
	}
}

func TestRunInit_Success(t *testing.T) {
	dir := t.TempDir()
	fakeHome := t.TempDir() // no AI env dirs exist here

	fake := &fakeGitOps{}
	var out bytes.Buffer

	err := runInit("https://example.com/repo.git", dir, fakeHome, fake, noReader, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fake.cloned {
		t.Error("expected Clone to be called")
	}

	output := out.String()
	if !strings.Contains(output, "initialized:") {
		t.Errorf("expected initialized message, got: %q", output)
	}
	if !strings.Contains(output, "https://example.com/repo.git") {
		t.Errorf("expected URL in output, got: %q", output)
	}

	// aim.local.yaml should be created
	if _, err := os.Stat(filepath.Join(dir, "aim.local.yaml")); err != nil {
		t.Errorf("aim.local.yaml not created: %v", err)
	}
}

// --- Issue #57: URL validation ---

func TestRunInit_EmptyURL(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	err := runInit("", dir, dir, &fakeGitOps{}, noReader, &out)
	if err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
	if !strings.Contains(err.Error(), "repo-url cannot be empty") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestRunInit_WhitespaceURL(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	err := runInit("   ", dir, dir, &fakeGitOps{}, noReader, &out)
	if err == nil {
		t.Fatal("expected error for whitespace URL, got nil")
	}
	if !strings.Contains(err.Error(), "repo-url cannot be empty") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

// --- Issue #58: classifyRepo ---

func TestClassifyRepo_Adoptable(t *testing.T) {
	dir := t.TempDir()
	class, err := classifyRepo(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if class != repoAdoptable {
		t.Errorf("expected repoAdoptable, got %v", class)
	}
}

func TestClassifyRepo_ExistingAIM(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aim.yaml"), []byte("skill_paths: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	class, err := classifyRepo(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if class != repoExistingAIM {
		t.Errorf("expected repoExistingAIM, got %v", class)
	}
}

func TestClassifyRepo_SkillsIsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "skills"), []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := classifyRepo(dir)
	if err == nil {
		t.Fatal("expected error for skills as file, got nil")
	}
	if !strings.Contains(err.Error(), "skills exists as a file") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestClassifyRepo_McpIsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mcp"), []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := classifyRepo(dir)
	if err == nil {
		t.Fatal("expected error for mcp as file, got nil")
	}
	if !strings.Contains(err.Error(), "mcp exists as a file") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

// --- Issue #58: createScaffold ---

func TestCreateScaffold_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := createScaffold(dir); err != nil {
		t.Fatalf("createScaffold error: %v", err)
	}

	checkFile(t, dir, "aim.yaml")
	checkFile(t, dir, ".gitignore")
	checkFile(t, dir, filepath.Join("skills", ".gitkeep"))
	checkFile(t, dir, filepath.Join("mcp", ".gitkeep"))

	gitignore, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !strings.Contains(string(gitignore), "aim.local.yaml") {
		t.Error(".gitignore should contain aim.local.yaml")
	}
}

func TestCreateScaffold_GitignoreExists_AppendsLine(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := createScaffold(dir); err != nil {
		t.Fatalf("createScaffold error: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if !strings.Contains(string(content), "*.log") {
		t.Error("existing .gitignore content should be preserved")
	}
	if !strings.Contains(string(content), "aim.local.yaml") {
		t.Error(".gitignore should have aim.local.yaml appended")
	}
}

func TestCreateScaffold_GitignoreAlreadyHasLine(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("aim.local.yaml\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := createScaffold(dir); err != nil {
		t.Fatalf("createScaffold error: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	count := strings.Count(string(content), "aim.local.yaml")
	if count != 1 {
		t.Errorf("aim.local.yaml should appear exactly once, got %d", count)
	}
}

func TestCreateScaffold_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := createScaffold(dir); err != nil {
		t.Fatalf("first createScaffold error: %v", err)
	}
	// Write something into aim.yaml to detect if it gets overwritten
	if err := os.WriteFile(filepath.Join(dir, "aim.yaml"), []byte("# custom\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := createScaffold(dir); err != nil {
		t.Fatalf("second createScaffold error: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(dir, "aim.yaml"))
	if string(content) != "# custom\n" {
		t.Error("existing aim.yaml should not be overwritten")
	}
}

// --- Issue #58: validateExistingRepo ---

func TestValidateExistingRepo_Valid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aim.yaml"), []byte("skill_paths: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("aim.local.yaml\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := validateExistingRepo(dir, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() > 0 {
		t.Errorf("expected no warnings, got: %q", out.String())
	}
}

func TestValidateExistingRepo_SkillsMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aim.yaml"), []byte("skill_paths: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := validateExistingRepo(dir, &out)
	if err == nil {
		t.Fatal("expected error for missing skills/, got nil")
	}
	if !strings.Contains(err.Error(), "aim.yaml exists but skills/ is missing") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestValidateExistingRepo_BadYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aim.yaml"), []byte(":\ninvalid: [yaml\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := validateExistingRepo(dir, &out)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "aim.yaml is not valid YAML") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestValidateExistingRepo_McpIsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aim.yaml"), []byte("skill_paths: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mcp"), []byte("bad"), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := validateExistingRepo(dir, &out)
	if err == nil {
		t.Fatal("expected error for mcp as file, got nil")
	}
	if !strings.Contains(err.Error(), "mcp exists as a file") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestValidateExistingRepo_NoGitignoreWarning(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "aim.yaml"), []byte("skill_paths: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	// No .gitignore
	var out bytes.Buffer
	if err := validateExistingRepo(dir, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "aim.local.yaml not in .gitignore") {
		t.Errorf("expected warning about .gitignore, got: %q", out.String())
	}
}

// --- Issue #58: runInit integration with repo states ---

// fakeCloneGitOps creates files in the target dir during Clone to simulate repo state.
type fakeCloneGitOps struct {
	fakeGitOps
	setup func(dir string) // called inside Clone to populate the "cloned" dir
}

func (f *fakeCloneGitOps) Clone(url, dir string) error {
	f.cloned = true
	if f.setup != nil {
		f.setup(dir)
	}
	return f.cloneErr
}

func TestRunInit_AdoptableRepo(t *testing.T) {
	dir := t.TempDir()
	fakeHome := t.TempDir()
	fake := &fakeCloneGitOps{} // Clone creates nothing → adoptable

	var out bytes.Buffer
	if err := runInit("https://example.com/repo.git", dir, fakeHome, fake, noReader, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "(new library)") {
		t.Errorf("expected '(new library)' in output, got: %q", output)
	}
	if !strings.Contains(output, "Library scaffold created") {
		t.Errorf("expected scaffold message in output, got: %q", output)
	}
	checkFile(t, dir, "aim.yaml")
	checkFile(t, dir, ".gitignore")
	checkFile(t, dir, filepath.Join("skills", ".gitkeep"))
	checkFile(t, dir, filepath.Join("mcp", ".gitkeep"))
}

func TestRunInit_ExistingAIMRepo(t *testing.T) {
	dir := t.TempDir()
	fakeHome := t.TempDir()
	fake := &fakeCloneGitOps{
		setup: func(d string) {
			os.WriteFile(filepath.Join(d, "aim.yaml"), []byte("skill_paths: {}\n"), 0644)
			os.Mkdir(filepath.Join(d, "skills"), 0755)
			os.WriteFile(filepath.Join(d, ".gitignore"), []byte("aim.local.yaml\n"), 0644)
		},
	}

	var out bytes.Buffer
	if err := runInit("https://example.com/repo.git", dir, fakeHome, fake, noReader, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "(existing library)") {
		t.Errorf("expected '(existing library)' in output, got: %q", output)
	}
	if !strings.Contains(output, "Library found") {
		t.Errorf("expected 'Library found' in output, got: %q", output)
	}
}

func TestRunInit_ConflictingSkillsIsFile(t *testing.T) {
	dir := t.TempDir()
	fakeHome := t.TempDir()
	fake := &fakeCloneGitOps{
		setup: func(d string) {
			os.WriteFile(filepath.Join(d, "skills"), []byte("not a dir"), 0644)
		},
	}

	var out bytes.Buffer
	err := runInit("https://example.com/repo.git", dir, fakeHome, fake, noReader, &out)
	if err == nil {
		t.Fatal("expected error for conflicting skills file, got nil")
	}
	if !strings.Contains(err.Error(), "skills exists as a file") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

func TestRunInit_ExistingAIM_SkillsMissing(t *testing.T) {
	dir := t.TempDir()
	fakeHome := t.TempDir()
	fake := &fakeCloneGitOps{
		setup: func(d string) {
			// aim.yaml present but no skills/
			os.WriteFile(filepath.Join(d, "aim.yaml"), []byte("skill_paths: {}\n"), 0644)
		},
	}

	var out bytes.Buffer
	err := runInit("https://example.com/repo.git", dir, fakeHome, fake, noReader, &out)
	if err == nil {
		t.Fatal("expected error for missing skills/, got nil")
	}
	if !strings.Contains(err.Error(), "aim.yaml exists but skills/ is missing") {
		t.Errorf("unexpected error: %q", err.Error())
	}
}

// --- Issue #69: repoNameFromURL ---

func TestRepoNameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/user/my-loadout.git", "my-loadout"},
		{"https://github.com/user/my-loadout", "my-loadout"},
		{"git@github.com:user/my-loadout.git", "my-loadout"},
		{"git@github.com:user/my-loadout", "my-loadout"},
		{"", "loadout"},
		{"   ", "loadout"},
		{"https://github.com/user/skills-repo.git", "skills-repo"},
	}
	for _, tc := range tests {
		got := repoNameFromURL(tc.url)
		if got != tc.want {
			t.Errorf("repoNameFromURL(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

// --- Issue #69: global config written after successful init ---

func TestRunInit_WritesGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	fakeHome := t.TempDir()

	fake := &fakeGitOps{}
	var out bytes.Buffer

	if err := runInit("https://example.com/repo.git", dir, fakeHome, fake, noReader, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gcfg, err := globalconfig.Load(fakeHome)
	if err != nil {
		t.Fatalf("cannot load global config: %v", err)
	}
	if gcfg.Repo == "" {
		t.Error("expected global config repo to be set after init")
	}
	absDir, _ := filepath.Abs(dir)
	if gcfg.Repo != absDir {
		t.Errorf("global config repo = %q, want %q", gcfg.Repo, absDir)
	}
}

// --- Issue #69: --path flag is registered on init command ---

func TestInitCmd_PathFlagRegistered(t *testing.T) {
	rootCmd := NewRootCmd("test")
	initCmd, _, err := rootCmd.Find([]string{"init"})
	if err != nil {
		t.Fatalf("init command not found: %v", err)
	}
	flag := initCmd.Flags().Lookup("path")
	if flag == nil {
		t.Fatal("expected --path flag to be registered on init command, got nil")
	}
	if flag.DefValue != "" {
		t.Errorf("expected --path default value to be empty, got %q", flag.DefValue)
	}
}

// --- Issue #78: synced_hash written on aiman init (existing library) ---

func TestRunInit_ExistingAIM_WritesSyncedHash(t *testing.T) {
	dir := t.TempDir()
	fakeHome := t.TempDir()

	expectedHash := "deadbeef1234567890123456789012345678abcd"
	fake := &fakeCloneGitOps{
		fakeGitOps: fakeGitOps{headHashResult: expectedHash},
		setup: func(d string) {
			os.WriteFile(filepath.Join(d, "aim.yaml"), []byte("skill_paths: {}\n"), 0644)
			os.Mkdir(filepath.Join(d, "skills"), 0755)
			os.WriteFile(filepath.Join(d, ".gitignore"), []byte("aim.local.yaml\n"), 0644)
		},
	}

	var out bytes.Buffer
	if err := runInit("https://example.com/repo.git", dir, fakeHome, fake, noReader, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := localconfig.Load(dir)
	if err != nil {
		t.Fatalf("cannot load config: %v", err)
	}
	if cfg.SyncedHash != expectedHash {
		t.Errorf("want SyncedHash=%q, got %q", expectedHash, cfg.SyncedHash)
	}
}

func TestRunInit_Adoptable_NoSyncedHash(t *testing.T) {
	dir := t.TempDir()
	fakeHome := t.TempDir()

	fake := &fakeCloneGitOps{} // Clone creates nothing → adoptable

	var out bytes.Buffer
	if err := runInit("https://example.com/repo.git", dir, fakeHome, fake, noReader, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := localconfig.Load(dir)
	if err != nil {
		t.Fatalf("cannot load config: %v", err)
	}
	if cfg.SyncedHash != "" {
		t.Errorf("want empty SyncedHash for adoptable repo, got %q", cfg.SyncedHash)
	}
}

// checkFile asserts that a file exists in dir at the given relative path.
func checkFile(t *testing.T, dir, relPath string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, relPath)); err != nil {
		t.Errorf("expected file %q to exist: %v", relPath, err)
	}
}
