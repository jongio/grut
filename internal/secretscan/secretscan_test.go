package secretscan

import (
	"fmt"
	"testing"
)

// Test fixtures are assembled from split literals so that no contiguous
// secret-like token appears in the source. This keeps the fixtures clear of
// GitHub push protection while still exercising the detection patterns at
// runtime. None of these are real credentials.
var (
	fxGitHub    = "ghp_" + "0123456789abcdefghijklmnopqrstuvwxyz"
	fxGitHubPAT = "github_pat_" + "11ABCDEFG0abcdefghij_KLMNOPqrstuvwxyz012345"
	fxSlack     = "xox" + "b-1234567890-abcdefghijklmno"
	fxGoogle    = "AIza" + "SyA1234567890abcdefghijklmnopqrstuv"
	fxStripe    = "sk_" + "live_0123456789abcdef0123"
)

func hasRule(findings []Finding, rule string) bool {
	for _, f := range findings {
		if f.Rule == rule {
			return true
		}
	}
	return false
}

func TestScan_ContentTruePositives(t *testing.T) {
	cases := []struct {
		name    string
		content string
		rule    string
	}{
		{"github classic", fmt.Sprintf("token = %q", fxGitHub), RuleGitHubToken},
		{"github fine-grained", "GH=" + fxGitHubPAT, RuleGitHubToken},
		{"aws access key", "AWS_ACCESS_KEY_ID=AKIA" + "IOSFODNN7EXAMPLE", RuleAWSAccessKey},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIE...\n", RulePrivateKey},
		{"openssh key", "-----BEGIN OPENSSH PRIVATE KEY-----\nb3Blbn...", RulePrivateKey},
		{"slack token", "slack=" + fxSlack, RuleSlackToken},
		{"google api key", "key=" + fxGoogle, RuleGoogleAPIKey},
		{"stripe key", "STRIPE=" + fxStripe, RuleStripeKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := Scan([]byte(tc.content), "config.txt")
			if !hasRule(findings, tc.rule) {
				t.Fatalf("expected rule %q, got %+v", tc.rule, findings)
			}
		})
	}
}

func TestScan_GenericHighEntropy(t *testing.T) {
	content := []byte(`api_key = "Zx83kLm92QpVn5RtWy71Bc48Df06Hg23"`)
	findings := Scan(content, "settings.conf")
	if !hasRule(findings, RuleHighEntropyValue) {
		t.Fatalf("expected high-entropy-value finding, got %+v", findings)
	}
}

func TestScan_FalsePositives(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"plain code", "func Add(a, b int) int { return a + b }\n"},
		{"placeholder env", `API_KEY=your-api-key-here`},
		{"example value", `token = "example-token-value-000"`},
		{"interpolation", "secret = ${MY_SECRET}"},
		{"short value", `password = "short"`},
		{"low entropy repeated", `api_key = "aaaaaaaaaaaaaaaaaaaa"`},
		{"template mustache", `token = {{ vault_token }}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if findings := Scan([]byte(tc.content), "notes.txt"); len(findings) != 0 {
				t.Fatalf("expected no findings, got %+v", findings)
			}
		})
	}
}

func TestScan_SensitiveFilenames(t *testing.T) {
	names := []string{
		".env", ".env.local", ".env.production",
		"server.pem", "tls.key", "id_rsa", "id_dsa",
		"credentials.json", "keystore.p12", ".netrc",
	}
	for _, n := range names {
		t.Run(n, func(t *testing.T) {
			findings := Scan([]byte("nothing secret here\n"), n)
			if !hasRule(findings, RuleSensitiveFilename) {
				t.Fatalf("expected sensitive-filename for %q, got %+v", n, findings)
			}
		})
	}
}

func TestScan_SafeEnvTemplatesNotFlagged(t *testing.T) {
	for _, n := range []string{".env.example", ".env.sample", ".env.template", ".env.dist"} {
		t.Run(n, func(t *testing.T) {
			if findings := Scan([]byte("API_KEY=your-key\n"), n); len(findings) != 0 {
				t.Fatalf("expected no findings for %q, got %+v", n, findings)
			}
		})
	}
}

func TestScan_BinarySkipsContentButKeepsFilename(t *testing.T) {
	// A NUL byte marks binary content; the embedded token must be ignored.
	binary := append([]byte(fxGitHub), 0x00, 0x01, 0x02)
	if findings := Scan(binary, "blob.bin"); len(findings) != 0 {
		t.Fatalf("expected binary content to be skipped, got %+v", findings)
	}
	// Filename rules still apply to binary files.
	if findings := Scan(binary, "server.pem"); !hasRule(findings, RuleSensitiveFilename) {
		t.Fatalf("expected sensitive-filename for binary .pem, got %+v", findings)
	}
}

func TestScan_LineNumbersReported(t *testing.T) {
	content := []byte("line one\nline two\ntoken = " + fxGitHub + "\n")
	findings := Scan(content, "x.txt")
	if len(findings) == 0 || findings[0].Line != 3 {
		t.Fatalf("expected finding on line 3, got %+v", findings)
	}
}

func TestHasFindings(t *testing.T) {
	if !HasFindings([]byte("k=AKIA"+"IOSFODNN7EXAMPLE"), "a.txt") {
		t.Fatal("expected HasFindings true for AWS key")
	}
	if HasFindings([]byte("just some text"), "a.txt") {
		t.Fatal("expected HasFindings false for plain text")
	}
}

func TestShannonEntropy(t *testing.T) {
	if got := shannonEntropy(""); got != 0 {
		t.Fatalf("empty entropy = %v, want 0", got)
	}
	if got := shannonEntropy("aaaa"); got != 0 {
		t.Fatalf("uniform entropy = %v, want 0", got)
	}
	if got := shannonEntropy("abcd"); got <= 0 {
		t.Fatalf("varied entropy = %v, want > 0", got)
	}
}
