import { createHash } from "node:crypto";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { FastifyInstance } from "fastify";

process.env.DB_PATH = ":memory:";
process.env.MIGRATIONS_DIR = "db/migrations";

async function freshServer(): Promise<FastifyInstance> {
  vi.resetModules();
  const { buildServer } = await import("../../src/server.js");
  return buildServer();
}

function pkce() {
  const verifier = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM";
  const challenge = createHash("sha256").update(verifier).digest("base64url");
  return { verifier, challenge };
}

function sha256Base64(text: string): string {
  return createHash("sha256").update(text).digest("base64");
}

async function authenticateAndGetCredentialId(app: FastifyInstance, challenge: string): Promise<string> {
  const res = await app.inject({
    method: "GET",
    url: `/integraicp/c/test-channel/icp/v3/authentications?subject_key=46404461013&secret_data=${challenge}&secret_type=code_challenge&callback_uri=${encodeURIComponent("https://my.app/callback")}&autostart=true`,
  });
  expect(res.statusCode).toBe(302);
  const location = new URL(res.headers.location as string);
  return location.searchParams.get("credentialId")!;
}

describe("IntegraICP signature flow", () => {
  let app: FastifyInstance;

  beforeEach(async () => {
    app = await freshServer();
  });

  it("authenticates, fetches credential, and signs content end to end", async () => {
    const { verifier, challenge } = pkce();
    const credentialId = await authenticateAndGetCredentialId(app, challenge);

    const credRes = await app.inject({
      method: "GET",
      url: `/integraicp/c/test-channel/icp/v3/credentials/${credentialId}?secret_data=${verifier}`,
    });
    expect(credRes.statusCode).toBe(200);
    const credBody = credRes.json();
    expect(credBody.data.executionStatus.currentStatus).toBe("PENDING_SIGNATURES");
    expect(credBody.data.subjectIdentification.identificationKey).toBe("46404461013");

    const digest = sha256Base64("hello world");
    const sigRes = await app.inject({
      method: "POST",
      url: "/integraicp/c/test-channel/icp/v3/signatures",
      payload: {
        credentialId,
        secretType: "code_verifier",
        secretData: verifier,
        requests: [{ contentId: "doc_001", contentDigest: digest, signaturePolicy: "RAW" }],
      },
    });
    expect(sigRes.statusCode).toBe(200);
    const sigBody = sigRes.json();
    expect(sigBody.data.executionStatus.currentStatus).toBe("COMPLETED_WITH_SUCCESS");
    expect(sigBody.data.signatures).toHaveLength(1);
    expect(sigBody.data.signatures[0].contentId).toBe("doc_001");
    expect(sigBody.data.signatures[0].signedContent).toBeTypeOf("string");
  });

  it("rejects credential lookup with wrong PKCE verifier", async () => {
    const { challenge } = pkce();
    const credentialId = await authenticateAndGetCredentialId(app, challenge);

    const res = await app.inject({
      method: "GET",
      url: `/integraicp/c/test-channel/icp/v3/credentials/${credentialId}?secret_data=wrong-verifier`,
    });
    expect(res.statusCode).toBe(403);
    expect(res.json().error.code).toBe(403201);
  });

  it("returns 404 for unknown credential", async () => {
    const res = await app.inject({
      method: "GET",
      url: "/integraicp/c/test-channel/icp/v3/credentials/does-not-exist?secret_data=x",
    });
    expect(res.statusCode).toBe(404);
    expect(res.json().error.code).toBe(404000);
  });

  it("rejects a malformed content digest", async () => {
    const { verifier, challenge } = pkce();
    const credentialId = await authenticateAndGetCredentialId(app, challenge);

    const res = await app.inject({
      method: "POST",
      url: "/integraicp/c/test-channel/icp/v3/signatures",
      payload: {
        credentialId,
        secretData: verifier,
        requests: [{ contentDigest: "not-a-real-digest" }],
      },
    });
    expect(res.statusCode).toBe(400);
    expect(res.json().error.code).toBe(400204);
  });

  it("returns a clearances list when autostart is omitted", async () => {
    const { challenge } = pkce();
    const res = await app.inject({
      method: "GET",
      url: `/integraicp/c/test-channel/icp/v3/authentications?secret_data=${challenge}&callback_uri=${encodeURIComponent("https://my.app/callback")}`,
    });
    expect(res.statusCode).toBe(200);
    const body = res.json();
    expect(body.data.executionStatus.currentStatus).toBe("PENDING_AUTHORIZATON");
    expect(body.data.clearances.length).toBeGreaterThan(0);
  });
});
