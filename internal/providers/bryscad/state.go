package bryscad

import (
	"database/sql"
	"encoding/json"
	"errors"

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
