package config

import (
	"log/slog"
	"os"
	"strings"
)

// nerdTerminals lists TERM_PROGRAM values for terminals that commonly ship
// with or strongly recommend a nerd font. The comparison is case-insensitive.
var nerdTerminals = map[string]bool{
	"wezterm":          true,
	"kitty":            true,
	"alacritty":        true,
	"iterm.app":        true,
	"hyper":            true,
	"warp":             true,
	"rio":              true,
	"ghostty":          true,
	"windows_terminal": true, // Windows Terminal often paired with nerd fonts
}

// ResolveIconMode resolves the "auto" icon_mode to either "nerd" or "ascii"
// by inspecting environment variables. If the mode is already "nerd" or
// "ascii" it is returned unchanged.
//
// Detection order:
//  1. GRUT_NERD_FONT=1 → "nerd" (explicit opt-in)
//  2. GRUT_NERD_FONT=0 → "ascii" (explicit opt-out)
//  3. TERM_PROGRAM matches a known nerd-font terminal → "nerd"
//  4. WT_SESSION is set (Windows Terminal) → "nerd"
//  5. Otherwise → "ascii" (safe default)
func ResolveIconMode(mode string) string {
	if mode != "auto" {
		return mode
	}

	// 1. Explicit env var override.
	if v := os.Getenv("GRUT_NERD_FONT"); v != "" {
		switch v {
		case "1", "true", "yes":
			return "nerd" //nolint:goconst // inline string is more readable here
		case "0", "false", "no":
			return "ascii"
		}
	}

	// 2. Check TERM_PROGRAM for known nerd-font terminals.
	if tp := os.Getenv("TERM_PROGRAM"); tp != "" {
		if nerdTerminals[strings.ToLower(tp)] {
			return "nerd"
		}
	}

	// 3. Windows Terminal sets WT_SESSION.
	if os.Getenv("WT_SESSION") != "" {
		return "nerd"
	}

	// 4. Fall back to ascii — safe for any terminal.
	slog.Info("icon_mode is \"auto\" but no nerd font terminal detected; using ascii icons. "+
		"Set icon_mode = \"nerd\" in config or GRUT_NERD_FONT=1 for nerd font icons.",
		"hint", "https://www.nerdfonts.com/")
	return "ascii"
}
