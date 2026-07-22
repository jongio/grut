package cmd

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jongio/grut/internal/config"
	"github.com/jongio/grut/internal/extension"
	"github.com/spf13/cobra"
)

// extManager returns a Manager rooted in the XDG data directory.
func extManager() *extension.Manager {
	dir := filepath.Join(config.DataDir(), "extensions")
	mgr := extension.NewManager(dir)
	if err := mgr.LoadAll(); err != nil {
		slog.Warn("load extensions", "error", err)
	}
	return mgr
}

// newExtCmd creates the extension management command and its subcommands.
func newExtCmd() *cobra.Command {
	extCmd := &cobra.Command{
		Use:   "ext",
		Short: "Manage grut extensions",
		Long:  `Create, validate, install, list, enable, disable, and remove grut extensions.`,
	}

	extCmd.AddCommand(newExtCreateCmd())
	extCmd.AddCommand(newExtValidateCmd())
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
	var jsonFlag bool
	cmd := &cobra.Command{
		Use:   cmdList,
		Short: "List installed extensions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			mgr := extManager()
			list := mgr.List()
			if jsonFlag {
				return printExtensionListJSON(cmd, list)
			}
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
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "Print machine-readable JSON")
	return cmd
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
	cmd.Flags().BoolVar(&listTemplates, cmdList, false,
		"List available templates")

	return cmd
}

