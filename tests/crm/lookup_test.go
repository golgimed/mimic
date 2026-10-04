package crm_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golgimed/mimic/internal/providers/crm"
	"github.com/golgimed/mimic/internal/testutil"
)

func call(t *testing.T, app *testutil.App, ct, inner string) *httptest.ResponseRecorder {
	t.Helper()
	env := `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ser="http://servico.cfm.org.br/"><soapenv:Body>` + inner + `</soapenv:Body></soapenv:Envelope>`
	req := httptest.NewRequest("POST", crm.Path, strings.NewReader(env))
	req.Header.Set("Content-Type", ct)
	req.Header.Set("SOAPAction", `""`)
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, req)
	return rec
}

func consultar(t *testing.T, crmN, uf, chave string) string {
	t.Helper()
	app := testutil.New(t, 0)
	rec := call(t, app, "text/xml; charset=utf-8",
		"<ser:Consultar><crm>"+crmN+"</crm><uf>"+uf+"</uf><chave>"+chave+"</chave></ser:Consultar>")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}
	b, _ := io.ReadAll(rec.Body)
	return string(b)
}

func TestConsultarFound(t *testing.T) {
	body := consultar(t, "98765", "SP", crm.DefaultAccessKey)
	for _, want := range []string{
		`<ns2:ConsultarResponse xmlns:ns2="http://servico.cfm.org.br/">`,
		"<crm>98765</crm>", "<nome>Ana Costa</nome>", "<situacao>A</situacao>",
		"<tipoInscricao>P</tipoInscricao>", "<uf>SP</uf>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, "codigoErro") || strings.Contains(body, "<especialidade>") {
		t.Errorf("unexpected fields: %s", body)
	}
}

func TestConsultarErrorCodes(t *testing.T) {
	k := crm.DefaultAccessKey
	cases := []struct{ name, crm, uf, chave, code string }{
		{"not found", "11111", "SP", k, "8101"},
		{"lowercase UF", "98765", "sp", k, "8101"},
		{"other UF", "98765", "RJ", k, "8101"},
		{"bad key", "98765", "SP", "wrongkey", "3010"},
		{"no crm", "0", "SP", k, "4010"},
		{"no uf", "98765", "", k, "4000"},
		{"no chave", "98765", "SP", "", "4020"},
		{"no uf and no chave -> 4000", "98765", "", "", "4000"},
	}
	for _, c := range cases {
		body := consultar(t, c.crm, c.uf, c.chave)
		if !strings.Contains(body, "<codigoErro>"+c.code+"</codigoErro>") || strings.Contains(body, "<nome>") {
			t.Errorf("%s: want only codigoErro %s, got %s", c.name, c.code, body)
		}
	}
}

func TestRejectsSOAP12AndUnknownOperation(t *testing.T) {
	app := testutil.New(t, 0)
	if rec := call(t, app, "application/soap+xml", "<ser:Consultar><crm>98765</crm><uf>SP</uf><chave>x</chave></ser:Consultar>"); rec.Code != 415 {
		t.Errorf("SOAP 1.2: expected 415, got %d", rec.Code)
	}
	if rec := call(t, app, "text/xml", "<ser:ConsultaCompleta><crm>98765</crm></ser:ConsultaCompleta>"); rec.Code != 400 {
		t.Errorf("ConsultaCompleta: expected 400, got %d", rec.Code)
	}
}
