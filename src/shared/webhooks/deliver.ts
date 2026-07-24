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

export function logDroppedWebhook(input: Omit<DeliverWebhookInput, "timeoutMs" | "headers">): void {
  getDb()
    .prepare(
      `INSERT INTO webhook_deliveries (provider, resource_type, resource_id, url, payload_json, status, response_code)
       VALUES (?, ?, ?, ?, ?, 'dropped', NULL)`,
    )
    .run(input.provider, input.resourceType, input.resourceId, input.url, JSON.stringify(input.payload));
}

interface WebhookDeliveryRow {
  url: string;
  payload_json: string;
  status: string;
  response_code: number | null;
  created_at: string;
}

export function listWebhookDeliveries(provider: string, resourceId: string) {
  const rows = getDb()
    .prepare(
      `SELECT url, payload_json, status, response_code, created_at
       FROM webhook_deliveries WHERE provider = ? AND resource_id = ? ORDER BY created_at`,
    )
    .all(provider, resourceId) as WebhookDeliveryRow[];

  return rows.map((d) => ({
    url: d.url,
    payload: JSON.parse(d.payload_json),
    status: d.status,
    responseCode: d.response_code,
    createdAt: d.created_at,
  }));
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
