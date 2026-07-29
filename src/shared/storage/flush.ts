import { getDb } from "./db.js";

/**
 * Deletes all rows from every provider/shared data table, but keeps schema
 * and _migrations intact so the app doesn't need to re-migrate on next boot.
 */
export function flushDb(): void {
  const db = getDb();
  const tables = db
    .prepare(
      `SELECT name FROM sqlite_master
       WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name != '_migrations'`,
    )
    .all() as { name: string }[];

  const flush = db.transaction(() => {
    for (const { name } of tables) {
      db.prepare(`DELETE FROM "${name}"`).run();
    }
  });
  flush();
}
