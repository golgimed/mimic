import { randomUUID } from "node:crypto";
import { registerJobHandler, scheduleJob } from "../../shared/scheduler/scheduler.js";
import { deliverWebhook } from "../../shared/webhooks/deliver.js";
import { findActiveSubscriptionsForChannel, getMessage, updateMessageStatus } from "./state.js";

const STATUS_DELAY_MS = Number(process.env.ZENVIA_STATUS_DELAY_MS ?? 2000);

const NEXT_STATUS: Record<string, string | undefined> = {
  ACCEPTED: "SENT",
  SENT: "DELIVERED",
};

interface AdvanceJobPayload {
  messageId: string;
}

export function scheduleStatusAdvance(messageId: string): void {
  scheduleJob("zenvia:advance", { messageId } satisfies AdvanceJobPayload, new Date(Date.now() + STATUS_DELAY_MS));
}

async function handleAdvance(payload: unknown): Promise<void> {
  const { messageId } = payload as AdvanceJobPayload;
  const message = getMessage(messageId);
  if (!message) return;

  const nextStatus = NEXT_STATUS[message.status];
  if (!nextStatus) return;

  updateMessageStatus(messageId, nextStatus);

  const subscriptions = findActiveSubscriptionsForChannel(message.channel);
  for (const subscription of subscriptions) {
    if (subscription.criteriaDirection && subscription.criteriaDirection !== "ALL" && subscription.criteriaDirection !== message.direction) {
      continue;
    }
    await deliverWebhook({
      provider: "zenvia",
      resourceType: "message",
      resourceId: message.id,
      url: subscription.webhookUrl,
      headers: subscription.webhookHeaders ?? undefined,
      payload: {
        id: randomUUID(),
        timestamp: new Date().toISOString(),
        subscriptionId: subscription.id,
        type: "MESSAGE_STATUS",
        channel: message.channel,
        message: {
          id: message.id,
          externalId: message.externalId ?? undefined,
          direction: message.direction,
          from: message.from,
          to: message.to,
        },
        messageStatus: {
          code: nextStatus,
          timestamp: new Date().toISOString(),
          channel: message.channel,
          direction: message.direction,
        },
      },
    });
  }

  if (NEXT_STATUS[nextStatus]) {
    scheduleStatusAdvance(messageId);
  }
}

export function registerZenviaScheduler(): void {
  registerJobHandler("zenvia:advance", handleAdvance);
}
