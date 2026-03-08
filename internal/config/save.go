package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// SaveUserSetting updates a single dotted key (e.g. "preview.position") in
// the user's config file (~/.config/grut/config.toml).
//
// If the file does not exist it is created with just the changed setting.
// The write is atomic: data is written to a temporary file in the same
// directory and then renamed over the target.
func SaveUserSetting(key, value string) error {
	return saveSettingValue(key, value)
}

// SaveUserSettingBool updates a single dotted key with a boolean value.
// It behaves identically to SaveUserSetting but stores a native TOML
// boolean so that toml.Unmarshal decodes it correctly into Go bool fields.
func SaveUserSettingBool(key string, value bool) error {
	return saveSettingValue(key, value)
}

// saveSettingValue is the shared implementation for SaveUserSetting and
// SaveUserSettingBool. It accepts any value type that go-toml can marshal.
func saveSettingValue(key string, value any) error {
	cfgPath := configFilePath()

	// Load existing user config as a generic map so we preserve all
	// user-set values while only touching the target key.
	data := make(map[string]any)
	raw, err := os.ReadFile(cfgPath)
	if err == nil {
		if err := toml.Unmarshal(raw, &data); err != nil {
			return fmt.Errorf("parsing existing config %s: %w", cfgPath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading config %s: %w", cfgPath, err)
	}

	// Set the nested key.
	setNestedKey(data, key, value)

	// Marshal back to TOML.
	out, err := toml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	// Ensure the config directory exists.
	dir := filepath.Dir(cfgPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config dir %s: %w", dir, err)
	}

	// Atomic write: temp file → rename.
	tmp := cfgPath + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("writing temp config %s: %w", tmp, err)
	}
	if err := renameWithRetry(tmp, cfgPath); err != nil {
		// Clean up the temp file on rename failure.
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming temp config to %s: %w", cfgPath, err)
	}

	return nil
}

// setNestedKey sets a dotted key like "preview.position" in a nested map.
// Intermediate maps are created as needed.
func setNestedKey(m map[string]any, key string, value any) {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) == 1 {
		m[key] = value
		return
	}
	section, rest := parts[0], parts[1]
	sub, ok := m[section].(map[string]any)
	if !ok {
		sub = make(map[string]any)
		m[section] = sub
	}
	setNestedKey(sub, rest, value)
}

// renameWithRetry wraps os.Rename with retries on Windows where antivirus or
// filesystem indexers can briefly hold locks on recently-written files.
func renameWithRetry(src, dst string) error {
	const maxAttempts = 5
	var err error
	for i := range maxAttempts {
		err = os.Rename(src, dst)
		if err == nil {
			return nil
		}
		if runtime.GOOS != "windows" {
			return err
		}
		time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
	}
	return err
}
