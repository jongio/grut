package update

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test certificate chain generation
// ---------------------------------------------------------------------------

// testCertChain holds a generated test CA hierarchy with a signing leaf.
type testCertChain struct {
	rootPool         *x509.CertPool
	intermediatePool *x509.CertPool
	leafCertPEM      []byte
	leafKey          *ecdsa.PrivateKey
}

// generateTestCertChain creates a root CA -> intermediate CA -> leaf cert
// chain suitable for testing cosign verification. The leaf cert includes
// the expected OIDC issuer extension and a URI SAN matching our repo.
func generateTestCertChain(t *testing.T) testCertChain {
	t.Helper()

	// Root CA (EC P-384, self-signed).
	rootKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("generating root key: %v", err)
	}
	rootTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"Test Sigstore Root"}},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	rootCertDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("creating root cert: %v", err)
	}
	rootCert, err := x509.ParseCertificate(rootCertDER)
	if err != nil {
		t.Fatalf("parsing root cert: %v", err)
	}

	// Intermediate CA (EC P-384, signed by root).
	intermediateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("generating intermediate key: %v", err)
	}
	intermediateTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{Organization: []string{"Test Fulcio Intermediate"}},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	intermediateDER, err := x509.CreateCertificate(rand.Reader, intermediateTemplate, rootCert, &intermediateKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("creating intermediate cert: %v", err)
	}
	intermediateCert, err := x509.ParseCertificate(intermediateDER)
	if err != nil {
		t.Fatalf("parsing intermediate cert: %v", err)
	}

	// Leaf signing cert (EC P-256, signed by intermediate).
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating leaf key: %v", err)
	}

	// Encode the OIDC issuer as ASN.1 UTF8String.
	issuerDER, err := asn1.Marshal(expectedOIDCIssuer)
	if err != nil {
		t.Fatalf("encoding OIDC issuer: %v", err)
	}

	sanURI, err := url.Parse("https://github.com/jongio/grut/.github/workflows/release.yml@refs/tags/v1.0.0")
	if err != nil {
		t.Fatalf("parsing SAN URI: %v", err)
	}

	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{Organization: []string{"Test Signer"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(10 * time.Minute),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		URIs:         []*url.URL{sanURI},
		ExtraExtensions: []pkix.Extension{
			{
				Id:    asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 1},
				Value: issuerDER,
			},
		},
	}
	leafCertDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, intermediateCert, &leafKey.PublicKey, intermediateKey)
	if err != nil {
		t.Fatalf("creating leaf cert: %v", err)
	}

	leafCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafCertDER})

	rootPool := x509.NewCertPool()
	rootPool.AddCert(rootCert)

	intermediatePool := x509.NewCertPool()
	intermediatePool.AddCert(intermediateCert)

	return testCertChain{
		rootPool:         rootPool,
		intermediatePool: intermediatePool,
		leafCertPEM:      leafCertPEM,
		leafKey:          leafKey,
	}
}

// signData creates a cosign-compatible base64-encoded ECDSA signature.
func signData(t *testing.T, key *ecdsa.PrivateKey, data []byte) []byte {
	t.Helper()
	hash := sha256.Sum256(data)
	sig, err := ecdsa.SignASN1(rand.Reader, key, hash[:])
	if err != nil {
		t.Fatalf("signing data: %v", err)
	}
	return []byte(base64.StdEncoding.EncodeToString(sig))
}

// withTestCertChain temporarily replaces the package-level Fulcio cert
// pools with test certs and restores them on test cleanup.
func withTestCertChain(t *testing.T, chain testCertChain) {
	t.Helper()
	origRoots, origInter := fulcioRoots, fulcioIntermediates
	fulcioRoots, fulcioIntermediates = chain.rootPool, chain.intermediatePool
	t.Cleanup(func() {
		fulcioRoots, fulcioIntermediates = origRoots, origInter
	})
}

// ---------------------------------------------------------------------------
// verifyCosignChecksums — orchestration tests
// ---------------------------------------------------------------------------

