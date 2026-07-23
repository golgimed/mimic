import { getDb } from "../storage/db.js";

type JobHandler = (payload: unknown) => Promise<void> | void;

interface JobRow {
  id: number;
  kind: string;
  payload_json: string;
  run_at: string;
  status: string;
}

const handlers = new Map<string, JobHandler>();

export function registerJobHandler(kind: string, handler: JobHandler): void {
  handlers.set(kind, handler);
}

export function scheduleJob(kind: string, payload: unknown, runAt: Date): void {
  getDb()
    .prepare("INSERT INTO jobs (kind, payload_json, run_at, status) VALUES (?, ?, ?, 'pending')")
    .run(kind, JSON.stringify(payload), runAt.toISOString());
}

export async function tick(): Promise<void> {
  const db = getDb();
  const due = db
    .prepare("SELECT * FROM jobs WHERE status = 'pending' AND run_at <= ? ORDER BY run_at")
    .all(new Date().toISOString()) as JobRow[];

  for (const job of due) {
    db.prepare("UPDATE jobs SET status = 'processing' WHERE id = ?").run(job.id);
    const handler = handlers.get(job.kind);
    try {
      if (handler) {
        await handler(JSON.parse(job.payload_json));
      }
      db.prepare("UPDATE jobs SET status = 'done' WHERE id = ?").run(job.id);
    } catch (err) {
      db.prepare("UPDATE jobs SET status = 'failed' WHERE id = ?").run(job.id);
      throw err;
    }
  }
}

export function startScheduler(intervalMs: number): () => void {
  const timer = setInterval(() => {
    tick().catch((err) => console.error("[scheduler] tick failed", err));
  }, intervalMs);
  return () => clearInterval(timer);
}
