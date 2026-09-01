package brymedical

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// CreateToken issues a pré-autorização token good for uses uses, expiring
// after ttl. BRy's real KMS tracks remaining uses server-side rather than
// trusting the caller, so ConsumeToken decrements and deletes on exhaustion.
func (s *Store) CreateToken(uuidChave string, uses int, ttl time.Duration) (token string, quantidadeRestante int, err error) {
	token = uuid.NewString()
	expiresAt := time.Now().UTC().Add(ttl).Format(time.RFC3339Nano)
	if _, err = s.db.Exec(
		`INSERT INTO bry_medical_preauth_tokens (token, uuid_chave, uses_remaining, expires_at) VALUES (?, ?, ?, ?)`,
		token, uuidChave, uses, expiresAt,
	); err != nil {
		return "", 0, err
	}
	return token, uses, nil
}

// ConsumeToken validates token against remaining uses and expiry, then spends
// one use. ok is false for an unknown, expired, or exhausted token — the
// medical signing handler treats all three as the same documented failure.
func (s *Store) ConsumeToken(token string) (ok bool, err error) {
	var expiresAt string
	var usesRemaining int
	err = s.db.QueryRow(
		`SELECT expires_at, uses_remaining FROM bry_medical_preauth_tokens WHERE token = ?`, token,
	).Scan(&expiresAt, &usesRemaining)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	expiry, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return false, err
	}
	if time.Now().UTC().After(expiry) || usesRemaining <= 0 {
		return false, nil
	}

	if usesRemaining <= 1 {
		_, err = s.db.Exec(`DELETE FROM bry_medical_preauth_tokens WHERE token = ?`, token)
	} else {
		_, err = s.db.Exec(`UPDATE bry_medical_preauth_tokens SET uses_remaining = uses_remaining - 1 WHERE token = ?`, token)
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
