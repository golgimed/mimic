package sncr

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// sessionTTL matches the real API's 30s one-time session_token lifetime.
const sessionTTL = 30 * time.Second

// Identity is the simulated Gov.br-authenticated prescriber a session or
// access token is bound to.
type Identity struct {
	Conselho  string
	UF        string
	Documento string
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

// CreateAuthSession mints a one-time session_token bound to identity, plus
// the access_token it will resolve to on exchange, mirroring the real
// SNCR API's post-Keycloak-callback step.
func (s *Store) CreateAuthSession(identity Identity) (sessionToken string, err error) {
	sessionToken = uuid.NewString()
	accessToken := "sncr_" + uuid.NewString()
	expiresAt := nowUTC().Add(sessionTTL).Format(time.RFC3339Nano)

	_, err = s.db.Exec(
		`INSERT INTO sncr_auth_sessions (session_token, access_token, conselho, uf, documento, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		sessionToken, accessToken, identity.Conselho, identity.UF, identity.Documento, expiresAt,
	)
	if err != nil {
		return "", err
	}
	return sessionToken, nil
}

// ErrSessionInvalid covers both "no such session" and "expired" — the real
// API returns the same "Sessão inválida ou expirada" message for both.
var ErrSessionInvalid = errors.New("session invalid or expired")

// ExchangeSession consumes sessionToken (single-use — deleted immediately)
// and, if it was valid and unexpired, issues the access token it was bound
// to.
func (s *Store) ExchangeSession(sessionToken string) (accessToken string, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		token, conselho, uf, documento, expiresAt string
	)
	row := tx.QueryRow(
		`SELECT access_token, conselho, uf, documento, expires_at FROM sncr_auth_sessions WHERE session_token = ?`,
		sessionToken,
	)
	if err := row.Scan(&token, &conselho, &uf, &documento, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSessionInvalid
		}
		return "", err
	}

	if _, err := tx.Exec(`DELETE FROM sncr_auth_sessions WHERE session_token = ?`, sessionToken); err != nil {
		return "", err
	}

	expiry, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return "", err
	}
	if nowUTC().After(expiry) {
		return "", ErrSessionInvalid
	}

	if _, err := tx.Exec(
		`INSERT INTO sncr_access_tokens (access_token, conselho, uf, documento) VALUES (?, ?, ?, ?)`,
		token, conselho, uf, documento,
	); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

// ResolveAccessToken returns the identity an access token was issued to, or
// found=false if it's unknown.
func (s *Store) ResolveAccessToken(accessToken string) (identity Identity, found bool, err error) {
	row := s.db.QueryRow(
		`SELECT conselho, uf, documento FROM sncr_access_tokens WHERE access_token = ?`, accessToken,
	)
	err = row.Scan(&identity.Conselho, &identity.UF, &identity.Documento)
	if errors.Is(err, sql.ErrNoRows) {
		return Identity{}, false, nil
	}
	if err != nil {
		return Identity{}, false, err
	}
	return identity, true, nil
}

// sequenceRow mirrors one row of sncr_numbering_sequences.
type sequenceRow struct {
	ID               string
	NextSequence     int64
	IssuedInPeriod   int64
	RequestsInPeriod int64
}

// getOrCreateSequence returns the (kind, receitaOrTipo, identity, periodKey)
// balance row, creating it with zeroed counters if it doesn't exist yet.
func (s *Store) getOrCreateSequence(kind, receitaOrTipo string, identity Identity, periodKey string) (*sequenceRow, error) {
	row, found, err := s.selectSequence(kind, receitaOrTipo, identity, periodKey)
	if err != nil {
		return nil, err
	}
	if found {
		return row, nil
	}

	_, err = s.db.Exec(
		`INSERT INTO sncr_numbering_sequences (id, kind, receita_or_tipo, conselho, uf, documento, period_key)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT (kind, receita_or_tipo, conselho, uf, documento, period_key) DO NOTHING`,
		uuid.NewString(), kind, receitaOrTipo, identity.Conselho, identity.UF, identity.Documento, periodKey,
	)
	if err != nil {
		return nil, err
	}

	row, _, err = s.selectSequence(kind, receitaOrTipo, identity, periodKey)
	return row, err
}

func (s *Store) selectSequence(kind, receitaOrTipo string, identity Identity, periodKey string) (*sequenceRow, bool, error) {
	var row sequenceRow
	err := s.db.QueryRow(
		`SELECT id, next_sequence, issued_in_period, requests_in_period FROM sncr_numbering_sequences
		 WHERE kind = ? AND receita_or_tipo = ? AND conselho = ? AND uf = ? AND documento = ? AND period_key = ?`,
		kind, receitaOrTipo, identity.Conselho, identity.UF, identity.Documento, periodKey,
	).Scan(&row.ID, &row.NextSequence, &row.IssuedInPeriod, &row.RequestsInPeriod)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &row, true, nil
}

// AllocateNotificationNumbers reserves quantidade discrete numbers against
// the (receita, identity, today) daily balance, returning the sequence
// numbers to format and the remaining daily balance after this call.
// ok=false means the caller must not issue numbers (limit already reached).
func (s *Store) AllocateNotificationNumbers(receita string, identity Identity, quantidade int64) (startSeq int64, remaining int64, ok bool, err error) {
	today := nowUTC().Format("2006-01-02")
	row, err := s.getOrCreateSequence("notificacao", receita, identity, today)
	if err != nil {
		return 0, 0, false, err
	}

	const dailyLimit = 50
	before := dailyLimit - row.IssuedInPeriod
	if before <= 0 || quantidade > before {
		return 0, before, false, nil
	}

	startSeq = row.NextSequence
	_, err = s.db.Exec(
		`UPDATE sncr_numbering_sequences
		 SET next_sequence = next_sequence + ?, issued_in_period = issued_in_period + ?, updated_at = datetime('now')
		 WHERE id = ?`,
		quantidade, quantidade, row.ID,
	)
	if err != nil {
		return 0, 0, false, err
	}
	return startSeq, dailyLimit - (row.IssuedInPeriod + quantidade), true, nil
}

// especialAllocationSize is the fixed block size the real API always
// allocates per successful especial-retencao call.
const especialAllocationSize = 1000

// especialMonthlyRequestLimit and especialMonthlyNumberLimit are combined
// across RCE+RET per prescriber per month.
const (
	especialMonthlyRequestLimit = 3
	especialMonthlyNumberLimit  = 3000
)

// AllocateEspecialRetencaoRange reserves a 1000-number range for tipo
// against the combined RCE+RET monthly limit, returning the range's
// starting sequence number. ok=false means the monthly limit is already
// reached.
func (s *Store) AllocateEspecialRetencaoRange(tipo string, identity Identity) (startSeq int64, ok bool, err error) {
	yearMonth := nowUTC().Format("2006-01")

	combined, err := s.getOrCreateSequence("especial_retencao", "COMBINED", identity, yearMonth)
	if err != nil {
		return 0, false, err
	}
	if combined.RequestsInPeriod >= especialMonthlyRequestLimit ||
		combined.IssuedInPeriod+especialAllocationSize > especialMonthlyNumberLimit {
		return 0, false, nil
	}

	typeSeq, err := s.getOrCreateSequence("especial_retencao", tipo, identity, yearMonth)
	if err != nil {
		return 0, false, err
	}

	if _, err := s.db.Exec(
		`UPDATE sncr_numbering_sequences
		 SET requests_in_period = requests_in_period + 1, issued_in_period = issued_in_period + ?, updated_at = datetime('now')
		 WHERE id = ?`,
		especialAllocationSize, combined.ID,
	); err != nil {
		return 0, false, err
	}
	if _, err := s.db.Exec(
		`UPDATE sncr_numbering_sequences
		 SET next_sequence = next_sequence + ?, updated_at = datetime('now')
		 WHERE id = ?`,
		especialAllocationSize, typeSeq.ID,
	); err != nil {
		return 0, false, err
	}

	return typeSeq.NextSequence, true, nil
}

// dashboard support — surfaces issued access tokens as the provider's
// dashboard items, the closest thing SNCR has to a "record" (there is no
// submission/registration object, see README).
type AccessTokenRecord struct {
	AccessToken string `json:"accessToken"`
	Conselho    string `json:"conselho"`
	UF          string `json:"uf"`
	Documento   string `json:"documento"`
	CreatedAt   string `json:"createdAt"`
}

func (s *Store) ListAccessTokens() ([]AccessTokenRecord, error) {
	rows, err := s.db.Query(
		`SELECT access_token, conselho, uf, documento, created_at FROM sncr_access_tokens ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []AccessTokenRecord{}
	for rows.Next() {
		var rec AccessTokenRecord
		if err := rows.Scan(&rec.AccessToken, &rec.Conselho, &rec.UF, &rec.Documento, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) GetAccessTokenRecord(accessToken string) (*AccessTokenRecord, error) {
	var rec AccessTokenRecord
	err := s.db.QueryRow(
		`SELECT access_token, conselho, uf, documento, created_at FROM sncr_access_tokens WHERE access_token = ?`,
		accessToken,
	).Scan(&rec.AccessToken, &rec.Conselho, &rec.UF, &rec.Documento, &rec.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}
