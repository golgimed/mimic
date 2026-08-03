package integraicp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
)

// CodeChallengeFromVerifier implements RFC 7636 PKCE:
// code_challenge = base64url(sha256(code_verifier)), no padding.
func CodeChallengeFromVerifier(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func VerifyPkce(verifier, storedChallenge string) bool {
	return CodeChallengeFromVerifier(verifier) == storedChallenge
}

type FakeCertificate struct {
	SerialNumber   string `json:"serialNumber"`
	IssuerName     string `json:"issuerName"`
	Validity       struct {
		NotBefore string `json:"notBefore"`
		NotAfter  string `json:"notAfter"`
	} `json:"validity"`
	SubjectName    string `json:"subjectName"`
	EncodedX509    string `json:"encodedX509"`
	Fingerprint256 string `json:"fingerprint256"`
}

func BuildFakeCertificate(subjectName string) FakeCertificate {
	now := time.Now().UTC()
	notAfter := now.AddDate(1, 0, 0)

	sum := sha256.Sum256([]byte(subjectName + uuid.NewString()))
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = strings.ToUpper(hex.EncodeToString([]byte{b}))
	}

	cert := FakeCertificate{
		SerialNumber:   strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")),
		IssuerName:     "AC IntegraICP Simulador v1",
		SubjectName:    subjectName,
		EncodedX509:    "-----BEGIN CERTIFICATE-----\nSIMULATED\n-----END CERTIFICATE-----\n",
		Fingerprint256: strings.Join(parts, ":"),
	}
	cert.Validity.NotBefore = now.Format(time.RFC3339Nano)
	cert.Validity.NotAfter = notAfter.Format(time.RFC3339Nano)
	return cert
}
