import { beforeEach, describe, expect, it, vi } from "vitest";
import type { FastifyInstance } from "fastify";

process.env.DB_PATH = ":memory:";
process.env.MIGRATIONS_DIR = "db/migrations";

async function freshServer(): Promise<FastifyInstance> {
  vi.resetModules();
  const { buildServer } = await import("../../src/server.js");
  return buildServer();
}

describe("POST /zenvia/channels/sms/messages", () => {
  let app: FastifyInstance;

  beforeEach(async () => {
    app = await freshServer();
  });

  it("creates a text SMS message and echoes the real Zenvia response shape", async () => {
    const res = await app.inject({
      method: "POST",
      url: "/zenvia/channels/sms/messages",
      headers: { "x-api-token": "test-token" },
      payload: {
        from: "sms-account",
        to: "55108888888888",
        contents: [{ type: "text", text: "Hi Zenvia!" }],
      },
    });

    expect(res.statusCode).toBe(200);
    const body = res.json();
    expect(body.id).toBeTypeOf("string");
    expect(body.from).toBe("sms-account");
    expect(body.to).toBe("55108888888888");
    expect(body.direction).toBe("OUT");
    expect(body.channel).toBe("sms");
    expect(body.contents).toEqual([{ type: "text", text: "Hi Zenvia!" }]);
    expect(body.status).toBeUndefined();
  });

  it("creates a template SMS message", async () => {
    const res = await app.inject({
      method: "POST",
      url: "/zenvia/channels/sms/messages",
      headers: { "x-api-token": "test-token" },
      payload: {
        from: "sms-account",
        to: "55108888888888",
        contents: [{ type: "template", templateId: "template_id", fields: { name: "Jhon" } }],
      },
    });

    expect(res.statusCode).toBe(200);
    expect(res.json().contents[0].templateId).toBe("template_id");
  });

  it("rejects requests without X-API-TOKEN", async () => {
    const res = await app.inject({
      method: "POST",
      url: "/zenvia/channels/sms/messages",
      payload: {
        from: "sms-account",
        to: "55108888888888",
        contents: [{ type: "text", text: "Hi Zenvia!" }],
      },
    });

    expect(res.statusCode).toBe(401);
  });

  it("rejects invalid payloads with a VALIDATION_ERROR shape", async () => {
    const res = await app.inject({
      method: "POST",
      url: "/zenvia/channels/sms/messages",
      headers: { "x-api-token": "test-token" },
      payload: { from: "sms-account" },
    });

    expect(res.statusCode).toBe(400);
    const body = res.json();
    expect(body.code).toBe("VALIDATION_ERROR");
    expect(Array.isArray(body.details)).toBe(true);
  });
});
