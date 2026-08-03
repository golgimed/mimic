package integraicp

import (
	"net/url"
	"strconv"
)

// Contract taken from docs/vendor/integraicp-api-reference-v3.md.

type AuthenticationsQuery struct {
	SubjectKey        *string
	SubjectType       *string
	SecretData        string
	CallbackURI       string
	Autostart         bool
	ClearanceLifetime *int64
}

func parseAuthenticationsQuery(q url.Values) (*AuthenticationsQuery, bool) {
	secretData := q.Get("secret_data")
	callbackURI := q.Get("callback_uri")
	if secretData == "" || callbackURI == "" {
		return nil, false
	}
	if _, err := url.ParseRequestURI(callbackURI); err != nil {
		return nil, false
	}

	out := &AuthenticationsQuery{SecretData: secretData, CallbackURI: callbackURI}

	if v := q.Get("subject_key"); v != "" {
		out.SubjectKey = &v
	}
	if v := q.Get("subject_type"); v != "" {
		out.SubjectType = &v
	}

	if v := q.Get("autostart"); v != "" {
		if v != "true" && v != "false" {
			return nil, false
		}
		out.Autostart = v == "true"
	}

	if v := q.Get("clearance_lifetime"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, false
		}
		out.ClearanceLifetime = &n
	}

	return out, true
}

type CredentialsQuery struct {
	SecretData string
}

func parseCredentialsQuery(q url.Values) (*CredentialsQuery, bool) {
	secretData := q.Get("secret_data")
	if secretData == "" {
		return nil, false
	}
	return &CredentialsQuery{SecretData: secretData}, true
}

type SignatureRequestItem struct {
	ContentID          *string `json:"contentId,omitempty"`
	ContentDigest      string  `json:"contentDigest"`
	ContentDescription *string `json:"contentDescription,omitempty"`
}

type SignaturesBody struct {
	CredentialID string                  `json:"credentialId"`
	SecretData   string                  `json:"secretData"`
	Requests     []SignatureRequestItem `json:"requests"`
}

func (b SignaturesBody) valid() bool {
	return b.CredentialID != "" && b.SecretData != "" && len(b.Requests) >= 1
}
