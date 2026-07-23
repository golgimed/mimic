import type { FastifyReply, FastifyRequest } from "fastify";
import type { ZodType } from "zod";
import { scheduleStatusAdvance } from "./scheduler.js";
import {
  createEmailMessageSchema,
  createSmsMessageSchema,
  createSubscriptionSchema,
  createWhatsappMessageSchema,
} from "./schemas.js";
import {
  createMessage,
  createSubscription,
  deleteSubscription,
  getSubscription,
  listSubscriptions,
} from "./state.js";

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

function createMessageHandler(channel: string, schema: ZodType) {
  return async (req: FastifyRequest, reply: FastifyReply) => {
    const parsed = schema.safeParse(req.body);
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

    const message = createMessage(channel, parsed.data as Parameters<typeof createMessage>[1]);
    scheduleStatusAdvance(message.id);
    return reply.code(200).send(toResponse(message));
  };
}

export const createSmsMessageHandler = createMessageHandler("sms", createSmsMessageSchema);
export const createWhatsappMessageHandler = createMessageHandler("whatsapp", createWhatsappMessageSchema);
export const createEmailMessageHandler = createMessageHandler("email", createEmailMessageSchema);

function toSubscriptionResponse(subscription: ReturnType<typeof createSubscription>) {
  return {
    id: subscription.id,
    eventType: subscription.eventType,
    webhook: {
      url: subscription.webhookUrl,
      headers: subscription.webhookHeaders ?? undefined,
    },
    criteria: {
      channel: subscription.criteriaChannel,
      direction: subscription.criteriaDirection ?? undefined,
    },
    status: subscription.status,
    version: "v2",
    createdAt: subscription.createdAt,
    updatedAt: subscription.updatedAt,
  };
}

export async function createSubscriptionHandler(req: FastifyRequest, reply: FastifyReply) {
  const parsed = createSubscriptionSchema.safeParse(req.body);
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

  const subscription = createSubscription(parsed.data);
  return reply.code(200).send(toSubscriptionResponse(subscription));
}

export async function listSubscriptionsHandler(_req: FastifyRequest, reply: FastifyReply) {
  return reply.code(200).send({ content: listSubscriptions().map(toSubscriptionResponse) });
}

export async function getSubscriptionHandler(
  req: FastifyRequest<{ Params: { subscriptionId: string } }>,
  reply: FastifyReply,
) {
  const subscription = getSubscription(req.params.subscriptionId);
  if (!subscription) {
    return reply.code(404).send({ code: "NOT_FOUND", message: "Subscription not found" });
  }
  return reply.code(200).send(toSubscriptionResponse(subscription));
}

export async function deleteSubscriptionHandler(
  req: FastifyRequest<{ Params: { subscriptionId: string } }>,
  reply: FastifyReply,
) {
  const deleted = deleteSubscription(req.params.subscriptionId);
  if (!deleted) {
    return reply.code(404).send({ code: "NOT_FOUND", message: "Subscription not found" });
  }
  return reply.code(204).send();
}
