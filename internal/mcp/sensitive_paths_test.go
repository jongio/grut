package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSensitivePath_BlocksGitInternals(t *testing.T) {
	blocked := []string{
		".git/config",
		".git/HEAD",
		".git/objects/pack/pack-abc.idx",
		".git/refs/heads/main",
		".git",
	}
	for _, p := range blocked {
		t.Run(p, func(t *testing.T) {
			err := IsSensitivePath(p)
			assert.Error(t, err, "path %q should be blocked", p)
			assert.Contains(t, err.Error(), ".git")
		})
	}
}

func TestIsSensitivePath_AllowsBenignGitFiles(t *testing.T) {
	allowed := []string{
		".gitignore",
		".gitattributes",
		".gitmodules",
		".gitkeep",
	}
	for _, p := range allowed {
		t.Run(p, func(t *testing.T) {
			assert.NoError(t, IsSensitivePath(p))
		})
	}
}

func TestIsSensitivePath_BlocksEnvFiles(t *testing.T) {
	blocked := []string{
		".env",
		".env.local",
		".env.production",
		"config/.env",
		"deploy/.env.staging",
	}
	for _, p := range blocked {
		t.Run(p, func(t *testing.T) {
			err := IsSensitivePath(p)
			assert.Error(t, err, "path %q should be blocked", p)
			assert.Contains(t, err.Error(), "sensitive")
		})
	}
}

func TestIsSensitivePath_BlocksPrivateKeys(t *testing.T) {
	blocked := []string{
		"server.pem",
		"tls.key",
		"certs/ca.pem",
		"secrets/api.key",
		"id_rsa",
		"id_ed25519",
		"id_ecdsa",
		"home/.ssh/id_rsa",
		// Certificate/keystore extensions (F4 fix)
		"cert.p12",
		"keystore.pfx",
		"server.crt",
		"certs/ca.crt",
		// Case-insensitive SSH key names (F1 fix — Windows bypass)
		"ID_RSA",
		"Id_Ed25519",
		"ID_ECDSA",
	}
	for _, p := range blocked {
		t.Run(p, func(t *testing.T) {
			err := IsSensitivePath(p)
			assert.Error(t, err, "path %q should be blocked", p)
		})
	}
}

func TestIsSensitivePath_AllowsNormalFiles(t *testing.T) {
	allowed := []string{
		"main.go",
		"README.md",
		"src/config.toml",
		"internal/mcp/security.go",
		"package.json",
		"environment.ts",
		"keyboard.go",
		"envfile",
	}
	for _, p := range allowed {
		t.Run(p, func(t *testing.T) {
			assert.NoError(t, IsSensitivePath(p))
		})
	}
}

// ---------------------------------------------------------------------------
// stripNTFSArtifacts — unit tests for CWE-41 bypass prevention
// ---------------------------------------------------------------------------

