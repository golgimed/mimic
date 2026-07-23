import { existsSync, readFileSync } from "node:fs";
import Fastify from "fastify";
import cors from "@fastify/cors";

if (existsSync(".env")) {
  process.loadEnvFile(".env");
}
import { getDb } from "./shared/storage/db.js";
import { runMigrations } from "./shared/storage/migrate.js";
import { startScheduler } from "./shared/scheduler/scheduler.js";
import { zenviaRoutes } from "./providers/zenvia/routes.js";
import { integraIcpRoutes } from "./providers/integraicp/routes.js";
import { adminRoutes } from "./shared/admin/routes.js";

const SCHEDULER_INTERVAL_MS = Number(process.env.SCHEDULER_INTERVAL_MS ?? 1000);
const DASHBOARD_PATH = process.env.DASHBOARD_PATH ?? "dashboard/index.html";

export async function buildServer() {
  const app = Fastify({
    logger: { level: process.env.LOG_LEVEL ?? "info" },
  });

  runMigrations(getDb());

  await app.register(cors, { origin: true });

  app.get("/health", async () => ({ status: "ok" }));

  app.get("/dashboard", async (_req, reply) => {
    reply.type("text/html").send(readFileSync(DASHBOARD_PATH, "utf-8"));
  });

  await app.register(zenviaRoutes, { prefix: "/zenvia" });
  await app.register(integraIcpRoutes, { prefix: "/integraicp" });
  await app.register(adminRoutes, { prefix: "/admin" });

  const stopScheduler = startScheduler(SCHEDULER_INTERVAL_MS);
  app.addHook("onClose", async () => stopScheduler());

  return app;
}

function printBanner(port: number) {
  const color = process.stdout.isTTY;
  const purple = (s: string) => (color ? `\x1b[35m${s}\x1b[0m` : s);
  const dim = (s: string) => (color ? `\x1b[2m${s}\x1b[0m` : s);

  console.log(`${purple("MIMIC")}  ${dim("looks like the provider, bites different")}`);
  console.log(dim(`listening on http://localhost:${port}`));
}

async function main() {
  const app = await buildServer();
  const port = Number(process.env.PORT ?? 3000);
  await app.listen({ port, host: "0.0.0.0" });
  printBanner(port);
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  main().catch((err) => {
    console.error(err);
    process.exit(1);
  });
}
