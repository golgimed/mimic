import type { Provider } from "../../core/registry.js";
import { integraIcpRoutes } from "./routes.js";
import { getCredential, listCredentials } from "./state.js";

export const integraIcpProvider: Provider = {
  name: "integraicp",

  register(app) {
    return integraIcpRoutes(app);
  },

  listItems() {
    return listCredentials().map((c) => ({
      provider: "integraicp",
      type: "signature",
      id: c.id,
      status: "AUTHENTICATED",
      createdAt: c.createdAt,
    }));
  },

  getItemDetail(id) {
    const credential = getCredential(id);
    if (!credential) return undefined;
    return { provider: "integraicp", payload: credential, webhookDeliveries: [] };
  },
};
