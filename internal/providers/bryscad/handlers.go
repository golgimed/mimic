package bryscad

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golgimed/mimic/internal/shared/httpx"
)

// The OpenAPI document declares a number of command and query endpoints whose
// request models are intentionally permissive. These handlers return their
// documented top-level response shape while keeping unsupported provider-side
// business rules out of the simulator.
func successHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"sucesso": true, "chaveI18n": "sucesso", "mensagem": "Operação realizada com sucesso."})
	}
}

func collectionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { httpx.WriteJSON(w, http.StatusOK, []any{}) }
}

func objectHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { httpx.WriteJSON(w, http.StatusOK, map[string]any{}) }
}

func downloadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
	}
}

func webhookMethodsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, []map[string]any{
			{"active": true, "key": "scad.coleta", "name": "SCAD Coleta", "description": "Webhook chamado quando a coleta muda de estado."},
			{"active": true, "key": "scad.assinatura", "name": "SCAD Assinatura", "description": "Webhook chamado quando uma assinatura é concluída."},
			{"active": true, "key": "scad.revisao", "name": "SCAD Revisão", "description": "Webhook chamado quando uma revisão é concluída."},
		})
	}
}

func locksHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"locks": []any{}})
	}
}

func startSigningHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"lotes": []any{}})
	}
}

func finishSigningHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"finalizado": true, "resultadoAssinatura": map[string]any{"map": map[string]any{"sucesso": true, "chaveI18n": "sucesso", "mensagem": "Assinatura finalizada."}}})
	}
}

func writeError(w http.ResponseWriter, status int, key, message string) {
	httpx.WriteJSON(w, status, map[string]any{"sucesso": false, "chaveI18n": key, "mensagem": message})
}

func collectionResponse(c *Collection) map[string]any {
	title, _ := c.Payload["nomeColeta"].(string)
	if title == "" {
		title, _ = c.Payload["nome"].(string)
	}
	description, _ := c.Payload["descricao"].(string)
	response := map[string]any{
		"titulo":                       title,
		"chave":                        c.Chave,
		"situacao":                     map[string]any{"chave": c.Situacao, "descricao": c.Situacao},
		"dataCriacao":                  c.CreatedAt,
		"responsavel":                  "Usuário Mimic",
		"pendenteExecucaoUsuarioToken": c.Situacao == "PENDENTE",
		"podeCancelar":                 c.Situacao == "PENDENTE",
		"descricao":                    description,
	}
	for _, key := range []string{"padraoAssinatura", "dataLimite", "exigirDownload", "proibirRejeicao"} {
		if value, ok := c.Payload[key]; ok {
			response[key] = value
		}
	}
	return response
}

func permissionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"permitido": true, "chaveMensagem": nil, "mensagem": nil})
	}
}

func signatureTypesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, []map[string]any{
			{"attached": true, "padraoAssinatura": "PDF"},
			{"attached": false, "padraoAssinatura": "PADES", "politicaAssinatura": "ADRB"},
			{"attached": false, "padraoAssinatura": "CADES", "politicaAssinatura": "ADRB"},
		})
	}
}

func createCollectionHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || len(payload) == 0 {
			writeError(w, http.StatusBadRequest, "requisicao_invalida", "O corpo da requisição deve conter os dados da coleta.")
			return
		}
		for key, values := range r.URL.Query() {
			if len(values) > 0 {
				payload[key] = values[len(values)-1]
			}
		}
		collection, err := store.CreateCollection(payload)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "erro_interno", err.Error())
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"sucesso": true, "chaveWorkflow": collection.Chave, "chaveI18n": "sucesso", "mensagem": "Coleta cadastrada com sucesso."})
	}
}

func listCollectionsHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		collections, err := store.ListCollections()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "erro_interno", err.Error())
			return
		}
		response := make([]map[string]any, 0, len(collections))
		for i := range collections {
			response = append(response, collectionResponse(&collections[i]))
		}
		w.Header().Set("x_scad_total", fmt.Sprint(len(response)))
		httpx.WriteJSON(w, http.StatusOK, response)
	}
}

func getCollectionHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		collection, err := store.GetCollection(r.PathValue("chave"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "erro_interno", err.Error())
			return
		}
		if collection == nil {
			writeError(w, http.StatusNotFound, "coleta_nao_encontrada", "Coleta não encontrada.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, []map[string]any{collectionResponse(collection)})
	}
}

func transitionCollectionHandler(store *Store, situacao string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		collection, err := store.TransitionCollection(r.PathValue("chave"), situacao)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "erro_interno", err.Error())
			return
		}
		if collection == nil {
			writeError(w, http.StatusNotFound, "coleta_nao_encontrada", "Coleta não encontrada.")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"sucesso": true, "chaveWorkflow": collection.Chave, "chaveI18n": "sucesso", "mensagem": "Coleta atualizada com sucesso."})
	}
}