func TestVerifyCosignChecksums_Success(t *testing.T) {
	checksumData := []byte("abc123  grut_1.0.0_linux_amd64.tar.gz\n")
	sigContent := []byte("dGVzdC1zaWduYXR1cmU=")
	certContent := []byte("-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, ".sig"):
			_, _ = w.Write(sigContent)
		case strings.HasSuffix(r.URL.Path, ".pem"):
			_, _ = w.Write(certContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	origBase := downloadBaseURL
	downloadBaseURL = srv.URL
	defer func() { downloadBaseURL = origBase }()

	origVerify := cosignVerifyFunc
	cosignVerifyFunc = func(data, sig, cert []byte) error {
		if string(data) != string(checksumData) {
			t.Errorf("verify received wrong data: %q", data)
		}
		if string(sig) != string(sigContent) {
			t.Errorf("verify received wrong sig: %q", sig)
		}
		if string(cert) != string(certContent) {
			t.Errorf("verify received wrong cert: %q", cert)
		}
		return nil
	}
	defer func() { cosignVerifyFunc = origVerify }()

	err := verifyCosignChecksums(context.Background(), checksumData, "1.0.0")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestVerifyCosignChecksums_VerificationFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("artifact-data"))
	}))
	defer srv.Close()

	origBase := downloadBaseURL
	downloadBaseURL = srv.URL
	defer func() { downloadBaseURL = origBase }()

	origVerify := cosignVerifyFunc
	cosignVerifyFunc = func(_, _, _ []byte) error {
		return fmt.Errorf("%w: test failure", ErrCosignVerification)
	}
	defer func() { cosignVerifyFunc = origVerify }()

	err := verifyCosignChecksums(context.Background(), []byte("data"), "1.0.0")
	if err == nil {
		t.Fatal("expected error when verification fails")
	}
	if !errors.Is(err, ErrCosignVerification) {
		t.Errorf("error should wrap ErrCosignVerification, got: %v", err)
	}
}

func TestVerifyCosignChecksums_SigNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sig") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("cert-data"))
	}))
	defer srv.Close()

	origBase := downloadBaseURL
	downloadBaseURL = srv.URL
	defer func() { downloadBaseURL = origBase }()

	origVerify := cosignVerifyFunc
	cosignVerifyFunc = func(_, _, _ []byte) error {
		t.Fatal("verify should not be called when sig is missing")
		return nil
	}
	defer func() { cosignVerifyFunc = origVerify }()

	err := verifyCosignChecksums(context.Background(), []byte("data"), "1.0.0")
	if err != nil {
		t.Fatalf("expected nil (graceful degradation), got: %v", err)
	}
}

func TestVerifyCosignChecksums_CertNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".pem") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("sig-data"))
	}))
	defer srv.Close()

	origBase := downloadBaseURL
	downloadBaseURL = srv.URL
	defer func() { downloadBaseURL = origBase }()

	origVerify := cosignVerifyFunc
	cosignVerifyFunc = func(_, _, _ []byte) error {
		t.Fatal("verify should not be called when cert is missing")
		return nil
	}
	defer func() { cosignVerifyFunc = origVerify }()

	err := verifyCosignChecksums(context.Background(), []byte("data"), "1.0.0")
	if err != nil {
		t.Fatalf("expected nil (graceful degradation), got: %v", err)
	}
}

func TestVerifyCosignChecksums_SigDownloadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sig") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("cert-data"))
	}))
	defer srv.Close()

	origBase := downloadBaseURL
	downloadBaseURL = srv.URL
	defer func() { downloadBaseURL = origBase }()

	err := verifyCosignChecksums(context.Background(), []byte("data"), "1.0.0")
	if err == nil {
		t.Fatal("expected error for sig download failure")
	}
	if !strings.Contains(err.Error(), "downloading cosign signature") {
		t.Errorf("error should mention signature download, got: %v", err)
	}
}

