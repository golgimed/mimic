import type { FastifyInstance } from "fastify";
import { requireApiToken } from "../../shared/auth/apiToken.js";
import { requestFaultHook } from "../../shared/admin/applyFaults.js";
import {
  createEmailMessageHandler,
  createSmsMessageHandler,
  createSubscriptionHandler,
  createWhatsappMessageHandler,
  deleteSubscriptionHandler,
  getSubscriptionHandler,
  listSubscriptionsHandler,
} from "./handlers.js";
import { registerZenviaScheduler } from "./scheduler.js";

export async function zenviaRoutes(app: FastifyInstance) {
  registerZenviaScheduler();

  app.addHook("onRequest", await requireApiToken("X-API-TOKEN"));
  app.addHook("preHandler", requestFaultHook("zenvia"));

  app.post("/channels/sms/messages", createSmsMessageHandler);
  app.post("/channels/whatsapp/messages", createWhatsappMessageHandler);
  app.post("/channels/email/messages", createEmailMessageHandler);

  app.post("/subscriptions", createSubscriptionHandler);
  app.get("/subscriptions", listSubscriptionsHandler);
  app.get("/subscriptions/:subscriptionId", getSubscriptionHandler);
  app.delete("/subscriptions/:subscriptionId", deleteSubscriptionHandler);
}
