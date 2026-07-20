package cmd

import (
	"fmt"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/keymap"
	"github.com/spf13/cobra"
)

type (
	configLoadFunc func() (*config.Config, error)
	configPathFunc func() string
	keymapLoadFunc func(string) (*keymap.Keymap, error)
)

func newConfigCmd() *cobra.Command {
	return newConfigCmdWithDeps(config.Load, config.UserConfigFilePath)
}

func newConfigCmdWithDeps(load configLoadFunc, path configPathFunc) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   cmdConfig,
		Short: "Inspect grut configuration",
	}
	configCmd.AddCommand(newConfigCheckCmdWithKeymap(load, path, keymap.NewKeymap))
	return configCmd
}

func newConfigCheckCmd(load configLoadFunc, path configPathFunc) *cobra.Command {
	return newConfigCheckCmdWithKeymap(load, path, keymap.NewKeymap)
}

func newConfigCheckCmdWithKeymap(load configLoadFunc, path configPathFunc, loadKeymap keymapLoadFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate the active grut configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfgPath := path()
			cfg, err := load()
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\n", cfgPath)
				return fmt.Errorf("config check failed: %w", err)
			}
			if err := checkKeybindings(cmd, cfg, cfgPath, loadKeymap); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\nOK\n", cfgPath)
			return nil
		},
	}
}

func checkKeybindings(cmd *cobra.Command, cfg *config.Config, cfgPath string, loadKeymap keymapLoadFunc) error {
	scheme := cfg.General.KeybindingScheme
	if scheme == "" {
		scheme = "default"
	}
	km, err := loadKeymap(scheme)
	if err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\n", cfgPath)
		return fmt.Errorf("config check failed: keybindings: %w", err)
	}
	conflicts := keymap.DetectConflicts(km.Bindings())
	if len(conflicts) == 0 {
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\nKeybinding conflicts:\n", cfgPath)
	for _, conflict := range conflicts {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", conflict.String())
	}
	return fmt.Errorf("config check failed: %d keybinding conflict(s) found", len(conflicts))
}
