# CRM (CFM Consulta Médicos)

Simulates the CFM **Web Service Consulta Médicos** (manual "WS CFM — Documento de Especificação de Integração",
v3.0, https://sistemas.cfm.org.br/listamedicos/arquivos/manualwebservices.pdf). It is **SOAP 1.1 only** —
there is no REST API. Dev-only.

## Endpoint

`POST /crm/WebServiceConsultaMedicos/ServicoConsultaMedicos` (the real service is
`https://ws.cfm.org.br:8080/WebServiceConsultaMedicos/ServicoConsultaMedicos`; Mimic prefixes `/crm`),
`Content-Type: text/xml; charset=utf-8`, namespace `http://servico.cfm.org.br/`.

### Operation `Consultar`

Request: `<ser:Consultar><crm>98765</crm><uf>SP</uf><chave>mimiccrm</chave></ser:Consultar>`.
Response: `ConsultarResponse/dadosMedico` with `crm`, `dataAtualizacao`, `nome`, `situacao`, `tipoInscricao`,
`uf` (empty fields omitted). Errors are **HTTP 200** with only `<codigoErro>` — the service never emits SOAP
Faults. Checks run in the documented order:

| Code | Condition |
|------|-----------|
| 4010 | `crm` missing, non-numeric or 0 |
| 4000 | `uf` missing |
| 4020 | `chave` missing |
| 3010 | `chave` invalid |
| 8101 | médico not found (UF is case-sensitive: `sp` → 8101) |

SOAP 1.2 (`application/soap+xml`) requests are rejected.

## Configuration

- `MIMIC_CRM_DOCTORS` — registered physicians, `UF:CRM:Nome` comma-separated (default `SP:98765:Ana Costa`,
  the dev seed persona). Unlisted → 8101. No real physician data.
- `MIMIC_CRM_ACCESS_KEY` — accepted `chave` (default `mimiccrm`).

Fault injection: provider `crm`, route `/crm/WebServiceConsultaMedicos/ServicoConsultaMedicos`.

## Known gaps / assumptions

- **`Validar` is not implemented**: the manual specifies its parameters conceptually but not its request
  element names, so implementing it would mean inventing the contract. It answers 400.
- Fixed values: `situacao` `A` (Regular), `tipoInscricao` `P` (Principal), `dataAtualizacao` `01/01/2026`, no
  `especialidade`.
- The manual does not give HTTP statuses for SOAP 1.2 rejection or malformed requests: Mimic uses 415 / 400.
- The `?wsdl` document is not served.
