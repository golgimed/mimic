package brymedical_test

import (
	"testing"

	"github.com/golgimed/mimic/internal/testutil"
)

func preAuthHeaders() map[string]string {
	return map[string]string{
		"Authorization":       "Bearer test-token",
		"kms_credencial":      "YnJ5MTIz",
		"kms_credencial_tipo": "PIN",
	}
}

func TestPreAuthorizeSuccess(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "POST", "/bry-kms/chaves/11111111-1111-1111-1111-111111111111/autorizacoes", preAuthHeaders(), map[string]any{
		"aplicacao":  "GolgiMed",
		"descricao":  "Assinatura de documento médico",
		"operacao":   "ASSINATURA",
		"quantidade": 10,
		"expiracao":  "3600",
	})
	if rec.Code != 201 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	testutil.DecodeJSON(t, rec, &body)
	if token, _ := body["token"].(string); token == "" {
		t.Errorf("token = %#v", body["token"])
	}
	if qtd, _ := body["quantidadeRestante"].(float64); qtd != 10 {
		t.Errorf("quantidadeRestante = %#v", body["quantidadeRestante"])
	}
}

func TestPreAuthorizeMissingCredential(t *testing.T) {
	app := testutil.New(t, 0)
	headers := map[string]string{"Authorization": "Bearer test-token"}
	rec := app.Do(t, "POST", "/bry-kms/chaves/11111111-1111-1111-1111-111111111111/autorizacoes", headers, map[string]any{
		"aplicacao": "GolgiMed", "descricao": "x", "operacao": "ASSINATURA", "quantidade": 1, "expiracao": "60",
	})
	if rec.Code != 401 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPreAuthorizeRequiresBearer(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "POST", "/bry-kms/chaves/11111111-1111-1111-1111-111111111111/autorizacoes", nil, nil)
	if rec.Code != 401 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}
