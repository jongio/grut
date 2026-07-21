package update

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCosignBundleSuffix_MatchesGoReleaser verifies that the bundle file
// extension expected by verifyCosignChecksums matches the signature filename
// template configured in .goreleaser.yml. This contract test prevents silent
// drift between the release pipeline and the verification code.
func TestCosignBundleSuffix_MatchesGoReleaser(t *testing.T) {
	// Locate repo root relative to this test file.
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	data, err := os.ReadFile(filepath.Join(repoRoot, ".goreleaser.yml"))
	if err != nil {
		t.Fatalf("reading .goreleaser.yml: %v", err)
	}

	content := string(data)

	// GoReleaser signs config uses signature: "${artifact}<suffix>".
	// Extract the suffix pattern from the YAML.
	// Expected line: `    signature: "${artifact}.sigstore.json"`
	var signatureLine string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "signature:") {
			signatureLine = trimmed
			break
		}
	}

	if signatureLine == "" {
		t.Fatal(".goreleaser.yml has no 'signature:' field in signs config")
	}

	// The signature template should end with the same suffix our code expects.
	// Template format: "${artifact}.sigstore.json" (with or without quotes).
	// We check that the template, after removing ${artifact}, yields our suffix.
	cleaned := strings.TrimPrefix(signatureLine, "signature:")
	cleaned = strings.TrimSpace(cleaned)
	cleaned = strings.Trim(cleaned, `"'`)

	// Replace the GoReleaser variable to get the suffix.
	suffix := strings.Replace(cleaned, "${artifact}", "", 1)

	if suffix != cosignBundleSuffix {
		t.Errorf("GoReleaser signature suffix %q does not match code constant cosignBundleSuffix %q\n"+
			"The release pipeline will produce files the updater cannot find.\n"+
			"Update cosignBundleSuffix in cosign.go or the signature template in .goreleaser.yml.",
			suffix, cosignBundleSuffix)
	}
}

// TestChecksumFileName_MatchesGoReleaser verifies the checksum filename
// constant matches .goreleaser.yml's checksum.name_template.
func TestChecksumFileName_MatchesGoReleaser(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	data, err := os.ReadFile(filepath.Join(repoRoot, ".goreleaser.yml"))
	if err != nil {
		t.Fatalf("reading .goreleaser.yml: %v", err)
	}

	content := string(data)

	// Find checksum name_template line.
	var nameTemplate string
	inChecksum := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "checksum:" {
			inChecksum = true
			continue
		}
		if inChecksum && strings.HasPrefix(trimmed, "name_template:") {
			nameTemplate = trimmed
			break
		}
		// Exit checksum block if we hit another top-level key.
		if inChecksum && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
			break
		}
	}

	if nameTemplate == "" {
		t.Fatal(".goreleaser.yml has no checksum name_template")
	}

	// Extract the value.
	value := strings.TrimPrefix(nameTemplate, "name_template:")
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)

	if value != checksumFileName {
		t.Errorf("GoReleaser checksum name_template %q does not match code constant checksumFileName %q\n"+
			"The release pipeline will produce a checksums file the updater cannot find.\n"+
			"Update checksumFileName in update.go or name_template in .goreleaser.yml.",
			value, checksumFileName)
	}
}
