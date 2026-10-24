# BRy Medical (KMS + HUB Signer)

Simulates the two BRy calls golgimed's BRy Medical adapter makes to sign
medical PDFs with a cloud (KMS) key:

1. **KMS pré-autorização**: `POST /bry-kms/chaves/{uuidChave}/autorizacoes`
   issues a token good for `quantidade` uses that expires after `expiracao`
   seconds. It requires the `kms_credencial` and `kms_credencial_tipo`
   headers.
2. **HUB Signer medical PDF signing**: `POST /bry-medical/fw/v1/pdf/kms/lote/assinaturas`
   takes multipart `documento[0]`, `dados_assinatura` (carrying the token) and
   `metadados`, and requires `kms_type: BRYKMS`. Each call spends one token use
   and returns a deterministic signed-document descriptor.

Both routes require `Authorization: Bearer <token>`. As with the other
providers, only its presence is checked.

## Compatibility

Format: [docs/compatibility.md](../../../docs/compatibility.md).

**Contract**: Partial. Only the two operations above are covered, in the shapes golgimed's adapter builds.
**Behavior**: Partial. Token use counts and expiry are enforced; signing is simulated.

- **Request validation:**
  - Bearer token present.
  - `kms_type: BRYKMS`.
  - Required KMS credential headers.
  - Multipart fields present and non-empty.
  - `expiracao` must be a positive number of seconds.
  - A missing or non-positive `quantidade` defaults to 1.
- **Responses & errors:**
  - Errors use the documented KMS (`codigoErro`/`descricao`) and HUB (`status`/`message`/`chave`) error schemas. Only the HTTP status matters to the adapter.
  - Unknown, expired and spent tokens all return the same 401.
- **Lifecycle & async:** a token lives until its uses run out or `expiracao` passes. Expiry follows the simulation clock, so a virtual clock can expire tokens instantly.
- **Webhooks:** none.
- **Fault injection:** provider `bry-medical`, using one of the two route paths.
- **Known differences:**
  - No PDF is actually signed. The response carries the input's SHA-256 and a `https://mimic.local/...` link that can't be fetched.
  - Identifiers are random UUIDs.
- **Unsupported:**
  - Credential variants other than `BRYKMS`.
  - Batches of more than one document.
  - Signature formats and policies.
