import { randomUUID } from "node:crypto";
import { getDb } from "../../shared/storage/db.js";
import type { CreateSmsMessageInput, CreateSubscriptionInput } from "./schemas.js";

export interface ZenviaMessage {
  id: string;
  externalId: string | null;
  channel: string;
  direction: "IN" | "OUT";
  from: string;
  to: string;
  contents: unknown[];
  status: string;
  createdAt: string;
}

interface MessageRow {
  id: string;
  external_id: string | null;
  channel: string;
  direction: string;
  sender: string;
  recipient: string;
  contents_json: string;
  status: string;
  created_at: string;
}

function toMessage(row: MessageRow): ZenviaMessage {
  return {
    id: row.id,
    externalId: row.external_id,
    channel: row.channel,
    direction: row.direction as "IN" | "OUT",
    from: row.sender,
    to: row.recipient,
    contents: JSON.parse(row.contents_json),
    status: row.status,
    createdAt: row.created_at,
  };
}

export function createMessage(channel: string, input: CreateSmsMessageInput): ZenviaMessage {
  const db = getDb();
  const id = randomUUID();
  db.prepare(
    `INSERT INTO zenvia_messages (id, external_id, channel, direction, sender, recipient, contents_json, status)
     VALUES (?, ?, ?, 'OUT', ?, ?, ?, 'ACCEPTED')`,
  ).run(id, input.externalId ?? null, channel, input.from, input.to, JSON.stringify(input.contents));

  const row = db.prepare("SELECT * FROM zenvia_messages WHERE id = ?").get(id) as MessageRow;
  return toMessage(row);
}

export function listMessages(): ZenviaMessage[] {
  const rows = getDb().prepare("SELECT * FROM zenvia_messages ORDER BY created_at DESC").all() as MessageRow[];
  return rows.map(toMessage);
}

export function getMessage(id: string): ZenviaMessage | undefined {
  const row = getDb().prepare("SELECT * FROM zenvia_messages WHERE id = ?").get(id) as MessageRow | undefined;
  return row ? toMessage(row) : undefined;
}

export function updateMessageStatus(id: string, status: string): void {
  getDb().prepare("UPDATE zenvia_messages SET status = ? WHERE id = ?").run(status, id);
}

export interface ZenviaSubscription {
  id: string;
  eventType: string;
  webhookUrl: string;
  webhookHeaders: Record<string, string> | null;
  criteriaChannel: string;
  criteriaDirection: string | null;
  status: string;
  createdAt: string;
  updatedAt: string;
}

interface SubscriptionRow {
  id: string;
  event_type: string;
  webhook_url: string;
  webhook_headers_json: string | null;
  criteria_channel: string;
  criteria_direction: string | null;
  status: string;
  created_at: string;
  updated_at: string;
}

function toSubscription(row: SubscriptionRow): ZenviaSubscription {
  return {
    id: row.id,
    eventType: row.event_type,
    webhookUrl: row.webhook_url,
    webhookHeaders: row.webhook_headers_json ? JSON.parse(row.webhook_headers_json) : null,
    criteriaChannel: row.criteria_channel,
    criteriaDirection: row.criteria_direction,
    status: row.status,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export function createSubscription(input: CreateSubscriptionInput): ZenviaSubscription {
  const db = getDb();
  const id = randomUUID();
  db.prepare(
    `INSERT INTO zenvia_subscriptions (id, event_type, webhook_url, webhook_headers_json, criteria_channel, criteria_direction, status)
     VALUES (?, ?, ?, ?, ?, ?, 'ACTIVE')`,
  ).run(
    id,
    input.eventType,
    input.webhook.url,
    input.webhook.headers ? JSON.stringify(input.webhook.headers) : null,
    input.criteria.channel,
    input.criteria.direction ?? null,
  );
  const row = db.prepare("SELECT * FROM zenvia_subscriptions WHERE id = ?").get(id) as SubscriptionRow;
  return toSubscription(row);
}

export function listSubscriptions(): ZenviaSubscription[] {
  const rows = getDb().prepare("SELECT * FROM zenvia_subscriptions ORDER BY created_at").all() as SubscriptionRow[];
  return rows.map(toSubscription);
}

export function getSubscription(id: string): ZenviaSubscription | undefined {
  const row = getDb().prepare("SELECT * FROM zenvia_subscriptions WHERE id = ?").get(id) as
    | SubscriptionRow
    | undefined;
  return row ? toSubscription(row) : undefined;
}

export function deleteSubscription(id: string): boolean {
  const result = getDb().prepare("DELETE FROM zenvia_subscriptions WHERE id = ?").run(id);
  return result.changes > 0;
}

export function findActiveSubscriptionsForChannel(channel: string): ZenviaSubscription[] {
  const rows = getDb()
    .prepare(
      `SELECT * FROM zenvia_subscriptions
       WHERE event_type = 'MESSAGE_STATUS' AND status = 'ACTIVE' AND criteria_channel = ?`,
    )
    .all(channel) as SubscriptionRow[];
  return rows.map(toSubscription);
}
