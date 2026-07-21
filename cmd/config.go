package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jongio/grut/internal/config"
	"github.com/spf13/cobra"
)

type (
	configLoadFunc func() (*config.Config, error)
	configPathFunc func() string
)

func newConfigCmd() *cobra.Command {
	return newConfigCmdWithDeps(config.Load, config.UserConfigFilePath)
}

func newConfigCmdWithDeps(load configLoadFunc, path configPathFunc) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   cmdConfig,
		Short: "Inspect grut configuration",
	}
	configCmd.AddCommand(newConfigCheckCmd(load, path))
	configCmd.AddCommand(newConfigDefaultsCmd(config.DefaultsTOML))
	return configCmd
}

func newConfigCheckCmd(load configLoadFunc, path configPathFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate the active grut configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := path()
			if _, err := load(); err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\n", cfgPath)
				return fmt.Errorf("config check failed: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\nOK\n", cfgPath)
			return nil
		},
	}
}

func newConfigDefaultsCmd(defaults func() []byte) *cobra.Command {
	var output string

	defaultsCmd := &cobra.Command{
		Use:   "defaults",
		Short: "Print the default grut configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			data := defaults()
			if output == "" {
				_, err := cmd.OutOrStdout().Write(data)
				return err
			}
			if err := writeConfigDefaults(output, data); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote default config to %s\n", output)
			return nil
		},
	}
	defaultsCmd.Flags().StringVarP(&output, "output", "o", "", "Write defaults to this file")
	return defaultsCmd
}

func writeConfigDefaults(path string, data []byte) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create config defaults directory: %w", err)
		}
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config defaults: %w", err)
	}
	return nil
}
