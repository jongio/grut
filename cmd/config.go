package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/keymap"
	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

type (
	configLoadFunc func() (*config.Config, error)
	configPathFunc func() string
	dataPathFunc   func() string
	keymapLoadFunc func(string) (*keymap.Keymap, error)
)

func newConfigCmd() *cobra.Command {
	return newConfigCmdWithDeps(config.Load, config.UserConfigFilePath, config.DataDir)
}

func newConfigCmdWithDeps(load configLoadFunc, path configPathFunc, dataPath dataPathFunc) *cobra.Command {
	configCmd := &cobra.Command{
		Use:   cmdConfig,
		Short: "Inspect grut configuration",
	}
	configCmd.AddCommand(newConfigCheckCmd(load, path))
	configCmd.AddCommand(newConfigPathCmd(path, dataPath))
	configCmd.AddCommand(newConfigGetCmd(load))
	configCmd.AddCommand(newConfigDefaultsCmd(config.DefaultsTOML))
	return configCmd
}

func newConfigCheckCmd(load configLoadFunc, path configPathFunc) *cobra.Command {
	return newConfigCheckCmdWithKeymap(load, path, keymap.NewKeymap)
}

func newConfigCheckCmdWithKeymap(load configLoadFunc, path configPathFunc, loadKeymap keymapLoadFunc) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:          "check",
		Short:        "Validate the active grut configuration",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigCheck(cmd, load, path, loadKeymap, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output the config check report as JSON")
	return cmd
}

type configCheckReport struct {
	OK                  bool                 `json:"ok"`
	ConfigPath          string               `json:"config_path"`
	Error               string               `json:"error,omitempty"`
	KeybindingConflicts []keybindingConflict `json:"keybinding_conflicts"`
}

type keybindingConflict struct {
	Key     string   `json:"key"`
	Mode    string   `json:"mode"`
	Context string   `json:"context"`
	Actions []string `json:"actions"`
}

func runConfigCheck(cmd *cobra.Command, load configLoadFunc, path configPathFunc, loadKeymap keymapLoadFunc, asJSON bool) error {
	cfgPath := path()
	report := configCheckReport{ConfigPath: cfgPath, KeybindingConflicts: []keybindingConflict{}}
	cfg, err := load()
	if err != nil {
		report.Error = err.Error()
		if asJSON {
			if encodeErr := writeConfigCheckJSON(cmd.OutOrStdout(), report); encodeErr != nil {
				return encodeErr
			}
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\n", cfgPath)
		}
		return fmt.Errorf("config check failed: %w", err)
	}

	conflicts, err := detectConfigKeybindingConflicts(cfg, loadKeymap)
	if err != nil {
		report.Error = fmt.Sprintf("keybindings: %v", err)
		if asJSON {
			if encodeErr := writeConfigCheckJSON(cmd.OutOrStdout(), report); encodeErr != nil {
				return encodeErr
			}
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\n", cfgPath)
		}
		return fmt.Errorf("config check failed: keybindings: %w", err)
	}
	report.KeybindingConflicts = conflicts
	if len(conflicts) > 0 {
		report.Error = fmt.Sprintf("%d keybinding conflict(s) found", len(conflicts))
		if asJSON {
			if encodeErr := writeConfigCheckJSON(cmd.OutOrStdout(), report); encodeErr != nil {
				return encodeErr
			}
		} else {
			writeKeybindingConflicts(cmd.OutOrStdout(), cfgPath, conflicts)
		}
		return fmt.Errorf("config check failed: %d keybinding conflict(s) found", len(conflicts))
	}

	report.OK = true
	if asJSON {
		return writeConfigCheckJSON(cmd.OutOrStdout(), report)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\nOK\n", cfgPath)
	return nil
}

const configPathCommandName = "path"

func newConfigPathCmd(path configPathFunc, dataPath dataPathFunc) *cobra.Command {
	var asJSON bool
	pathCmd := &cobra.Command{
		Use:   configPathCommandName,
		Short: "Print resolved grut config and data paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := configPaths{
				Config: path(),
				Data:   dataPath(),
			}
			if asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(paths)
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\nData:   %s\n", paths.Config, paths.Data)
			return err
		},
	}
	pathCmd.Flags().BoolVar(&asJSON, "json", false, "Print resolved paths as JSON")
	return pathCmd
}

type configPaths struct {
	Config string `json:"config"`
	Data   string `json:"data"`
}

const defaultKeybindingScheme = "default"

func detectConfigKeybindingConflicts(cfg *config.Config, loadKeymap keymapLoadFunc) ([]keybindingConflict, error) {
	scheme := cfg.General.KeybindingScheme
	if scheme == "" {
		scheme = defaultKeybindingScheme
	}
	km, err := loadKeymap(scheme)
	if err != nil {
		return nil, err
	}
	conflicts := keymap.DetectConflicts(km.Bindings())
	out := make([]keybindingConflict, 0, len(conflicts))
	for _, conflict := range conflicts {
		out = append(out, keybindingConflict{
			Key:     conflict.Key,
			Mode:    conflict.Mode.String(),
			Context: conflict.Context,
			Actions: conflict.Actions,
		})
	}
	return out, nil
}

func writeKeybindingConflicts(out io.Writer, cfgPath string, conflicts []keybindingConflict) {
	fmt.Fprintf(out, "Config: %s\nKeybinding conflicts:\n", cfgPath)
	for _, conflict := range conflicts {
		ctx := conflict.Context
		if ctx == "" {
			ctx = "(all)"
		}
		fmt.Fprintf(out, "- key %q in mode %s context %s: actions %v\n", conflict.Key, conflict.Mode, ctx, conflict.Actions)
	}
}

func writeConfigCheckJSON(out io.Writer, report configCheckReport) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
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
