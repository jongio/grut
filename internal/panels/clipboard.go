package panels

import (
	"context"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// ansiRe matches ANSI escape sequences (CSI, OSC, and simple escapes).
var ansiRe = regexp.MustCompile(`\x1b(?:\[[0-9;]*[a-zA-Z]|\][^\x07]*\x07|\[[^\x1b]*|\(.)`)

// StripANSI removes ANSI/VT escape sequences from text.
func StripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

// CopyToClipboard copies text to the OS clipboard using platform-native
// commands. All platforms pipe via stdin to avoid shell interpretation of
// the text content (prevents command injection via crafted text).
func CopyToClipboard(ctx context.Context, text string) error {
	text = StripANSI(text)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows": //nolint:goconst // inline string is more readable here
		cmd = exec.CommandContext(ctx, "clip")
		cmd.Stdin = strings.NewReader(text)
	case "darwin": //nolint:goconst // inline string is more readable here
		cmd = exec.CommandContext(ctx, "pbcopy")
		cmd.Stdin = strings.NewReader(text)
	default:
		cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(text)
	}
	return cmd.Run()
}
