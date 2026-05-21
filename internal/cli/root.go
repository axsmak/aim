package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewRootCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "aiman",
		Short: "Agent Inventory Manager — manage skills and MCP configs across AI environments",
		Long: `AIM — CLI for publishing and applying skills and MCP configs
across Claude Code, Cursor, and Codex CLI.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				// cobra adds "help" as a subcommand only when other subcommands exist;
				// at stage 0 we handle it explicitly here.
				if args[0] == "help" {
					return cmd.Help()
				}
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.Name())
			}
			return cmd.Help()
		},
	}
	cmd.SetVersionTemplate("aiman version {{.Version}}\n")
	cmd.AddCommand(newSyncCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newDoctorCmd())
	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newPushCmd())
	cmd.AddCommand(newSwitchCmd())
	cmd.AddCommand(newApplyCmd())
	cmd.AddCommand(newAddCmd())
	cmd.AddCommand(newImportCmd())
	return cmd
}

func Execute(version string) error {
	root := NewRootCmd(version)
	return root.Execute()
}
