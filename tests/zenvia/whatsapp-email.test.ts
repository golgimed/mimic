import { beforeEach, describe, expect, it, vi } from "vitest";
import type { FastifyInstance } from "fastify";

process.env.DB_PATH = ":memory:";
process.env.MIGRATIONS_DIR = "db/migrations";

async function freshServer(): Promise<FastifyInstance> {
  vi.resetModules();
  const { buildServer } = await import("../../src/server.js");
  return buildServer();
}

describe("POST /zenvia/channels/whatsapp/messages", () => {
  let app: FastifyInstance;

  beforeEach(async () => {
    app = await freshServer();
  });

  it("sends a WhatsApp text message", async () => {
    const res = await app.inject({
      method: "POST",
      url: "/zenvia/channels/whatsapp/messages",
      headers: { "x-api-token": "test-token" },
      payload: {
        from: "whatsapp-account",
        to: "55119999999999",
        contents: [{ type: "text", text: "Hello via WhatsApp" }],
      },
    });

    expect(res.statusCode).toBe(200);
    const body = res.json();
    expect(body.channel).toBe("whatsapp");
    expect(body.contents).toEqual([{ type: "text", text: "Hello via WhatsApp" }]);
  });

  it("sends a WhatsApp file message with idRef/contentRef", async () => {
    const res = await app.inject({
      method: "POST",
      url: "/zenvia/channels/whatsapp/messages",
      headers: { "x-api-token": "test-token" },
      payload: {
        from: "whatsapp-account",
        to: "55119999999999",
        idRef: "7390113b-e120-41b5-8a07-c4567726abc2",
        contentRef: 0,
        contents: [{ type: "file", fileUrl: "https://example.com/file.pdf", fileCaption: "Prescription" }],
      },
    });

    expect(res.statusCode).toBe(200);
    expect(res.json().contents[0].fileUrl).toBe("https://example.com/file.pdf");
  });

  it("rejects an unknown content type", async () => {
    const res = await app.inject({
      method: "POST",
      url: "/zenvia/channels/whatsapp/messages",
      headers: { "x-api-token": "test-token" },
      payload: {
        from: "whatsapp-account",
        to: "55119999999999",
        contents: [{ type: "sticker", stickerId: "abc" }],
      },
    });

    expect(res.statusCode).toBe(400);
  });
});

describe("POST /zenvia/channels/email/messages", () => {
  let app: FastifyInstance;

  beforeEach(async () => {
    app = await freshServer();
  });

  it("sends an email message", async () => {
    const res = await app.inject({
      method: "POST",
      url: "/zenvia/channels/email/messages",
      headers: { "x-api-token": "test-token" },
      payload: {
        from: "no-reply@example.com",
        to: "patient@example.com",
        representative: { name: "GolgiMed" },
        contents: [{ type: "email", subject: "Your results", html: "<b>Hi!</b>" }],
      },
    });

    expect(res.statusCode).toBe(200);
    const body = res.json();
    expect(body.channel).toBe("email");
    expect(body.contents[0].subject).toBe("Your results");
  });

  it("sends an email template message", async () => {
    const res = await app.inject({
      method: "POST",
      url: "/zenvia/channels/email/messages",
      headers: { "x-api-token": "test-token" },
      payload: {
        from: "no-reply@example.com",
        to: "patient@example.com",
        contents: [{ type: "template", templateId: "welcome-email", fields: { name: "Alex" } }],
      },
    });

    expect(res.statusCode).toBe(200);
  });

  it("rejects an email content missing subject", async () => {
    const res = await app.inject({
      method: "POST",
      url: "/zenvia/channels/email/messages",
      headers: { "x-api-token": "test-token" },
      payload: {
        from: "no-reply@example.com",
        to: "patient@example.com",
        contents: [{ type: "email", html: "<b>Hi!</b>" }],
      },
    });

    expect(res.statusCode).toBe(400);
  });
});