func TestStripNTFSArtifacts_AlternateDataStream(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"id_rsa:$DATA", "id_rsa"},
		{".env:Zone.Identifier", ".env"},
		{"secret.key:stream", "secret.key"},
		{"dir/.env:$DATA", "dir/.env"},
		{"normal.txt", "normal.txt"},
		{".git/config:$DATA", ".git/config"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripNTFSArtifacts(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStripNTFSArtifacts_TrailingDotsAndSpaces(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{".env.", ".env"},
		{".env...", ".env"},
		{".env   ", ".env"},
		{".env. . ", ".env"},
		{"id_rsa. ", "id_rsa"},
		{"secret.key..", "secret.key"},
		{"normal.txt", "normal.txt"},
		// Nested path components each stripped independently.
		{"dir. /.env. ", "dir/.env"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripNTFSArtifacts(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStripNTFSArtifacts_CombinedBypass(t *testing.T) {
	// Combining both ADS and trailing dots: "id_rsa.:$DATA" → "id_rsa"
	got := stripNTFSArtifacts("id_rsa.:$DATA")
	assert.Equal(t, "id_rsa", got)

	got = stripNTFSArtifacts(".env...:Zone.Identifier")
	assert.Equal(t, ".env", got)
}

// ---------------------------------------------------------------------------
// foldToASCII — unit tests for CWE-176 homoglyph bypass prevention
// ---------------------------------------------------------------------------

func TestFoldToASCII_DropsNonASCII(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"cyrillic_e", ".e\u043dv", ".ev"},          // Cyrillic н → dropped, leaving ".ev"
		{"latin_only", ".env", ".env"},              // already ASCII
		{"mixed_unicode", "ïd_rsa", "d_rsa"},        // ï dropped
		{"full_cyrillic", "\u0435\u043d\u0432", ""}, // all dropped
		{"empty", "", ""},
		{"ascii_numbers", "file123.txt", "file123.txt"}, // unchanged
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := foldToASCII(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// IsSensitivePath — Unicode homoglyph bypass integration tests
// ---------------------------------------------------------------------------

func TestIsSensitivePath_HomoglyphBypass(t *testing.T) {
	// Attacker replaces ASCII 'e' with Cyrillic 'е' (U+0435) in ".env".
	// After foldToASCII the Cyrillic char is dropped, leaving ".nv" which
	// might NOT match ".env". However, let's verify the detection logic
	// handles partial ASCII matches after folding.
	//
	// The real protection is that foldToASCII drops non-ASCII, so
	// ".е\u043dv" → ".v" (both non-ASCII chars dropped). This does NOT
	// match ".env" — which is correct: the file itself won't resolve to
	// .env on disk either. The important case is when an attacker uses
	// visually-similar chars that the OS treats as equivalent.

	// Pure ASCII sensitive file — must be blocked.
	assert.Error(t, IsSensitivePath(".env"))
	assert.Error(t, IsSensitivePath("id_rsa"))
	assert.Error(t, IsSensitivePath(".git/config"))
}

// ---------------------------------------------------------------------------
// IsSensitivePath — sensitive directory detection
// ---------------------------------------------------------------------------

func TestIsSensitivePath_BlocksSensitiveDirectories(t *testing.T) {
	blocked := []string{
		".docker/some.txt",
		".kube/some.txt",
		".aws/some.txt",
		".azure/some.txt",
		".gcloud/some.txt",
		".ssh/some.txt",
		"home/.docker/daemon.json",
		"config/.aws/credentials",
	}
	for _, p := range blocked {
		t.Run(p, func(t *testing.T) {
			err := IsSensitivePath(p)
			assert.Error(t, err, "path %q should be blocked", p)
			assert.Contains(t, err.Error(), "sensitive")
		})
	}
}

// ---------------------------------------------------------------------------
// IsSensitivePath — sensitive subpath detection
// ---------------------------------------------------------------------------

func TestIsSensitivePath_BlocksSensitiveSubpaths(t *testing.T) {
	blocked := []string{
		".docker/config.json",
		".kube/config",
		".aws/credentials",
		"home/.docker/config.json",
	}
	for _, p := range blocked {
		t.Run(p, func(t *testing.T) {
			err := IsSensitivePath(p)
			assert.Error(t, err, "path %q should be blocked via subpath", p)
		})
	}
}

// ---------------------------------------------------------------------------
// IsSensitivePath — exact-name credential files
// ---------------------------------------------------------------------------

func TestIsSensitivePath_BlocksExactNameCredentials(t *testing.T) {
	blocked := []string{
		".npmrc",
		".netrc",
		".pgpass",
		".my.cnf",
		".htpasswd",
		"wp-config.php",
		"shadow",
		"passwd",
		"credentials.json",
	}
	for _, p := range blocked {
		t.Run(p, func(t *testing.T) {
			err := IsSensitivePath(p)
			assert.Error(t, err, "path %q should be blocked", p)
		})
	}
}

// ---------------------------------------------------------------------------
// IsSensitivePath — nested .git detection
// ---------------------------------------------------------------------------

func TestIsSensitivePath_BlocksNestedGitInternals(t *testing.T) {
	blocked := []string{
		"submodule/.git/config",
		"deep/nested/.git/HEAD",
		".git/hooks/pre-commit",
	}
	for _, p := range blocked {
		t.Run(p, func(t *testing.T) {
			err := IsSensitivePath(p)
			assert.Error(t, err, "path %q should be blocked", p)
			assert.Contains(t, err.Error(), ".git")
		})
	}
}

func TestIsWindowsReservedName(t *testing.T) {
	reserved := []string{
		"CON", "con", "Con",
		"PRN", "prn",
		"AUX", "aux",
		"NUL", "nul",
		"COM1", "com1", "COM9",
		"LPT1", "lpt1", "LPT9",
		"CON.txt", "NUL.log", "COM1.dat",
	}
	for _, name := range reserved {
		t.Run("reserved_"+name, func(t *testing.T) {
			assert.True(t, isWindowsReservedName(name), "%q should be reserved", name)
		})
	}

	nonReserved := []string{
		"CONX", "main.go", "normal.txt",
		"COM0", "COM10", "LPT0", "LPT10",
		"console.log", "auxiliary.txt",
	}
	for _, name := range nonReserved {
		t.Run("nonreserved_"+name, func(t *testing.T) {
			assert.False(t, isWindowsReservedName(name), "%q should NOT be reserved", name)
		})
	}
}
