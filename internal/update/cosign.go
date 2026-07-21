package update

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// cosignBundleSuffix is the file extension for Sigstore bundle files
	// produced by cosign sign-blob --bundle.
	cosignBundleSuffix = ".sigstore.json"

	// expectedOIDCIssuer is the OIDC issuer for GitHub Actions keyless signing.
	expectedOIDCIssuer = "https://token.actions.githubusercontent.com"

	// expectedRepoURI is the expected GitHub repository URI for certificate
	// identity verification.
	expectedRepoURI = "https://github.com/jongio/grut"

	// maxCosignArtifactSize limits cosign artifact downloads to 256 KiB.
	maxCosignArtifactSize int64 = 256 << 10
)

// Fulcio OIDC extension OIDs per the Sigstore Fulcio certificate specification.
var (
	// oidcIssuerV1OID is the Fulcio v1 OID for the OIDC issuer extension.
	// Ref: https://github.com/sigstore/fulcio/blob/main/docs/oid-info.md
	oidcIssuerV1OID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 1}

	// oidcIssuerV2OID is the Fulcio v2 OID for the OIDC issuer extension.
	oidcIssuerV2OID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 8}

	// sourceRepoV2OID is the Fulcio v2 OID for the source repository URI.
	sourceRepoV2OID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 57264, 1, 12}
)

var (
	// ErrCosignVerification indicates the cosign signature did not verify.
	ErrCosignVerification = errors.New("cosign signature verification failed")

	// ErrCosignCertInvalid indicates the cosign certificate is invalid or
	// does not match expected identity constraints.
	ErrCosignCertInvalid = errors.New("cosign certificate validation failed")

	// ErrCosignArtifactNotFound indicates cosign signature artifacts were
	// not present in the release (HTTP 404).
	ErrCosignArtifactNotFound = errors.New("cosign artifact not found")

	// cosignVerifyFunc is the active cosign verification function.
	// Override in tests to mock cryptographic verification.
	cosignVerifyFunc = defaultCosignVerify

	// fulcioRoots contains the Sigstore Fulcio root CA certificates used
	// for certificate chain verification.
	fulcioRoots *x509.CertPool

	// fulcioIntermediates contains the Sigstore Fulcio intermediate CA
	// certificates used for certificate chain verification.
	fulcioIntermediates *x509.CertPool
)

func init() {
	fulcioRoots = x509.NewCertPool()
	if !fulcioRoots.AppendCertsFromPEM([]byte(fulcioRootPEM)) {
		panic("update: failed to parse embedded Fulcio root certificate")
	}

	fulcioIntermediates = x509.NewCertPool()
	if !fulcioIntermediates.AppendCertsFromPEM([]byte(fulcioIntermediatePEM)) {
		panic("update: failed to parse embedded Fulcio intermediate certificate")
	}
}

// cosignRequiredSince is the minimum version from which cosign signature
// artifacts are mandatory. Releases before this version may lack signatures
// and are allowed to proceed without verification. Releases at or after this
// version MUST have valid cosign artifacts — a missing signature is treated
// as a verification failure to prevent downgrade attacks.
const cosignRequiredSince = "1.0.0"

// verifyCosignChecksums downloads the Sigstore bundle for the given release
// version and verifies the cryptographic signature over checksumData.
//
// For releases >= cosignRequiredSince, missing signature artifacts are treated
// as errors (preventing downgrade attacks). For older releases, missing
// artifacts log a warning and allow the update to proceed.
func verifyCosignChecksums(ctx context.Context, checksumData []byte, version string) error {
	bundleURL := fmt.Sprintf("%s/v%s/%s%s", downloadBaseURL, version, checksumFileName, cosignBundleSuffix)

	bundleData, err := downloadCosignArtifact(ctx, bundleURL)
	if errors.Is(err, ErrCosignArtifactNotFound) {
		if !versionPredatesCosign(version) {
			return fmt.Errorf("cosign signature required for v%s but not found (possible downgrade attack)", version)
		}
		fmt.Fprintf(os.Stderr, "Warning: cosign signature not found for v%s, skipping signature verification\n", version)
		return nil
	}
	if err != nil {
		return fmt.Errorf("downloading cosign bundle: %w", err)
	}

	sig, cert, err := parseSigstoreBundle(bundleData)
	if err != nil {
		return fmt.Errorf("parsing cosign bundle: %w", err)
	}

	return cosignVerifyFunc(checksumData, sig, cert)
}

