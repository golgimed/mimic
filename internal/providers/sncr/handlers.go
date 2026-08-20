package sncr

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/golgimed/mimic/internal/shared/httpx"
)

func sendError(w http.ResponseWriter, status int, message string) {
	httpx.WriteJSON(w, status, map[string]any{"error": message})
}

// loginHandler simulates GET /api/v1/auth/login. The real flow bounces the
// browser through Keycloak + Gov.br before landing back here — a human
// login the simulator can't perform. Like IntegraICP's autostart, we skip
// that hop but keep the flow's shape: the simulator-only conselho/uf/documento
// query params (see README) stand in for "whichever Gov.br professional
// logged in", and the handler still redirects to client_url?session_id=...
// exactly like the real callback does.
func loginHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		clientURL := q.Get("client_url")
		if clientURL == "" {
			sendError(w, http.StatusBadRequest, "client_url is required.")
			return
		}

		target, err := url.Parse(clientURL)
		if err != nil || (target.Scheme != "http" && target.Scheme != "https") {
			sendError(w, http.StatusBadRequest, "client_url must be a valid http(s) URL.")
			return
		}
		// Real API: "Only .br domains are whitelisted as callback targets."
		if !strings.HasSuffix(strings.ToLower(target.Hostname()), ".br") {
			sendError(w, http.StatusBadRequest, "client_url must be a .br domain.")
			return
		}

		identity := Identity{
			Conselho:  defaultIfEmpty(q.Get("conselho"), "CRM"),
			UF:        defaultIfEmpty(q.Get("uf"), "SP"),
			Documento: defaultIfEmpty(q.Get("documento"), "000000"),
		}

		sessionToken, err := store.CreateAuthSession(identity)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}

		values := target.Query()
		values.Set("session_id", sessionToken)
		if state := q.Get("state"); state != "" {
			values.Set("state", state)
		}
		target.RawQuery = values.Encode()

		w.Header().Set("Location", target.String())
		w.WriteHeader(http.StatusFound)
	}
}

func defaultIfEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// tokenHandler simulates GET /api/v1/auth/token — exchanges the one-time
// session_id minted by loginHandler for the real Bearer access_token.
func tokenHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" {
			sendError(w, http.StatusUnauthorized, "Sessão inválida ou expirada")
			return
		}

		accessToken, err := store.ExchangeSession(sessionID)
		if err != nil {
			sendError(w, http.StatusUnauthorized, "Sessão inválida ou expirada")
			return
		}

		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"access_token": accessToken,
			"token_type":   "Bearer",
		})
	}
}

// bearerIdentity resolves the Authorization: Bearer <token> header to the
// simulated authenticated professional, writing a 401 response and
// returning ok=false if the header is missing or the token is unknown.
func bearerIdentity(store *Store, w http.ResponseWriter, r *http.Request) (identity Identity, ok bool) {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) || auth == prefix {
		sendError(w, http.StatusUnauthorized, "Não autorizado.")
		return Identity{}, false
	}

	identity, found, err := store.ResolveAccessToken(strings.TrimPrefix(auth, prefix))
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return Identity{}, false
	}
	if !found {
		sendError(w, http.StatusUnauthorized, "Não autorizado.")
		return Identity{}, false
	}
	return identity, true
}

var (
	validReceitas = map[string]bool{"NRA": true, "NRB": true, "NRB2": true, "NRR": true, "NRT": true}
	validConselho = map[string]bool{"CRM": true, "CRMV": true, "CRO": true}
	validTipos    = map[string]bool{"RCE": true, "RET": true}
)

type notificationRequest struct {
	Receita    string `json:"receita"`
	Conselho   string `json:"conselho"`
	UF         string `json:"uf"`
	Documento  string `json:"documento"`
	Quantidade int64  `json:"quantidade"`
}

func (b notificationRequest) missingField() bool {
	return b.Receita == "" || b.Conselho == "" || b.UF == "" || b.Documento == ""
}

