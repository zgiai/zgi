package zgimarket

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

// How a market tool call ended, as this instance saw it.
const (
	// CallSucceeded is a call the tool answered without an error.
	CallSucceeded = "succeeded"
	// CallToolError is a call the tool or the market answered with an error.
	CallToolError = "tool_error"
	// CallFailed is a call that got no answer: the gateway could not be reached or broke off.
	CallFailed = "failed"
)

// Whether what the market charged for a call is known. They are the states the market answers,
// plus CallChargeUnknown for a call the gateway named no request id for.
const (
	CallChargePending = "pending"
	CallChargeCharged = "charged"
	CallChargeNone    = "none"
	CallChargeUnknown = "unknown"
)

const (
	// maxCallLogInput bounds the stored input of one call, in bytes.
	maxCallLogInput = 16 << 10
	// maxCallLogOutput bounds the stored output of one call, in bytes. A result handed to the
	// model is already cut at maxToolResultChars.
	maxCallLogOutput = maxToolResultChars
	maxCallLogError  = 500
	// callLogWriteTimeout bounds the write of one row; the call has already been answered.
	callLogWriteTimeout = 5 * time.Second
)

// CallChargeLine is the charge of one metering event of a call.
type CallChargeLine struct {
	Meter        string `json:"meter"`
	UnitName     string `json:"unit_name"`
	Quantity     int64  `json:"quantity"`
	AmountMicros int64  `json:"amount_micros"`
}