func newExtValidateCmd() *cobra.Command {
	var jsonFlag bool
	cmd := &cobra.Command{
		Use:          "validate [path]",
		Short:        "Validate a local extension project",
		Long:         `Validate a local extension project manifest and entry point without installing it.`,
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}

			result := validateExtensionProject(path)
			if jsonFlag {
				if err := writeExtensionJSON(cmd, result); err != nil {
					return err
				}
			} else {
				printExtensionValidation(cmd, result)
			}
			if result.Status != extValidationStatusValid {
				return fmt.Errorf("ext validate: %s", strings.Join(result.Errors, "; "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "Print machine-readable JSON")
	return cmd
}

func newExtInfoCmd() *cobra.Command {
	var jsonFlag bool
	cmd := &cobra.Command{
		Use:   "info <name>",
		Short: "Show details about an installed extension",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr := extManager()
			info, err := mgr.Get(args[0])
			if err != nil {
				return fmt.Errorf("ext info: %w", err)
			}
			if jsonFlag {
				return printExtensionJSON(cmd, *info)
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
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "Print machine-readable JSON")
	return cmd
}

const (
	extValidationStatusValid   = "valid"
	extValidationStatusInvalid = "invalid"
	extRuntimeLua              = "lua"
	extRuntimeMCP              = "mcp"
	extRuntimeWasm             = "wasm"
)

type extensionValidationManifestJSON struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Runtime     string   `json:"runtime"`
	EntryPoint  string   `json:"entry_point"`
	MinGrut     string   `json:"min_grut"`
	Permissions []string `json:"permissions"`
}

type extensionValidationJSON struct {
	Manifest *extensionValidationManifestJSON `json:"manifest,omitempty"`
	Status   string                           `json:"status"`
	Path     string                           `json:"path"`
	Warnings []string                         `json:"warnings"`
	Errors   []string                         `json:"errors"`
}

type extensionInventoryJSON struct {
	InstalledAt time.Time `json:"installed_at"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Author      string    `json:"author"`
	License     string    `json:"license"`
	Runtime     string    `json:"runtime"`
	EntryPoint  string    `json:"entry_point"`
	MinGrut     string    `json:"min_grut"`
	Permissions []string  `json:"permissions"`
	Directory   string    `json:"directory"`
	SourceURL   string    `json:"source_url,omitempty"`
	CommitHash  string    `json:"commit_hash,omitempty"`
	Enabled     bool      `json:"enabled"`
}

func validateExtensionProject(path string) extensionValidationJSON {
	result := extensionValidationJSON{
		Status:   extValidationStatusValid,
		Path:     path,
		Warnings: []string{},
		Errors:   []string{},
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return invalidExtensionValidation(result, fmt.Sprintf("resolve path %q: %v", path, err))
	}
	result.Path = absPath

	info, err := os.Stat(absPath)
	if err != nil {
		return invalidExtensionValidation(result, fmt.Sprintf("extension path %q is not accessible: %v", absPath, err))
	}
	if !info.IsDir() {
		return invalidExtensionValidation(result, fmt.Sprintf("extension path %q is not a directory", absPath))
	}

	manifest, err := extension.LoadManifest(absPath)
	if err != nil {
		return invalidExtensionValidation(result, fmt.Sprintf("manifest validation failed: %v", err))
	}
	result.Manifest = &extensionValidationManifestJSON{
		Name:        manifest.Name,
		Version:     manifest.Version,
		Description: manifest.Description,
		Runtime:     manifest.Runtime,
		EntryPoint:  manifest.EntryPoint,
		MinGrut:     manifest.MinGrut,
		Permissions: append([]string(nil), manifest.Permissions...),
	}

	validateExtensionEntryPoint(&result, absPath, manifest)
	addExtensionRuntimeHints(&result, manifest)
	if len(result.Errors) > 0 {
		result.Status = extValidationStatusInvalid
	}
	return result
}

func validateExtensionEntryPoint(result *extensionValidationJSON, projectDir string, manifest *extension.Manifest) {
	if manifest.EntryPoint == "" {
		result.Warnings = append(result.Warnings, "entry_point is not set; grut may not know which file to load")
		return
	}

	entryPath := filepath.Clean(filepath.Join(projectDir, manifest.EntryPoint))
	rel, err := filepath.Rel(projectDir, entryPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		result.Errors = append(result.Errors, fmt.Sprintf("entry_point %q escapes the extension directory", manifest.EntryPoint))
		return
	}

	info, err := os.Stat(entryPath)
	if err != nil {
		message := fmt.Sprintf("entry_point %q does not exist under %s", manifest.EntryPoint, projectDir)
		if manifest.Runtime == extRuntimeWasm {
			result.Warnings = append(result.Warnings, message+"; build the WebAssembly module before install or runtime use")
			return
		}
		result.Errors = append(result.Errors, message)
		return
	}
	if info.IsDir() {
		result.Errors = append(result.Errors, fmt.Sprintf("entry_point %q is a directory, not a file", manifest.EntryPoint))
	}
}

func addExtensionRuntimeHints(result *extensionValidationJSON, manifest *extension.Manifest) {
	switch manifest.Runtime {
	case extRuntimeLua:
		if manifest.EntryPoint != "" && !strings.HasSuffix(manifest.EntryPoint, ".lua") {
			result.Warnings = append(result.Warnings, "lua extensions usually use a .lua entry point")
		}
	case extRuntimeWasm:
		if manifest.EntryPoint != "" && !strings.HasSuffix(manifest.EntryPoint, ".wasm") {
			result.Warnings = append(result.Warnings, "wasm extensions should point entry_point at the built .wasm module")
		}
	case extRuntimeMCP:
		if manifest.EntryPoint != "" &&
			!strings.HasSuffix(manifest.EntryPoint, ".py") &&
			!strings.HasSuffix(manifest.EntryPoint, ".js") &&
			!strings.HasSuffix(manifest.EntryPoint, ".ts") {
			result.Warnings = append(result.Warnings, "mcp extensions usually use a Python or Node.js server entry point")
		}
	}
}

func invalidExtensionValidation(result extensionValidationJSON, message string) extensionValidationJSON {
	result.Status = extValidationStatusInvalid
	result.Errors = append(result.Errors, message)
	return result
}

func printExtensionValidation(cmd *cobra.Command, result extensionValidationJSON) {
	w := cmd.OutOrStdout()
	if result.Status == extValidationStatusValid {
		fmt.Fprintf(w, "✓ Extension project is valid: %s\n", result.Path)
		if result.Manifest != nil {
			fmt.Fprintf(w, "Name:        %s\n", result.Manifest.Name)
			fmt.Fprintf(w, "Version:     %s\n", result.Manifest.Version)
			fmt.Fprintf(w, "Runtime:     %s\n", result.Manifest.Runtime)
			fmt.Fprintf(w, "Entry Point: %s\n", result.Manifest.EntryPoint)
		}
	} else {
		fmt.Fprintf(w, "✗ Extension project is invalid: %s\n", result.Path)
	}
	if len(result.Warnings) > 0 {
		fmt.Fprintln(w, "Warnings:")
		for _, warning := range result.Warnings {
			fmt.Fprintf(w, "  ⚠ %s\n", warning)
		}
	}
	if len(result.Errors) > 0 {
		fmt.Fprintln(w, "Errors:")
		for _, validationErr := range result.Errors {
			fmt.Fprintf(w, "  ✗ %s\n", validationErr)
		}
	}
}

func printExtensionListJSON(cmd *cobra.Command, list []extension.ExtensionInfo) error {
	out := make([]extensionInventoryJSON, 0, len(list))
	for _, info := range list {
		out = append(out, extensionInventory(info))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return writeExtensionJSON(cmd, out)
}

func printExtensionJSON(cmd *cobra.Command, info extension.ExtensionInfo) error {
	return writeExtensionJSON(cmd, extensionInventory(info))
}

func extensionInventory(info extension.ExtensionInfo) extensionInventoryJSON {
	permissions := append([]string(nil), info.Manifest.Permissions...)
	return extensionInventoryJSON{
		Name:        info.Manifest.Name,
		Version:     info.Manifest.Version,
		Description: info.Manifest.Description,
		Author:      info.Manifest.Author,
		License:     info.Manifest.License,
		Runtime:     info.Manifest.Runtime,
		EntryPoint:  info.Manifest.EntryPoint,
		MinGrut:     info.Manifest.MinGrut,
		Permissions: permissions,
		Enabled:     info.Enabled,
		SourceURL:   info.SourceURL,
		CommitHash:  info.CommitHash,
		InstalledAt: info.InstalledAt,
		Directory:   info.Dir,
	}
}

func writeExtensionJSON(cmd *cobra.Command, value any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
