package bryscad

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Collection struct {
	Chave     string         `json:"chave"`
	Situacao  string         `json:"situacao"`
	Payload   map[string]any `json:"-"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
}

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) CreateCollection(payload map[string]any) (*Collection, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	chave := uuid.NewString()
	if _, err := s.db.Exec(`INSERT INTO bry_scad_collections (chave, payload_json) VALUES (?, ?)`, chave, string(b)); err != nil {
		return nil, err
	}
	return s.GetCollection(chave)
}

func scanCollection(row interface{ Scan(...any) error }) (*Collection, error) {
	var c Collection
	var payload string
	if err := row.Scan(&c.Chave, &payload, &c.Situacao, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(payload), &c.Payload); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) GetCollection(chave string) (*Collection, error) {
	c, err := scanCollection(s.db.QueryRow(`SELECT chave, payload_json, situacao, created_at, updated_at FROM bry_scad_collections WHERE chave = ?`, chave))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

func (s *Store) ListCollections() ([]Collection, error) {
	rows, err := s.db.Query(`SELECT chave, payload_json, situacao, created_at, updated_at FROM bry_scad_collections ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	collections := []Collection{}
	for rows.Next() {
		c, err := scanCollection(rows)
		if err != nil {
			return nil, err
		}
		collections = append(collections, *c)
	}
	return collections, rows.Err()
}

func (s *Store) TransitionCollection(chave, situacao string) (*Collection, error) {
	result, err := s.db.Exec(`UPDATE bry_scad_collections SET situacao = ?, updated_at = datetime('now') WHERE chave = ?`, situacao, chave)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		return nil, err
	}
	return s.GetCollection(chave)
}

// CompleteCollection simulates every signing participant concluding the
// collection: it stamps each "assinante" participant's situacaoAssinatura as
// CONCLUIDO (read back by GET .../participantes), appends a matching
// "Assinatura" entry per participant (read back by GET .../historico), and
// fabricates one signed-document record per submitted document (read back by
// GET .../documentos-assinados) — the three authenticated re-fetches
// golgimed's BRy adapter relies on instead of trusting the webhook body
// (ParseWebhook's SECURITY note). Reviewer-only participants (assinante ==
// false) are left untouched, matching the real provider's distinction.
func (s *Store) CompleteCollection(chave string) (*Collection, error) {
	c, err := s.GetCollection(chave)
	if err != nil || c == nil {
		return c, err
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")

	participants, _ := c.Payload["participantes"].([]any)
	historico := make([]map[string]any, 0, len(participants))
	for _, raw := range participants {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		assinante, _ := p["assinante"].(bool)
		if !assinante {
			continue
		}
		p["situacaoAssinatura"] = map[string]any{"chave": "CONCLUIDO", "descricao": "CONCLUIDO"}
		historico = append(historico, map[string]any{
			"nome":         p["nome"],
			"codigo":       p["codigo"],
			"processo":     "Assinatura",
			"dataExecucao": now,
		})
	}
	c.Payload["participantes"] = participants
	c.Payload["_historico"] = historico

	documentos, _ := c.Payload["documentos"].([]any)
	assinados := make([]map[string]any, 0, len(documentos))
	for _, raw := range documentos {
		d, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		nome, _ := d["nome"].(string)
		size := 0
		if b64, ok := d["base64"].(string); ok {
			if decoded, err := base64.StdEncoding.DecodeString(b64); err == nil {
				size = len(decoded)
			}
		}
		assinados = append(assinados, map[string]any{
			"nome":           nome,
			"chaveDocumento": uuid.NewString(),
			"tamanho":        size,
		})
	}
	c.Payload["_documentosAssinados"] = assinados

	payloadJSON, err := json.Marshal(c.Payload)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(
		`UPDATE bry_scad_collections SET payload_json = ?, situacao = 'CONCLUIDO', updated_at = datetime('now') WHERE chave = ?`,
		string(payloadJSON), chave,
	); err != nil {
		return nil, err
	}
	return s.GetCollection(chave)
}
