import type { FastifyInstance } from "fastify";
import { requestFaultHook } from "../../shared/admin/applyFaults.js";
import { authenticationsHandler, credentialsHandler, signaturesHandler } from "./handlers.js";

export async function integraIcpRoutes(app: FastifyInstance) {
  app.addHook("preHandler", requestFaultHook("integraicp"));

  app.get("/c/:channelId/icp/v3/authentications", authenticationsHandler);
  app.get("/c/:channelId/icp/v3/credentials/:credentialId", credentialsHandler);
  app.post("/c/:channelId/icp/v3/signatures", signaturesHandler);
}
