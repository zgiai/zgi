// Package socialevents stores verified Facebook Page entry batches. It never
// sends messages or triggers agents. SourceID correlates an entry batch, not an
// individual message within that batch, for caller-controlled feedback handoff.
package socialevents

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxDeliveryBytes = 1024 * 1024
const MaxPageSize = 100
const provider = "facebook-social"

type Options struct {
	FacebookAppSecret   string `json:"-"`
	FacebookVerifyToken string `json:"-"`
}
type Service struct {
	db      *gorm.DB
	vault   *credentials.Store
	options Options
}
type Event struct {
	ID                int64           `json:"id"`
	Provider          string          `json:"provider"`
	Kind              string          `json:"kind"`
	SourceID          string          `json:"source_id"`
	ExternalAccountID string          `json:"external_account_id"`
	ReceivedAt        time.Time       `json:"received_at"`
	Payload           json.RawMessage `json:"payload"`
}
type Page struct {
	Items       []Event `json:"items"`
	NextAfterID int64   `json:"next_after_id"`
}
type connectionRow struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID    uuid.UUID `gorm:"type:uuid"`
	OwnerAccountID    uuid.UUID `gorm:"type:uuid"`
	ConnectorID       string
	ExternalAccountID string
	State             string
	DeletedAt         *time.Time
}

func (connectionRow) TableName() string { return "connector_connections" }

type eventRow struct {
	ID                int64      `gorm:"primaryKey;autoIncrement"`
	EventID           uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"-"`
	OrganizationID    uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	OwnerAccountID    uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	ConnectionID      uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_connector_social_event_source" json:"-"`
	ExternalAccountID string     `gorm:"not null"`
	SourceID          string     `gorm:"size:64;not null;uniqueIndex:idx_connector_social_event_source"`
	EncryptedPayload  string     `gorm:"type:text;not null" json:"-"`
	ReceivedAt        time.Time  `gorm:"not null"`
	AcknowledgedAt    *time.Time `json:"-"`
}