// sigstoreBundle represents the relevant fields of a Sigstore bundle JSON
// file produced by cosign sign-blob --bundle.
type sigstoreBundle struct {
	VerificationMaterial struct {
		Certificate struct {
			RawBytes string `json:"rawBytes"`
		} `json:"certificate"`
	} `json:"verificationMaterial"`
	MessageSignature struct {
		Signature string `json:"signature"`
	} `json:"messageSignature"`
}

// parseSigstoreBundle extracts the base64-encoded signature and PEM certificate
// from a Sigstore bundle JSON file. Returns (sigBase64, certPEM, error).
func parseSigstoreBundle(data []byte) ([]byte, []byte, error) {
	var bundle sigstoreBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, nil, fmt.Errorf("invalid bundle JSON: %w", err)
	}

	if bundle.MessageSignature.Signature == "" {
		return nil, nil, fmt.Errorf("%w: bundle missing messageSignature.signature", ErrCosignVerification)
	}
	if bundle.VerificationMaterial.Certificate.RawBytes == "" {
		return nil, nil, fmt.Errorf("%w: bundle missing verificationMaterial.certificate.rawBytes", ErrCosignCertInvalid)
	}

	// The signature in the bundle is already base64-encoded, which is what
	// defaultCosignVerify expects.
	sig := []byte(bundle.MessageSignature.Signature)

	// The certificate rawBytes is base64-encoded DER. Decode to DER then
	// wrap in PEM for the existing verification function.
	derBytes, err := base64.StdEncoding.DecodeString(bundle.VerificationMaterial.Certificate.RawBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: decoding certificate from bundle: %v", ErrCosignCertInvalid, err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})

	return sig, certPEM, nil
}

// versionPredatesCosign returns true if version is strictly older than
// cosignRequiredSince, meaning it predates mandatory cosign signing.
func versionPredatesCosign(version string) bool {
	return CompareVersions(version, cosignRequiredSince) < 0
}

// downloadCosignArtifact downloads a small cosign artifact (signature or
// certificate) from the given URL. Returns ErrCosignArtifactNotFound for
// HTTP 404 responses, indicating the release predates cosign signing.
func downloadCosignArtifact(ctx context.Context, url string) ([]byte, error) {
	client := newSecureClient(apiTimeout)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // URL built from constants+version
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", url, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort cleanup

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrCosignArtifactNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCosignArtifactSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}
	if int64(len(data)) > maxCosignArtifactSize {
		return nil, fmt.Errorf("cosign artifact exceeds %d bytes: %w", maxCosignArtifactSize, ErrPayloadTooLarge)
	}

	return data, nil
}

