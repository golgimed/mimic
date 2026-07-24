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
