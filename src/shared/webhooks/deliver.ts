import { getDb } from "../storage/db.js";

interface DeliverWebhookInput {
  provider: string;
  resourceType: string;
  resourceId: string;
  url: string;
  payload: unknown;
  headers?: Record<string, string>;
  timeoutMs?: number;
}

export async function deliverWebhook(input: DeliverWebhookInput): Promise<void> {
  const { provider, resourceType, resourceId, url, payload, headers, timeoutMs = 5000 } = input;
  const db = getDb();
  const payloadJson = JSON.stringify(payload);

  let status = "delivered";
  let responseCode: number | null = null;

  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    try {
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json", ...headers },
        body: payloadJson,
        signal: controller.signal,
      });
      responseCode = res.status;
      if (!res.ok) status = "failed";
    } finally {
      clearTimeout(timer);
    }
  } catch {
    status = "failed";
  }

  db.prepare(
    `INSERT INTO webhook_deliveries (provider, resource_type, resource_id, url, payload_json, status, response_code)
     VALUES (?, ?, ?, ?, ?, ?, ?)`,
  ).run(provider, resourceType, resourceId, url, payloadJson, status, responseCode);
}
