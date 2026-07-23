import type { FastifyReply, FastifyRequest } from "fastify";

/**
 * Trivial API-key check: both providers require a token header on real
 * requests. The simulator only checks presence, not a real credential,
 * since it isn't reproducing provider auth/authorization.
 */
export async function requireApiToken(headerName: string) {
  return async (req: FastifyRequest, reply: FastifyReply) => {
    const token = req.headers[headerName.toLowerCase()];
    if (!token) {
      return reply.code(401).send({
        code: "UNAUTHORIZED",
        message: `Missing ${headerName} header`,
      });
    }
  };
}
