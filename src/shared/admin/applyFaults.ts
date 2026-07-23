import type { FastifyReply, FastifyRequest } from "fastify";
import { consumeMatchingFault } from "./faults.js";
import { simulateLatency } from "../faults/delay.js";

const NEVER_RESOLVES = new Promise<never>(() => {});

/**
 * preHandler hook: checks for a configured fault matching this provider +
 * route before the real handler runs. Only request-time fault kinds are
 * handled here (delay_ms, http_status, timeout, invalid_payload) -
 * webhook_dropped/webhook_invalid are applied at webhook delivery time.
 */
export function requestFaultHook(provider: string) {
  return async (req: FastifyRequest, reply: FastifyReply) => {
    const routePattern = req.routeOptions?.url ?? req.url.split("?")[0];
    const fault = consumeMatchingFault(provider, routePattern);
    if (!fault) return;

    switch (fault.faultKind) {
      case "delay_ms":
        await simulateLatency(Number(fault.faultValue ?? 0));
        return;
      case "http_status": {
        const status = Number(fault.faultValue ?? 500);
        return reply.code(status).send({
          error: { code: status, message: `Simulated ${status} via fault injection` },
        });
      }
      case "invalid_payload":
        reply.type("application/json");
        return reply.send('{"truncated": tr');
      case "timeout":
        return NEVER_RESOLVES;
      default:
        return;
    }
  };
}
