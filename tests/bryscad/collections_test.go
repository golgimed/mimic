package bryscad_test

import (
	"testing"

	"github.com/golgimed/mimic/internal/testutil"
)

var bearer = map[string]string{"Authorization": "Bearer test-token"}

func TestCollectionsLifecycle(t *testing.T) {
	app := testutil.New(t, 0)
	create := app.Do(t, "POST", "/bry-scad/coletas/cadastrar", bearer, map[string]any{
		"nome":          "Contrato de teste",
		"participantes": []map[string]string{{"email": "signer@example.test"}},
	})
	if create.Code != 200 {
		t.Fatalf("create status = %d: %s", create.Code, create.Body.String())
	}
	var created map[string]any
	testutil.DecodeJSON(t, create, &created)
	chave, ok := created["chaveWorkflow"].(string)
	if !ok || chave == "" {
		t.Fatalf("missing chave: %#v", created)
	}
	if created["sucesso"] != true {
		t.Errorf("sucesso = %v", created["sucesso"])
	}

	get := app.Do(t, "GET", "/bry-scad/coletas/"+chave, bearer, nil)
	if get.Code != 200 {
		t.Fatalf("get status = %d: %s", get.Code, get.Body.String())
	}
	var fetched []map[string]any
	testutil.DecodeJSON(t, get, &fetched)
	if len(fetched) != 1 || fetched[0]["titulo"] != "Contrato de teste" {
		t.Errorf("coleta = %#v", fetched)
	}

	cancel := app.Do(t, "POST", "/bry-scad/coletas/"+chave+"/cancelar", bearer, nil)
	if cancel.Code != 200 {
		t.Fatalf("cancel status = %d: %s", cancel.Code, cancel.Body.String())
	}
	var cancelled map[string]any
	testutil.DecodeJSON(t, cancel, &cancelled)
	if cancelled["sucesso"] != true {
		t.Errorf("sucesso = %v", cancelled["sucesso"])
	}
}

func TestCollectionsRequireBearerToken(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "GET", "/bry-scad/tipos-assinatura", nil, nil)
	if rec.Code != 401 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	testutil.DecodeJSON(t, rec, &body)
	if body["sucesso"] != false {
		t.Errorf("sucesso = %v", body["sucesso"])
	}
}

func createCollection(t *testing.T, app *testutil.App) string {
	t.Helper()
	create := app.Do(t, "POST", "/bry-scad/coletas/cadastrar", bearer, map[string]any{
		"nome":          "Contrato",
		"participantes": []map[string]any{{"email": "signer@example.test", "assinante": true}},
	})
	var created map[string]any
	testutil.DecodeJSON(t, create, &created)
	return created["chaveWorkflow"].(string)
}

func TestCollectionTransitionsAreGuarded(t *testing.T) {
	app := testutil.New(t, 0)

	cases := []struct{ first, second string }{
		{"cancelar", "rejeitar"},
		{"rejeitar", "concluir"},
		{"concluir", "cancelar"},
	}
	for _, tc := range cases {
		chave := createCollection(t, app)
		if rec := app.Do(t, "POST", "/bry-scad/coletas/"+chave+"/"+tc.first, bearer, nil); rec.Code != 200 {
			t.Fatalf("%s status = %d: %s", tc.first, rec.Code, rec.Body.String())
		}
		rec := app.Do(t, "POST", "/bry-scad/coletas/"+chave+"/"+tc.second, bearer, nil)
		if rec.Code != 409 {
			t.Fatalf("%s after %s: status = %d, want 409: %s", tc.second, tc.first, rec.Code, rec.Body.String())
		}
		var body map[string]any
		testutil.DecodeJSON(t, rec, &body)
		if body["chaveI18n"] != "coleta_situacao_invalida" {
			t.Errorf("chaveI18n = %v", body["chaveI18n"])
		}
	}

	if rec := app.Do(t, "POST", "/bry-scad/coletas/missing/cancelar", bearer, nil); rec.Code != 404 {
		t.Fatalf("unknown collection: status = %d, want 404", rec.Code)
	}
}
