// Package crm simulates the CFM "Consulta Médicos" web service
// (WS CFM, manual v3.0): SOAP 1.1 document/literal, operation Consultar.
// See README.md for what is and is not covered.
package crm

import (
	"encoding/xml"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/golgimed/mimic/internal/registry"
	"github.com/golgimed/mimic/internal/shared/admin"
)

const (
	Name = "crm"

	// Path is the documented service path (/WebServiceConsultaMedicos/ServicoConsultaMedicos)
	// under Mimic's /crm provider prefix.
	Path = "/crm/WebServiceConsultaMedicos/ServicoConsultaMedicos"

	DefaultAccessKey = "mimiccrm"
	DefaultDoctor    = "SP:98765:Ana Costa"

	serviceNS = "http://servico.cfm.org.br/"
)

type doctor struct{ uf, crm, nome string }

// New builds the provider. doctorSpecs are "UF:CRM:Nome" entries (the only
// registered physicians); key is the 8-char chave de identificação accepted.
func New(faultStore *admin.Store, key string, doctorSpecs []string) *registry.Provider {
	doctors := map[string]doctor{}
	for _, spec := range doctorSpecs {
		parts := strings.SplitN(spec, ":", 3)
		if len(parts) != 3 {
			continue
		}
		d := doctor{uf: strings.TrimSpace(parts[0]), crm: strings.TrimSpace(parts[1]), nome: strings.TrimSpace(parts[2])}
		doctors[d.uf+":"+d.crm] = d
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		// SOAP 1.2 clients send application/soap+xml and are rejected.
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/soap+xml") {
			http.Error(w, "SOAP 1.2 not supported; use SOAP 1.1 (text/xml)", http.StatusUnsupportedMediaType)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "unreadable body", http.StatusBadRequest)
			return
		}
		var env envelope
		if err := xml.Unmarshal(body, &env); err != nil || env.Body.Op.XMLName.Local != "Consultar" {
			http.Error(w, "unsupported or malformed SOAP request", http.StatusBadRequest)
			return
		}
		op := env.Body.Op
		writeResponse(w, consultar(doctors, key, strings.TrimSpace(op.CRM), strings.TrimSpace(op.UF), strings.TrimSpace(op.Chave)))
	}

	return &registry.Provider{
		Name: Name,
		Register: func(mux *http.ServeMux) {
			mux.Handle("POST "+Path, admin.RequestFaultHook(faultStore, Name, Path)(http.HandlerFunc(handler)))
		},
	}
}

type envelope struct {
	Body struct {
		Op struct {
			XMLName xml.Name
			CRM     string `xml:"crm"`
			UF      string `xml:"uf"`
			Chave   string `xml:"chave"`
		} `xml:",any"`
	} `xml:"Body"`
}

// consultar returns the <dadosMedico> inner XML, applying the checks in the
// manual's documented order: CRM, UF, chave present, chave valid, médico found.
func consultar(doctors map[string]doctor, key, crm, uf, chave string) string {
	if n, err := strconv.Atoi(crm); err != nil || n == 0 {
		return errorXML("4010")
	}
	if uf == "" {
		return errorXML("4000")
	}
	if chave == "" {
		return errorXML("4020")
	}
	if chave != key {
		return errorXML("3010")
	}
	n, _ := strconv.Atoi(crm)
	d, ok := doctors[uf+":"+strconv.Itoa(n)]
	if !ok {
		return errorXML("8101")
	}
	// Fields in the manual's example order; empty ones are omitted.
	return "<crm>" + esc(d.crm) + "</crm>" +
		"<dataAtualizacao>01/01/2026</dataAtualizacao>" +
		"<nome>" + esc(d.nome) + "</nome>" +
		"<situacao>A</situacao>" +
		"<tipoInscricao>P</tipoInscricao>" +
		"<uf>" + esc(d.uf) + "</uf>"
}

func errorXML(code string) string { return "<codigoErro>" + code + "</codigoErro>" }

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// writeResponse always uses HTTP 200: the real service never emits SOAP Faults.
func writeResponse(w http.ResponseWriter, dadosMedico string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>`+
		`<ns2:ConsultarResponse xmlns:ns2="`+serviceNS+`"><dadosMedico>`+dadosMedico+`</dadosMedico></ns2:ConsultarResponse>`+
		`</soap:Body></soap:Envelope>`)
}
