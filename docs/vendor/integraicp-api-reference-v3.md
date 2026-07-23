
<img src="https://developers.integraicp.com.br/images/integraicp-logo.png" height="64px" width="auto"/>

<small>revisado em 2024-05-22</small>

A IntegraICP é uma plataforma que reúne os principais Prestadores de Serviços de Confiança do Brasil, oferecendo APIs simples para a criação de Assinaturas Digitais, assim como para a Identificação de Pessoas e Entidades utilizando Certificados Digitais em Nuvem.

Para utilizar os serviços da IntegraICP, as aplicações (sejam móveis ou baseadas na web) precisam ter acesso a um **Channel**. Esse Channel é equivalente a uma API-Key, e pode ser solicitado através do formulário presente [nesta página](https://validcertificadora.com.br/pages/vidaas-health).
# Assinatura Eletrônica Qualificada

Para realizar a assinatura digital, siga estes passos:

1. **Invocação da API de Autenticação:** Esta etapa resulta em uma lista de Clearances.
2. **Escolha de Clearance:** Selecione uma das opções da lista para que o usuário possa se identificar.
3. **Processo de Autenticação:** Após a autenticação entre o Proponente e o Provedor, o IntegraICP envia um CredentialId para a Aplicação através da URL de Retorno fornecida.
4. **Preparação da Assinatura Eletrônica:** A Aplicação deve preparar sua assinatura eletrônica, calculando o SHA256 dos conteúdos a serem assinados.
5. **Execução do Serviço de Assinatura Eletrônica:** Com o CredentialId e os hashes, a Aplicação pode executar o serviço de assinatura eletrônica.
6. **Consulta de Credenciais:** A qualquer momento, após a autorização do Proponente, a API Credentials pode ser consultada para obter informações sobre o Certificado Digital utilizado.

## Diagrama de Sequência

<img src="https://developers.integraicp.com.br/images/sequence-diagram.png">

---

# Referência da APIs

##  Authentications

Este serviço fornece uma lista de provedores de serviços de confiança, conhecidos como [Clearances](#clearancesresult), 
capazes de autenticar e realizar assinaturas em nome de um Proponente. 
A escolha do provedor é de responsabilidade da Aplicação, e uma vez selecionado, o processo de autenticação e autorização pode ser iniciado.

---

<span class="badge badge--read">GET&nbsp;</span> **/c/<span class="parameter">{channelId}</span>/icp/v3/authentications**

---


### Requisição

A solicitação para o serviço de autenticação é realizada por meio de um simples HTTP GET, no qual os parâmetros são passados através da **query string**. 
Essa abordagem simplifica a integração com diversos modelos de clientes.

#### Parâmentros para Consulta (Query String)

1. **<span class="parameter">subject_key</span> - Identificação do Proponente (Opcional):**
   Indica a identificação única do proponente, geralmente o CPF da pessoa, contendo apenas números e letras, sem traços ou pontos.

2. **<span class="parameter">subject_type</span> - Tipo de Proponente (Opcional):**
   Define o tipo de proponente, atualmente limitado à constante CPF, uma vez que o IntegraICP lida exclusivamente com CPFs.

3. **<span class="parameter">secret_data</span> - Informação de Segurança:**
   Refere-se ao código de segurança gerado pela aplicação, conforme especificado na [RFC 7636 - vide exemplos](#implementacão-rfc-7636). Este código é utilizado como entropia para criptografar e anonimizar as informações enviadas ao IntegraICP.

4. **<span class="parameter">secret_type</span> - Tipo de Segurança (Opcional):**
   Define o tipo de segurança aplicado, atualmente restrito à constante **code_challenge**. O Integra ICP só aceita códigos de segurança relacionados a S256.

5. **<span class="parameter">callback_uri</span> - URI de Retorno para a Aplicação:**
   Indica a URI para a qual o IntegraICP enviará o resultado da autenticação.

6. **<span class="parameter">autostart</span> - Início Automático (Opcional):**
   Ativa o processo de autenticação, selecionando automaticamente a primeira opção da lista de **Clearances** e redirecionando para a URL indicada. Valores **true** ou **false**.

7. **<span class="parameter">credential_lifetime</span> - Tempo de Expiração da Credencial (Opcional):**
   Define o tempo máximo de uso da credencial. Valores em **segundos**, não são permitidos valores maiores que 168h;

8. **<span class="parameter">clearance_lifetime</span> -  Tempo de Expiração da Liberação (Opcional):**
   Define o tempo máximo de ativação da liberação. Valores em **segundos**, não são permitidos valores maiores que 24h;

<span class="attention">

> Todos os provedores registrados no Canal serão consultados em busca do <span class="parameter">subject_key</span> indicado, 
> então o serviço retornará os provedores aos quais o Proponente está registrado. No entanto, quando da omissão deste parâmetro, 
> todos os provedores serão listados como Clearances, sem consulta alguma.

</span>

<span class="attention">

> Quando um proponente tem vários certificados válidos com o provedor, 
> o IntegraICP realiza uma Assinatura de Prospecção durante o processo de autenticação. 
> Essa etapa é crucial para determinar com precisão qual certificado digital está sendo selecionado. 
> A ação ocorre apenas uma vez por sessão ou quando solicitado ao proponente para autenticar novamente, 
> e é registrada no histórico de assinaturas do provedor.

</span>


#### Exemplos

<!-- tabs:start -->

##### **Shell Script**

```bash
CHANNEL="063363c6-e614-4b48-b55c-f5a0ed458d88"
SUBJECT=46404461013
SECRET="E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
CALLBACK="https://my.application.com/receive/credential?option=my-opaque-token"

curl "https://services.integraicp.com.br/c/${CHANNEL}/icp/v3/authentications?subject_key=${SUBJECT}&secret_data=${SECRET}&callback_uri=${CALLBACK}";
```

##### **Browser Javascript**

```javascript
const CHANNEL="063363c6-e614-4b48-b55c-f5a0ed458d88";
const SUBJECT=46404461013;
const SECRET="E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM";
const CALLBACK="https://my.application.com/receive/credential?option=my-opaque-token";

// Open new Window 
window.open(`https://services.integraicp.com.br/c/${CHANNEL}/icp/v3/authentications?subject_key=${SUBJECT}&secret_data=${SECRET}&callback_uri=${CALLBACK}&autostart=true`);
```
<!-- tabs:end -->

### Respostas

<span class="responce--success">

> #### 200 (Provedores Encontrados)
> Retorna um JSON do tipo [ClearancesResult](#ClearancesResult), contendo Identificação do Proponente,
> responsável pela Assinatura, bem como os resultados das assinaturas eletrônicas.
>
> #### Exemplo
>```javascript
> {
>    "data": {
>        
>        "requestId": "01HQVEDSKHQC31V61SMXYZ3WNV",
>
>        "channelName": "Aplicação de Exemplo",
>        "channelDescription": "IntegraICP - Broker ICP Brasil",
>        
>        // as URLs relacionadas as clearances são invalidadas após este instante.
>        "expireTimestamp": "2023-08-24T13:15:22Z",
>
>        "executionStatus": {
>            "currentStatus": "PENDING_AUTHORIZATON",
>            "requestTimestamp": "2023-08-24T14:15:22Z",
>            "executionTimestamp": "2023-08-24T14:15:22Z"
>        },
>        
>        "clearances": [
>            {
>                "clearanceId": "01HQVFV1GF35CSRTTXFPPV0EV2",
>                "productName": "VIDaaS",
>                "providerName": "Valid",
>                "clearanceEndpoint": "https://services.integraicp.com.br/063363c6-e614-4b48-b55c-f5a0ed458d88/icp/v3/clearances/01HQVFV1GF35CSRTTXFPPV0EV2",
>                "clearanceType": "IDENTIFICATION"
>            },
>>            {
>                "clearanceId": "01HQVFVGMCVSTY8F7Z7MWJA8B3",
>                "productName": "BirdID",
>                "providerName": "Soluti",
>                "clearanceEndpoint": "https://services.integraicp.com.br/063363c6-e614-4b48-b55c-f5a0ed458d88/icp/v3/clearances/01HQVFVGMCVSTY8F7Z7MWJA8B3",
>                "clearanceType": "IDENTIFICATION"
>            }
>        ]
>    }
> }
>```

> #### 200 (Não existem provedores)
> Quando não há provedores disponíveis para autenticar o proponente, indicando que possivelmente ele não está registrado em nenhum deles, o campo **currentStatus** é definido como **UNAVAILABLE_CLEARANCES**.
>
> #### Exemplo
>```javascript
> {
>    "data": {
>        
>        "requestId": "01HQVEDJZ3MNKC7EC9W16S0NA9",
>
>        "channelName": "Aplicação de Exemplo",
>        "channelDescription": "IntegraICP - Broker ICP Brasil",
>        
>        "expireTimestamp": "2023-08-24T14:15:22Z",
>
>        "executionStatus": {
>            "currentStatus": "UNAVAILABLE_CLEARANCES",
>            "requestTimestamp": "2023-08-24T14:15:22Z",
>            "executionTimestamp": "2023-08-24T14:15:22Z"
>        },
>        
>        "clearances": []
>    }
> }
>```
>

> #### 302 
> Encaminha para a autenticação do provedor, utilizando o valor do cabeçalho HTTP especificado em Location.
> 

</span>

<span class="responce--failure">

> #### 400
> Problemas com as informações enviadas.
>
> #### Exemplo
> ```javascript
> {
>     "error" : {
>         "code" : 400101,
>         "message" : "Invalid Channel."
>     }
> }
> ```
>

</span>


## Credentials

Detalhes do Proponente autenticado e do Certificado Digital selecionado.

---

<span class="badge badge--read">GET&nbsp;</span> **/c/<span class="parameter">{channelId}</span>/icp/v3/credentials/<span class="parameter">{credentialId}</span>**

---


### Requisição

A solicitação para o serviço de credenciais é realizada por meio de um simples HTTP GET, no qual os parâmetros são passados através da **query string**.
Essa abordagem simplifica a integração com diversos modelos de clientes.

#### Parâmetros de Caminho (URL Path)

1. **<span class="parameter">channelId</span> - Identificação do Canal:**
   Durante o processo de Autenticação e Identificação do proponente, é obtido o  **credentialId**. A Assinatura é realizada utilizando este mesmo código. Para garantir a segurança e a criptografia das informações, é necessário fornecer no campo **secretData** o código de verificação correspondente ao utilizado no processo de autenticação (conforme especificado na [RFC 7636](https://datatracker.ietf.org/doc/html/rfc7636)).

2. **<span class="parameter">credentialId</span> - Identificação da Credentical:**
   Durante o processo de Autenticação e Identificação do proponente, é obtido o  **credentialId**. A Assinatura é realizada utilizando este mesmo código. Para garantir a segurança e a criptografia das informações, é necessário fornecer no campo **secretData** o código de verificação correspondente ao utilizado no processo de autenticação (conforme especificado na [RFC 7636](https://datatracker.ietf.org/doc/html/rfc7636)).


#### Parâmetros de Consulta (Query String)


1. **<span class="parameter">secret_data</span> - Informação de Segurança:**
   Código de **verificação** correspondente ao utilizado no processo de autenticação (conforme especificado na [RFC 7636](#implementacão-rfc-7636)).

2. **<span class="parameter">secret_type</span> - Tipo de Segurança (Opcional):**
   Tipo de segurança, atualmente restrito à constante **code_verifier**. O Integra ICP só aceita códigos de segurança relacionados a S256.


#### Exemplo
```bash
CREDENTIAL="01HR4RC603SRP585PGQ7GVJTQ8"
CHANNEL="063363c6-e614-4b48-b55c-f5a0ed458d88"
SECRET="E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

curl "https://services.integraicp.com.br/c/${CHANNEL}/icp/v3/credentials/${CREDENTIAL}?secret_data=${SECRET}";
```


### Respostas

<span class="responce--success">

> **200**
> Retorna um JSON do tipo [CredentialResult](#credentialresult), contendo Identificação do Proponente,
> responsável pela Assinatura, bem como os resultados das assinaturas eletrônicas.
>
> #### Exemplo
> ```javascript
> {
>    "data": {
>
>        "credentialId": "string",
>
>        "executionStatus": {
>            "currentStatus": "PENDING_SIGNATURES",
>            "requestTimestamp": "2019-08-24T14:15:22Z",
>            "executionTimestamp": "2019-08-24T14:15:22Z"
>        },
>        
>        "subjectIdentification": {
>            "identificationKey": "46404461013",
>            "identificationType": "CPF"
>        },
>
>        "certificateInformation": {
>
>            "serialNumber": "string",
>            "issuerName": "string",
>    
>            "validity": {
>                "notBefore": "1977-01-01T00:00:00Z",
>                "notAfter": "1977-01-01T00:00:00Z"
>            },
>            
>            "subjectName": "string",
>            "encodedX509": "-----BEGIN CERTIFICATE-----\nMIIH2z...\n-----END CERTIFICATE-----\n",
>            "fingerprint256": "8E:00:43:DA:63:ED:EB:DF:D5:F1:75:04:71:76:FA:54:D8:D8:C1:72:E7:0B:3C:0A:F1:C5:38:58:F3:37:55:D0"
>        }
>    }
> }
> ```

</span>

<span class="responce--failure">

> #### 403
> Problemas relacionados ao **secretData** ou credenciais expiradas.
>
> #### Exemplo
> ```javascript
> {
>     "error" : {
>         "code" : 403201,
>         "message" : "Invalid Verification Code (PKCE)."
>     }
> }
> ```
>

> #### 404
> Credencial não pode ser encontrada.
> #### Exemplo
>
> ```javascript
> {
>     "error" : {
>         "code": 404000,
>         "message" : "Credential Not Found"
>     }
> }
> ```
>

</span>

## Signatures

A API de Assinatura Digital oferece funcionalidades para a assinatura eletrônica de múltiplos conteúdos de forma eficiente e segura.

---

<span class="badge badge--update">POST</span> **/c/<span class="parameter">{channelId}</span>/icp/v3/signatures**

---


### Requisição

Para solicitar assinaturas, é utilizado o JSON to tipp [SignaturesRequest](#signaturesrequest).

#### Parâmentros da Requesisição (JSON Request Body)

1. **Identificação da Credencial:**
   Durante o processo de Autenticação e Identificação do proponente, é obtido o  **credentialId**. A Assinatura é realizada utilizando este mesmo código. Para garantir a segurança e a criptografia das informações, é necessário fornecer no campo **secretData** o código de verificação correspondente ao utilizado no processo de autenticação (conforme especificado na [RFC 7636](https://datatracker.ietf.org/doc/html/rfc7636)).

2. **Assinatura Eletrônica de Múltiplos Conteúdos:**
   A aplicação pode efetuar a assinatura eletrônica de múltiplos conteúdos. Para isso, é necessário calcular o Hash SHA256 do conteúdo a ser assinado, e codifica-lo em Base64. Diferente da especificação do **secretData**, esta codificação *não* deve ser Base64 URL Enconded.

3. **Identificação com ID Proprietário (Opcional):**
   Opcionalmente, a aplicação pode fornecer um ID proprietário para identificar de forma mais efetiva a resposta relacionada à assinatura.

4. **Política de Assinatura (Opcional):**
   O campo `signaturePolicy` é opcional e pode ser configurado de acordo com a necessidade. Se omitido, a API utilizará o padrão configurado junto ao Channel. Existem duas políticas de assinatura suportadas:

    - **RAW:**
      O RAW é uma opção onde o Hash é assinado diretamente. O tamanho da assinatura RAW é igual ao tamanho da chave utilizada. Por exemplo, se a chave tem 2048 bits, o resultado será uma assinatura de 2048 bits codificados em Base64.

    - **CMS (Cryptographic Message Syntax):**
      O padrão CMS é um formato binário (ASN.1), especificado pela norma [RFC 5652 - Cryptographic Message Syntax (CMS)](https://datatracker.ietf.org/doc/html/rfc5652). Esta especificação define a estrutura e o formato dos dados utilizados para a criptografia e assinatura digital em sistemas de comunicação segura.

#### Exemplo
```javascript
{
    // Id recebido pela aplicação durante o processo de Autenticação do Proponente.
    "credentialId": "01HQNT0RBF8VFPQ6MAVAG1BWPG",
    
    // Opcional. Deve ser sempre 'code_verifier'    
    "secretType": "code_verifier",

    // código verificador correspondente ao usado com campo 'secretData' durante a AutenticaÇão. RFC 7636.
    "secretData": "4yrDHoTpVMTvMPemeZlIzfCPMOhd5QXiNxVcmycmWqU", 
        
    "requests": [
        {
            "contentId": "doc_001",
            
            // SHA256 do Conteúdo codificado em Base64
            "contentDigest": "4yrDHoTpVMTvMPemeZlIzfCPMOhd5QXiNxVcmycmWqU=",
            "contentDescription": "Documento de teste",
            "signaturePolicy": "RAW"
        },
        {
            "contentId": "image_002",
            "contentDigest": "pSdPO8617lJseJ+o1YQGJESu6fbMB7y61wUhQP4SIUE=",
            "contentDescription": "Imagem de amostra",
            "signaturePolicy": "CMS"
        }
    ]
}
```


### Respostas

<span class="responce--success">

> **200**
> Retorna um JSON do tipo [SignaturesResult](#signaturesresult), contendo Identificação do Proponente,
> responsável pela Assinatura, bem como os resultados das assinaturas eletrônicas.
>
> #### Exemplo
> ```javascript
> {
>    "data": {
>
>        "requestId": "string",
>
>        "executionStatus": {
>            "currentStatus": "COMPLETED_WITH_SUCCESS",
>            "requestTimestamp": "2019-08-24T14:15:22Z",
>            "executionTimestamp": "2019-08-24T14:15:22Z"
>        },
>        
>        "subjectIdentification": {
>            "identificationKey": "46404461013",
>            "identificationType": "CPF"
>        },
>
>        "certificateInformation": {
>
>            "serialNumber": "string",
>            "issuerName": "string",
>    
>            "validity": {
>                "notBefore": "1977-01-01T00:00:00Z",
>                "notAfter": "1977-01-01T00:00:00Z"
>            },
>            
>            "subjectName": "string",
>            "encodedX509": "-----BEGIN CERTIFICATE-----\nMIIH2z...\n-----END CERTIFICATE-----\n",
>            "fingerprint256": "8E:00:43:DA:63:ED:EB:DF:D5:F1:75:04:71:76:FA:54:D8:D8:C1:72:E7:0B:3C:0A:F1:C5:38:58:F3:37:55:D0"
>        },
>
>        "signatures": [
>            {
>                "signatureId": "string", 
>                "contentId": "string",
>                "contentDigest": "4yrDHoTpVMTvMPemeZlIzfCPMOhd5QXiNxVcmycmWqU",
>                "contentDescription": "string",
>                "signedContent": "Z8NskMcJaPVcFEmj4HyK0hX7rSqGyNtAehisjncJN3tTttv0nVVYppmjzG6lyPU0YTJLIXjUWbqnvi0a/MQvB47p0n/nd2sTATANNg+hzDDdMa5BmTcT46JUfnYw3k+dlifYRv6ALgVsVzhgrN831sJ9awEdZPUqpD8HLF8bEI7qTtA06+hOQxS1Hi0j09Jxfmfszb+nITSJQkxTJshImnrXRj16bb8eB1+9wvMohMyPyooiWoJvTWhKcMzjMZFP7zsBZCuyT/LXOgUH30IQ8GOvxFxQQrM7Qjfw0HXLz/RS5PXIhUlsKjr2iMCov8HnPBKbS+XXSYKyWpncOVs+Sw==",
>                "signatureTimestamp": "1977-01-01T00:00:00Z"
>            }
>        ]
>    }
> }
> ```

</span>

<span class="responce--failure">

> #### 400
> Problemas com a estrutura ou informações enviadas.
>
> #### Exemplo
> ```javascript
> {
>     "error" : {
>         "code" : 400204,
>         "message" : "Ivalid Content Digest. Not SHA256 Base64 Encoded."
>     }
> }
> ```
>
 
> #### 403
> Problemas relacionados ao **secretData** ou credenciais expiradas.
>
> #### Exemplo
> ```javascript
> {
>     "error" : {
>         "code" : 403201,
>         "message" : "Invalid Verification Code (PKCE)."
>     }
> }
> ```
>

> #### 404
> Credencial não pode ser encontrada.
> #### Exemplo
>
> ```javascript
> {
>     "error" : {
>         "code": 404000,
>         "message" : "Credential Not Found"
>     }
> }
> ```
>

</span>


---

# Estrutura de Entidades

## ClearancesResult

```javascript
{
    "data": {
        
        "requestId": "string",
        "channelName": "string",
        "channelDescription": "string",
        
        "expireTimestamp": "string",

        "executionStatus": {
            "currentStatus": "PENDING_AUTHORIZATON",
            "requestTimestamp": "2019-08-24T14:15:22Z",
            "executionTimestamp": "2019-08-24T14:15:22Z"
        },
        
        "clearances": [
            {
                "clearanceId": "string",
                "productName": "string",
                "providerName": "string",
                "clearanceEndpoint": "string",
                "clearanceType": "string"
            }
        ]
    }
}
```

## CredentialResult

```javascript
{
  "data": {
      
    "credentialId": "string",
            
    "executionStatus": {
      "currentStatus": "PENDING_SIGNATURE",
      "requestTimestamp": "2019-08-24T14:15:22Z",
      "executionTimestamp": "2019-08-24T14:15:22Z"
    },
        
    "subjectIdentification": {
      "identificationKey": "string",
      "identificationType": "CPF"
    },

    "certificateInformation": {
        
        "serialNumber": "string",
        "issuerName": "string",

        "validity": {
            "notBefore": "1977-01-01T00:00:00Z",
            "notAfter": "1977-01-01T00:00:00Z"
        },

        "subjectName": "string",
        "encodedX509": "string",
        "fingerprint256": "string"
    }    
  }
}
```

## SignaturesRequest
<!--
### Atributos 
| Atributo                     | Tipo          | Opcional                          | Descrição                                       |
|------------------------------|---------------|-----------------------------------|-------------------------------------------------|
| credentialId                 | string        | Não                               | Id da Credencial.                               |
| secretType                   | string        | Sim                               | Por padrão **code_verifier**.                   |
| secretData                   | string        | Não                               | SHA256 do conteúdo que deve ser assinado        |
| requests[i]                  | array[object] | Não                               | Lista contentod as solicitações para assinatura |
| requests[i].contentId        | string        | Sim                               | Identificador do Conteúdo a ser Assinado.       |
| requests[i].contentDigest    | string        | Não                               | SHA256 do conteúdo que deve ser assinado        |
| requests[i].contenDescrition | string        | Não                               | Breve descrição da Assinatura                   |
| requests[i].signaturePolicy  | string        | Sim | Tipo de Assinatura RAW ou CMS                   |
-->

```javascript
{
  "credentialId": "string", 
  "secretType": "code_verifier",
  "secretData": "string",
  "requests": [
    {
      "contentId": "string",
      "contentDigest": "string",
      "contentDescription": "string",
      "signaturePolicy": "RAW"
    }
  ]
}
```

## SignaturesResult
```javascript
{
    "data": {

        "requestId": "string",

        "executionStatus": {
            "currentStatus": "PENDING_AUTHORIZATON",
            "requestTimestamp": "2019-08-24T14:15:22Z",
            "executionTimestamp": "2019-08-24T14:15:22Z"
        },
        
        "subjectIdentification": {
            "identificationKey": "string",
            "identificationType": "CPF"
        },

        "certificateInformation": {
            
            "serialNumber": "string",
            "issuerName": "string",
    
            "validity": {
                "notBefore": "1977-01-01T00:00:00Z",
                "notAfter": "1977-01-01T00:00:00Z"
            },
            
            "subjectName": "string",
            "encodedX509": "string"
            "fingerprint256": "string"
        },

        "signatures": [
            {
                "signatureId": "string", 
                "contentId": "string",
                "contentDigest": "string",
                "contentDescription": "string",
                "signedContent": "string",
                "signatureTimestamp": "1977-01-01T00:00:00Z"
            }
        ]
    }
}
```


# Implementacão RFC 7636

A [RFC 7636](https://datatracker.ietf.org/doc/html/rfc7636), também conhecida como "Proof Key for Code Exchange by OAuth Public Clients" (PKCE), é uma especificação do Internet Engineering Task Force (IETF) que aborda um método de segurança crucial para aplicativos OAuth chamados "clientes públicos". Essa RFC introduz um mecanismo de segurança destinado a mitigar possíveis ataques de interceptação de código de autorização. Ela desempenha um papel essencial na proteção das comunicações entre clientes e servidores OAuth, garantindo a integridade e confidencialidade dos tokens de acesso.


Implementações de referência da RFC 7636.

### Exemplos

<!-- tabs:start -->

##### **NodeJS**

```javascript
// How to Generate secret_data With NodeJS
const crypto = require('crypto');
const base64url = require('base64url');

function generateSecretData() {
   const characters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-';
   let secretDataForSignautresAndCredentials = '';

   // Generate a random string with the given length - 43
   for (let i = 0; i < 43; i++) {
      let randomIndex = Math.floor(Math.random() * characters.length);
      secretDataForSignautresAndCredentials += characters.charAt(randomIndex);
   }

   // Generate a code challenge according to the PKCE spec
   const secretDataForAuthentications = base64url.encode( crypto.createHash('sha256')
           .update( secretDataForSignautresAndCredentials ).digest() );

   return { secretDataForAuthentications, secretDataForSignautresAndCredentials } ;
   
}

// Usage example
const { secretDataForAuthentications, secretDataForSignautresAndCredentials } = generateSecretData();
console.log('SecretData For Athentications:', secretDataForAuthentications);
console.log('SecretData For Signatures and Credentials:', secretDataForSignautresAndCredentials);
```

##### **Java**

```java
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.security.SecureRandom;
import java.util.Base64;

public class PKCEGenerator {

    public static String generateCodeVerifier() {
        byte[] bytes = new byte[43]; // Define o tamanho do code verifier
        SecureRandom secureRandom = new SecureRandom();
        secureRandom.nextBytes(bytes);
        return base64UrlEncode(bytes);
    }

    public static String generateCodeChallenge(String codeVerifier) throws NoSuchAlgorithmException {
        byte[] bytes = codeVerifier.getBytes();
        MessageDigest messageDigest = MessageDigest.getInstance("SHA-256");
        messageDigest.update(bytes);
        byte[] digest = messageDigest.digest();
        return base64UrlEncode(digest);
    }

    private static String base64UrlEncode(byte[] bytes) {
        return Base64.getUrlEncoder().withoutPadding().encodeToString(bytes);
    }

    public static void main(String[] args) {
        try {
            // Gerar um code verifier
            String codeVerifier = generateCodeVerifier();
            System.out.println("SecretData For Signatures and Credentials : " + codeVerifier);

            // Gerar um code challenge
            String codeChallenge = generateCodeChallenge(codeVerifier);
            System.out.println("SecretData For Athentications: " + codeChallenge);
        } catch (NoSuchAlgorithmException e) {
            e.printStackTrace();
        }
    }
}
```

<!-- tabs:end -->
