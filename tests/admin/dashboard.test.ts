import { beforeEach, describe, expect, it, vi } from "vitest";
import type { FastifyInstance } from "fastify";

process.env.DB_PATH = ":memory:";
process.env.MIGRATIONS_DIR = "db/migrations";
process.env.ZENVIA_STATUS_DELAY_MS = "60000";

async function freshServer(): Promise<FastifyInstance> {
  vi.resetModules();
  const { buildServer } = await import("../../src/server.js");
  return buildServer();
}

describe("Dashboard admin endpoints", () => {
  let app: FastifyInstance;

  beforeEach(async () => {
    app = await freshServer();
  });

  it("lists items across providers, newest first", async () => {
    const zenviaRes = await app.inject({
      method: "POST",
      url: "/zenvia/channels/sms/messages",
      headers: { "x-api-token": "test-token" },
      payload: { from: "sms-account", to: "5510888", contents: [{ type: "text", text: "hi" }] },
    });
    const messageId = zenviaRes.json().id;

    const authRes = await app.inject({
      method: "GET",
      url: `/integraicp/c/test-channel/icp/v3/authentications?secret_data=abc&callback_uri=${encodeURIComponent("https://my.app/cb")}&autostart=true`,
    });
    const credentialId = new URL(authRes.headers.location as string).searchParams.get("credentialId");

    const listRes = await app.inject({ method: "GET", url: "/admin/items" });
    expect(listRes.statusCode).toBe(200);
    const { content } = listRes.json();
    expect(content).toHaveLength(2);
    expect(content.map((i: any) => i.provider).sort()).toEqual(["integraicp", "zenvia"]);

    const zenviaDetail = await app.inject({ method: "GET", url: `/admin/items/zenvia/${messageId}` });
    expect(zenviaDetail.statusCode).toBe(200);
    expect(zenviaDetail.json().payload.id).toBe(messageId);

    const icpDetail = await app.inject({ method: "GET", url: `/admin/items/integraicp/${credentialId}` });
    expect(icpDetail.statusCode).toBe(200);
    expect(icpDetail.json().payload.id).toBe(credentialId);
  });

  it("returns 404 for an unknown item", async () => {
    const res = await app.inject({ method: "GET", url: "/admin/items/zenvia/does-not-exist" });
    expect(res.statusCode).toBe(404);
  });

  it("serves the dashboard HTML page", async () => {
    const res = await app.inject({ method: "GET", url: "/dashboard" });
    expect(res.statusCode).toBe(200);
    expect(res.headers["content-type"]).toContain("text/html");
    expect(res.payload).toContain("Provider Simulator");
  });
});
