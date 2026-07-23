import type { FastifyInstance } from "fastify";
import { authenticationsHandler, credentialsHandler, signaturesHandler } from "./handlers.js";

export async function integraIcpRoutes(app: FastifyInstance) {
  app.get("/c/:channelId/icp/v3/authentications", authenticationsHandler);
  app.get("/c/:channelId/icp/v3/credentials/:credentialId", credentialsHandler);
  app.post("/c/:channelId/icp/v3/signatures", signaturesHandler);
}
