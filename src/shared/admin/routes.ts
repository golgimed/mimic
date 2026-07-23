import type { FastifyInstance } from "fastify";
import { createFaultSchema } from "./schemas.js";
import { createFault, deleteFault, listFaults } from "./faults.js";

export async function adminRoutes(app: FastifyInstance) {
  app.put("/faults", async (req, reply) => {
    const parsed = createFaultSchema.safeParse(req.body);
    if (!parsed.success) {
      return reply.code(400).send({ error: { code: "VALIDATION_ERROR", issues: parsed.error.issues } });
    }
    const fault = createFault(parsed.data);
    return reply.code(201).send(fault);
  });

  app.get("/faults", async (_req, reply) => {
    return reply.code(200).send({ content: listFaults() });
  });

  app.delete("/faults/:id", async (req, reply) => {
    const { id } = req.params as { id: string };
    const deleted = deleteFault(id);
    if (!deleted) {
      return reply.code(404).send({ error: { code: "NOT_FOUND", message: "Fault not found" } });
    }
    return reply.code(204).send();
  });
}
