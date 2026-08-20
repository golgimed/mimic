package sncr_test

import (
	"net/url"
	"testing"

	"github.com/golgimed/mimic/internal/testutil"
)

const validCNPJ = "11222333000181"

func login(t *testing.T, app *testutil.App, conselho, uf, documento string) string {
	t.Helper()
	q := url.Values{
		"client_url": {"https://minha-app.com.br/callback"},
		"conselho":   {conselho},
		"uf":         {uf},
		"documento":  {documento},
	}
	rec := app.Do(t, "GET", "/sncr/api/v1/auth/login?"+q.Encode(), nil, nil)
	if rec.Code != 302 {
		t.Fatalf("login: expected 302, got %d: %s", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	sessionID := loc.Query().Get("session_id")
	if sessionID == "" {
		t.Fatalf("no session_id in Location: %s", rec.Header().Get("Location"))
	}
	return sessionID
}

func exchangeToken(t *testing.T, app *testutil.App, sessionID string) string {
	t.Helper()
	rec := app.Do(t, "GET", "/sncr/api/v1/auth/token?session_id="+sessionID, nil, nil)
	if rec.Code != 200 {
		t.Fatalf("token: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.TokenType != "Bearer" {
		t.Errorf("token_type = %v", body.TokenType)
	}
	if body.AccessToken == "" {
		t.Fatalf("empty access_token")
	}
	return body.AccessToken
}

func authenticate(t *testing.T, app *testutil.App, conselho, uf, documento string) string {
	t.Helper()
	sessionID := login(t, app, conselho, uf, documento)
	return exchangeToken(t, app, sessionID)
}

func authHeader(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func TestAuthFlowEndToEnd(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")
	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestSessionTokenIsSingleUse(t *testing.T) {
	app := testutil.New(t, 0)
	sessionID := login(t, app, "CRM", "RJ", "123456")
	exchangeToken(t, app, sessionID)

	rec := app.Do(t, "GET", "/sncr/api/v1/auth/token?session_id="+sessionID, nil, nil)
	if rec.Code != 401 {
		t.Fatalf("expected 401 on reuse, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginRejectsNonBrCallback(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "GET", "/sncr/api/v1/auth/login?client_url="+url.QueryEscape("https://minha-app.com/callback"), nil, nil)
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginRequiresClientURL(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "GET", "/sncr/api/v1/auth/login", nil, nil)
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotificationNumberingHappyPath(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")

	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", authHeader(token), map[string]any{
		"receita": "NRA", "conselho": "CRM", "uf": "RJ", "documento": "123456", "quantidade": 25,
	})
	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		NumeroReceita []string `json:"numeroReceita"`
		SaldoReceitas int64    `json:"saldoReceitas"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if len(body.NumeroReceita) != 25 {
		t.Fatalf("expected 25 numbers, got %d", len(body.NumeroReceita))
	}
	if body.SaldoReceitas != 25 {
		t.Errorf("saldoReceitas = %d, want 25", body.SaldoReceitas)
	}
}

func TestNotificationNumberingInvalidQuantidade(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")

	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", authHeader(token), map[string]any{
		"receita": "NRA", "conselho": "CRM", "uf": "RJ", "documento": "123456", "quantidade": 5,
	})
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotificationNumberingMissingFields(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")

	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", authHeader(token), map[string]any{
		"conselho": "CRM", "uf": "RJ", "documento": "123456", "quantidade": 25,
	})
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotificationNumberingInvalidReceitaEnum(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")

	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", authHeader(token), map[string]any{
		"receita": "NOT_REAL", "conselho": "CRM", "uf": "RJ", "documento": "123456", "quantidade": 25,
	})
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotificationNumberingIdentityMismatch404(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")

	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", authHeader(token), map[string]any{
		"receita": "NRA", "conselho": "CRM", "uf": "SP", "documento": "999999", "quantidade": 25,
	})
	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotificationNumberingRequiresBearer(t *testing.T) {
	app := testutil.New(t, 0)

	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", nil, map[string]any{
		"receita": "NRA", "conselho": "CRM", "uf": "RJ", "documento": "123456", "quantidade": 25,
	})
	if rec.Code != 401 {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotificationNumberingExceedsRemainingBalance(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")

	body := map[string]any{"receita": "NRA", "conselho": "CRM", "uf": "RJ", "documento": "123456", "quantidade": 40}
	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", authHeader(token), body)
	if rec.Code != 201 {
		t.Fatalf("first call: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Remaining balance is now 10; requesting more than that (20) must not
	// silently truncate — it's the documented 400 "limit reached" path.
	body["quantidade"] = 20
	rec = app.Do(t, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", authHeader(token), body)
	if rec.Code != 400 {
		t.Fatalf("second call: expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotificationNumberingBalanceExhausted204(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")

	body := map[string]any{"receita": "NRA", "conselho": "CRM", "uf": "RJ", "documento": "123456", "quantidade": 50}
	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", authHeader(token), body)
	if rec.Code != 201 {
		t.Fatalf("first call: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Balance is now exhausted (0 remaining) for today: any further request
	// returns 204 with no body, not a 400.
	body["quantidade"] = 10
	rec = app.Do(t, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", authHeader(token), body)
	if rec.Code != 204 {
		t.Fatalf("second call: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEspecialRetencaoHappyPath(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")

	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/receita-especial-retencao", authHeader(token), map[string]any{
		"conselho": "CRM", "tipo": "RCE", "documento": "123456", "uf": "RJ", "cnpj": validCNPJ,
	})
	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Inicio     string `json:"inicio"`
		Fim        string `json:"fim"`
		Quantidade int    `json:"quantidade"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.Quantidade != 1000 {
		t.Errorf("quantidade = %d, want 1000", body.Quantidade)
	}
	if body.Inicio == "" || body.Fim == "" {
		t.Errorf("expected non-empty inicio/fim, got %q/%q", body.Inicio, body.Fim)
	}
}

func TestEspecialRetencaoInvalidCNPJ(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")

	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/receita-especial-retencao", authHeader(token), map[string]any{
		"conselho": "CRM", "tipo": "RCE", "documento": "123456", "uf": "RJ", "cnpj": "00000000000000",
	})
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEspecialRetencaoInvalidTipo(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")

	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/receita-especial-retencao", authHeader(token), map[string]any{
		"conselho": "CRM", "tipo": "NOT_REAL", "documento": "123456", "uf": "RJ", "cnpj": validCNPJ,
	})
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEspecialRetencaoMonthlyRequestLimit(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")
	body := map[string]any{"conselho": "CRM", "tipo": "RCE", "documento": "123456", "uf": "RJ", "cnpj": validCNPJ}

	for i := 0; i < 3; i++ {
		rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/receita-especial-retencao", authHeader(token), body)
		if rec.Code != 201 {
			t.Fatalf("call %d: expected 201, got %d: %s", i+1, rec.Code, rec.Body.String())
		}
	}

	// 4th request this month (across RCE+RET combined) must be rejected.
	body["tipo"] = "RET"
	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/receita-especial-retencao", authHeader(token), body)
	if rec.Code != 400 {
		t.Fatalf("expected 400 on 4th request, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestEspecialRetencaoIdentityMismatch404(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")

	rec := app.Do(t, "POST", "/sncr/api/v1/numeracoes/receita-especial-retencao", authHeader(token), map[string]any{
		"conselho": "CRM", "tipo": "RCE", "documento": "999999", "uf": "RJ", "cnpj": validCNPJ,
	})
	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestFaultInjectionAppliesToNumeracoes exercises the fault-injection path
// (PUT /admin/faults) against the notification numbering route, the same
// pattern used across other providers' fault tests.
func TestFaultInjectionAppliesToNumeracoes(t *testing.T) {
	app := testutil.New(t, 0)
	token := authenticate(t, app, "CRM", "RJ", "123456")

	rec := app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider":     "sncr",
		"routePattern": "/sncr/api/v1/numeracoes/notificacao-receita",
		"faultKind":    "http_status",
		"faultValue":   "503",
		"times":        1,
	})
	if rec.Code != 200 && rec.Code != 201 {
		t.Fatalf("create fault: expected 200/201, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = app.Do(t, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", authHeader(token), map[string]any{
		"receita": "NRA", "conselho": "CRM", "uf": "RJ", "documento": "123456", "quantidade": 25,
	})
	if rec.Code != 503 {
		t.Fatalf("expected fault-injected 503, got %d: %s", rec.Code, rec.Body.String())
	}

	// Fault was single-use: the next call should succeed normally.
	rec = app.Do(t, "POST", "/sncr/api/v1/numeracoes/notificacao-receita", authHeader(token), map[string]any{
		"receita": "NRA", "conselho": "CRM", "uf": "RJ", "documento": "123456", "quantidade": 25,
	})
	if rec.Code != 201 {
		t.Fatalf("expected 201 after fault consumed, got %d: %s", rec.Code, rec.Body.String())
	}
}
