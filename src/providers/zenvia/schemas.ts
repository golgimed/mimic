import { z } from "zod";

// Contract taken from docs/vendor/zenvia-openapi-v2.json:
// components.schemas["message.base"], ["content.base"], ["content.text"], ["content.template"],
// ["content.file"], ["content.email"]
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

const fileContentSchema = z.object({
  type: z.literal("file"),
  fileUrl: z.string().url(),
  fileName: z.string().optional(),
  fileCaption: z.string().optional(),
});

const emailContentSchema = z.object({
  type: z.literal("email"),
  subject: z.string().min(1),
  html: z.string().max(32768).optional(),
  attachments: z
    .array(z.object({ fileUrl: z.string().url(), fileName: z.string().optional() }))
    .optional(),
});

const baseMessageFields = {
  externalId: z.string().optional(),
  from: z.string().min(1).max(64),
  to: z.string().min(1).max(64),
};

// SMS: components.schemas["message.sms"], content is text | template.
export const smsContentSchema = z.discriminatedUnion("type", [textContentSchema, templateContentSchema]);
export const createSmsMessageSchema = z.object({
  ...baseMessageFields,
  contents: z.array(smsContentSchema).min(1),
});
export type CreateSmsMessageInput = z.infer<typeof createSmsMessageSchema>;

// WhatsApp: components.schemas["message.whatsapp"], content restricted here to
// text | template | file (the common cases) - the real API also supports
// buttons/lists/products/flows/location/contacts, out of scope for now.
export const whatsappContentSchema = z.discriminatedUnion("type", [
  textContentSchema,
  templateContentSchema,
  fileContentSchema,
]);
export const createWhatsappMessageSchema = z.object({
  ...baseMessageFields,
  contents: z.array(whatsappContentSchema).min(1),
  idRef: z.string().optional(),
  contentRef: z.number().optional(),
});
export type CreateWhatsappMessageInput = z.infer<typeof createWhatsappMessageSchema>;

// Email: components.schemas["message.email"], content is email | template.
export const emailMessageContentSchema = z.discriminatedUnion("type", [
  emailContentSchema,
  templateContentSchema,
]);
export const createEmailMessageSchema = z.object({
  ...baseMessageFields,
  contents: z.array(emailMessageContentSchema).min(1),
  representative: z.object({ type: z.enum(["BOT", "HUMAN"]).optional(), name: z.string().optional() }).optional(),
});
export type CreateEmailMessageInput = z.infer<typeof createEmailMessageSchema>;

export type CreateMessageInput = { externalId?: string; from: string; to: string; contents: unknown[] };

// Contract taken from docs/vendor/zenvia-openapi-v2.json:
// components.schemas["subscription.base"], ["subscription.partial-status.message-status-subscription"]
export const createSubscriptionSchema = z.object({
  eventType: z.literal("MESSAGE_STATUS"),
  webhook: z.object({
    url: z.string().url(),
    headers: z.record(z.string()).optional(),
  }),
  criteria: z.object({
    channel: z.string(),
    direction: z.enum(["IN", "OUT", "ALL"]).optional(),
  }),
});

export type CreateSubscriptionInput = z.infer<typeof createSubscriptionSchema>;