func (eventRow) TableName() string { return "connector_social_events" }
func New(db *gorm.DB, vault *credentials.Store, opts Options) (*Service, error) {
	if db == nil || vault == nil {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	// Fully absent configuration disables public verification; a partial pair is
	// a deployment error rather than an accidentally working callback.
	if (opts.FacebookAppSecret == "") != (opts.FacebookVerifyToken == "") {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return &Service{db: db, vault: vault, options: opts}, nil
}
func (s *Service) enabled() bool {
	return s != nil && s.options.FacebookAppSecret != "" && s.options.FacebookVerifyToken != ""
}
func (s *Service) VerifyChallenge(mode, token, challenge string) (string, error) {
	if !s.enabled() {
		return "", core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
	}
	a, b := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(s.options.FacebookVerifyToken))
	if mode != "subscribe" || subtle.ConstantTimeCompare(a[:], b[:]) != 1 || len(challenge) == 0 || len(challenge) > 512 {
		return "", core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	return challenge, nil
}
func invalid() error           { return core.NewError(core.ErrorCodeInvalidInput, false, nil) }
func internal(err error) error { return core.NewError(core.ErrorCodeInternal, false, err) }
func source(raw []byte) string { digest := sha256.Sum256(raw); return hex.EncodeToString(digest[:]) }
func subject(row eventRow) credentials.Subject {
	return credentials.Subject{Purpose: credentials.MessagingContext, OrganizationID: row.OrganizationID, ID: row.EventID, ConnectorID: provider, Version: 1}
}

// sealedEntry binds owner, connection and source metadata inside authenticated
// ciphertext as well as binding organization and event UUID through vault AAD.
type sealedEntry struct {
	ConnectionID      uuid.UUID
	OwnerAccountID    uuid.UUID
	ExternalAccountID string
	SourceID          string
	Payload           json.RawMessage
}

func (s *Service) seal(row eventRow, raw []byte) (string, error) {
	encoded, err := json.Marshal(sealedEntry{row.ConnectionID, row.OwnerAccountID, row.ExternalAccountID, row.SourceID, json.RawMessage(raw)})
	if err != nil {
		return "", err
	}
	defer clear(encoded)
	chunks := []string{}
	for rest := encoded; len(rest) > 0; {
		n := min(len(rest), credentials.MaxPayloadBytes)
		sealed, err := s.vault.Protect(subject(row), rest[:n])
		if err != nil {
			return "", err
		}
		chunks = append(chunks, sealed)
		rest = rest[n:]
	}
	output, err := json.Marshal(chunks)
	return string(output), err
}
func (s *Service) open(row eventRow) (json.RawMessage, error) {
	var chunks []string
	if err := json.Unmarshal([]byte(row.EncryptedPayload), &chunks); err != nil || len(chunks) == 0 || len(chunks) > 5 {
		return nil, credentials.ErrEnvelope
	}
	var encoded []byte
	defer func() { clear(encoded) }()
	for _, chunk := range chunks {
		part, err := s.vault.Open(subject(row), chunk)
		if err != nil {
			return nil, err
		}
		encoded = append(encoded, part...)
		clear(part)
		if len(encoded) > MaxDeliveryBytes+4096 {
			return nil, credentials.ErrEnvelope
		}
	}
	var entry sealedEntry
	if json.Unmarshal(encoded, &entry) != nil || entry.ConnectionID != row.ConnectionID || entry.OwnerAccountID != row.OwnerAccountID || entry.ExternalAccountID != row.ExternalAccountID || entry.SourceID != row.SourceID || len(entry.Payload) > MaxDeliveryBytes || source(entry.Payload) != row.SourceID || !json.Valid(entry.Payload) {
		clear(entry.Payload)
		return nil, credentials.ErrEnvelope
	}
	return entry.Payload, nil
}
func (s *Service) ReceiveFacebook(ctx context.Context, signature string, raw []byte) error {
	if !s.enabled() {
		return core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
	}
	if len(raw) == 0 || len(raw) > MaxDeliveryBytes {
		return invalid()
	}
	if len(signature) != 71 || !strings.HasPrefix(signature, "sha256=") {
		return core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	expected, err := hex.DecodeString(signature[7:])
	if err != nil {
		return core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	mac := hmac.New(sha256.New, []byte(s.options.FacebookAppSecret))
	mac.Write(raw)
	if !hmac.Equal(expected, mac.Sum(nil)) {
		return core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	var delivery struct {
		Object string                       `json:"object"`
		Entry  []map[string]json.RawMessage `json:"entry"`
	}
	if json.Unmarshal(raw, &delivery) != nil || delivery.Object != "page" || delivery.Entry == nil || len(delivery.Entry) > 100 {
		return invalid()
	}
	type entry struct {
		pageID string
		raw    []byte
	}
	entries := make([]entry, 0, len(delivery.Entry))
	ids := []string{}
	for _, item := range delivery.Entry {
		var id string
		if item == nil || json.Unmarshal(item["id"], &id) != nil || id == "" || len(id) > 255 {
			return invalid()
		}
		// Decode arbitrary objects recursively before marshal to normalize key order
		// while preserving large numeric IDs without float64 rounding.
		decoder := json.NewDecoder(strings.NewReader(mustJSON(item)))
		decoder.UseNumber()
		var value any
		if decoder.Decode(&value) != nil {
			return invalid()
		}
		canonical, err := json.Marshal(value)
		if err != nil || len(canonical) > MaxDeliveryBytes {
			return invalid()
		}
		entries = append(entries, entry{id, canonical})
		ids = append(ids, id)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var connections []connectionRow
		if len(ids) == 0 {
			return nil
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("connector_id = ? AND external_account_id IN ? AND state = ? AND deleted_at IS NULL", provider, ids, "ready").Order("id ASC").Limit(101).Find(&connections).Error; err != nil {
			return err
		}
		if len(connections) > 100 {
			return core.NewError(core.ErrorCodeProviderUnavailable, true, nil)
		}
		writes, payloadBytes := 0, 0
		for _, item := range entries {
			for _, c := range connections {
				if c.ExternalAccountID != item.pageID {
					continue
				}
				writes++
				payloadBytes += len(item.raw)
				if writes > 200 || payloadBytes > 8*MaxDeliveryBytes {
					return core.NewError(core.ErrorCodeProviderUnavailable, true, nil)
				}
				row := eventRow{EventID: uuid.New(), OrganizationID: c.OrganizationID, OwnerAccountID: c.OwnerAccountID, ConnectionID: c.ID, ExternalAccountID: c.ExternalAccountID, SourceID: source(item.raw), ReceivedAt: time.Now().UTC()}
				sealed, err := s.seal(row, item.raw)
				if err != nil {
					return err
				}
				row.EncryptedPayload = sealed
				if err = tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "connection_id"}, {Name: "source_id"}}, DoNothing: true}).Create(&row).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		var ce *core.Error
		if errors.As(err, &ce) {
			return err
		}
		return internal(err)
	}
	return nil
}
func mustJSON(value any) string { raw, _ := json.Marshal(value); return string(raw) }
func lockConnection(tx *gorm.DB, org, owner, conn uuid.UUID) (connectionRow, error) {
	var c connectionRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND organization_id = ? AND owner_account_id = ? AND connector_id = ? AND deleted_at IS NULL", conn, org, owner, provider).Take(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c, core.NewError(core.ErrorCodeNotFound, false, nil)
	}
	if err != nil {
		return c, internal(err)
	}
	if c.State != "ready" || c.ExternalAccountID == "" {
		return c, core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
	}
	return c, nil
}
func scope(tx *gorm.DB, c connectionRow) *gorm.DB {
	return tx.Where("organization_id = ? AND owner_account_id = ? AND connection_id = ? AND external_account_id = ?", c.OrganizationID, c.OwnerAccountID, c.ID, c.ExternalAccountID)
}
func validScope(org, owner, conn uuid.UUID) bool {
	return org != uuid.Nil && owner != uuid.Nil && conn != uuid.Nil
}
func (s *Service) List(ctx context.Context, org, owner, conn uuid.UUID, afterID int64, limit int) (Page, error) {
	page := Page{Items: []Event{}, NextAfterID: afterID}
	if !validScope(org, owner, conn) || afterID < 0 || limit < 1 || limit > MaxPageSize {
		return page, invalid()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := lockConnection(tx, org, owner, conn)
		if err != nil {
			return err
		}
		rows, err := scope(tx, c).Model(&eventRow{}).Where("id > ? AND acknowledged_at IS NULL", afterID).Order("id ASC").Limit(limit).Rows()
		if err != nil {
			return internal(err)
		}
		defer rows.Close()
		pageBytes := 0
		for rows.Next() {
			var row eventRow
			if err := tx.ScanRows(rows, &row); err != nil {
				return internal(err)
			}
			raw, err := s.open(row)
			if err != nil {
				return internal(fmt.Errorf("open social event: %w", err))
			}
			if len(page.Items) > 0 && pageBytes+len(raw) > 512*1024 {
				clear(raw)
				break
			}
			pageBytes += len(raw)
			page.Items = append(page.Items, Event{ID: row.ID, Provider: provider, Kind: "page_entry", SourceID: row.SourceID, ExternalAccountID: row.ExternalAccountID, ReceivedAt: row.ReceivedAt, Payload: raw})
			page.NextAfterID = row.ID
		}
		if err := rows.Err(); err != nil {
			return internal(err)
		}
		return nil
	})
	if err != nil {
		return Page{Items: []Event{}}, err
	}
	return page, nil
}
func (s *Service) Acknowledge(ctx context.Context, org, owner, conn uuid.UUID, throughID int64) error {
	if !validScope(org, owner, conn) || throughID < 1 {
		return invalid()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := lockConnection(tx, org, owner, conn)
		if err != nil {
			return err
		}
		var target eventRow
		err = scope(tx, c).Where("id = ?", throughID).Take(&target).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalid()
		}
		if err != nil {
			return internal(err)
		}
		if err = scope(tx, c).Model(&eventRow{}).Where("id <= ? AND acknowledged_at IS NULL", throughID).Update("acknowledged_at", time.Now().UTC()).Error; err != nil {
			return internal(err)
		}
		return nil
	})
}
