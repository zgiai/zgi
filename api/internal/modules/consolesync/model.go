package consolesync

import (
	"time"

	"github.com/google/uuid"
)

// PreSyncState is the singleton row that persists the sync cursor across restarts.
//
// The time columns are pinned to timestamptz to match migration
// 20260925120000_create_console_sync_tables: the DB schema must not drift from the model.
// Values are read back and written as real UTC instants (see normalizeTimesUTC).
type PreSyncState struct {
	ID                  int        `gorm:"column:id;primaryKey" json:"id"`
	DownstreamClientID  *uuid.UUID `gorm:"column:downstream_client_id;type:uuid" json:"downstream_client_id,omitempty"`
	CursorSeq           int64      `gorm:"column:cursor_seq;not null;default:0" json:"cursor_seq"`
	AppliedVersion      int64      `gorm:"column:applied_version;not null;default:0" json:"applied_version"`
	LastHeartbeatAt     *time.Time `gorm:"column:last_heartbeat_at;type:timestamptz" json:"last_heartbeat_at,omitempty"`
	LastPullAt          *time.Time `gorm:"column:last_pull_at;type:timestamptz" json:"last_pull_at,omitempty"`
	ConsecutiveFailures int        `gorm:"column:consecutive_failures;not null;default:0" json:"consecutive_failures"`
	NextPollAt          *time.Time `gorm:"column:next_poll_at;type:timestamptz" json:"next_poll_at,omitempty"`
	CredentialStatus    string     `gorm:"column:credential_status;size:32" json:"credential_status,omitempty"`
	LastError           string     `gorm:"column:last_error;type:text" json:"last_error,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// normalizeTimesUTC pins every persisted timestamp to its UTC instant.
//
// The column is timestamptz, so the instant itself is preserved by the driver, but drivers
// hand it back in their own location: pgx's binary timestamptz codec builds the value with
// time.Unix, i.e. time.Local, and sqlite returns a fixed-offset zone. A value written as
// 04:03:36Z (12:03:36 on a +08 wall clock) then reads back as 12:03:36+08. Normalising on
// read (and before every write) makes the stored and read value agree on the real UTC
// instant, which is what status reporting and the stale comparison need.
func (s *PreSyncState) normalizeTimesUTC() {
	s.LastHeartbeatAt = utcPtr(s.LastHeartbeatAt)
	s.LastPullAt = utcPtr(s.LastPullAt)
	s.NextPollAt = utcPtr(s.NextPollAt)
	s.CreatedAt = s.CreatedAt.UTC()
	s.UpdatedAt = s.UpdatedAt.UTC()
}

// utcPtr converts an optional timestamp to the same instant in UTC, preserving nil.
func utcPtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}

// TableName pins the table created by the console sync migration.
func (PreSyncState) TableName() string { return "pre_sync_state" }

// PreAppliedCommand is the idempotency ledger: a command id may only ever appear once,
// and a console envelope idempotency key may only ever appear once on a successful row.
type PreAppliedCommand struct {
	CommandID uuid.UUID `gorm:"column:command_id;type:uuid;primaryKey" json:"command_id"`
	Seq       int64     `gorm:"column:seq;not null" json:"seq"`
	Type      string    `gorm:"column:type;size:64;not null" json:"type"`
	// IdempotencyKey is the key the console attached to the envelope. It is nullable and
	// only stored for a successful apply, so failed commands stay retryable and keyless
	// commands keep their previous behaviour.
	IdempotencyKey *string   `gorm:"column:idempotency_key;size:128" json:"idempotency_key,omitempty"`
	Status         string    `gorm:"column:status;size:20;not null" json:"status"`
	BeforeState    string    `gorm:"column:before_state;type:jsonb" json:"before_state,omitempty"`
	AfterState     string    `gorm:"column:after_state;type:jsonb" json:"after_state,omitempty"`
	Error          string    `gorm:"column:error;type:text" json:"error,omitempty"`
	AppliedAt      time.Time `gorm:"column:applied_at;not null" json:"applied_at"`
}

// TableName pins the table created by the console sync migration.
func (PreAppliedCommand) TableName() string { return "pre_applied_commands" }

// Command application statuses.
const (
	StatusApplied = "applied"
	StatusFailed  = "failed"
)

// Credential statuses reported back to console.
const (
	CredentialStatusActive = "active"
	CredentialStatusStale  = "stale"
)

// Command types understood by the applier. They mirror the console side exactly.
const (
	CommandAccountStatusSet             = "account.status.set"
	CommandAPIKeySet                    = "apikey.set"
	CommandAPIKeyQuotaSet               = "apikey.quota.set"
	CommandQuotaSet                     = "quota.set"
	CommandCredentialDisable            = "credential.disable"
	CommandOrganizationMemberCreate     = "organization.member.create"
	CommandOrganizationCreate           = "organization.create"
	CommandOrganizationDepartmentCreate = "organization.department.create"
)
