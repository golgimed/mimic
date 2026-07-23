import Fastify from "fastify";
import { getDb } from "./shared/storage/db.js";
import { runMigrations } from "./shared/storage/migrate.js";
import { zenviaRoutes } from "./providers/zenvia/routes.js";

export async function buildServer() {
  const app = Fastify({
    logger: { level: process.env.LOG_LEVEL ?? "info" },
  });

  runMigrations(getDb());

  app.get("/health", async () => ({ status: "ok" }));

  await app.register(zenviaRoutes, { prefix: "/zenvia" });

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
