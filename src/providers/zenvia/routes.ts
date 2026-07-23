import type { FastifyInstance } from "fastify";
import { requireApiToken } from "../../shared/auth/apiToken.js";
import {
  createSmsMessageHandler,
  createSubscriptionHandler,
  deleteSubscriptionHandler,
  getSubscriptionHandler,
  listSubscriptionsHandler,
} from "./handlers.js";
import { registerZenviaScheduler } from "./scheduler.js";

export async function zenviaRoutes(app: FastifyInstance) {
  registerZenviaScheduler();

  app.addHook("onRequest", await requireApiToken("X-API-TOKEN"));

  app.post("/channels/sms/messages", createSmsMessageHandler);

  app.post("/subscriptions", createSubscriptionHandler);
  app.get("/subscriptions", listSubscriptionsHandler);
  app.get("/subscriptions/:subscriptionId", getSubscriptionHandler);
  app.delete("/subscriptions/:subscriptionId", deleteSubscriptionHandler);
}
