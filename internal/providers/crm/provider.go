// Package crm simulates the CFM "Consulta Médicos" web service
// (WS CFM, manual v3.0 + published WSDL): SOAP 1.1 document/literal,
// operations Consultar and Validar. See README.md for what is and is not covered.
package crm

import (
	_ "embed"
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

	serviceNS = "http://servico.cfm.org.br/"

	// dataAtualizacao returned for every fixture (DD/MM/YYYY).
	dataAtualizacao = "01/01/2026"
)

// wsdl is the official WSDL as published by CFM at
// https://ws.cfm.org.br:8080/WebServiceConsultaMedicos/ServicoConsultaMedicos?wsdl
// (fetched 2026-10-04), served verbatim on GET Path?wsdl.
//
//go:embed servico.wsdl
var wsdl []byte

// DefaultDoctors are the built-in fixtures, always registered. Every value is
// fictitious, styled after the manual's examples. Format:
// UF:CRM:Nome[:situacao[:tipoInscricao[:crmExibicao[:cpf[:dataNascimento[:especialidade|especialidade...]]]]]]
var DefaultDoctors = []string{
	"SP:98765:Ana Costa:A:P::52998224725:15/05/1985",
	"SP:123456:João Exemplo da Silva:A:P::11144477735:20/02/1970:" +
		"CARDIOLOGIA - RQE Nº: 1111 (Áreas de atuação: Cardiologia Pediátrica - RQE Nº: 3333)|" +
		"DIAGNÓSTICO POR IMAGEM - RQE Nº: 2222 / Ultrassonografia Geral (atuação exclusiva)",
	"RJ:1234:Beatriz Exemplo Rocha:A:S",
	"SP:1234567:Diego Exemplo Prates:A:V:1234567-P",
	"SP:7654321:Elisa Exemplo Moura:A:E:300-EMFE-7654321",
	"SP:200001:Carlos Exemplo Cassado:C:P",
	"SP:200002:Fernanda Exemplo Suspensa:S:P",
	"SP:200003:Gilberto Exemplo Falecido:F:P",
	"SP:200004:Helena Exemplo Transferida:T:P",
	"SP:200005:Igor Exemplo Parcial:J:P",
	"SP:200006:Joana Exemplo Aposentada:P:P",
}

// transientCodes are Mimic-only triggers: these SP CRMs answer the manual's
// transient 2xxx codes (HTTP 200, codigoErro only) once the request is valid.
var transientCodes = map[string]string{
	"SP:999201": "2010",
	"SP:999203": "2030",
	"SP:999204": "2040",
}

type doctor struct {
	uf, crm, nome, situacao, tipo, crmExibicao, cpf, nascimento string
	especialidades                                              []string
}

func parseDoctor(spec string) (doctor, bool) {
	p := strings.SplitN(spec, ":", 9)
	if len(p) < 3 {
		return doctor{}, false
	}
	for len(p) < 9 {
		p = append(p, "")
	}
	for i := range p {
		p[i] = strings.TrimSpace(p[i])
	}
	n, err := strconv.Atoi(p[1])
	if err != nil || n <= 0 {
		return doctor{}, false
	}
	d := doctor{uf: p[0], crm: strconv.Itoa(n), nome: p[2], situacao: p[3], tipo: p[4], crmExibicao: p[5], cpf: p[6], nascimento: p[7]}
	if d.situacao == "" {
		d.situacao = "A"
	}
	if d.tipo == "" {
		d.tipo = "P"
	}
	if d.crmExibicao == "" {
		d.crmExibicao = d.crm
	}
	if p[8] != "" {
		for _, e := range strings.Split(p[8], "|") {
			if e = strings.TrimSpace(e); e != "" {
				d.especialidades = append(d.especialidades, e)
			}
		}
	}
	return d, true
}

