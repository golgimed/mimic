import Fastify from "fastify";
import { getDb } from "./shared/storage/db.js";
import { runMigrations } from "./shared/storage/migrate.js";
import { startScheduler } from "./shared/scheduler/scheduler.js";
import { zenviaRoutes } from "./providers/zenvia/routes.js";

const SCHEDULER_INTERVAL_MS = Number(process.env.SCHEDULER_INTERVAL_MS ?? 1000);

export async function buildServer() {
  const app = Fastify({
    logger: { level: process.env.LOG_LEVEL ?? "info" },
  });

  runMigrations(getDb());

  app.get("/health", async () => ({ status: "ok" }));

  await app.register(zenviaRoutes, { prefix: "/zenvia" });

  const stopScheduler = startScheduler(SCHEDULER_INTERVAL_MS);
  app.addHook("onClose", async () => stopScheduler());

  return app;
}

async function main() {
  const app = await buildServer();
  const port = Number(process.env.PORT ?? 3000);
  await app.listen({ port, host: "0.0.0.0" });
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  main().catch((err) => {
    console.error(err);
    process.exit(1);
  });
}
