package zgimarket

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"
)

const (
	// SourceCredential is a call made with a market credential outside this instance, such as
	// a script holding a credential an administrator created. Its log row is read back from the
	// market, which keeps no input or output.
	SourceCredential = "credential"
	// SubjectCredential is the subject of such a call: the credential, not an agent.
	SubjectCredential = "credential"
	// CallRefused is a call the market refused before it reached the tool: credential,
	// activation, balance, rate limit.
	CallRefused = "refused"

	callSyncPerPage  = 100
	callSyncMaxPages = 10
	// callSyncSettle is how old a market record must be before it is read: a call this instance
	// made is logged right after its answer, and a younger record may be one it has not logged.
	callSyncSettle = 15 * time.Second
	// callSyncWindow is how far back the market's records are read; the market answers at most
	// 31 days at a time.
	callSyncWindow = 30 * 24 * time.Hour
)

// CallLogCursor is how far the market's call records of one organization have been read.
type CallLogCursor struct {
	OrganizationID uuid.UUID `gorm:"type:uuid;primaryKey"`
	AfterID        uint64    `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

// TableName names the table of call log cursors.
func (CallLogCursor) TableName() string { return "zgi_market_call_log_cursors" }

// marketCallRecord is one call record as the market answers it.
type marketCallRecord struct {
	ID               uint64    `json:"id"`
	RequestID        string    `json:"request_id"`
	Route            string    `json:"route"`
	Method           string    `json:"method"`
	Operation        string    `json:"operation"`
	CredentialID     uint64    `json:"credential_id"`
	ConsumerType     *string   `json:"consumer_type"`
	ConsumerRef      *string   `json:"consumer_ref"`
	Target           string    `json:"target"`
	ToolName         string    `json:"tool_name"`
	Outcome          string    `json:"outcome"`
	ErrorCode        string    `json:"error_code"`
	DurationMS       int64     `json:"duration_ms"`
	MeteringEventIDs []uint64  `json:"metering_event_ids"`
	OccurredAt       time.Time `json:"occurred_at"`
}

// syncLocks keeps one organization's reads of the market's records from running twice at once.
var syncLocks sync.Map

// syncDirectCalls reads the organization's call records written at the market since the last read
// and logs the ones this instance did not make. A call this instance made is already logged under
// the same request id, and an MCP handshake is no call of a tool; both are passed over.
func (t *Tools) syncDirectCalls(ctx context.Context, who caller, organizationID uuid.UUID) error {
	lock, _ := syncLocks.LoadOrStore(organizationID, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	var cursor CallLogCursor
	if err := t.logs.db.WithContext(ctx).Where("organization_id = ?", organizationID).Limit(1).Find(&cursor).Error; err != nil {
		return err
	}
	now := t.now()
	from, to := now.Add(-callSyncWindow).UTC().Format(time.RFC3339), now.Add(time.Minute).UTC().Format(time.RFC3339)
	after := cursor.AfterID
	var credentials map[uint64]string
	for range callSyncMaxPages {
		var page struct {
			Calls   []marketCallRecord `json:"calls"`
			HasMore bool               `json:"has_more"`
		}
		path := fmt.Sprintf("calls?from=%s&to=%s&after_id=%d&per_page=%d", url.QueryEscape(from), url.QueryEscape(to), after, callSyncPerPage)
		if err := t.marketData(ctx, who, http.MethodGet, path, nil, &page); err != nil {
			return err
		}
		read, young := []marketCallRecord{}, false
		for _, record := range page.Calls {
			if now.Sub(record.OccurredAt) < callSyncSettle {
				young = true
				break
			}
			after = record.ID
			if record.Route == "mcp" && record.Operation != "tools/call" {
				continue
			}
			read = append(read, record)
		}
		if len(read) > 0 && credentials == nil {
			credentials = t.credentialNames(ctx, who)
		}
		if err := t.logDirectCalls(ctx, organizationID, read, credentials); err != nil {
			return err
		}
		if after != cursor.AfterID {
			cursor = CallLogCursor{OrganizationID: organizationID, AfterID: after, UpdatedAt: now}
			if err := t.logs.db.WithContext(ctx).Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "organization_id"}}, DoUpdates: clause.AssignmentColumns([]string{"after_id", "updated_at"}),
			}).Create(&cursor).Error; err != nil {
				return err
			}
		}
		if young || !page.HasMore {
			return nil
		}
	}
	return nil
}

// logDirectCalls logs the market records whose request id this instance has not logged.
func (t *Tools) logDirectCalls(ctx context.Context, organizationID uuid.UUID, records []marketCallRecord, credentials map[uint64]string) error {
	if len(records) == 0 {
		return nil
	}
	ids := make([]string, 0, len(records))
	for _, record := range records {
		if record.RequestID != "" {
			ids = append(ids, record.RequestID)
		}
	}
	var known []string
	if err := t.logs.db.WithContext(ctx).Model(&CallLog{}).
		Where("organization_id = ? AND request_id IN ?", organizationID, ids).Pluck("request_id", &known).Error; err != nil {
		return err
	}
	logged := map[string]bool{}
	for _, id := range known {
		logged[id] = true
	}
	rows := []*CallLog{}
	for _, record := range records {
		if record.RequestID != "" && logged[record.RequestID] {
			continue
		}
		logged[record.RequestID] = true
		rows = append(rows, directCallLog(organizationID, record, credentials[record.CredentialID]))
	}
	if len(rows) == 0 {
		return nil
	}
	return t.logs.db.WithContext(ctx).Create(&rows).Error
}

func directCallLog(organizationID uuid.UUID, record marketCallRecord, credentialName string) *CallLog {
	row := &CallLog{
		ID: uuid.New(), OrganizationID: organizationID, SubjectType: SubjectCredential, SubjectID: uuid.Nil,
		ItemKind: ItemKindMCP, ListingSlug: cut(record.Target, 128), ToolName: cut(record.ToolName, 128),
		Source: SourceCredential, ErrorMessage: cut(record.ErrorCode, maxCallLogError),
		DurationMS: record.DurationMS, RequestID: cut(record.RequestID, 64), ChargeState: CallChargeNone,
		ChargeLines: []CallChargeLine{}, CreatedAt: record.OccurredAt.UTC(),
		CredentialID: record.CredentialID, CredentialName: cut(credentialName, 100),
	}
	if record.Route == "api" {
		row.ItemKind, row.OfferingSlug = ItemKindAPI, row.ListingSlug
		if row.ToolName == "" {
			row.ToolName = cut(strings.TrimSpace(record.Method+" "+record.Operation), 128)
		}
	}
	// A binding's credential used outside this instance still names its agent or digital human.
	if record.ConsumerType != nil && record.ConsumerRef != nil {
		if id, err := uuid.Parse(strings.TrimPrefix(*record.ConsumerRef, *record.ConsumerType+":")); err == nil &&
			(*record.ConsumerType == SubjectAgent || *record.ConsumerType == SubjectDigitalHuman) {
			row.SubjectType, row.SubjectID = *record.ConsumerType, id
		}
	}
	switch record.Outcome {
	case "succeeded":
		row.Status = CallSucceeded
	case "refused":
		row.Status = CallRefused
	case "vendor_error":
		row.Status = CallToolError
	default:
		row.Status = CallFailed
	}
	if len(record.MeteringEventIDs) > 0 {
		row.ChargeState = CallChargePending
	}
	return row
}

// credentialNames returns the names of the organization's market credentials by id. A market
// that cannot answer leaves them unnamed.
func (t *Tools) credentialNames(ctx context.Context, who caller) map[uint64]string {
	var credentials []struct {
		ID   uint64 `json:"id"`
		Name string `json:"name"`
	}
	names := map[uint64]string{}
	if err := t.marketData(ctx, who, http.MethodGet, "credentials?per_page=100", nil, &credentials); err != nil {
		return names
	}
	for _, credential := range credentials {
		names[credential.ID] = credential.Name
	}
	return names
}
