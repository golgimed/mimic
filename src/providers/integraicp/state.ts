import { randomUUID } from "node:crypto";
import { getDb } from "../../shared/storage/db.js";
import type { FakeCertificate } from "./crypto.js";

export interface IntegraIcpCredential {
  id: string;
  channelId: string;
  subjectKey: string | null;
  subjectType: string | null;
  codeChallenge: string;
  callbackUri: string;
  certificate: FakeCertificate;
  createdAt: string;
}

interface CredentialRow {
  id: string;
  channel_id: string;
  subject_key: string | null;
  subject_type: string | null;
  code_challenge: string;
  callback_uri: string;
  certificate_json: string;
  created_at: string;
}

function toCredential(row: CredentialRow): IntegraIcpCredential {
  return {
    id: row.id,
    channelId: row.channel_id,
    subjectKey: row.subject_key,
    subjectType: row.subject_type,
    codeChallenge: row.code_challenge,
    callbackUri: row.callback_uri,
    certificate: JSON.parse(row.certificate_json),
    createdAt: row.created_at,
  };
}

export function createCredential(input: {
  channelId: string;
  subjectKey?: string;
  subjectType?: string;
  codeChallenge: string;
  callbackUri: string;
  certificate: FakeCertificate;
}): IntegraIcpCredential {
  const db = getDb();
  const id = randomUUID();
  db.prepare(
    `INSERT INTO integraicp_credentials (id, channel_id, subject_key, subject_type, code_challenge, callback_uri, certificate_json)
     VALUES (?, ?, ?, ?, ?, ?, ?)`,
  ).run(
    id,
    input.channelId,
    input.subjectKey ?? null,
    input.subjectType ?? null,
    input.codeChallenge,
    input.callbackUri,
    JSON.stringify(input.certificate),
  );
  const row = db.prepare("SELECT * FROM integraicp_credentials WHERE id = ?").get(id) as CredentialRow;
  return toCredential(row);
}

export function getCredential(id: string): IntegraIcpCredential | undefined {
  const row = getDb().prepare("SELECT * FROM integraicp_credentials WHERE id = ?").get(id) as
    | CredentialRow
    | undefined;
  return row ? toCredential(row) : undefined;
}
