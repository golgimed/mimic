import { getDb } from "../storage/db.js";
import { listMessages, getMessage } from "../../providers/zenvia/state.js";
import { listCredentials, getCredential } from "../../providers/integraicp/state.js";

export interface DashboardItem {
  provider: "zenvia" | "integraicp";
  type: string;
  id: string;
  status: string;
  createdAt: string;
}

export function listItems(): DashboardItem[] {
  const messages: DashboardItem[] = listMessages().map((m) => ({
    provider: "zenvia",
    type: m.channel,
    id: m.id,
    status: m.status,
    createdAt: m.createdAt,
  }));

  const credentials: DashboardItem[] = listCredentials().map((c) => ({
    provider: "integraicp",
    type: "signature",
    id: c.id,
    status: "AUTHENTICATED",
    createdAt: c.createdAt,
  }));

  return [...messages, ...credentials].sort((a, b) => (a.createdAt < b.createdAt ? 1 : -1));
}

function listWebhookDeliveries(provider: string, resourceId: string) {
  return getDb()
    .prepare(
      `SELECT url, payload_json, status, response_code, created_at
       FROM webhook_deliveries WHERE provider = ? AND resource_id = ? ORDER BY created_at`,
    )
    .all(provider, resourceId) as Array<{
    url: string;
    payload_json: string;
    status: string;
    response_code: number | null;
    created_at: string;
  }>;
}

export function getItemDetail(provider: string, id: string) {
  if (provider === "zenvia") {
    const message = getMessage(id);
    if (!message) return undefined;
    return {
      provider,
      payload: message,
      webhookDeliveries: listWebhookDeliveries("zenvia", id).map((d) => ({
        url: d.url,
        payload: JSON.parse(d.payload_json),
        status: d.status,
        responseCode: d.response_code,
        createdAt: d.created_at,
      })),
    };
  }

  if (provider === "integraicp") {
    const credential = getCredential(id);
    if (!credential) return undefined;
    return { provider, payload: credential, webhookDeliveries: [] };
  }

  return undefined;
}
