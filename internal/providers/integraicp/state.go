package integraicp

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

type Credential struct {
	ID            string          `json:"id"`
	ChannelID     string          `json:"channelId"`
	SubjectKey    *string         `json:"subjectKey"`
	SubjectType   *string         `json:"subjectType"`
	CodeChallenge string          `json:"codeChallenge"`
	CallbackURI   string          `json:"callbackUri"`
	Certificate   FakeCertificate `json:"certificate"`
	CreatedAt     string          `json:"createdAt"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) scanCredential(row interface {
	Scan(dest ...any) error
}) (*Credential, error) {
	var (
		c        Credential
		certJSON string
	)
	if err := row.Scan(&c.ID, &c.ChannelID, &c.SubjectKey, &c.SubjectType, &c.CodeChallenge, &c.CallbackURI, &certJSON, &c.CreatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(certJSON), &c.Certificate); err != nil {
		return nil, err
	}
	return &c, nil
}

type CreateCredentialInput struct {
	ChannelID     string
	SubjectKey    *string
	SubjectType   *string
	CodeChallenge string
	CallbackURI   string
	Certificate   FakeCertificate
}

func (s *Store) CreateCredential(input CreateCredentialInput) (*Credential, error) {
	id := uuid.NewString()
	certJSON, err := json.Marshal(input.Certificate)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(
		`INSERT INTO integraicp_credentials (id, channel_id, subject_key, subject_type, code_challenge, callback_uri, certificate_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, input.ChannelID, input.SubjectKey, input.SubjectType, input.CodeChallenge, input.CallbackURI, string(certJSON),
	)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(
		`SELECT id, channel_id, subject_key, subject_type, code_challenge, callback_uri, certificate_json, created_at
		 FROM integraicp_credentials WHERE id = ?`, id,
	)
	return s.scanCredential(row)
}

func (s *Store) ListCredentials() ([]Credential, error) {
	rows, err := s.db.Query(
		`SELECT id, channel_id, subject_key, subject_type, code_challenge, callback_uri, certificate_json, created_at
		 FROM integraicp_credentials ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []Credential{}
	for rows.Next() {
		c, err := s.scanCredential(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *Store) GetCredential(id string) (*Credential, bool, error) {
	row := s.db.QueryRow(
		`SELECT id, channel_id, subject_key, subject_type, code_challenge, callback_uri, certificate_json, created_at
		 FROM integraicp_credentials WHERE id = ?`, id,
	)
	c, err := s.scanCredential(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return c, true, nil
}
