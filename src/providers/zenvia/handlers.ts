import type { FastifyReply, FastifyRequest } from "fastify";
import { createSmsMessageSchema } from "./schemas.js";
import { createMessage } from "./state.js";

function toResponse(message: ReturnType<typeof createMessage>) {
  return {
    id: message.id,
    externalId: message.externalId ?? undefined,
    from: message.from,
    to: message.to,
    direction: message.direction,
    channel: message.channel,
    contents: message.contents,
    timestamp: message.createdAt,
  };
}

export async function createSmsMessageHandler(req: FastifyRequest, reply: FastifyReply) {
  const parsed = createSmsMessageSchema.safeParse(req.body);
  if (!parsed.success) {
    return reply.code(400).send({
      code: "VALIDATION_ERROR",
      message: "Validation error",
      details: parsed.error.issues.map((issue) => ({
        code: issue.code,
        path: issue.path.join("."),
        message: issue.message,
      })),
    });
  }

  const message = createMessage("sms", parsed.data);
  return reply.code(200).send(toResponse(message));
}
