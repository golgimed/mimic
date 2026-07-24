import { existsSync, readFileSync } from "node:fs";
import Fastify from "fastify";
import cors from "@fastify/cors";

if (existsSync(".env")) {
  process.loadEnvFile(".env");
}
import { getDb } from "../shared/storage/db.js";
import { runMigrations } from "../shared/storage/migrate.js";
import { startScheduler } from "../shared/scheduler/scheduler.js";
import { adminRoutes } from "../shared/admin/routes.js";
import { registerAllProviders } from "../providers/index.js";
import { getEnabledProviders } from "./registry.js";

const SCHEDULER_INTERVAL_MS = Number(process.env.SCHEDULER_INTERVAL_MS ?? 1000);
const DASHBOARD_PATH = process.env.DASHBOARD_PATH ?? "dashboard/index.html";

export async function buildServer() {
  const app = Fastify({
    logger: { level: process.env.LOG_LEVEL ?? "info" },
  });

  runMigrations(getDb());
  registerAllProviders();

  await app.register(cors, { origin: true });

  app.get("/health", async () => ({ status: "ok" }));

  app.get("/dashboard", async (_req, reply) => {
    reply.type("text/html").send(readFileSync(DASHBOARD_PATH, "utf-8"));
  });

  for (const provider of getEnabledProviders()) {
    await app.register((instance) => provider.register(instance), { prefix: `/${provider.name}` });
  }

  await app.register(adminRoutes, { prefix: "/admin" });

  const stopScheduler = startScheduler(SCHEDULER_INTERVAL_MS);
  app.addHook("onClose", async () => stopScheduler());

  return app;
}
