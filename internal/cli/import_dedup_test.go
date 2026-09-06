package cli

import (
	"testing"

	"github.com/axsmak/aim/internal/adapter"
)

// TestDeduplicateMCP_ReturnsNilOnConflictingEntries covers the guard in
// newImportMCPCmd's RunE (see import.go: `found := deduplicateMCP(matches); if
// found == nil { ... }`) that decides whether the ambiguous-error branch fires.
//
// The branch itself — building the "adapter %s returned conflicting entries"
// message from name/from/sources (issue #184) — lives inside the RunE closure
// and cannot be exercised end-to-end: adapter.Registry() always returns the
// three real adapters (claude-code, cursor, codex), and each one already
// guarantees a unique ServerName within a single ScanMCP call (cursor/codex
// read one config with unique keys; claude-code dedupes explicitly between
// ~/.claude.json and settings.json, see internal/adapter/claude_code.go). So
// matches never has >1 entry with differing Command/Args in practice, and
// there is no seam to inject a fake MCPScanner without a broader refactor
// that is out of scope for this fix. This test instead pins down the
// dedupe/conflict-detection logic that feeds that branch, so a regression
// there (e.g. a future adapter violating the uniqueness contract) is still
// caught even though the message text itself isn't covered by an automated
// test.
func TestDeduplicateMCP_ReturnsNilOnConflictingEntries(t *testing.T) {
	matches := []adapter.DiscoveredMCP{
		{ServerName: "jira", Source: "cursor", Command: "npx", Args: []string{"-y", "server-a"}},
		{ServerName: "jira", Source: "claude-code", Command: "npx", Args: []string{"-y", "server-b"}},
	}

	if got := deduplicateMCP(matches); got != nil {
		t.Fatalf("expected nil for conflicting Command/Args, got %+v", got)
	}
}

func TestDeduplicateMCP_ReturnsSingleMatchWhenIdentical(t *testing.T) {
	matches := []adapter.DiscoveredMCP{
		{ServerName: "jira", Source: "cursor", Command: "npx", Args: []string{"-y", "server"}},
		{ServerName: "jira", Source: "claude-code", Command: "npx", Args: []string{"-y", "server"}},
	}

	got := deduplicateMCP(matches)
	if got == nil {
		t.Fatal("expected a match for identical Command/Args, got nil")
	}
	if got.Command != "npx" || len(got.Args) != 2 {
		t.Errorf("unexpected result: %+v", got)
	}
}
