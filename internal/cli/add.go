package cli

import (
	"fmt"
	"os"

	"github.com/axsmak/aim/internal/adder"
	"github.com/axsmak/aim/internal/errs"
	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <type>",
		Short: "Add a skill or MCP item to the local inventory",
	}
	cmd.AddCommand(newAddSkillCmd())
	cmd.AddCommand(newAddMCPCmd())
	return cmd
}

func newAddSkillCmd() *cobra.Command {
	var name string
	var overwrite bool

	cmd := &cobra.Command{
		Use:   "skill <file|->",
		Short: "Add a skill file to the local inventory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				errs.Fatalf("cannot determine home directory: %v", err)
			}
			workDir := resolveWorkDir(homeDir)

			src := args[0]
			var r *os.File
			if src == "-" {
				r = os.Stdin
			} else {
				f, openErr := os.Open(src)
				if openErr != nil {
					return fmt.Errorf("cannot open %s: %w", src, openErr)
				}
				defer f.Close()
				r = f
			}

			opts := adder.AddOptions{
				WorkDir:   workDir,
				Name:      name,
				Overwrite: overwrite,
			}

			result, err := adder.Add("skill", r, opts)
			if err != nil {
				return err
			}
			if result.Identical {
				fmt.Fprintf(cmd.OutOrStdout(), "up to date: skill %s · already identical\n", result.Name)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "added: skill %s\n", result.Name)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "override skill name from frontmatter")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace existing skill if content differs")
	return cmd
}

func newAddMCPCmd() *cobra.Command {
	var name string
	var overwrite bool

	cmd := &cobra.Command{
		Use:   "mcp <file|->",
		Short: "Add an MCP server config to the local inventory",
		Long:  "Add an MCP server config to the local inventory.\nEnv values are stored in aim.local.yaml; mcp/<name>.yaml contains descriptors only.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				errs.Fatalf("cannot determine home directory: %v", err)
			}
			workDir := resolveWorkDir(homeDir)

			src := args[0]
			var r *os.File
			if src == "-" {
				r = os.Stdin
			} else {
				f, openErr := os.Open(src)
				if openErr != nil {
					return fmt.Errorf("cannot open %s: %w", src, openErr)
				}
				defer f.Close()
				r = f
			}

			opts := adder.AddOptions{
				WorkDir:   workDir,
				Name:      name,
				Overwrite: overwrite,
			}

			result, err := adder.Add("mcp", r, opts)
			if err != nil {
				return err
			}
			if result.Identical {
				fmt.Fprintf(cmd.OutOrStdout(), "up to date: mcp %s · already identical\n", result.Name)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "added: mcp %s\n", result.Name)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "override MCP server name from config")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "replace existing MCP server config if content differs")
	return cmd
}
