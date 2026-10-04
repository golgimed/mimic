package crm_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golgimed/mimic/internal/providers/crm"
	"github.com/golgimed/mimic/internal/testutil"
)

func TestConsultarSpecialtiesInOrder(t *testing.T) {
	body := consultar(t, "123456", "SP", crm.DefaultAccessKey)
	want := []string{
		"<crm>123456</crm>",
		"<dataAtualizacao>01/01/2026</dataAtualizacao>",
		"<especialidade>CARDIOLOGIA - RQE Nº: 1111 (Áreas de atuação: Cardiologia Pediátrica - RQE Nº: 3333)</especialidade>",
		"<especialidade>DIAGNÓSTICO POR IMAGEM - RQE Nº: 2222 / Ultrassonografia Geral (atuação exclusiva)</especialidade>",
		"<nome>João Exemplo da Silva</nome>",
		"<situacao>A</situacao>",
	}
	pos := 0
	for _, w := range want {
		i := strings.Index(body[pos:], w)
		if i < 0 {
			t.Fatalf("missing or out of order %q in %s", w, body)
		}
		pos += i + len(w)
	}
}

func TestConsultarStandingsAndDisplayCRM(t *testing.T) {
	k := crm.DefaultAccessKey
	cases := []struct{ crm, uf, wantCRM, situacao, tipo string }{
		{"200001", "SP", "200001", "C", "P"},
		{"200002", "SP", "200002", "S", "P"},
		{"200003", "SP", "200003", "F", "P"},
		{"200004", "SP", "200004", "T", "P"},
		{"200005", "SP", "200005", "J", "P"},
		{"200006", "SP", "200006", "P", "P"},
		{"1234", "RJ", "1234", "A", "S"},
		{"1234567", "SP", "1234567-P", "A", "V"},
		{"7654321", "SP", "300-EMFE-7654321", "A", "E"},
	}
	for _, c := range cases {
		body := consultar(t, c.crm, c.uf, k)
		for _, w := range []string{"<crm>" + c.wantCRM + "</crm>", "<situacao>" + c.situacao + "</situacao>", "<tipoInscricao>" + c.tipo + "</tipoInscricao>"} {
			if !strings.Contains(body, w) {
				t.Errorf("%s/%s: missing %q in %s", c.uf, c.crm, w, body)
			}
		}
		if strings.Contains(body, "codigoErro") {
			t.Errorf("%s/%s: unexpected codigoErro: %s", c.uf, c.crm, body)
		}
	}
}

func TestConsultarTransientCodesAreHTTP200(t *testing.T) {
	for crmN, code := range map[string]string{"999201": "2010", "999203": "2030", "999204": "2040"} {
		body := consultar(t, crmN, "SP", crm.DefaultAccessKey)
		if !strings.Contains(body, "<codigoErro>"+code+"</codigoErro>") || strings.Contains(body, "Fault") {
			t.Errorf("crm %s: want codigoErro %s without SOAP Fault, got %s", crmN, code, body)
		}
	}
	// Parameter checks still come first.
	if body := consultar(t, "999204", "SP", "wrongkey"); !strings.Contains(body, "<codigoErro>3010</codigoErro>") {
		t.Errorf("bad key on transient CRM: want 3010, got %s", body)
	}
}

func validar(t *testing.T, crmN, uf, cpf, nasc, chave string) string {
	t.Helper()
	app := testutil.New(t, 0)
	rec := call(t, app, "text/xml; charset=utf-8",
		"<ser:Validar><crm>"+crmN+"</crm><uf>"+uf+"</uf><cpf>"+cpf+"</cpf><dataNascimento>"+nasc+"</dataNascimento><chave>"+chave+"</chave></ser:Validar>")
	if rec.Code != 200 {
		t.Fatalf("Validar: expected HTTP 200, got %d: %s", rec.Code, rec.Body.String())
	}
	b, _ := io.ReadAll(rec.Body)
	return string(b)
}

func TestValidar(t *testing.T) {
	k := crm.DefaultAccessKey
	cases := []struct {
		name, crm, uf, cpf, nasc, chave string
		want                            bool
	}{
		{"match", "98765", "SP", "52998224725", "15/05/1985", k, true},
		{"match with specialties", "123456", "SP", "11144477735", "20/02/1970", k, true},
		{"wrong birth date", "98765", "SP", "52998224725", "16/05/1985", k, false},
		{"wrong cpf", "98765", "SP", "11144477735", "15/05/1985", k, false},
		{"masked cpf", "98765", "SP", "529.982.247-25", "15/05/1985", k, false},
		{"missing cpf", "98765", "SP", "", "15/05/1985", k, false},
		{"bad key", "98765", "SP", "52998224725", "15/05/1985", "wrongkey", false},
		{"lowercase uf", "98765", "sp", "52998224725", "15/05/1985", k, false},
		{"unknown", "11111", "SP", "52998224725", "15/05/1985", k, false},
		{"fixture without cpf", "200001", "SP", "52998224725", "15/05/1985", k, false},
	}
	for _, c := range cases {
		body := validar(t, c.crm, c.uf, c.cpf, c.nasc, c.chave)
		want := "<resultadoConsulta>false</resultadoConsulta>"
		if c.want {
			want = "<resultadoConsulta>true</resultadoConsulta>"
		}
		if !strings.Contains(body, `<ns2:ValidarResponse xmlns:ns2="http://servico.cfm.org.br/">`+want) {
			t.Errorf("%s: want %s, got %s", c.name, want, body)
		}
		if strings.Contains(body, "codigoErro") {
			t.Errorf("%s: Validar must not return codigoErro: %s", c.name, body)
		}
	}
}

func TestServesOfficialWSDL(t *testing.T) {
	app := testutil.New(t, 0)
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, httptest.NewRequest("GET", crm.Path+"?wsdl", nil))
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, `targetNamespace="http://servico.cfm.org.br/"`) ||
		!strings.Contains(body, `<wsdl:operation name="Validar">`) {
		t.Fatalf("expected official WSDL, got %d: %.200s", rec.Code, body)
	}
}

func TestFaultInjection(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "PUT", "/admin/faults", nil, map[string]any{
		"provider": crm.Name, "routePattern": crm.Path, "faultKind": "http_status", "faultValue": "503", "times": 1,
	})
	if rec.Code != 200 && rec.Code != 201 {
		t.Fatalf("create fault: got %d: %s", rec.Code, rec.Body.String())
	}
	inner := "<ser:Consultar><crm>98765</crm><uf>SP</uf><chave>" + crm.DefaultAccessKey + "</chave></ser:Consultar>"
	if rec := call(t, app, "text/xml; charset=utf-8", inner); rec.Code != 503 {
		t.Fatalf("expected injected 503, got %d", rec.Code)
	}
	if rec := call(t, app, "text/xml; charset=utf-8", inner); rec.Code != 200 {
		t.Fatalf("fault should be consumed after one use, got %d", rec.Code)
	}
}