// defaultCosignVerify performs cryptographic verification of a cosign
// keyless signature. It verifies:
//  1. The certificate chains to the embedded Fulcio root CA.
//  2. The OIDC issuer extension matches GitHub Actions.
//  3. The certificate identity (SAN) matches the expected repository.
//  4. The ECDSA signature is valid over the SHA-256 hash of data.
func defaultCosignVerify(data, sigBase64, certPEM []byte) error {
	// Parse the signing certificate from PEM.
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("%w: no PEM block found in certificate", ErrCosignCertInvalid)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCosignCertInvalid, err)
	}

	// Verify the certificate chain against Fulcio root and intermediate CAs.
	// Use the cert's NotBefore as CurrentTime because Fulcio certs are
	// short-lived (10 min) and will always be expired by the time a user
	// runs an update. The chain validation proves the cert was legitimately
	// issued by Fulcio; the OIDC issuer check proves it came from our CI.
	opts := x509.VerifyOptions{
		Roots:         fulcioRoots,
		Intermediates: fulcioIntermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		CurrentTime:   cert.NotBefore.Add(time.Second),
	}
	if _, err := cert.Verify(opts); err != nil {
		return fmt.Errorf("%w: certificate chain verification: %v", ErrCosignCertInvalid, err)
	}

	// Verify the OIDC issuer matches GitHub Actions.
	if err := verifyOIDCIssuer(cert); err != nil {
		return err
	}

	// Verify the certificate identity matches our repository.
	if err := verifyCertIdentity(cert); err != nil {
		return err
	}

	// Decode the base64-encoded signature.
	sigRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigBase64)))
	if err != nil {
		return fmt.Errorf("%w: decoding signature: %v", ErrCosignVerification, err)
	}

	// Extract the ECDSA public key from the certificate.
	pubKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("%w: certificate does not contain an ECDSA public key", ErrCosignVerification)
	}

	// Verify the ECDSA signature over the SHA-256 hash of the data.
	hash := sha256.Sum256(data)
	if !ecdsa.VerifyASN1(pubKey, hash[:], sigRaw) {
		return fmt.Errorf("%w: ECDSA signature does not match checksum data", ErrCosignVerification)
	}

	return nil
}

// verifyOIDCIssuer checks that the certificate contains a Fulcio OIDC
// issuer extension matching the expected GitHub Actions issuer.
func verifyOIDCIssuer(cert *x509.Certificate) error {
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(oidcIssuerV1OID) && !ext.Id.Equal(oidcIssuerV2OID) {
			continue
		}

		issuer := extractExtensionString(ext.Value)
		if issuer == expectedOIDCIssuer {
			return nil
		}
		return fmt.Errorf("%w: OIDC issuer %q does not match expected %q",
			ErrCosignCertInvalid, issuer, expectedOIDCIssuer)
	}

	return fmt.Errorf("%w: certificate missing OIDC issuer extension", ErrCosignCertInvalid)
}

// verifyCertIdentity checks that the certificate's identity (URI SAN or
// Fulcio v2 source repository extension) matches the expected repository.
func verifyCertIdentity(cert *x509.Certificate) error {
	// Check URI SANs (standard for GitHub Actions cosign signing).
	for _, uri := range cert.URIs {
		if strings.HasPrefix(uri.String(), expectedRepoURI+"/") {
			return nil
		}
	}

	// Check the Fulcio v2 source repository extension as fallback.
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(sourceRepoV2OID) {
			continue
		}
		repo := extractExtensionString(ext.Value)
		if repo == expectedRepoURI {
			return nil
		}
	}

	return fmt.Errorf("%w: certificate identity does not match expected repository %q",
		ErrCosignCertInvalid, expectedRepoURI)
}

// extractExtensionString decodes an X.509 extension value. It first
// attempts ASN.1 UTF8String decoding (Fulcio v2 and newer v1 certs),
// then falls back to interpreting the raw bytes as UTF-8 (older v1 certs).
func extractExtensionString(value []byte) string {
	var s string
	if rest, err := asn1.Unmarshal(value, &s); err == nil && len(rest) == 0 {
		return s
	}
	return string(value)
}

// ---------------------------------------------------------------------------
// Embedded Sigstore Fulcio CA certificates
// ---------------------------------------------------------------------------

