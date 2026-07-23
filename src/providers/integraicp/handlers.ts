import { createHash, randomUUID } from "node:crypto";
import type { FastifyReply, FastifyRequest } from "fastify";
import { authenticationsQuerySchema, credentialsQuerySchema, signaturesBodySchema } from "./schemas.js";
import { buildFakeCertificate, verifyPkce } from "./crypto.js";
import { createCredential, getCredential } from "./state.js";

function sendError(reply: FastifyReply, status: number, code: number, message: string) {
  return reply.code(status).send({ error: { code, message } });
}

function nowIso() {
  return new Date().toISOString();
}

export async function authenticationsHandler(
  req: FastifyRequest<{ Params: { channelId: string } }>,
  reply: FastifyReply,
) {
  const parsed = authenticationsQuerySchema.safeParse(req.query);
  if (!parsed.success) {
    return sendError(reply, 400, 400101, "Invalid Channel.");
  }
  const { channelId } = req.params;
  const query = parsed.data;
  const subjectName = query.subject_key ?? "Simulated Subject";

  if (query.autostart) {
    const credential = createCredential({
      channelId,
      subjectKey: query.subject_key,
      subjectType: query.subject_type,
      codeChallenge: query.secret_data,
      callbackUri: query.callback_uri,
      certificate: buildFakeCertificate(subjectName),
    });

    const location = new URL(query.callback_uri);
    location.searchParams.set("credentialId", credential.id);
    return reply.code(302).header("Location", location.toString()).send();
  }

  const clearanceId = randomUUID();
  return reply.code(200).send({
    data: {
      requestId: randomUUID(),
      channelName: "Mimic",
      channelDescription: "IntegraICP - Simulated Broker",
      expireTimestamp: new Date(Date.now() + (query.clearance_lifetime ?? 86400) * 1000).toISOString(),
      executionStatus: {
        currentStatus: "PENDING_AUTHORIZATON",
        requestTimestamp: nowIso(),
        executionTimestamp: nowIso(),
      },
      clearances: [
        {
          clearanceId,
          productName: "Simulated Provider",
          providerName: "Simulator",
          clearanceEndpoint: `https://simulated-provider.local/clearance/${clearanceId}`,
          clearanceType: "IDENTIFICATION",
        },
      ],
    },
  });
}

export async function credentialsHandler(
  req: FastifyRequest<{ Params: { channelId: string; credentialId: string } }>,
  reply: FastifyReply,
) {
  const parsed = credentialsQuerySchema.safeParse(req.query);
  if (!parsed.success) {
    return sendError(reply, 400, 400101, "Invalid request.");
  }

  const credential = getCredential(req.params.credentialId);
  if (!credential) {
    return sendError(reply, 404, 404000, "Credential Not Found");
  }

  if (!verifyPkce(parsed.data.secret_data, credential.codeChallenge)) {
    return sendError(reply, 403, 403201, "Invalid Verification Code (PKCE).");
  }

  return reply.code(200).send({
    data: {
      credentialId: credential.id,
      executionStatus: {
        currentStatus: "PENDING_SIGNATURES",
        requestTimestamp: credential.createdAt,
        executionTimestamp: nowIso(),
      },
      subjectIdentification: {
        identificationKey: credential.subjectKey ?? "00000000000",
        identificationType: credential.subjectType ?? "CPF",
      },
      certificateInformation: credential.certificate,
    },
  });
}

function isValidSha256Base64(value: string): boolean {
  try {
    const buf = Buffer.from(value, "base64");
    return buf.length === 32 && buf.toString("base64") === value;
  } catch {
    return false;
  }
}

export async function signaturesHandler(
  req: FastifyRequest<{ Params: { channelId: string } }>,
  reply: FastifyReply,
) {
  const parsed = signaturesBodySchema.safeParse(req.body);
  if (!parsed.success) {
    return sendError(reply, 400, 400000, "Invalid request body.");
  }
  const body = parsed.data;

  const credential = getCredential(body.credentialId);
  if (!credential) {
    return sendError(reply, 404, 404000, "Credential Not Found");
  }

  if (!verifyPkce(body.secretData, credential.codeChallenge)) {
    return sendError(reply, 403, 403201, "Invalid Verification Code (PKCE).");
  }

  for (const item of body.requests) {
    if (!isValidSha256Base64(item.contentDigest)) {
      return sendError(reply, 400, 400204, "Invalid Content Digest. Not SHA256 Base64 Encoded.");
    }
  }

  const signatures = body.requests.map((item) => {
    const signatureId = randomUUID();
    const signedContent = createHash("sha256")
      .update(`${credential.id}:${item.contentDigest}:${signatureId}`)
      .digest("base64");
    return {
      signatureId,
      contentId: item.contentId,
      contentDigest: item.contentDigest,
      contentDescription: item.contentDescription,
      signedContent,
      signatureTimestamp: nowIso(),
    };
  });

  return reply.code(200).send({
    data: {
      requestId: randomUUID(),
      executionStatus: {
        currentStatus: "COMPLETED_WITH_SUCCESS",
        requestTimestamp: nowIso(),
        executionTimestamp: nowIso(),
      },
      subjectIdentification: {
        identificationKey: credential.subjectKey ?? "00000000000",
        identificationType: credential.subjectType ?? "CPF",
      },
      certificateInformation: credential.certificate,
      signatures,
    },
  });
}
