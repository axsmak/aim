package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/axsmak/aim/internal/errs"
	"github.com/axsmak/aim/internal/skill"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "list",
		Short:  "List skills from skills/",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				errs.Fatalf("cannot determine home directory: %v", err)
			}
			workDir := resolveWorkDir(homeDir)
			return runList(workDir+"/skills/", cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

func runList(skillsDir string, out, _ io.Writer) error {
	valid, invalid, err := skill.ReadAll(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(out, "No skills found in %s\n", skillsDir)
			return nil
		}
		return fmt.Errorf("cannot read %s: %w", skillsDir, err)
	}

	if len(valid) == 0 && len(invalid) == 0 {
		fmt.Fprintf(out, "No skills found in %s\n", skillsDir)
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tSTATUS")
	for _, s := range valid {
		fmt.Fprintf(w, "%s\t%s\tvalid\n", s.Name, s.Description)
	}
	for _, ve := range invalid {
		name := strings.TrimSuffix(filepath.Base(ve.FilePath), ".md")
		fmt.Fprintf(w, "%s\t\tinvalid: %s\n", name, formatValidationReason(ve))
	}
	w.Flush()

	fmt.Fprintf(out, "\nTotal: %d valid, %d invalid\n", len(valid), len(invalid))
	return nil
}

func formatValidationReason(ve skill.ValidationError) string {
	switch ve.Field {
	case "name":
		return "missing name"
	case "description":
		return "missing description"
	case "body":
		return "empty body"
	default:
		return ve.Field + ": " + ve.Message
	}
}
