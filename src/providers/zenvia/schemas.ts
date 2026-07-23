import { z } from "zod";

// Contract taken from docs/vendor/zenvia-openapi-v2.json:
// components.schemas["content.sms.text"], ["content.template"], ["message.sms"], ["message.base"]
const textContentSchema = z.object({
  type: z.literal("text"),
  text: z.string(),
  encodingStrategy: z.enum(["AUTO", "MORE_CHARACTER_SUPPORT", "MORE_CHARACTERS_PER_MESSAGE"]).optional(),
  reportId: z.number().optional(),
});

const templateContentSchema = z.object({
  type: z.literal("template"),
  templateId: z.string(),
  fields: z.record(z.string()).optional(),
});

export const smsContentSchema = z.discriminatedUnion("type", [textContentSchema, templateContentSchema]);

export const createSmsMessageSchema = z.object({
  externalId: z.string().optional(),
  from: z.string().min(1).max(64),
  to: z.string().min(1).max(64),
  contents: z.array(smsContentSchema).min(1),
});

export type CreateSmsMessageInput = z.infer<typeof createSmsMessageSchema>;
