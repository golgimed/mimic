# IntegraICP

Simulates [IntegraICP](https://developers.integraicp.com.br/api-reference/icp/v3/index.html) — Brazilian digital signature / ICP-Brasil certificate flows.

Routes are mounted under `/integraicp`.

## Digital signature flow

Three steps — see the [IntegraICP API reference](https://developers.integraicp.com.br/api-reference/icp/v3/index.html) for the full picture (PKCE, clearances, certificates):

```bash
# 1. Authenticate (autostart=true skips the real human-in-the-loop provider login
#    and immediately redirects to callback_uri with a credentialId)
curl -i "http://localhost:3000/integraicp/c/my-channel/icp/v3/authentications?secret_data=<code_challenge>&callback_uri=https://your-app/callback&autostart=true"

# 2. Fetch the credential (secret_data here is the code_verifier for the challenge above)
curl "http://localhost:3000/integraicp/c/my-channel/icp/v3/credentials/<credentialId>?secret_data=<code_verifier>"

# 3. Sign content (contentDigest is base64(sha256(content)))
curl http://localhost:3000/integraicp/c/my-channel/icp/v3/signatures \
  -H "Content-Type: application/json" \
  -d '{"credentialId":"<credentialId>","secretData":"<code_verifier>","requests":[{"contentDigest":"<base64-sha256>"}]}'
```

## Known limitations

- IntegraICP's real auth requires a human choosing a provider and logging in; the simulator auto-authenticates instead. The non-autostart "list clearances" response always returns one fake entry.

## State model

Unlike Zenvia (which persists a `status` column and advances it asynchronously via the scheduler — `ACCEPTED → SENT → DELIVERED`), IntegraICP's `executionStatus.currentStatus` values (`PENDING_AUTHORIZATON`, `PENDING_SIGNATURES`, `COMPLETED_WITH_SUCCESS`, etc.) are computed directly in each handler and never persisted. This is intentional: per the official API reference, authentication, credential-fetch, and signing are synchronous request/response operations, not a polled or webhook-driven pipeline — there is no real "state" to track between calls. `integraicp_credentials` therefore has no `status` column by design. If a future IntegraICP endpoint is genuinely asynchronous per the official docs, add a `status` column and drive it the same way Zenvia does, rather than hardcoding another literal.
