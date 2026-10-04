# CRM

Dev-only simulation of the CFM physician-search endpoint used by golgimed's platform-services CRM
verification adapter (`HTTPCRMClient.QueryLicense`). Routes are mounted under `/crm`.

**Assumption:** the real CFM endpoint is undocumented. This mimics only what the client consumes —
`GET {CRM_BASE_URL}/busca-medico-by-name/{licenseNumber}`; HTTP 200 means the license is active, any
other status is treated as unavailable, and the body is ignored. No real physician data is served.

## Endpoint

`GET /crm/busca-medico-by-name/{license}`

| License | Response |
|---------|----------|
| in allow-list | `200 {}` |
| anything else | `404 {"error":"license not found"}` |

The license is matched verbatim (the client does not URL-escape it, so a space arrives as `%20`):
`/crm/busca-medico-by-name/CRM-SP%2098765`.

## Configuration

`MIMIC_CRM_ALLOWED_LICENSES` — comma-separated allow-list. Default: `CRM-SP 98765` (dev seed persona
Ana Costa, `db/seeds/dev/008`).

Point the consumer at `CRM_BASE_URL=http://mimic:3000/crm`. Faults (`PUT /admin/faults`, provider `crm`,
route `/crm/busca-medico-by-name/{license}`) work as for other providers.
