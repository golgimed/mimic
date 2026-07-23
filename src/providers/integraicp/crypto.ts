import { createHash, randomUUID } from "node:crypto";

// RFC 7636 PKCE: code_challenge = base64url(sha256(code_verifier)), no padding.
export function codeChallengeFromVerifier(verifier: string): string {
  return createHash("sha256").update(verifier).digest("base64url");
}

export function verifyPkce(verifier: string, storedChallenge: string): boolean {
  return codeChallengeFromVerifier(verifier) === storedChallenge;
}

export interface FakeCertificate {
  serialNumber: string;
  issuerName: string;
  validity: { notBefore: string; notAfter: string };
  subjectName: string;
  encodedX509: string;
  fingerprint256: string;
}

export function buildFakeCertificate(subjectName: string): FakeCertificate {
  const now = new Date();
  const notAfter = new Date(now.getTime() + 365 * 24 * 60 * 60 * 1000);
  const fingerprintBytes = createHash("sha256").update(subjectName + randomUUID()).digest();
  const fingerprint256 = Array.from(fingerprintBytes)
    .map((b) => b.toString(16).padStart(2, "0").toUpperCase())
    .join(":");

  return {
    serialNumber: randomUUID().replace(/-/g, "").toUpperCase(),
    issuerName: "AC IntegraICP Simulador v1",
    validity: { notBefore: now.toISOString(), notAfter: notAfter.toISOString() },
    subjectName,
    encodedX509: "-----BEGIN CERTIFICATE-----\nSIMULATED\n-----END CERTIFICATE-----\n",
    fingerprint256,
  };
}