func TestVerifyCosignChecksums_CertDownloadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".pem") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("sig-data"))
	}))
	defer srv.Close()

	origBase := downloadBaseURL
	downloadBaseURL = srv.URL
	defer func() { downloadBaseURL = origBase }()

	err := verifyCosignChecksums(context.Background(), []byte("data"), "1.0.0")
	if err == nil {
		t.Fatal("expected error for cert download failure")
	}
	if !strings.Contains(err.Error(), "downloading cosign certificate") {
		t.Errorf("error should mention certificate download, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// downloadCosignArtifact
// ---------------------------------------------------------------------------

func TestDownloadCosignArtifact_Success(t *testing.T) {
	body := "test-artifact-content"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	data, err := downloadCosignArtifact(context.Background(), srv.URL+"/artifact.sig")
	if err != nil {
		t.Fatalf("downloadCosignArtifact: %v", err)
	}
	if string(data) != body {
		t.Errorf("got %q, want %q", data, body)
	}
}

func TestDownloadCosignArtifact_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := downloadCosignArtifact(context.Background(), srv.URL+"/missing.sig")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !errors.Is(err, ErrCosignArtifactNotFound) {
		t.Errorf("error should wrap ErrCosignArtifactNotFound, got: %v", err)
	}
}

func TestDownloadCosignArtifact_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := downloadCosignArtifact(context.Background(), srv.URL+"/artifact.sig")
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if errors.Is(err, ErrCosignArtifactNotFound) {
		t.Error("500 should not be treated as not-found")
	}
}

func TestDownloadCosignArtifact_OversizedResponse(t *testing.T) {
	bigBody := strings.Repeat("x", int(maxCosignArtifactSize)+100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(bigBody))
	}))
	defer srv.Close()

	_, err := downloadCosignArtifact(context.Background(), srv.URL+"/huge.sig")
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Errorf("error should wrap ErrPayloadTooLarge, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// defaultCosignVerify — full crypto path
// ---------------------------------------------------------------------------

func TestDefaultCosignVerify_ValidSignature(t *testing.T) {
	chain := generateTestCertChain(t)
	withTestCertChain(t, chain)

	data := []byte("test checksum data for cosign verification")
	sig := signData(t, chain.leafKey, data)

	err := defaultCosignVerify(data, sig, chain.leafCertPEM)
	if err != nil {
		t.Fatalf("expected valid verification, got: %v", err)
	}
}

func TestDefaultCosignVerify_InvalidSignature(t *testing.T) {
	chain := generateTestCertChain(t)
	withTestCertChain(t, chain)

	data := []byte("original data")
	sig := signData(t, chain.leafKey, []byte("different data"))

	err := defaultCosignVerify(data, sig, chain.leafCertPEM)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
	if !errors.Is(err, ErrCosignVerification) {
		t.Errorf("error should wrap ErrCosignVerification, got: %v", err)
	}
}

func TestDefaultCosignVerify_InvalidPEM(t *testing.T) {
	err := defaultCosignVerify([]byte("data"), []byte("c2ln"), []byte("not-pem"))
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
	if !errors.Is(err, ErrCosignCertInvalid) {
		t.Errorf("error should wrap ErrCosignCertInvalid, got: %v", err)
	}
}

func TestDefaultCosignVerify_InvalidCertDER(t *testing.T) {
	invalidPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("not-der")})

	err := defaultCosignVerify([]byte("data"), []byte("c2ln"), invalidPEM)
	if err == nil {
		t.Fatal("expected error for invalid certificate DER")
	}
	if !errors.Is(err, ErrCosignCertInvalid) {
		t.Errorf("error should wrap ErrCosignCertInvalid, got: %v", err)
	}
}

func TestDefaultCosignVerify_UntrustedChain(t *testing.T) {
	// Generate a chain but do NOT install it as the trusted roots.
	chain := generateTestCertChain(t)

	data := []byte("test data")
	sig := signData(t, chain.leafKey, data)

	err := defaultCosignVerify(data, sig, chain.leafCertPEM)
	if err == nil {
		t.Fatal("expected error for untrusted certificate chain")
	}
	if !errors.Is(err, ErrCosignCertInvalid) {
		t.Errorf("error should wrap ErrCosignCertInvalid, got: %v", err)
	}
}

