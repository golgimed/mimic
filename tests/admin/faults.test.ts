import { createServer } from "node:http";
import type { AddressInfo } from "node:net";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FastifyInstance } from "fastify";

process.env.DB_PATH = ":memory:";
process.env.MIGRATIONS_DIR = "db/migrations";
process.env.ZENVIA_STATUS_DELAY_MS = "0";

async function freshServer(): Promise<{ app: FastifyInstance; tick: () => Promise<void> }> {
  vi.resetModules();
  const { buildServer } = await import("../../src/server.js");
  const { tick } = await import("../../src/shared/scheduler/scheduler.js");
  const app = await buildServer();
  return { app, tick };
}

function startWebhookReceiver(): Promise<{ url: string; events: any[]; close: () => Promise<void> }> {
  const events: any[] = [];
  const server = createServer((req, res) => {
    const chunks: Buffer[] = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      events.push(JSON.parse(Buffer.concat(chunks).toString("utf-8")));
      res.writeHead(200);
      res.end();
    });
  });
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => {
      const { port } = server.address() as AddressInfo;
      resolve({
        url: `http://127.0.0.1:${port}/webhook`,
        events,
        close: () => new Promise((r) => server.close(() => r())),
      });
    });
  });
}

async function createSmsMessage(app: FastifyInstance) {
  return app.inject({
    method: "POST",
    url: "/zenvia/channels/sms/messages",
    headers: { "x-api-token": "test-token" },
    payload: {
      from: "sms-account",
      to: "55108888888888",
      contents: [{ type: "text", text: "Hi!" }],
    },
  });
}

describe("Fault injection", () => {
  let app: FastifyInstance;
  let tick: () => Promise<void>;

  beforeEach(async () => {
    ({ app, tick } = await freshServer());
  });

  it("delay_ms delays the response by at least the configured amount", async () => {
    await app.inject({
      method: "PUT",
      url: "/admin/faults",
      payload: { provider: "zenvia", routePattern: "/zenvia/channels/sms/messages", faultKind: "delay_ms", faultValue: "80" },
    });

    const start = Date.now();
    const res = await createSmsMessage(app);
    const elapsed = Date.now() - start;

    expect(res.statusCode).toBe(200);
    expect(elapsed).toBeGreaterThanOrEqual(75);
  });

  it("http_status forces the configured status code", async () => {
    await app.inject({
      method: "PUT",
      url: "/admin/faults",
      payload: { provider: "zenvia", routePattern: "/zenvia/channels/sms/messages", faultKind: "http_status", faultValue: "503" },
    });

    const res = await createSmsMessage(app);
    expect(res.statusCode).toBe(503);
    expect(res.json().error.code).toBe(503);
  });

  it("invalid_payload returns a body that fails JSON parsing", async () => {
    await app.inject({
      method: "PUT",
      url: "/admin/faults",
      payload: { provider: "zenvia", routePattern: "/zenvia/channels/sms/messages", faultKind: "invalid_payload" },
    });

    const res = await createSmsMessage(app);
    expect(() => JSON.parse(res.payload)).toThrow();
  });

  it("timeout never resolves the request within a short window", async () => {
    await app.inject({
      method: "PUT",
      url: "/admin/faults",
      payload: { provider: "zenvia", routePattern: "/zenvia/channels/sms/messages", faultKind: "timeout" },
    });

    const NEVER = Symbol("never");
    const result = await Promise.race([
      createSmsMessage(app),
      new Promise((resolve) => setTimeout(() => resolve(NEVER), 150)),
    ]);
    expect(result).toBe(NEVER);
  });

  it("a fault is consumed once and does not apply to the next request", async () => {
    await app.inject({
      method: "PUT",
      url: "/admin/faults",
      payload: {
        provider: "zenvia",
        routePattern: "/zenvia/channels/sms/messages",
        faultKind: "http_status",
        faultValue: "500",
        times: 1,
      },
    });

    const first = await createSmsMessage(app);
    expect(first.statusCode).toBe(500);

    const second = await createSmsMessage(app);
    expect(second.statusCode).toBe(200);
  });

  it("webhook_dropped skips webhook delivery silently", async () => {
    const receiver = await startWebhookReceiver();
    await app.inject({
      method: "POST",
      url: "/zenvia/subscriptions",
      headers: { "x-api-token": "test-token" },
      payload: { eventType: "MESSAGE_STATUS", webhook: { url: receiver.url }, criteria: { channel: "sms" } },
    });
    await app.inject({
      method: "PUT",
      url: "/admin/faults",
      payload: { provider: "zenvia", routePattern: "webhook", faultKind: "webhook_dropped" },
    });

    await createSmsMessage(app);
    await tick();

    expect(receiver.events).toHaveLength(0);
    await receiver.close();
  });

  it("webhook_invalid delivers a mangled payload", async () => {
    const receiver = await startWebhookReceiver();
    await app.inject({
      method: "POST",
      url: "/zenvia/subscriptions",
      headers: { "x-api-token": "test-token" },
      payload: { eventType: "MESSAGE_STATUS", webhook: { url: receiver.url }, criteria: { channel: "sms" } },
    });
    await app.inject({
      method: "PUT",
      url: "/admin/faults",
      payload: { provider: "zenvia", routePattern: "webhook", faultKind: "webhook_invalid" },
    });

    await createSmsMessage(app);
    await tick();

    expect(receiver.events).toHaveLength(1);
    expect(receiver.events[0].messageStatus).toBeUndefined();
    expect(receiver.events[0].malformed).toBe(true);
    await receiver.close();
  });

  it("lists and deletes configured faults", async () => {
    const createRes = await app.inject({
      method: "PUT",
      url: "/admin/faults",
      payload: { provider: "integraicp", faultKind: "delay_ms", faultValue: "10" },
    });
    const id = createRes.json().id;

    const listRes = await app.inject({ method: "GET", url: "/admin/faults" });
    expect(listRes.json().content).toHaveLength(1);

    const deleteRes = await app.inject({ method: "DELETE", url: `/admin/faults/${id}` });
    expect(deleteRes.statusCode).toBe(204);

    const listAfter = await app.inject({ method: "GET", url: "/admin/faults" });
    expect(listAfter.json().content).toHaveLength(0);
  });
});
