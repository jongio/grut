package cmd

import (
	"fmt"

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