// New builds the provider. doctorSpecs (MIMIC_CRM_DOCTORS) are added on top of
// DefaultDoctors, replacing a default with the same UF:CRM. key is the 8-char
// chave de identificação accepted.
func New(faultStore *admin.Store, key string, doctorSpecs []string) *registry.Provider {
	doctors := map[string]doctor{}
	for _, spec := range append(append([]string{}, DefaultDoctors...), doctorSpecs...) {
		if d, ok := parseDoctor(spec); ok {
			doctors[d.uf+":"+d.crm] = d
		}
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
		if err := xml.Unmarshal(body, &env); err != nil {
			http.Error(w, "malformed SOAP request", http.StatusBadRequest)
			return
		}
		op := env.Body.Op
		crmN, uf, chave := strings.TrimSpace(op.CRM), strings.TrimSpace(op.UF), strings.TrimSpace(op.Chave)
		switch op.XMLName.Local {
		case "Consultar":
			writeResponse(w, "ConsultarResponse", "<dadosMedico>"+consultar(doctors, key, crmN, uf, chave)+"</dadosMedico>")
		case "Validar":
			ok := validar(doctors, key, crmN, uf, strings.TrimSpace(op.CPF), strings.TrimSpace(op.DataNascimento), chave)
			writeResponse(w, "ValidarResponse", "<resultadoConsulta>"+strconv.FormatBool(ok)+"</resultadoConsulta>")
		default:
			// ConsultaCompleta, buscarImagem, ... are declared in the WSDL but
			// require a separate contract; their behavior is undocumented.
			http.Error(w, "operation not simulated: "+op.XMLName.Local, http.StatusBadRequest)
		}
	}

	wsdlHandler := func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.Query().Has("wsdl") {
			http.Error(w, "only GET ?wsdl is supported", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = w.Write(wsdl)
	}

	return &registry.Provider{
		Name: Name,
		Register: func(mux *http.ServeMux) {
			mux.Handle("POST "+Path, admin.RequestFaultHook(faultStore, Name, Path)(http.HandlerFunc(handler)))
			mux.HandleFunc("GET "+Path, wsdlHandler)
		},
	}
}

type envelope struct {
	Body struct {
		Op struct {
			XMLName        xml.Name
			CRM            string `xml:"crm"`
			UF             string `xml:"uf"`
			CPF            string `xml:"cpf"`
			DataNascimento string `xml:"dataNascimento"`
			Chave          string `xml:"chave"`
		} `xml:",any"`
	} `xml:"Body"`
}

// consultar returns the <dadosMedico> inner XML, applying the checks in the
// manual's documented order: CRM, UF, chave present, chave valid, médico found.
func consultar(doctors map[string]doctor, key, crm, uf, chave string) string {
	n, err := strconv.Atoi(crm)
	if err != nil || n <= 0 {
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
	id := uf + ":" + strconv.Itoa(n)
	if code, ok := transientCodes[id]; ok {
		return errorXML(code)
	}
	d, ok := doctors[id]
	if !ok {
		return errorXML("8101")
	}
	// Elements in the manual's example order; empty ones are omitted.
	out := "<crm>" + esc(d.crmExibicao) + "</crm><dataAtualizacao>" + dataAtualizacao + "</dataAtualizacao>"
	for _, e := range d.especialidades {
		out += "<especialidade>" + esc(e) + "</especialidade>"
	}
	return out + "<nome>" + esc(d.nome) + "</nome>" +
		"<situacao>" + esc(d.situacao) + "</situacao>" +
		"<tipoInscricao>" + esc(d.tipo) + "</tipoInscricao>" +
		"<uf>" + esc(d.uf) + "</uf>"
}

// validar never returns an error code: any condition preventing confirmation
// (missing parameter, bad key, unknown médico, CPF/birth-date mismatch) is false.
func validar(doctors map[string]doctor, key, crm, uf, cpf, nascimento, chave string) bool {
	n, err := strconv.Atoi(crm)
	if err != nil || n <= 0 || uf == "" || cpf == "" || nascimento == "" || chave == "" || chave != key {
		return false
	}
	d, ok := doctors[uf+":"+strconv.Itoa(n)]
	return ok && d.cpf != "" && d.cpf == cpf && d.nascimento == nascimento
}

func errorXML(code string) string { return "<codigoErro>" + code + "</codigoErro>" }

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// writeResponse always uses HTTP 200: the real service never emits SOAP Faults.
func writeResponse(w http.ResponseWriter, element, inner string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>`+
		`<ns2:`+element+` xmlns:ns2="`+serviceNS+`">`+inner+`</ns2:`+element+`>`+
		`</soap:Body></soap:Envelope>`)
}
