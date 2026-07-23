import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import type { Database } from "better-sqlite3";

const MIGRATIONS_DIR = process.env.MIGRATIONS_DIR ?? "db/migrations";

export function runMigrations(db: Database): void {
  db.exec(`
    CREATE TABLE IF NOT EXISTS _migrations (
      name TEXT PRIMARY KEY,
      applied_at TEXT NOT NULL DEFAULT (datetime('now'))
    )
  `);

  const applied = new Set(
    db.prepare("SELECT name FROM _migrations").all().map((row) => (row as { name: string }).name),
  );

  const files = readdirSync(MIGRATIONS_DIR)
    .filter((f) => f.endsWith(".sql"))
    .sort();

  const insertMigration = db.prepare("INSERT INTO _migrations (name) VALUES (?)");

  for (const file of files) {
    if (applied.has(file)) continue;
    const sql = readFileSync(join(MIGRATIONS_DIR, file), "utf-8");
    const runMigration = db.transaction(() => {
      db.exec(sql);
      insertMigration.run(file);
    });
    runMigration();
  }
}