// CallLog is one market tool call this instance made for an agent or a digital human.
type CallLog struct {
	ID              uuid.UUID        `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID  uuid.UUID        `gorm:"type:uuid;not null" json:"-"`
	SubjectType     string           `gorm:"type:varchar(32);not null" json:"subject_type"`
	SubjectID       uuid.UUID        `gorm:"type:uuid;not null" json:"subject_id"`
	// SubjectName is the agent's or digital human's name when the logs are read; empty when it
	// no longer exists.
	SubjectName string `gorm:"-" json:"subject_name"`
	ItemKind        string           `gorm:"type:varchar(8);not null" json:"item_kind"`
	ListingSlug     string           `gorm:"type:varchar(128);not null" json:"listing_slug"`
	ListingTitle    string           `gorm:"type:varchar(200);not null" json:"listing_title"`
	OfferingSlug    string           `gorm:"type:varchar(128);not null" json:"offering_slug"`
	ToolName        string           `gorm:"type:varchar(128);not null" json:"tool_name"`
	Input           string           `gorm:"type:text;not null" json:"input"`
	Output          string           `gorm:"type:text;not null" json:"output"`
	Status          string           `gorm:"type:varchar(16);not null" json:"status"`
	ErrorMessage    string           `gorm:"type:varchar(500);not null" json:"error_message"`
	DurationMS      int64            `gorm:"not null" json:"duration_ms"`
	RequestID       string           `gorm:"type:varchar(64);not null" json:"request_id"`
	ChargeState     string           `gorm:"type:varchar(16);not null" json:"charge_state"`
	ChargeMicros    int64            `gorm:"not null" json:"charge_micros"`
	ChargeLines     []CallChargeLine `gorm:"type:jsonb;serializer:json;not null" json:"charge_lines"`
	ChargeCheckedAt *time.Time       `json:"-"`
	CreatedAt       time.Time        `json:"created_at"`
}

// TableName names the table of market call logs.
func (CallLog) TableName() string { return "zgi_market_call_logs" }

type callLogStore struct {
	db *gorm.DB
}

// callTarget is what one call reached: the bound item and the tool.
type callTarget struct {
	binding *ToolBinding
	item    ToolBindingItem
	tool    string
}

// record keeps one finished call. A row that cannot be written is logged and dropped: the call
// has been answered and must not fail for its log.
func (s callLogStore) record(target callTarget, arguments map[string]any, result *GatewayResult, callErr error, started, finished time.Time) {
	if s.db == nil || target.binding == nil {
		return
	}
	row := &CallLog{
		ID: uuid.New(), OrganizationID: target.binding.OrganizationID,
		SubjectType: target.binding.SubjectType, SubjectID: target.binding.SubjectID,
		ItemKind: ItemKindMCP, ListingSlug: target.item.Slug, ListingTitle: cut(target.item.Title, 200),
		ToolName: cut(target.tool, 128), Input: callInput(arguments),
		DurationMS: finished.Sub(started).Milliseconds(), ChargeState: CallChargeUnknown,
		ChargeLines: []CallChargeLine{}, CreatedAt: started.UTC(),
	}
	if target.item.IsAPI() {
		row.ItemKind, row.OfferingSlug = ItemKindAPI, target.item.OfferingSlug
	}
	if result != nil {
		row.Output = cut(result.Text, maxCallLogOutput)
		row.RequestID = cut(result.RequestID, 64)
	}
	switch {
	case callErr != nil:
		row.Status, row.ErrorMessage = CallFailed, cut(callErr.Error(), maxCallLogError)
	case result != nil && result.IsError:
		row.Status = CallToolError
	default:
		row.Status = CallSucceeded
	}
	if row.RequestID != "" {
		row.ChargeState = CallChargePending
	}
	ctx, cancel := context.WithTimeout(context.Background(), callLogWriteTimeout)
	defer cancel()
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		logger.WarnContext(ctx, "a ZGI Market call was made but its log row could not be written",
			"organization_id", row.OrganizationID.String(), "listing", row.ListingSlug, "tool", row.ToolName,
			"request_id", row.RequestID, "error", err.Error())
	}
}

func callInput(arguments map[string]any) string {
	if arguments == nil {
		arguments = map[string]any{}
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return ""
	}
	return cut(string(encoded), maxCallLogInput)
}

// cut bounds text to limit bytes without ending inside a character.
func cut(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	end := limit
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end]
}

// CallLogQuery selects the call logs of an organization: a period and optional filters.
type CallLogQuery struct {
	From        time.Time
	To          time.Time
	Status      string
	SubjectType string
	SubjectID   *uuid.UUID
	Listing     string
	Page        int
	PerPage     int
}

// CallLogSummary sums the calls a query selects.
type CallLogSummary struct {
	Calls        int64 `json:"calls"`
	Succeeded    int64 `json:"succeeded"`
	ChargeMicros int64 `json:"charge_micros"`
	Pending      int64 `json:"pending"`
}

// CallLogPage is one page of call logs, newest first, with the sums of every call selected.
type CallLogPage struct {
	Data    []*CallLog     `json:"data"`
	Total   int64          `json:"total"`
	Page    int            `json:"page"`
	PerPage int            `json:"per_page"`
	Summary CallLogSummary `json:"summary"`
}

var errCallLogQuery = errors.New("invalid call log query")

func (s callLogStore) scope(ctx context.Context, organizationID uuid.UUID, query CallLogQuery) *gorm.DB {
	db := s.db.WithContext(ctx).Model(&CallLog{}).
		Where("organization_id = ? AND created_at >= ? AND created_at < ?", organizationID, query.From, query.To)
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if query.SubjectType != "" {
		db = db.Where("subject_type = ?", query.SubjectType)
	}
	if query.SubjectID != nil {
		db = db.Where("subject_id = ?", *query.SubjectID)
	}
	if query.Listing != "" {
		db = db.Where("listing_slug = ?", query.Listing)
	}
	return db
}

func (s callLogStore) list(ctx context.Context, organizationID uuid.UUID, query CallLogQuery) (*CallLogPage, error) {
	page := &CallLogPage{Data: []*CallLog{}, Page: query.Page, PerPage: query.PerPage}
	var sums struct {
		Calls        int64
		Succeeded    int64
		ChargeMicros int64
		Pending      int64
	}
	if err := s.scope(ctx, organizationID, query).Select(
		"COUNT(*) AS calls, " +
			"COUNT(*) FILTER (WHERE status = 'succeeded') AS succeeded, " +
			"COALESCE(SUM(charge_micros), 0) AS charge_micros, " +
			"COUNT(*) FILTER (WHERE charge_state = 'pending') AS pending",
	).Scan(&sums).Error; err != nil {
		return nil, err
	}
	page.Total = sums.Calls
	page.Summary = CallLogSummary(sums)
	if err := s.scope(ctx, organizationID, query).Order("created_at DESC, id DESC").
		Offset((query.Page - 1) * query.PerPage).Limit(query.PerPage).Find(&page.Data).Error; err != nil {
		return nil, err
	}
	s.nameSubjects(ctx, page.Data)
	return page, nil
}

// nameSubjects fills in the current name of each row's agent or digital human. A name that
// cannot be read is left empty; the page is answered all the same.
func (s callLogStore) nameSubjects(ctx context.Context, rows []*CallLog) {
	ids := map[string][]uuid.UUID{}
	for _, row := range rows {
		ids[row.SubjectType] = append(ids[row.SubjectType], row.SubjectID)
	}
	names := map[uuid.UUID]string{}
	for subjectType, table := range map[string]string{SubjectAgent: "agents", SubjectDigitalHuman: "digital_humans"} {
		if len(ids[subjectType]) == 0 {
			continue
		}
		var found []struct {
			ID   uuid.UUID
			Name string
		}
		if err := s.db.WithContext(ctx).Table(table).Select("id, name").Where("id IN ?", ids[subjectType]).
			Scan(&found).Error; err != nil {
			logger.WarnContext(ctx, "the names of market call subjects could not be read", "table", table, "error", err.Error())
			continue
		}
		for _, subject := range found {
			names[subject.ID] = subject.Name
		}
	}
	for _, row := range rows {
		row.SubjectName = names[row.SubjectID]
	}
}

func (s callLogStore) settle(ctx context.Context, id uuid.UUID, state string, micros int64, lines []CallChargeLine, at time.Time) error {
	encoded, err := json.Marshal(lines)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Model(&CallLog{}).Where("id = ?", id).Updates(map[string]any{
		"charge_state": state, "charge_micros": micros, "charge_lines": string(encoded), "charge_checked_at": at,
	}).Error
}

func trimmedStatus(status string) (string, bool) {
	switch status = strings.TrimSpace(status); status {
	case "", CallSucceeded, CallToolError, CallFailed:
		return status, true
	}
	return "", false
}