func TestDefaultCosignVerify_BadBase64Signature(t *testing.T) {
	chain := generateTestCertChain(t)
	withTestCertChain(t, chain)

	err := defaultCosignVerify([]byte("data"), []byte("!!!not-base64!!!"), chain.leafCertPEM)
	if err == nil {
		t.Fatal("expected error for invalid base64 signature")
	}
	if !errors.Is(err, ErrCosignVerification) {
		t.Errorf("error should wrap ErrCosignVerification, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// verifyOIDCIssuer
// ---------------------------------------------------------------------------

func TestVerifyOIDCIssuer_ValidV1(t *testing.T) {
	issuerDER, err := asn1.Marshal(expectedOIDCIssuer)
	if err != nil {
		t.Fatal(err)
	}

	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidcIssuerV1OID, Value: issuerDER},
		},
	}

	if err := verifyOIDCIssuer(cert); err != nil {
		t.Fatalf("expected valid OIDC issuer, got: %v", err)
	}
}

func TestVerifyOIDCIssuer_ValidV2(t *testing.T) {
	issuerDER, err := asn1.Marshal(expectedOIDCIssuer)
	if err != nil {
		t.Fatal(err)
	}

	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidcIssuerV2OID, Value: issuerDER},
		},
	}

	if err := verifyOIDCIssuer(cert); err != nil {
		t.Fatalf("expected valid v2 OIDC issuer, got: %v", err)
	}
}

func TestVerifyOIDCIssuer_RawStringEncoding(t *testing.T) {
	// Older Fulcio v1 certs stored OIDC issuer as raw UTF-8 bytes.
	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidcIssuerV1OID, Value: []byte(expectedOIDCIssuer)},
		},
	}

	if err := verifyOIDCIssuer(cert); err != nil {
		t.Fatalf("expected valid raw OIDC issuer, got: %v", err)
	}
}

func TestVerifyOIDCIssuer_WrongIssuer(t *testing.T) {
	issuerDER, err := asn1.Marshal("https://evil.example.com")
	if err != nil {
		t.Fatal(err)
	}

	cert := &x509.Certificate{
		Extensions: []pkix.Extension{
			{Id: oidcIssuerV1OID, Value: issuerDER},
		},
	}

	err = verifyOIDCIssuer(cert)
	if err == nil {
		t.Fatal("expected error for wrong OIDC issuer")
	}
	if !errors.Is(err, ErrCosignCertInvalid) {
		t.Errorf("error should wrap ErrCosignCertInvalid, got: %v", err)
	}
}

