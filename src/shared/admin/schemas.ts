import { z } from "zod";

export const createFaultSchema = z.object({
  provider: z.enum(["zenvia", "integraicp"]),
  routePattern: z.string().optional(),
  faultKind: z.enum(["delay_ms", "http_status", "timeout", "invalid_payload", "webhook_dropped", "webhook_invalid"]),
  faultValue: z.string().optional(),
  times: z.number().int().positive().optional(),
});

export type CreateFaultInput = z.infer<typeof createFaultSchema>;
