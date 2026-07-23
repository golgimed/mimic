import type { FastifyInstance } from "fastify";
import { requireApiToken } from "../../shared/auth/apiToken.js";
import { createSmsMessageHandler } from "./handlers.js";

export async function zenviaRoutes(app: FastifyInstance) {
  app.addHook("onRequest", await requireApiToken("X-API-TOKEN"));

  app.post("/channels/sms/messages", createSmsMessageHandler);
}
