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

describe("Zenvia MESSAGE_STATUS webhook flow", () => {
  let app: FastifyInstance;
  let tick: () => Promise<void>;
  let receiver: Awaited<ReturnType<typeof startWebhookReceiver>>;

  beforeEach(async () => {
    ({ app, tick } = await freshServer());
    receiver = await startWebhookReceiver();
  });

  afterEach(async () => {
    await receiver.close();
    await app.close();
  });

  it("drives ACCEPTED -> SENT -> DELIVERED and posts a MESSAGE_STATUS event per hop", async () => {
    const subRes = await app.inject({
      method: "POST",
      url: "/zenvia/subscriptions",
      headers: { "x-api-token": "test-token" },
      payload: {
        eventType: "MESSAGE_STATUS",
        webhook: { url: receiver.url },
        criteria: { channel: "sms" },
      },
    });
    expect(subRes.statusCode).toBe(200);

    const msgRes = await app.inject({
      method: "POST",
      url: "/zenvia/channels/sms/messages",
      headers: { "x-api-token": "test-token" },
      payload: {
        from: "sms-account",
        to: "55108888888888",
        contents: [{ type: "text", text: "Hi Zenvia!" }],
      },
    });
    expect(msgRes.statusCode).toBe(200);
    const messageId = msgRes.json().id;

    await tick(); // ACCEPTED -> SENT
    await tick(); // SENT -> DELIVERED

    expect(receiver.events).toHaveLength(2);
    expect(receiver.events[0].type).toBe("MESSAGE_STATUS");
    expect(receiver.events[0].messageStatus.code).toBe("SENT");
    expect(receiver.events[0].message.id).toBe(messageId);
    expect(receiver.events[1].messageStatus.code).toBe("DELIVERED");
  });

  it("lists and deletes a subscription", async () => {
    const createRes = await app.inject({
      method: "POST",
      url: "/zenvia/subscriptions",
      headers: { "x-api-token": "test-token" },
      payload: {
        eventType: "MESSAGE_STATUS",
        webhook: { url: receiver.url },
        criteria: { channel: "sms" },
      },
    });
    const id = createRes.json().id;

    const listRes = await app.inject({
      method: "GET",
      url: "/zenvia/subscriptions",
      headers: { "x-api-token": "test-token" },
    });
    expect(listRes.json().content).toHaveLength(1);

    const deleteRes = await app.inject({
      method: "DELETE",
      url: `/zenvia/subscriptions/${id}`,
      headers: { "x-api-token": "test-token" },
    });
    expect(deleteRes.statusCode).toBe(204);

    const getRes = await app.inject({
      method: "GET",
      url: `/zenvia/subscriptions/${id}`,
      headers: { "x-api-token": "test-token" },
    });
    expect(getRes.statusCode).toBe(404);
  });

  it("returns 404 for an unknown subscription id", async () => {
    const res = await app.inject({
      method: "GET",
      url: "/zenvia/subscriptions/does-not-exist",
      headers: { "x-api-token": "test-token" },
    });
    expect(res.statusCode).toBe(404);
  });

  it("skips a subscription whose criteria.direction doesn't match the message", async () => {
    await app.inject({
      method: "POST",
      url: "/zenvia/subscriptions",
      headers: { "x-api-token": "test-token" },
      payload: {
        eventType: "MESSAGE_STATUS",
        webhook: { url: receiver.url },
        criteria: { channel: "sms", direction: "IN" },
      },
    });

    await app.inject({
      method: "POST",
      url: "/zenvia/channels/sms/messages",
      headers: { "x-api-token": "test-token" },
      payload: {
        from: "sms-account",
        to: "55108888888888",
        contents: [{ type: "text", text: "Hi Zenvia!" }],
      },
    });

    await tick();
    await tick();

    expect(receiver.events).toHaveLength(0);
  });

  it("delivers to a subscription with criteria.direction ALL", async () => {
    await app.inject({
      method: "POST",
      url: "/zenvia/subscriptions",
      headers: { "x-api-token": "test-token" },
      payload: {
        eventType: "MESSAGE_STATUS",
        webhook: { url: receiver.url },
        criteria: { channel: "sms", direction: "ALL" },
      },
    });

    await app.inject({
      method: "POST",
      url: "/zenvia/channels/sms/messages",
      headers: { "x-api-token": "test-token" },
      payload: {
        from: "sms-account",
        to: "55108888888888",
        contents: [{ type: "text", text: "Hi Zenvia!" }],
      },
    });

    await tick();

    expect(receiver.events).toHaveLength(1);
  });
});
