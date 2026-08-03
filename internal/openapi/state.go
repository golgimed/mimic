package openapi

import (
	"database/sql"
	"encoding/json"
)

// Store persists CRUD-shaped resources for OpenAPI-mocked routes. Unlike
// zenvia/integraicp's Store, this is one generic table shared by every spec:
// schemas are only known at runtime from arbitrary dropped-in spec files, so
// per-schema tables aren't viable here. specName scopes resources so two
// specs can both declare a "pets" resource type without colliding.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Resource is a persisted CRUD-shaped resource, keyed by (specName,
// resourceType, id).
type Resource struct {
	ID      string
	Payload map[string]any
}

func scanResource(row interface{ Scan(dest ...any) error }) (*Resource, error) {
	var (
		res         Resource
		payloadJSON string
	)
	if err := row.Scan(&res.ID, &payloadJSON); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(payloadJSON), &res.Payload); err != nil {
		return nil, err
	}
	return &res, nil
}

func (s *Store) Create(specName, resourceType, id string, payload map[string]any) (*Resource, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(
		`INSERT INTO openapi_resources (spec_name, resource_type, resource_id, payload_json)
		 VALUES (?, ?, ?, ?)`,
		specName, resourceType, id, string(payloadJSON),
	)
	if err != nil {
		return nil, err
	}
	return s.Get(specName, resourceType, id)
}

func (s *Store) Get(specName, resourceType, id string) (*Resource, error) {
	row := s.db.QueryRow(
		`SELECT resource_id, payload_json FROM openapi_resources
		 WHERE spec_name = ? AND resource_type = ? AND resource_id = ?`,
		specName, resourceType, id,
	)
	res, err := scanResource(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return res, err
}

func (s *Store) List(specName, resourceType string) ([]Resource, error) {
	rows, err := s.db.Query(
		`SELECT resource_id, payload_json FROM openapi_resources
		 WHERE spec_name = ? AND resource_type = ? ORDER BY created_at`,
		specName, resourceType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Resource{}
	for rows.Next() {
		res, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *res)
	}
	return out, rows.Err()
}

func (s *Store) Update(specName, resourceType, id string, payload map[string]any) (*Resource, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	res, err := s.db.Exec(
		`UPDATE openapi_resources SET payload_json = ?, updated_at = datetime('now')
		 WHERE spec_name = ? AND resource_type = ? AND resource_id = ?`,
		string(payloadJSON), specName, resourceType, id,
	)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	return s.Get(specName, resourceType, id)
}

func (s *Store) Delete(specName, resourceType, id string) (bool, error) {
	res, err := s.db.Exec(
		`DELETE FROM openapi_resources WHERE spec_name = ? AND resource_type = ? AND resource_id = ?`,
		specName, resourceType, id,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