// notificationNumberingHandler simulates POST /api/v1/numeracoes/notificacao-receita.
func notificationNumberingHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body notificationRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.missingField() {
			sendError(w, http.StatusBadRequest, "Campos obrigatórios ausentes ou inválidos.")
			return
		}
		if !validReceitas[body.Receita] {
			sendError(w, http.StatusBadRequest, "Valor de receita inválido.")
			return
		}
		if !validConselho[body.Conselho] {
			sendError(w, http.StatusBadRequest, "Valor de conselho inválido.")
			return
		}
		if body.Quantidade < 10 || body.Quantidade > 50 {
			sendError(w, http.StatusBadRequest, "quantidade deve estar entre 10 e 50.")
			return
		}

		identity, ok := bearerIdentity(store, w, r)
		if !ok {
			return
		}
		requested := Identity{Conselho: body.Conselho, UF: body.UF, Documento: body.Documento}
		if identity != requested {
			sendError(w, http.StatusNotFound, "Identidade autenticada não corresponde ao prescritor solicitado.")
			return
		}

		startSeq, remaining, allocated, err := store.AllocateNotificationNumbers(body.Receita, requested, body.Quantidade)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !allocated {
			if remaining <= 0 {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			sendError(w, http.StatusBadRequest, "Limite diário de 50 receitas atingido.")
			return
		}

		numbers := make([]string, body.Quantidade)
		for i := range numbers {
			numbers[i] = formatPrescriptionNumber(body.Receita, body.UF, startSeq+int64(i))
		}

		resp := map[string]any{
			"numeroReceita": numbers,
			"saldoReceitas": remaining,
		}
		if remaining < 50 {
			resp["mensagem"] = "Saldo inferior a 50 receitas disponíveis."
		}
		httpx.WriteJSON(w, http.StatusCreated, resp)
	}
}

type especialRetencaoRequest struct {
	Conselho  string `json:"conselho"`
	Tipo      string `json:"tipo"`
	Documento string `json:"documento"`
	UF        string `json:"uf"`
	CNPJ      string `json:"cnpj"`
}

func (b especialRetencaoRequest) missingField() bool {
	return b.Conselho == "" || b.Tipo == "" || b.Documento == "" || b.UF == "" || b.CNPJ == ""
}

// especialRetencaoHandler simulates POST /api/v1/numeracoes/receita-especial-retencao.
func especialRetencaoHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body especialRetencaoRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.missingField() {
			sendError(w, http.StatusBadRequest, "Campos obrigatórios ausentes ou inválidos.")
			return
		}
		if !validConselho[body.Conselho] {
			sendError(w, http.StatusBadRequest, "Valor de conselho inválido.")
			return
		}
		if !validTipos[body.Tipo] {
			sendError(w, http.StatusBadRequest, "Valor de tipo inválido.")
			return
		}
		if !validCNPJ(body.CNPJ) {
			sendError(w, http.StatusBadRequest, "CNPJ inválido.")
			return
		}

		identity, ok := bearerIdentity(store, w, r)
		if !ok {
			return
		}
		requested := Identity{Conselho: body.Conselho, UF: body.UF, Documento: body.Documento}
		if identity != requested {
			sendError(w, http.StatusNotFound, "Identidade autenticada não corresponde ao prescritor solicitado.")
			return
		}

		startSeq, allocated, err := store.AllocateEspecialRetencaoRange(body.Tipo, requested)
		if err != nil {
			sendError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !allocated {
			sendError(w, http.StatusBadRequest, "Limite mensal de solicitações ou numerações atingido.")
			return
		}

		inicio := formatPrescriptionNumber(body.Tipo, body.UF, startSeq)
		fim := formatPrescriptionNumber(body.Tipo, body.UF, startSeq+especialAllocationSize-1)

		httpx.WriteJSON(w, http.StatusCreated, map[string]any{
			"inicio":     inicio,
			"fim":        fim,
			"quantidade": especialAllocationSize,
			"mensagem":   "Numeração gerada com sucesso.",
		})
	}
}
