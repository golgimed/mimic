import { randomUUID } from "node:crypto";
import { getDb } from "../../shared/storage/db.js";
import type { CreateSmsMessageInput } from "./schemas.js";

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
