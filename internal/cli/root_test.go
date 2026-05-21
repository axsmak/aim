package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/axsmak/aim/internal/cli"
)

func TestVersion(t *testing.T) {
	root := cli.NewRootCmd("0.0.1-test")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "0.0.1-test") {
		t.Errorf("want version string in output, got: %q", buf.String())
	}
}

func TestHelp(t *testing.T) {
	root := cli.NewRootCmd("dev")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("help output must not be empty")
	}
	if !strings.Contains(buf.String(), "aiman") {
		t.Errorf("help output must mention 'aiman', got: %q", buf.String())
	}
}

func TestHelpFlag(t *testing.T) {
	root := cli.NewRootCmd("dev")
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"--help"})
	// --help exits via cobra's help handler; cobra returns nil here
	root.Execute() //nolint:errcheck
	if buf.Len() == 0 {
		t.Error("--help output must not be empty")
	}
}

func TestUnknownCommand(t *testing.T) {
	root := cli.NewRootCmd("dev")
	errBuf := new(bytes.Buffer)
	root.SetErr(errBuf)
	root.SetArgs([]string{"nonexistent"})
	err := root.Execute()
	if err == nil {
		t.Error("unknown command must return an error")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error message must mention the unknown command, got: %q", err.Error())
	}
}
