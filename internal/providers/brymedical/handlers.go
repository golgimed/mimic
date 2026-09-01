package brymedical

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/golgimed/mimic/internal/shared/httpx"
)

// writeKMSError matches the KMS host's documented Erro schema
// (codigoErro/descricao/atributos/ctxId). golgimed's adapter never parses
// this body — only the HTTP status drives its error path — so fidelity here
// is for readability/debugging, not contract correctness.
func writeKMSError(w http.ResponseWriter, status int, codigoErro, descricao string) {
	httpx.WriteJSON(w, status, map[string]any{
		"codigoErro": codigoErro,
		"descricao":  descricao,
		"atributos":  map[string]any{},
		"ctxId":      uuid.NewString(),
	})
}

// writeHubError matches the HUB Signer's documented Erro schema
// (status/message/chave/timestamp/traceId). Same caveat as writeKMSError:
// cosmetic only, since the adapter checks status codes exclusively.
func writeHubError(w http.ResponseWriter, status int, message, chave string) {
	httpx.WriteJSON(w, status, map[string]any{
		"status":    status,
		"message":   message,
		"chave":     chave,
		"timestamp": time.Now().UnixMilli(),
		"traceId":   uuid.NewString(),
	})
}

type autorizacaoRequest struct {
	Aplicacao  string `json:"aplicacao"`
	Descricao  string `json:"descricao"`
	Operacao   string `json:"operacao"`
	Quantidade int    `json:"quantidade"`
	Expiracao  string `json:"expiracao"`
}

// preAuthorizeHandler simulates POST /chaves/{uuid-chave}/autorizacoes: BRy
// KMS's pré-autorização call. It requires the kms_credencial and
// kms_credencial_tipo headers golgimed's adapter always sends, and issues a
// token good for the requested quantidade uses and expiracao (seconds).
func preAuthorizeHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uuidChave := r.PathValue("uuidChave")

		if r.Header.Get("kms_credencial") == "" || r.Header.Get("kms_credencial_tipo") == "" {
			writeKMSError(w, http.StatusUnauthorized, "credencial_invalida", "kms_credencial e kms_credencial_tipo são obrigatórios.")
			return
		}

		var req autorizacaoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeKMSError(w, http.StatusBadRequest, "requisicao_invalida", "Corpo da requisição inválido.")
			return
		}
		if req.Quantidade <= 0 {
			req.Quantidade = 1
		}

		ttlSeconds, err := strconv.Atoi(req.Expiracao)
		if err != nil || ttlSeconds <= 0 {
			writeKMSError(w, http.StatusBadRequest, "requisicao_invalida", "expiracao deve ser um número de segundos.")
			return
		}

		token, quantidadeRestante, err := store.CreateToken(uuidChave, req.Quantidade, time.Duration(ttlSeconds)*time.Second)
		if err != nil {
			writeKMSError(w, http.StatusInternalServerError, "erro_interno", "Falha ao emitir pré-autorização.")
			return
		}

		httpx.WriteJSON(w, http.StatusCreated, map[string]any{
			"token":              token,
			"quantidadeRestante": quantidadeRestante,
		})
	}
}

type dadosAssinatura struct {
	Perfil        string `json:"perfil"`
	AlgoritmoHash string `json:"algoritmoHash"`
	KMSData       struct {
		Token string `json:"token"`
	} `json:"kms_data"`
}

var supportedProfiles = map[string]bool{"TIMESTAMP": true, "COMPLETE": true}

// signHandler simulates POST /fw/v1/pdf/kms/lote/assinaturas: the HUB
// Signer's medical PDF signing call. It expects the same multipart shape
// golgimed's SignWithPreAuthToken builds (documento[0], dados_assinatura,
// metadados), validates the pré-autorização token issued by
// preAuthorizeHandler, and returns a deterministic signed-document link.
func signHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeHubError(w, http.StatusBadRequest, "Corpo multipart/form-data inválido.", "requisicao_invalida")
			return
		}

		file, _, err := r.FormFile("documento[0]")
		if err != nil {
			writeHubError(w, http.StatusBadRequest, "Campo documento[0] é obrigatório.", "documento_ausente")
			return
		}
		defer func() { _ = file.Close() }()
		pdfBytes, err := io.ReadAll(file)
		if err != nil || len(pdfBytes) == 0 {
			writeHubError(w, http.StatusBadRequest, "Documento vazio ou ilegível.", "documento_invalido")
			return
		}

		var dados dadosAssinatura
		if err := json.Unmarshal([]byte(r.FormValue("dados_assinatura")), &dados); err != nil {
			writeHubError(w, http.StatusBadRequest, "Campo dados_assinatura é obrigatório e deve ser um JSON válido.", "requisicao_invalida")
			return
		}
		if !supportedProfiles[dados.Perfil] {
			writeHubError(w, http.StatusBadRequest, "perfil não suportado (esperado TIMESTAMP ou COMPLETE).", "perfil_invalido")
			return
		}
		if dados.KMSData.Token == "" {
			writeHubError(w, http.StatusUnauthorized, "kms_data.token é obrigatório.", "token_ausente")
			return
		}

		ok, err := store.ConsumeToken(dados.KMSData.Token)
		if err != nil {
			writeHubError(w, http.StatusInternalServerError, "Falha ao validar pré-autorização.", "erro_interno")
			return
		}
		if !ok {
			writeHubError(w, http.StatusUnauthorized, "Pré-autorização inválida, expirada ou esgotada.", "autorizacao_invalida")
			return
		}

		hash := sha256.Sum256(pdfBytes)
		identificador := uuid.NewString()
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"identificador":         identificador,
			"quantidadeAssinaturas": 1,
			"documentos": []map[string]any{
				{
					"hash":        hex.EncodeToString(hash[:]),
					"nomeArquivo": "documento.pdf",
					"links": []map[string]any{
						{"rel": "assinado", "href": "https://mimic.local/bry-medical/documentos/" + identificador},
					},
				},
			},
		})
	}
}
