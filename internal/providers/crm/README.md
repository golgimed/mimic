# CRM (CFM Consulta Médicos)

Simulates the CFM **Web Service Consulta Médicos** (manual "WS CFM — Documento de Especificação de Integração",
v3.0, https://sistemas.cfm.org.br/listamedicos/arquivos/manualwebservices.pdf, plus the published WSDL). It is
**SOAP 1.1 only** — there is no REST API. Dev-only.

## Endpoint

`POST /crm/WebServiceConsultaMedicos/ServicoConsultaMedicos` (the real service is
`https://ws.cfm.org.br:8080/WebServiceConsultaMedicos/ServicoConsultaMedicos`; Mimic prefixes `/crm`),
`Content-Type: text/xml; charset=utf-8`, namespace `http://servico.cfm.org.br/`.

`GET /crm/WebServiceConsultaMedicos/ServicoConsultaMedicos?wsdl` serves the official WSDL verbatim
(`servico.wsdl`, fetched from CFM on 2026-10-04). Its `soap:address` still points at CFM — override the endpoint
in generated clients.

### Operation `Consultar`

Request: `<ser:Consultar><crm>98765</crm><uf>SP</uf><chave>mimiccrm</chave></ser:Consultar>`.
Response: `ConsultarResponse/dadosMedico` with `crm` (display form), `dataAtualizacao`, `especialidade` (repeated),
`nome`, `situacao`, `tipoInscricao`, `uf` (empty fields omitted). Errors are **HTTP 200** with only
`<codigoErro>` — the service never emits SOAP Faults. Checks run in the documented order:

| Code | Condition |
|------|-----------|
| 4010 | `crm` missing, non-numeric or 0 |
| 4000 | `uf` missing |
| 4020 | `chave` missing |
| 3010 | `chave` invalid |
| 2010 / 2030 / 2040 | transient — Mimic-only trigger: SP CRMs `999201` / `999203` / `999204` |
| 8101 | médico not found (UF is case-sensitive: `sp` → 8101) |

### Operation `Validar`

Request (element names from the WSDL): `<ser:Validar><crm/><uf/><cpf/><dataNascimento/><chave/></ser:Validar>`
(`cpf` unmasked, `dataNascimento` `DD/MM/YYYY`). Response: `ValidarResponse/resultadoConsulta` `true|false`,
HTTP 200. **Never** returns `codigoErro`: missing parameter, bad key, unknown médico or CPF/birth-date mismatch
are all `false` (manual §4.1). A fixture without CPF/birth date always validates `false`.

Other WSDL operations (`ConsultaCompleta`, `buscarImagem`, `init`, …) need a separate CFM contract and are
undocumented: they answer 400. SOAP 1.2 (`application/soap+xml`) answers 415.

## Fixtures

All data is fictitious (manual-style). Always present:

| UF:CRM | Nome | situacao | tipo | `<crm>` returned | Notes |
|--------|------|----------|------|------------------|-------|
| SP:98765 | Ana Costa | A | P | 98765 | Validar: CPF `52998224725`, `15/05/1985` |
| SP:123456 | João Exemplo da Silva | A | P | 123456 | 2 specialties (área de atuação, atuação exclusiva); Validar: `11144477735`, `20/02/1970` |
| RJ:1234 | Beatriz Exemplo Rocha | A | S | 1234 | secondary registration |
| SP:1234567 | Diego Exemplo Prates | A | V | 1234567-P | provisional |
| SP:7654321 | Elisa Exemplo Moura | A | E | 300-EMFE-7654321 | foreign-trained student |
| SP:200001…200006 | — | C, S, F, T, J, P | P | same | cassado, suspenso total, falecido, transferido, suspenso judicial parcial, aposentado |

Situação/tipo codes follow manual §5 (A = Regular; anything else is not regular).

## Configuration

- `MIMIC_CRM_DOCTORS` — extra physicians, comma-separated, added on top of the fixtures (same `UF:CRM` replaces
  one): `UF:CRM:Nome[:situacao[:tipoInscricao[:crmExibicao[:cpf[:dataNascimento[:esp|esp…]]]]]]`. Defaults: `A`, `P`,
  `crmExibicao` = CRM. Specialties go last (they may contain `:`), separated by `|`; no commas.
- `MIMIC_CRM_ACCESS_KEY` — accepted `chave` (default `mimiccrm`).

Fault injection: provider `crm`, route `/crm/WebServiceConsultaMedicos/ServicoConsultaMedicos` (`http_status`,
`timeout`, `delay_ms`, `invalid_payload` for an unparseable body).

## Known gaps / assumptions

- `dataAtualizacao` is fixed at `01/01/2026`.
- `Validar` returns `true` for any registered médico whose CPF and birth date match, regardless of `situacao`
  (the manual: "true = cadastro válido (médico encontrado)").
- Codes 4030/4040 are cited in manual §3.4 but never defined: not emitted.
- The manual gives no HTTP status for SOAP 1.2 rejection or malformed requests: Mimic uses 415 / 400.
- The real service accepts TLS 1.2 only; Mimic serves plain HTTP.
- The TOTAL.ZIP batch file (separate spec) is not simulated.
