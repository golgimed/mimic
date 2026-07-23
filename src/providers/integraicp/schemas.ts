import { z } from "zod";

// Contract taken from docs/vendor/integraicp-api-reference-v3.md
export const authenticationsQuerySchema = z.object({
  subject_key: z.string().optional(),
  subject_type: z.string().optional(),
  secret_data: z.string(),
  secret_type: z.string().optional(),
  callback_uri: z.string().url(),
  autostart: z
    .enum(["true", "false"])
    .optional()
    .transform((v) => v === "true"),
  credential_lifetime: z.coerce.number().optional(),
  clearance_lifetime: z.coerce.number().optional(),
});

export const credentialsQuerySchema = z.object({
  secret_data: z.string(),
  secret_type: z.string().optional(),
});

export const credentialsParamsSchema = z.object({
  channelId: z.string(),
  credentialId: z.string(),
});

const signatureRequestItemSchema = z.object({
  contentId: z.string().optional(),
  contentDigest: z.string(),
  contentDescription: z.string().optional(),
  signaturePolicy: z.enum(["RAW", "CMS"]).optional(),
});

export const signaturesBodySchema = z.object({
  credentialId: z.string(),
  secretType: z.string().optional(),
  secretData: z.string(),
  requests: z.array(signatureRequestItemSchema).min(1),
});

export type AuthenticationsQuery = z.infer<typeof authenticationsQuerySchema>;
export type CredentialsQuery = z.infer<typeof credentialsQuerySchema>;
export type SignaturesBody = z.infer<typeof signaturesBodySchema>;
