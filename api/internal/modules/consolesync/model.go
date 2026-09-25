package consolesync

import (
	"time"

	"github.com/google/uuid"
)

// PreSyncState is the singleton row that persists the sync cursor across restarts.
type PreSyncState struct {
	ID                  int        `gorm:"column:id;primaryKey" json:"id"`
	DownstreamClientID  *uuid.UUID `gorm:"column:downstream_client_id;type:uuid" json:"downstream_client_id,omitempty"`
	CursorSeq           int64      `gorm:"column:cursor_seq;not null;default:0" json:"cursor_seq"`
	AppliedVersion      int64      `gorm:"column:applied_version;not null;default:0" json:"applied_version"`
	LastHeartbeatAt     *time.Time `gorm:"column:last_heartbeat_at" json:"last_heartbeat_at,omitempty"`
	LastPullAt          *time.Time `gorm:"column:last_pull_at" json:"last_pull_at,omitempty"`
	ConsecutiveFailures int        `gorm:"column:consecutive_failures;not null;default:0" json:"consecutive_failures"`
	NextPollAt          *time.Time `gorm:"column:next_poll_at" json:"next_poll_at,omitempty"`
	CredentialStatus    string     `gorm:"column:credential_status;size:32" json:"credential_status,omitempty"`
	LastError           string     `gorm:"column:last_error;type:text" json:"last_error,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// TableName pins the table created by the console sync migration.
func (PreSyncState) TableName() string { return "pre_sync_state" }

// PreAppliedCommand is the idempotency ledger: a command id may only ever appear once.
type PreAppliedCommand struct {
	CommandID   uuid.UUID `gorm:"column:command_id;type:uuid;primaryKey" json:"command_id"`
	Seq         int64     `gorm:"column:seq;not null" json:"seq"`
	Type        string    `gorm:"column:type;size:64;not null" json:"type"`
	Status      string    `gorm:"column:status;size:20;not null" json:"status"`
	BeforeState string    `gorm:"column:before_state;type:jsonb" json:"before_state,omitempty"`
	AfterState  string    `gorm:"column:after_state;type:jsonb" json:"after_state,omitempty"`
	Error       string    `gorm:"column:error;type:text" json:"error,omitempty"`
	AppliedAt   time.Time `gorm:"column:applied_at;not null" json:"applied_at"`
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
	CommandAccountStatusSet  = "account.status.set"
	CommandAPIKeySet         = "apikey.set"
	CommandAPIKeyQuotaSet    = "apikey.quota.set"
	CommandQuotaSet          = "quota.set"
	CommandCredentialDisable = "credential.disable"
)
