package integraicp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/golgimed/mimic/internal/shared/httpx"
)

func sendError(w http.ResponseWriter, status, code int, message string) {
	httpx.WriteJSON(w, status, map[string]any{
		"error": map[string]any{"code": code, "message": message},
	})
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func authenticationsHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, ok := parseAuthenticationsQuery(r.URL.Query())
		if !ok {
			sendError(w, http.StatusBadRequest, 400101, "Invalid Channel.")
			return
		}
		channelID := r.PathValue("channelId")
		subjectName := "Simulated Subject"
		if query.SubjectKey != nil {
			subjectName = *query.SubjectKey
		}

		if query.Autostart {
			cert := BuildFakeCertificate(subjectName)
			credential, err := store.CreateCredential(CreateCredentialInput{
				ChannelID:     channelID,
				SubjectKey:    query.SubjectKey,
				SubjectType:   query.SubjectType,
				CodeChallenge: query.SecretData,
				CallbackURI:   query.CallbackURI,
				Certificate:   cert,
			})
			if err != nil {
				sendError(w, http.StatusInternalServerError, 500000, err.Error())
				return
			}

			location, err := url.Parse(query.CallbackURI)
			if err != nil {
				sendError(w, http.StatusInternalServerError, 500000, err.Error())
				return
			}
			// Restrict to http(s) callback_uri values: real IntegraICP clients
			// only ever register http(s) callbacks, and rejecting anything
			// else (javascript:, data:, etc.) closes the open-redirect vector
			// without narrowing the documented contract.
			if location.Scheme != "http" && location.Scheme != "https" {
				sendError(w, http.StatusBadRequest, 400101, "Invalid Channel.")
				return
			}
			q := location.Query()
			q.Set("credentialId", credential.ID)
			location.RawQuery = q.Encode()

			w.Header().Set("Location", location.String())
			w.WriteHeader(http.StatusFound)
			return
		}

		clearanceID := uuid.NewString()
		lifetime := int64(86400)
		if query.ClearanceLifetime != nil {
			lifetime = *query.ClearanceLifetime
		}
		expireTimestamp := time.Now().UTC().Add(time.Duration(lifetime) * time.Second).Format(time.RFC3339Nano)

		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"requestId":          uuid.NewString(),
				"channelName":        "Mimic",
				"channelDescription": "IntegraICP - Simulated Broker",
				"expireTimestamp":    expireTimestamp,
				"executionStatus": map[string]any{
					"currentStatus":      "PENDING_AUTHORIZATON", //nolint:misspell // matches IntegraICP's official (misspelled) contract value, not a typo
					"requestTimestamp":   nowISO(),
					"executionTimestamp": nowISO(),
				},
				"clearances": []map[string]any{
					{
						"clearanceId":       clearanceID,
						"productName":       "Simulated Provider",
						"providerName":      "Simulator",
						"clearanceEndpoint": "https://simulated-provider.local/clearance/" + clearanceID,
						"clearanceType":     "IDENTIFICATION",
					},
				},
			},
		})
	}
}

func credentialsHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query, ok := parseCredentialsQuery(r.URL.Query())
		if !ok {
			sendError(w, http.StatusBadRequest, 400101, "Invalid request.")
			return
		}

		credential, found, err := store.GetCredential(r.PathValue("credentialId"))
		if err != nil {
			sendError(w, http.StatusInternalServerError, 500000, err.Error())
			return
		}
		if !found {
			sendError(w, http.StatusNotFound, 404000, "Credential Not Found")
			return
		}

		if !VerifyPkce(query.SecretData, credential.CodeChallenge) {
			sendError(w, http.StatusForbidden, 403201, "Invalid Verification Code (PKCE).")
			return
		}

		identificationKey, identificationType := subjectIdentification(credential)

		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"credentialId": credential.ID,
				"executionStatus": map[string]any{
					"currentStatus":      "PENDING_SIGNATURES",
					"requestTimestamp":   credential.CreatedAt,
					"executionTimestamp": nowISO(),
				},
				"subjectIdentification": map[string]any{
					"identificationKey":  identificationKey,
					"identificationType": identificationType,
				},
				"certificateInformation": credential.Certificate,
			},
		})
	}
}

func isValidSha256Base64(value string) bool {
	buf, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	return len(buf) == 32 && base64.StdEncoding.EncodeToString(buf) == value
}

func signaturesHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleSignatures(store, w, r)
	}
}

type signatureResult struct {
	SignatureID        string  `json:"signatureId"`
	ContentID          *string `json:"contentId,omitempty"`
	ContentDigest      string  `json:"contentDigest"`
	ContentDescription *string `json:"contentDescription,omitempty"`
	SignedContent      string  `json:"signedContent"`
	SignatureTimestamp string  `json:"signatureTimestamp"`
}

func handleSignatures(store *Store, w http.ResponseWriter, r *http.Request) {
	var body SignaturesBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !body.valid() {
		sendError(w, http.StatusBadRequest, 400000, "Invalid request body.")
		return
	}

	credential, found, err := store.GetCredential(body.CredentialID)
	if err != nil {
		sendError(w, http.StatusInternalServerError, 500000, err.Error())
		return
	}
	if !found {
		sendError(w, http.StatusNotFound, 404000, "Credential Not Found")
		return
	}

	if !VerifyPkce(body.SecretData, credential.CodeChallenge) {
		sendError(w, http.StatusForbidden, 403201, "Invalid Verification Code (PKCE).")
		return
	}

	for _, item := range body.Requests {
		if !isValidSha256Base64(item.ContentDigest) {
			sendError(w, http.StatusBadRequest, 400204, "Invalid Content Digest. Not SHA256 Base64 Encoded.")
			return
		}
	}

	signatures := buildSignatures(credential.ID, body.Requests)
	identificationKey, identificationType := subjectIdentification(credential)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"requestId": uuid.NewString(),
			"executionStatus": map[string]any{
				"currentStatus":      "COMPLETED_WITH_SUCCESS",
				"requestTimestamp":   nowISO(),
				"executionTimestamp": nowISO(),
			},
			"subjectIdentification": map[string]any{
				"identificationKey":  identificationKey,
				"identificationType": identificationType,
			},
			"certificateInformation": credential.Certificate,
			"signatures":             signatures,
		},
	})
}

func buildSignatures(credentialID string, requests []SignatureRequestItem) []signatureResult {
	signatures := make([]signatureResult, 0, len(requests))
	for _, item := range requests {
		signatureID := uuid.NewString()
		sum := sha256.Sum256([]byte(credentialID + ":" + item.ContentDigest + ":" + signatureID))
		signatures = append(signatures, signatureResult{
			SignatureID:        signatureID,
			ContentID:          item.ContentID,
			ContentDigest:      item.ContentDigest,
			ContentDescription: item.ContentDescription,
			SignedContent:      base64.StdEncoding.EncodeToString(sum[:]),
			SignatureTimestamp: nowISO(),
		})
	}
	return signatures
}

// subjectIdentification returns credential's subject key/type, falling back
// to the same simulator defaults used by credentialsHandler.
func subjectIdentification(credential *Credential) (key, kind string) {
	key = "00000000000"
	if credential.SubjectKey != nil {
		key = *credential.SubjectKey
	}
	kind = "CPF"
	if credential.SubjectType != nil {
		kind = *credential.SubjectType
	}
	return key, kind
}
