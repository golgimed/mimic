package brymedical_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golgimed/mimic/internal/shared/admin"
	"github.com/golgimed/mimic/internal/testutil"
)

const signPath = "/bry-medical/fw/v1/pdf/kms/lote/assinaturas"

func preAuthorize(t *testing.T, app *testutil.App, expiracao string) string {
	t.Helper()
	rec := app.Do(t, "POST", "/bry-kms/chaves/11111111-1111-1111-1111-111111111111/autorizacoes", preAuthHeaders(), map[string]any{
		"aplicacao": "GolgiMed", "descricao": "x", "operacao": "ASSINATURA", "quantidade": 1, "expiracao": expiracao,
	})
	if rec.Code != 201 {
		t.Fatalf("pre-authorize status = %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	testutil.DecodeJSON(t, rec, &body)
	return body["token"].(string)
}

// signRequest builds the multipart/form-data request golgimed's
// SignWithPreAuthToken sends. app.Do only JSON-encodes, so this bypasses it
// and hits app.Handler directly, matching how a real multipart caller would.
func signRequest(t *testing.T, app *testutil.App, headers map[string]string, dadosAssinatura, metadados any, pdfBody []byte, omitDocumento bool) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	if !omitDocumento {
		fw, err := mw.CreateFormFile("documento[0]", "documento.pdf")
		if err != nil {
			t.Fatalf("create documento[0] part: %v", err)
		}
		if _, err := fw.Write(pdfBody); err != nil {
			t.Fatalf("write documento[0] bytes: %v", err)
		}
	}

	if dadosAssinatura != nil {
		var raw string
		if s, ok := dadosAssinatura.(string); ok {
			raw = s
		} else {
			b, err := json.Marshal(dadosAssinatura)
			if err != nil {
				t.Fatalf("marshal dados_assinatura: %v", err)
			}
			raw = string(b)
		}
		if err := mw.WriteField("dados_assinatura", raw); err != nil {
			t.Fatalf("write dados_assinatura field: %v", err)
		}
	}

	if metadados != nil {
		b, err := json.Marshal(metadados)
		if err != nil {
			t.Fatalf("marshal metadados: %v", err)
		}
		if err := mw.WriteField("metadados", string(b)); err != nil {
			t.Fatalf("write metadados field: %v", err)
		}
	}

	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest("POST", signPath, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, req)
	return rec
}

func signHeaders(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer test-token", "kms_type": "BRYKMS"}
}

func dadosAssinatura(perfil, token string) map[string]any {
	return map[string]any{
		"perfil":        perfil,
		"algoritmoHash": "SHA256",
		"kms_data":      map[string]any{"token": token},
	}
}

func sampleMetadados() []map[string]string {
	return []map[string]string{{"2.16.76.1.12.1.1": "", "2.16.76.1.4.2.2.1": "1234"}}
}

func TestSignSuccess(t *testing.T) {
	app := testutil.New(t, 0)
	token := preAuthorize(t, app, "3600")

	rec := signRequest(t, app, signHeaders(token), dadosAssinatura("TIMESTAMP", token), sampleMetadados(), []byte("%PDF-1.4 fake"), false)
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	testutil.DecodeJSON(t, rec, &body)
	if id, _ := body["identificador"].(string); id == "" {
		t.Errorf("identificador = %#v", body["identificador"])
	}
	docs, _ := body["documentos"].([]any)
	if len(docs) != 1 {
		t.Fatalf("documentos = %#v", body["documentos"])
	}
	doc := docs[0].(map[string]any)
	if hash, _ := doc["hash"].(string); hash == "" {
		t.Errorf("hash = %#v", doc["hash"])
	}
	links, _ := doc["links"].([]any)
	if len(links) != 1 || links[0].(map[string]any)["href"] == "" {
		t.Errorf("links = %#v", doc["links"])
	}
}

func TestSignCompleteProfile(t *testing.T) {
	app := testutil.New(t, 0)
	token := preAuthorize(t, app, "3600")

	rec := signRequest(t, app, signHeaders(token), dadosAssinatura("COMPLETE", token), sampleMetadados(), []byte("%PDF-1.4 fake"), false)
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSignUnsupportedProfile(t *testing.T) {
	app := testutil.New(t, 0)
	token := preAuthorize(t, app, "3600")

	rec := signRequest(t, app, signHeaders(token), dadosAssinatura("BASIC", token), sampleMetadados(), []byte("%PDF-1.4 fake"), false)
	if rec.Code == 200 {
		t.Fatalf("expected failure, got 200: %s", rec.Body.String())
	}
}

func TestSignUnknownToken(t *testing.T) {
	app := testutil.New(t, 0)

	rec := signRequest(t, app, signHeaders("nonexistent-token"), dadosAssinatura("TIMESTAMP", "nonexistent-token"), sampleMetadados(), []byte("%PDF-1.4 fake"), false)
	if rec.Code == 200 {
		t.Fatalf("expected failure, got 200: %s", rec.Body.String())
	}
}

func TestSignExpiredToken(t *testing.T) {
	app := testutil.New(t, 0)
	token := preAuthorize(t, app, "1")
	time.Sleep(1100 * time.Millisecond)

	rec := signRequest(t, app, signHeaders(token), dadosAssinatura("TIMESTAMP", token), sampleMetadados(), []byte("%PDF-1.4 fake"), false)
	if rec.Code == 200 {
		t.Fatalf("expected failure, got 200: %s", rec.Body.String())
	}
}

func TestSignMissingDocumento(t *testing.T) {
	app := testutil.New(t, 0)
	token := preAuthorize(t, app, "3600")

	rec := signRequest(t, app, signHeaders(token), dadosAssinatura("TIMESTAMP", token), sampleMetadados(), nil, true)
	if rec.Code == 200 {
		t.Fatalf("expected failure, got 200: %s", rec.Body.String())
	}
}

func TestSignMalformedDadosAssinatura(t *testing.T) {
	app := testutil.New(t, 0)
	token := preAuthorize(t, app, "3600")

	rec := signRequest(t, app, signHeaders(token), "not-json", sampleMetadados(), []byte("%PDF-1.4 fake"), false)
	if rec.Code == 200 {
		t.Fatalf("expected failure, got 200: %s", rec.Body.String())
	}
}

func TestSignMissingKMSTypeHeader(t *testing.T) {
	app := testutil.New(t, 0)
	token := preAuthorize(t, app, "3600")

	headers := map[string]string{"Authorization": "Bearer test-token"}
	rec := signRequest(t, app, headers, dadosAssinatura("TIMESTAMP", token), sampleMetadados(), []byte("%PDF-1.4 fake"), false)
	if rec.Code == 200 {
		t.Fatalf("expected failure, got 200: %s", rec.Body.String())
	}
}

func TestSignProviderFailure(t *testing.T) {
	app := testutil.New(t, 0)
	token := preAuthorize(t, app, "3600")

	faultStore := admin.NewStore(app.DB)
	routePattern := signPath
	faultValue := "500"
	if _, err := faultStore.CreateFault(admin.CreateFaultInput{
		Provider:     "bry-medical",
		RoutePattern: &routePattern,
		FaultKind:    admin.FaultHTTPStatus,
		FaultValue:   &faultValue,
	}); err != nil {
		t.Fatalf("seed fault: %v", err)
	}

	rec := signRequest(t, app, signHeaders(token), dadosAssinatura("TIMESTAMP", token), sampleMetadados(), []byte("%PDF-1.4 fake"), false)
	if rec.Code != 500 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}
