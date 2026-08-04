package zenvia

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

type Message struct {
	ID         string  `json:"id"`
	ExternalID *string `json:"externalId"`
	Channel    string  `json:"channel"`
	Direction  string  `json:"direction"`
	From       string  `json:"from"`
	To         string  `json:"to"`
	Contents   []any   `json:"contents"`
	Status     string  `json:"status"`
	CreatedAt  string  `json:"createdAt"`
}

type Subscription struct {
	ID                string            `json:"id"`
	EventType         string            `json:"eventType"`
	WebhookURL        string            `json:"webhookUrl"`
	WebhookHeaders    map[string]string `json:"webhookHeaders"`
	CriteriaChannel   string            `json:"criteriaChannel"`
	CriteriaDirection *string           `json:"criteriaDirection"`
	Status            string            `json:"status"`
	CreatedAt         string            `json:"createdAt"`
	UpdatedAt         string            `json:"updatedAt"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func scanMessage(row interface{ Scan(dest ...any) error }) (*Message, error) {
	var (
		m            Message
		contentsJSON string
	)
	if err := row.Scan(&m.ID, &m.ExternalID, &m.Channel, &m.Direction, &m.From, &m.To, &contentsJSON, &m.Status, &m.CreatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(contentsJSON), &m.Contents); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Store) CreateMessage(channel string, externalID *string, from, to string, contents []any) (*Message, error) {
	id := uuid.NewString()
	contentsJSON, err := json.Marshal(contents)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(
		`INSERT INTO zenvia_messages (id, external_id, channel, direction, sender, recipient, contents_json, status)
		 VALUES (?, ?, ?, 'OUT', ?, ?, ?, 'ACCEPTED')`,
		id, externalID, channel, from, to, string(contentsJSON),
	)
	if err != nil {
		return nil, err
	}
	return s.GetMessage(id)
}

func (s *Store) ListMessages() ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, external_id, channel, direction, sender, recipient, contents_json, status, created_at
		 FROM zenvia_messages ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []Message{}
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

func (s *Store) GetMessage(id string) (*Message, error) {
	row := s.db.QueryRow(
		`SELECT id, external_id, channel, direction, sender, recipient, contents_json, status, created_at
		 FROM zenvia_messages WHERE id = ?`, id,
	)
	m, err := scanMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func (s *Store) UpdateMessageStatus(id, status string) error {
	_, err := s.db.Exec("UPDATE zenvia_messages SET status = ? WHERE id = ?", status, id)
	return err
}

func scanSubscription(row interface{ Scan(dest ...any) error }) (*Subscription, error) {
	var (
		sub         Subscription
		headersJSON sql.NullString
		criteriaDir sql.NullString
	)
	if err := row.Scan(&sub.ID, &sub.EventType, &sub.WebhookURL, &headersJSON, &sub.CriteriaChannel, &criteriaDir, &sub.Status, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
		return nil, err
	}
	if headersJSON.Valid {
		if err := json.Unmarshal([]byte(headersJSON.String), &sub.WebhookHeaders); err != nil {
			return nil, err
		}
	}
	if criteriaDir.Valid {
		sub.CriteriaDirection = &criteriaDir.String
	}
	return &sub, nil
}

type CreateSubscriptionInput struct {
	EventType         string
	WebhookURL        string
	WebhookHeaders    map[string]string
	CriteriaChannel   string
	CriteriaDirection *string
}

func (s *Store) CreateSubscription(input CreateSubscriptionInput) (*Subscription, error) {
	id := uuid.NewString()
	var headersJSON *string
	if input.WebhookHeaders != nil {
		b, err := json.Marshal(input.WebhookHeaders)
		if err != nil {
			return nil, err
		}
		str := string(b)
		headersJSON = &str
	}
	_, err := s.db.Exec(
		`INSERT INTO zenvia_subscriptions (id, event_type, webhook_url, webhook_headers_json, criteria_channel, criteria_direction, status)
		 VALUES (?, ?, ?, ?, ?, ?, 'ACTIVE')`,
		id, input.EventType, input.WebhookURL, headersJSON, input.CriteriaChannel, input.CriteriaDirection,
	)
	if err != nil {
		return nil, err
	}
	return s.GetSubscription(id)
}

func (s *Store) ListSubscriptions() ([]Subscription, error) {
	rows, err := s.db.Query(
		`SELECT id, event_type, webhook_url, webhook_headers_json, criteria_channel, criteria_direction, status, created_at, updated_at
		 FROM zenvia_subscriptions ORDER BY created_at`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []Subscription{}
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sub)
	}
	return out, rows.Err()
}

func (s *Store) GetSubscription(id string) (*Subscription, error) {
	row := s.db.QueryRow(
		`SELECT id, event_type, webhook_url, webhook_headers_json, criteria_channel, criteria_direction, status, created_at, updated_at
		 FROM zenvia_subscriptions WHERE id = ?`, id,
	)
	sub, err := scanSubscription(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return sub, err
}

func (s *Store) DeleteSubscription(id string) (bool, error) {
	res, err := s.db.Exec("DELETE FROM zenvia_subscriptions WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (s *Store) FindActiveSubscriptionsForChannel(channel string) ([]Subscription, error) {
	rows, err := s.db.Query(
		`SELECT id, event_type, webhook_url, webhook_headers_json, criteria_channel, criteria_direction, status, created_at, updated_at
		 FROM zenvia_subscriptions
		 WHERE event_type = 'MESSAGE_STATUS' AND status = 'ACTIVE' AND criteria_channel = ?`,
		channel,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := []Subscription{}
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sub)
	}
	return out, rows.Err()
}
