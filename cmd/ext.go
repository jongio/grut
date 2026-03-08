package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/extension"
	"github.com/spf13/cobra"
)

// extManager returns a Manager rooted in the XDG data directory.
func extManager() *extension.Manager {
	dir := filepath.Join(config.DataDir(), "extensions")
	mgr := extension.NewManager(dir)
	_ = mgr.LoadAll()
	return mgr
}

// newExtCmd creates the extension management command and its subcommands.
func newExtCmd() *cobra.Command {
	extCmd := &cobra.Command{
		Use:   "ext",
		Short: "Manage grut extensions",
		Long:  `Install, list, enable, disable, and remove grut extensions.`,
	}

	extCmd.AddCommand(newExtCreateCmd())
	extCmd.AddCommand(newExtInstallCmd())
	extCmd.AddCommand(newExtRemoveCmd())
	extCmd.AddCommand(newExtListCmd())
	extCmd.AddCommand(newExtEnableCmd())
	extCmd.AddCommand(newExtDisableCmd())
	extCmd.AddCommand(newExtInfoCmd())

	return extCmd
}

func newExtInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install <url-or-path>",
		Short: "Install an extension from a git URL or local path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := extManager()
			if err := mgr.Install(cmd.Context(), args[0]); err != nil {
				return fmt.Errorf("ext install: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Installed extension from %s\n", args[0])
			return nil
		},
	}
}

func newExtRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed extension",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := extManager()
			if err := mgr.Remove(args[0]); err != nil {
				return fmt.Errorf("ext remove: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Removed extension %s\n", args[0])
			return nil
		},
	}
}

func newExtListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed extensions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr := extManager()
			list := mgr.List()
			if len(list) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No extensions installed.")
				return nil
			}
			for _, ext := range list {
				status := "enabled"
				if !ext.Enabled {
					status = "disabled"
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-10s %-8s %s\n",
					ext.Manifest.Name, ext.Manifest.Version, status, ext.Manifest.Runtime)
			}
			return nil
		},
	}
}

func newExtEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <name>",
		Short: "Enable a disabled extension",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := extManager()
			if err := mgr.Enable(args[0]); err != nil {
				return fmt.Errorf("ext enable: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Enabled extension %s\n", args[0])
			return nil
		},
	}
}

func newExtDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <name>",
		Short: "Disable an extension without removing it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := extManager()
			if err := mgr.Disable(args[0]); err != nil {
				return fmt.Errorf("ext disable: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Disabled extension %s\n", args[0])
			return nil
		},
	}
}

func newExtCreateCmd() *cobra.Command {
	var templateName string
	var listTemplates bool

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Scaffold a new extension project",
		Long:  `Create a new extension project from a template. Use --list to see available templates.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if listTemplates {
				for _, tmpl := range extension.ListTemplates() {
					fmt.Fprintf(cmd.OutOrStdout(), "%-15s %-8s %s\n",
						tmpl.Name, tmpl.Runtime, tmpl.Description)
				}
				return nil
			}

			if len(args) == 0 {
				return fmt.Errorf("extension name is required (use --list to see available templates)")
			}

			name := args[0]
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			if err := extension.Scaffold(dir, name, templateName); err != nil {
				return fmt.Errorf("ext create: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created extension %q using %q template in ./%s\n",
				name, templateName, name)
			return nil
		},
	}

	cmd.Flags().StringVarP(&templateName, "template", "t", "lua",
		"Template to use (lua, wasm-go, mcp-python, mcp-node)")
	cmd.Flags().BoolVar(&listTemplates, "list", false,
		"List available templates")

	return cmd
}

func newExtInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show details about an installed extension",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := extManager()
			info, err := mgr.Get(args[0])
			if err != nil {
				return fmt.Errorf("ext info: %w", err)
			}
			w := cmd.OutOrStdout()
			status := "enabled"
			if !info.Enabled {
				status = "disabled"
			}
			fmt.Fprintf(w, "Name:        %s\n", info.Manifest.Name)
			fmt.Fprintf(w, "Version:     %s\n", info.Manifest.Version)
			fmt.Fprintf(w, "Runtime:     %s\n", info.Manifest.Runtime)
			fmt.Fprintf(w, "Status:      %s\n", status)
			fmt.Fprintf(w, "Description: %s\n", info.Manifest.Description)
			fmt.Fprintf(w, "Author:      %s\n", info.Manifest.Author)
			fmt.Fprintf(w, "License:     %s\n", info.Manifest.License)
			fmt.Fprintf(w, "Entry Point: %s\n", info.Manifest.EntryPoint)
			fmt.Fprintf(w, "Permissions: %s\n", strings.Join(info.Manifest.Permissions, ", "))
			fmt.Fprintf(w, "Min Grut:    %s\n", info.Manifest.MinGrut)
			fmt.Fprintf(w, "Installed:   %s\n", info.InstalledAt.Format("2006-01-02 15:04:05 UTC"))
			fmt.Fprintf(w, "Directory:   %s\n", info.Dir)
			return nil
		},
	}
}
