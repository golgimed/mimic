package integraicp_test

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"testing"

	"github.com/golgimed/mimic/internal/testutil"
)

// executionStatusBody mirrors IntegraICP's executionStatus response object,
// shared across the several endpoints that return it.
type executionStatusBody struct {
	CurrentStatus string `json:"currentStatus"`
}

type subjectIdentificationBody struct {
	IdentificationKey string `json:"identificationKey"`
}

func pkce() (verifier, challenge string) {
	verifier = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return
}

func sha256Base64(text string) string {
	sum := sha256.Sum256([]byte(text))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func authenticateAndGetCredentialID(t *testing.T, app *testutil.App, challenge string) string {
	t.Helper()
	q := url.Values{
		"subject_key":  {"46404461013"},
		"secret_data":  {challenge},
		"secret_type":  {"code_challenge"},
		"callback_uri": {"https://my.app/callback"},
		"autostart":    {"true"},
	}
	rec := app.Do(t, "GET", "/integraicp/c/test-channel/icp/v3/authentications?"+q.Encode(), nil, nil)
	if rec.Code != 302 {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse Location: %v", err)
	}
	return loc.Query().Get("credentialId")
}

func TestSignatureFlowEndToEnd(t *testing.T) {
	app := testutil.New(t, 0)
	verifier, challenge := pkce()
	credentialID := authenticateAndGetCredentialID(t, app, challenge)

	credRec := app.Do(t, "GET", "/integraicp/c/test-channel/icp/v3/credentials/"+credentialID+"?secret_data="+url.QueryEscape(verifier), nil, nil)
	if credRec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", credRec.Code, credRec.Body.String())
	}
	var credBody struct {
		Data struct {
			ExecutionStatus       executionStatusBody       `json:"executionStatus"`
			SubjectIdentification subjectIdentificationBody `json:"subjectIdentification"`
		} `json:"data"`
	}
	testutil.DecodeJSON(t, credRec, &credBody)
	if credBody.Data.ExecutionStatus.CurrentStatus != "PENDING_SIGNATURES" {
		t.Errorf("currentStatus = %v", credBody.Data.ExecutionStatus.CurrentStatus)
	}
	if credBody.Data.SubjectIdentification.IdentificationKey != "46404461013" {
		t.Errorf("identificationKey = %v", credBody.Data.SubjectIdentification.IdentificationKey)
	}

	digest := sha256Base64("hello world")
	sigRec := app.Do(t, "POST", "/integraicp/c/test-channel/icp/v3/signatures", nil, map[string]any{
		"credentialId": credentialID,
		"secretType":   "code_verifier",
		"secretData":   verifier,
		"requests":     []map[string]any{{"contentId": "doc_001", "contentDigest": digest, "signaturePolicy": "RAW"}},
	})
	if sigRec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", sigRec.Code, sigRec.Body.String())
	}
	var sigBody struct {
		Data struct {
			ExecutionStatus executionStatusBody `json:"executionStatus"`
			Signatures      []map[string]any    `json:"signatures"`
		} `json:"data"`
	}
	testutil.DecodeJSON(t, sigRec, &sigBody)
	if sigBody.Data.ExecutionStatus.CurrentStatus != "COMPLETED_WITH_SUCCESS" {
		t.Errorf("currentStatus = %v", sigBody.Data.ExecutionStatus.CurrentStatus)
	}
	if len(sigBody.Data.Signatures) != 1 {
		t.Fatalf("expected 1 signature, got %d", len(sigBody.Data.Signatures))
	}
	if sigBody.Data.Signatures[0]["contentId"] != "doc_001" {
		t.Errorf("contentId = %v", sigBody.Data.Signatures[0]["contentId"])
	}
	if _, ok := sigBody.Data.Signatures[0]["signedContent"].(string); !ok {
		t.Errorf("signedContent not a string: %v", sigBody.Data.Signatures[0]["signedContent"])
	}
}

func TestRejectsWrongPkceVerifier(t *testing.T) {
	app := testutil.New(t, 0)
	_, challenge := pkce()
	credentialID := authenticateAndGetCredentialID(t, app, challenge)

	rec := app.Do(t, "GET", "/integraicp/c/test-channel/icp/v3/credentials/"+credentialID+"?secret_data=wrong-verifier", nil, nil)
	if rec.Code != 403 {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.Error.Code != 403201 {
		t.Errorf("error.code = %v", body.Error.Code)
	}
}

func TestUnknownCredential404(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "GET", "/integraicp/c/test-channel/icp/v3/credentials/does-not-exist?secret_data=x", nil, nil)
	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.Error.Code != 404000 {
		t.Errorf("error.code = %v", body.Error.Code)
	}
}

func TestRejectsMalformedContentDigest(t *testing.T) {
	app := testutil.New(t, 0)
	verifier, challenge := pkce()
	credentialID := authenticateAndGetCredentialID(t, app, challenge)

	rec := app.Do(t, "POST", "/integraicp/c/test-channel/icp/v3/signatures", nil, map[string]any{
		"credentialId": credentialID,
		"secretData":   verifier,
		"requests":     []map[string]any{{"contentDigest": "not-a-real-digest"}},
	})
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.Error.Code != 400204 {
		t.Errorf("error.code = %v", body.Error.Code)
	}
}

func TestClearancesListWhenAutostartOmitted(t *testing.T) {
	app := testutil.New(t, 0)
	_, challenge := pkce()

	q := url.Values{"secret_data": {challenge}, "callback_uri": {"https://my.app/callback"}}
	rec := app.Do(t, "GET", "/integraicp/c/test-channel/icp/v3/authentications?"+q.Encode(), nil, nil)
	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			ExecutionStatus executionStatusBody `json:"executionStatus"`
			Clearances      []map[string]any    `json:"clearances"`
		} `json:"data"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.Data.ExecutionStatus.CurrentStatus != "PENDING_AUTHORIZATON" { //nolint:misspell // official IntegraICP contract value
		t.Errorf("currentStatus = %v", body.Data.ExecutionStatus.CurrentStatus)
	}
	if len(body.Data.Clearances) == 0 {
		t.Errorf("expected clearances")
	}
}

func TestRejectsMissingRequiredQueryParam(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "GET", "/integraicp/c/test-channel/icp/v3/authentications?secret_data=abc", nil, nil)
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.Error.Code != 400101 {
		t.Errorf("error.code = %v", body.Error.Code)
	}
}

func TestSignaturesUnknownCredential404(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "POST", "/integraicp/c/test-channel/icp/v3/signatures", nil, map[string]any{
		"credentialId": "does-not-exist",
		"secretData":   "whatever",
		"requests":     []map[string]any{{"contentDigest": sha256Base64("hi")}},
	})
	if rec.Code != 404 {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.Error.Code != 404000 {
		t.Errorf("error.code = %v", body.Error.Code)
	}
}

func TestRejectsSignaturesBodyMissingFields(t *testing.T) {
	app := testutil.New(t, 0)
	rec := app.Do(t, "POST", "/integraicp/c/test-channel/icp/v3/signatures", nil, map[string]any{
		"requests": []map[string]any{},
	})
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	var body struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	testutil.DecodeJSON(t, rec, &body)
	if body.Error.Code != 400000 {
		t.Errorf("error.code = %v", body.Error.Code)
	}
}
