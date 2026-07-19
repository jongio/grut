package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/jongio/grut/internal/config"
	toml "github.com/pelletier/go-toml/v2"
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
	configCmd.AddCommand(newConfigGetCmd(load))
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

// newConfigGetCmd builds the "config get" subcommand, which prints a single
// resolved configuration value addressed by a dotted key (for example
// "git.default_branch" or "preview"). Defaults are applied, so a key the user
// never set still returns grut's effective value. A scalar leaf prints as a
// bare value so it pipes cleanly into scripts; a section prints as TOML.
func newConfigGetCmd(load configLoadFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print a single resolved config value",
		Long: "Print the resolved value for a dotted config key.\n\n" +
			"Scalars print as a bare value so they pipe cleanly into scripts. " +
			"Sections print as TOML. Unknown keys exit non-zero.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			value, err := lookupConfigKey(cfg, args[0])
			if err != nil {
				return err
			}
			return writeConfigValue(cmd.OutOrStdout(), value)
		},
	}
}

// lookupConfigKey resolves a dotted key against the config by round-tripping
// through TOML into a generic map. This keeps the lookup in sync with the TOML
// tags that already define the on-disk schema, so there is no second source of
// truth for key names.
func lookupConfigKey(cfg *config.Config, key string) (any, error) {
	raw, err := toml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encoding config: %w", err)
	}
	var tree map[string]any
	if err := toml.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("decoding config: %w", err)
	}

	segments := strings.Split(key, ".")
	var current any = tree
	for _, segment := range segments {
		node, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("unknown config key: %s", key)
		}
		next, ok := node[segment]
		if !ok {
			return nil, fmt.Errorf("unknown config key: %s", key)
		}
		current = next
	}
	return current, nil
}

// writeConfigValue prints a resolved value. Sections print as TOML; slices
// print one element per line; scalars print as a bare value.
func writeConfigValue(out io.Writer, value any) error {
	switch v := value.(type) {
	case map[string]any:
		encoded, err := toml.Marshal(v)
		if err != nil {
			return fmt.Errorf("encoding value: %w", err)
		}
		_, err = fmt.Fprint(out, string(encoded))
		return err
	case []any:
		for _, item := range v {
			if _, err := fmt.Fprintf(out, "%v\n", item); err != nil {
				return err
			}
		}
		return nil
	default:
		_, err := fmt.Fprintf(out, "%v\n", v)
		return err
	}
}