// fulcioRootPEM is the Sigstore Fulcio root CA certificate (v1).
// Source: https://github.com/sigstore/root-signing/blob/main/targets/fulcio_v1.crt.pem
// Subject: O=sigstore.dev, CN=sigstore
// Validity: 2021-10-07 to 2031-10-05
// Key: EC P-384
const fulcioRootPEM = `-----BEGIN CERTIFICATE-----
MIIB9zCCAXygAwIBAgIUALZNAPFdxHPwjeDloDwyYChAO/4wCgYIKoZIzj0EAwMw
KjEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MREwDwYDVQQDEwhzaWdzdG9yZTAeFw0y
MTEwMDcxMzU2NTlaFw0zMTEwMDUxMzU2NThaMCoxFTATBgNVBAoTDHNpZ3N0b3Jl
LmRldjERMA8GA1UEAxMIc2lnc3RvcmUwdjAQBgcqhkjOPQIBBgUrgQQAIgNiAAT7
XeFT4rb3PQGwS4IajtLk3/OlnpgangaBclYpsYBr5i+4ynB07ceb3LP0OIOZdxex
X69c5iVuyJRQ+Hz05yi+UF3uBWAlHpiS5sh0+H2GHE7SXrk1EC5m1Tr19L9gg92j
YzBhMA4GA1UdDwEB/wQEAwIBBjAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBRY
wB5fkUWlZql6zJChkyLQKsXF+jAfBgNVHSMEGDAWgBRYwB5fkUWlZql6zJChkyLQ
KsXF+jAKBggqhkjOPQQDAwNpADBmAjEAj1nHeXZp+13NWBNa+EDsDP8G1WWg1tCM
WP/WHPqpaVo0jhsweNFZgSs0eE7wYI4qAjEA2WB9ot98sIkoF3vZYdd3/VtWB5b9
TNMea7Ix/stJ5TfcLLeABLE4BNJOsQ4vnBHJ
-----END CERTIFICATE-----`

// fulcioIntermediatePEM is the Sigstore Fulcio intermediate CA certificate (v1).
// Source: https://github.com/sigstore/root-signing/blob/main/targets/fulcio_intermediate_v1.crt.pem
// Subject: O=sigstore.dev, CN=sigstore-intermediate
// Validity: 2022-04-13 to 2031-10-05
// Key: EC P-384
const fulcioIntermediatePEM = `-----BEGIN CERTIFICATE-----
MIICGjCCAaGgAwIBAgIUALnViVfnU0brJasmRkHrn/UnfaQwCgYIKoZIzj0EAwMw
KjEVMBMGA1UEChMMc2lnc3RvcmUuZGV2MREwDwYDVQQDEwhzaWdzdG9yZTAeFw0y
MjA0MTMyMDA2MTVaFw0zMTEwMDUxMzU2NThaMDcxFTATBgNVBAoTDHNpZ3N0b3Jl
LmRldjEeMBwGA1UEAxMVc2lnc3RvcmUtaW50ZXJtZWRpYXRlMHYwEAYHKoZIzj0C
AQYFK4EEACIDYgAE8RVS/ysH+NOvuDZyPIZtilgUF9NlarYpAd9HP1vBBH1U5CV7
7LSS7s0ZiH4nE7Hv7ptS6LvvR/STk798LVgMzLlJ4HeIfF3tHSaexLcYpSASr1kS
0N/RgBJz/9jWCiXno3sweTAOBgNVHQ8BAf8EBAMCAQYwEwYDVR0lBAwwCgYIKwYB
BQUHAwMwEgYDVR0TAQH/BAgwBgEB/wIBADAdBgNVHQ4EFgQU39Ppz1YkEZb5qNjp
KFWixi4YZD8wHwYDVR0jBBgwFoAUWMAeX5FFpWapesyQoZMi0CrFxfowCgYIKoZI
zj0EAwMDZwAwZAIwPCsQK4DYiZYDPIaDi5HFKnfxXx6ASSVmERfsynYBiX2X6SJR
nZU84/9DZdnFvvxmAjBOt6QpBlc4J/0DxvkTCqpclvziL6BCCPnjdlIB3Pu3BxsP
mygUY7Ii2zbdCdliiow=
-----END CERTIFICATE-----`