func TestVerifyOIDCIssuer_MissingExtension(t *testing.T) {
	cert := &x509.Certificate{
		Extensions: []pkix.Extension{},
	}

	err := verifyOIDCIssuer(cert)
	if err == nil {
		t.Fatal("expected error for missing OIDC issuer extension")
	}
	if !errors.Is(err, ErrCosignCertInvalid) {
		t.Errorf("error should wrap ErrCosignCertInvalid, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// verifyCertIdentity
// ---------------------------------------------------------------------------

func TestVerifyCertIdentity_ValidURISAN(t *testing.T) {
	u, _ := url.Parse("https://github.com/jongio/grut/.github/workflows/release.yml@refs/tags/v1.0.0")
	cert := &x509.Certificate{
		URIs: []*url.URL{u},
	}

	if err := verifyCertIdentity(cert); err != nil {
		t.Fatalf("expected valid identity, got: %v", err)
	}
}

func TestVerifyCertIdentity_WrongURISAN(t *testing.T) {
	u, _ := url.Parse("https://github.com/evil/repo/.github/workflows/release.yml@refs/tags/v1.0.0")
	cert := &x509.Certificate{
		URIs: []*url.URL{u},
	}

	err := verifyCertIdentity(cert)
	if err == nil {
		t.Fatal("expected error for wrong URI SAN")
	}
	if !errors.Is(err, ErrCosignCertInvalid) {
		t.Errorf("error should wrap ErrCosignCertInvalid, got: %v", err)
	}
}

func TestVerifyCertIdentity_SourceRepoV2Extension(t *testing.T) {
	repoDER, err := asn1.Marshal(expectedRepoURI)
	if err != nil {
		t.Fatal(err)
	}

	cert := &x509.Certificate{
		URIs: []*url.URL{},
		Extensions: []pkix.Extension{
			{Id: sourceRepoV2OID, Value: repoDER},
		},
	}

	if err := verifyCertIdentity(cert); err != nil {
		t.Fatalf("expected valid identity via v2 extension, got: %v", err)
	}
}

func TestVerifyCertIdentity_NoMatchingIdentity(t *testing.T) {
	cert := &x509.Certificate{
		URIs:       []*url.URL{},
		Extensions: []pkix.Extension{},
	}

	err := verifyCertIdentity(cert)
	if err == nil {
		t.Fatal("expected error for missing identity")
	}
	if !errors.Is(err, ErrCosignCertInvalid) {
		t.Errorf("error should wrap ErrCosignCertInvalid, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// extractExtensionString
// ---------------------------------------------------------------------------

func TestExtractExtensionString_ASN1Encoded(t *testing.T) {
	der, err := asn1.Marshal("hello world")
	if err != nil {
		t.Fatal(err)
	}

	got := extractExtensionString(der)
	if got != "hello world" {
		t.Errorf("extractExtensionString(ASN1) = %q, want %q", got, "hello world")
	}
}

func TestExtractExtensionString_RawBytes(t *testing.T) {
	raw := []byte("raw string value")

	got := extractExtensionString(raw)
	if got != "raw string value" {
		t.Errorf("extractExtensionString(raw) = %q, want %q", got, "raw string value")
	}
}

// ---------------------------------------------------------------------------
// downloadChecksumData
// ---------------------------------------------------------------------------

func TestDownloadChecksumData_Success(t *testing.T) {
	body := "abc123  archive.tar.gz\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	data, err := downloadChecksumData(context.Background(), srv.URL+"/checksums.txt")
	if err != nil {
		t.Fatalf("downloadChecksumData: %v", err)
	}
	if string(data) != body {
		t.Errorf("got %q, want %q", data, body)
	}
}

func TestDownloadChecksumData_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := downloadChecksumData(context.Background(), srv.URL+"/checksums.txt")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

// ---------------------------------------------------------------------------
// verifyArchiveChecksum
// ---------------------------------------------------------------------------

func TestVerifyArchiveChecksum_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	content := []byte("archive content for checksum test")
	archivePath := filepath.Join(tmpDir, "archive.tar.gz")
	if err := os.WriteFile(archivePath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(content)
	hash := hex.EncodeToString(h[:])
	checksumData := []byte(hash + "  archive.tar.gz\n")

	err := verifyArchiveChecksum(archivePath, checksumData, "archive.tar.gz")
	if err != nil {
		t.Fatalf("verifyArchiveChecksum: %v", err)
	}
}

func TestVerifyArchiveChecksum_Mismatch(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "archive.tar.gz")
	if err := os.WriteFile(archivePath, []byte("actual content"), 0o644); err != nil {
		t.Fatal(err)
	}

	checksumData := []byte("0000000000000000000000000000000000000000000000000000000000000000  archive.tar.gz\n")

	err := verifyArchiveChecksum(archivePath, checksumData, "archive.tar.gz")
	if err == nil {
		t.Fatal("expected error for checksum mismatch")
	}
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("error should wrap ErrChecksumMismatch, got: %v", err)
	}
}

func TestVerifyArchiveChecksum_MissingEntry(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "archive.tar.gz")
	if err := os.WriteFile(archivePath, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	checksumData := []byte("deadbeef  different_file.tar.gz\n")

	err := verifyArchiveChecksum(archivePath, checksumData, "archive.tar.gz")
	if err == nil {
		t.Fatal("expected error for missing checksum entry")
	}
	if !errors.Is(err, ErrChecksumNotFound) {
		t.Errorf("error should wrap ErrChecksumNotFound, got: %v", err)
	}
}
