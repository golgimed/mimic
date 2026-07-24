import type { Provider } from "../../core/registry.js";
import { listWebhookDeliveries } from "../../shared/webhooks/deliver.js";
import { zenviaRoutes } from "./routes.js";
import { getMessage, listMessages } from "./state.js";

export const zenviaProvider: Provider = {
  name: "zenvia",

  register(app) {
    return zenviaRoutes(app);
  },

  listItems() {
    return listMessages().map((m) => ({
      provider: "zenvia",
      type: m.channel,
      id: m.id,
      status: m.status,
      createdAt: m.createdAt,
    }));
  },

  getItemDetail(id) {
    const message = getMessage(id);
    if (!message) return undefined;
    return {
      provider: "zenvia",
      payload: message,
      webhookDeliveries: listWebhookDeliveries("zenvia", id),
    };
  },
};
