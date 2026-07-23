import { randomUUID } from "node:crypto";
import { getDb } from "../storage/db.js";

export type FaultKind =
  | "delay_ms"
  | "http_status"
  | "timeout"
  | "invalid_payload"
  | "webhook_dropped"
  | "webhook_invalid";

export interface FaultConfig {
  id: string;
  provider: string;
  routePattern: string | null;
  faultKind: FaultKind;
  faultValue: string | null;
  remainingUses: number | null;
  createdAt: string;
}

interface FaultRow {
  id: string;
  provider: string;
  route_pattern: string | null;
  fault_kind: string;
  fault_value: string | null;
  remaining_uses: number | null;
  created_at: string;
}

function toFault(row: FaultRow): FaultConfig {
  return {
    id: row.id,
    provider: row.provider,
    routePattern: row.route_pattern,
    faultKind: row.fault_kind as FaultKind,
    faultValue: row.fault_value,
    remainingUses: row.remaining_uses,
    createdAt: row.created_at,
  };
}

export function createFault(input: {
  provider: string;
  routePattern?: string;
  faultKind: FaultKind;
  faultValue?: string;
  times?: number;
}): FaultConfig {
  const db = getDb();
  const id = randomUUID();
  db.prepare(
    `INSERT INTO fault_config (id, provider, route_pattern, fault_kind, fault_value, remaining_uses)
     VALUES (?, ?, ?, ?, ?, ?)`,
  ).run(
    id,
    input.provider,
    input.routePattern ?? null,
    input.faultKind,
    input.faultValue ?? null,
    input.times ?? null,
  );
  const row = db.prepare("SELECT * FROM fault_config WHERE id = ?").get(id) as FaultRow;
  return toFault(row);
}

export function listFaults(): FaultConfig[] {
  const rows = getDb().prepare("SELECT * FROM fault_config ORDER BY created_at").all() as FaultRow[];
  return rows.map(toFault);
}

export function deleteFault(id: string): boolean {
  const result = getDb().prepare("DELETE FROM fault_config WHERE id = ?").run(id);
  return result.changes > 0;
}

/**
 * Finds the best-matching active fault for a provider + route, consumes one
 * use (deleting it once remaining_uses hits 0), and returns it. Exact
 * route_pattern matches win over provider-wide (null pattern) faults.
 */
export function consumeMatchingFault(provider: string, routePattern: string): FaultConfig | undefined {
  const db = getDb();
  const rows = db
    .prepare(
      `SELECT * FROM fault_config
       WHERE provider = ? AND (route_pattern = ? OR route_pattern IS NULL)
       ORDER BY route_pattern IS NULL ASC, created_at ASC`,
    )
    .all(provider, routePattern) as FaultRow[];

  const match = rows[0];
  if (!match) return undefined;

  if (match.remaining_uses !== null) {
    if (match.remaining_uses <= 1) {
      db.prepare("DELETE FROM fault_config WHERE id = ?").run(match.id);
    } else {
      db.prepare("UPDATE fault_config SET remaining_uses = remaining_uses - 1 WHERE id = ?").run(match.id);
    }
  }

  return toFault(match);
}
