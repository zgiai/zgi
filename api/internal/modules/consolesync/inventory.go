package consolesync

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// This file implements plan P0/pre: reporting the local organization inventory to console
// via POST /v1/internal/pre/organizations.
//
// The report is best-effort telemetry. It runs on its own goroutine and its own ticker,
// never on the heartbeat tick, and every failure is logged and swallowed so the sync loop
// keeps running.

// organizationMemberStatusActive mirrors workspace/model.OrganizationMemberStatusActive.
//
// pre's `members.status` DOES have an active concept ("active"/"inactive"), so
// member_count counts only rows whose status is active: inactive members are not part of
// the usable inventory console mirrors.
const organizationMemberStatusActive = "active"

// OrganizationRecord is one organization read from the local database, before it is shaped
// for the wire.
type OrganizationRecord struct {
	ID          string
	Name        string
	ShortName   string
	Status      string
	MemberCount int64
	UpdatedAt   time.Time
}

// OrganizationInventory is one organization in the console report body.
type OrganizationInventory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ShortName   string `json:"short_name"`
	Status      string `json:"status"`
	MemberCount int64  `json:"member_count"`
	// UpdatedAt is RFC3339 in UTC, the format console stores and compares.
	UpdatedAt string `json:"updated_at"`
}

// OrganizationInventoryRequest is the body of POST /v1/internal/pre/organizations.
type OrganizationInventoryRequest struct {
	Organizations []OrganizationInventory `json:"organizations"`
	SyncedAt      string                  `json:"synced_at"`
}

// OrganizationInventoryResponse is the `data` object of the console answer.
type OrganizationInventoryResponse struct {
	Upserted int64 `json:"upserted"`
}

// BuildOrganizationInventory shapes database rows into the console wire payload.
//
// It is a pure function so the field mapping and the RFC3339/UTC formatting are testable
// without a database or an HTTP server.
func BuildOrganizationInventory(records []OrganizationRecord, syncedAt time.Time) OrganizationInventoryRequest {
	organizations := make([]OrganizationInventory, 0, len(records))
	for _, record := range records {
		organizations = append(organizations, OrganizationInventory{
			ID:          record.ID,
			Name:        record.Name,
			ShortName:   record.ShortName,
			Status:      record.Status,
			MemberCount: record.MemberCount,
			UpdatedAt:   formatConsoleTime(record.UpdatedAt),
		})
	}
	return OrganizationInventoryRequest{
		Organizations: organizations,
		SyncedAt:      formatConsoleTime(syncedAt),
	}
}

// formatConsoleTime renders a timestamp the way console expects it: RFC3339, UTC.
func formatConsoleTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

// ListOrganizationRecords reads every organization together with its active member count.
//
// Both queries are plain GORM over the shared table names, so postgres, sqlite and mysql
// all work without dialect-specific SQL. Organizations without active members are still
// reported, with member_count 0.
func (s *Store) ListOrganizationRecords(ctx context.Context) ([]OrganizationRecord, error) {
	var rows []struct {
		ID        string     `gorm:"column:id"`
		Name      string     `gorm:"column:name"`
		ShortName *string    `gorm:"column:short_name"`
		Status    string     `gorm:"column:status"`
		UpdatedAt *time.Time `gorm:"column:updated_at"`
	}
	if err := s.db.WithContext(ctx).
		Table("organizations").
		Select("id, name, short_name, status, updated_at").
		Order("id ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read organization inventory: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	counts, err := s.activeMemberCounts(ctx)
	if err != nil {
		return nil, err
	}

	records := make([]OrganizationRecord, 0, len(rows))
	for _, row := range rows {
		record := OrganizationRecord{
			ID:          row.ID,
			Name:        row.Name,
			Status:      row.Status,
			MemberCount: counts[row.ID],
		}
		if row.ShortName != nil {
			record.ShortName = *row.ShortName
		}
		if row.UpdatedAt != nil {
			record.UpdatedAt = *row.UpdatedAt
		}
		records = append(records, record)
	}
	return records, nil
}

// activeMemberCounts returns member_count per organization for active members only.
func (s *Store) activeMemberCounts(ctx context.Context) (map[string]int64, error) {
	var rows []struct {
		OrganizationID string `gorm:"column:organization_id"`
		MemberCount    int64  `gorm:"column:member_count"`
	}
	if err := s.db.WithContext(ctx).
		Table("members").
		Select("organization_id, COUNT(*) AS member_count").
		Where("status = ?", organizationMemberStatusActive).
		Group("organization_id").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("count active organization members: %w", err)
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.OrganizationID] = row.MemberCount
	}
	return counts, nil
}

// reportOrganizationInventory collects the local inventory and pushes it to console.
//
// It returns the error so a caller (or a test) can observe a failure; the running loop
// logs and swallows it, because a failed inventory report must never stop sync/heartbeat.
func (s *Service) reportOrganizationInventory(ctx context.Context) error {
	// Bound the whole report: collection plus the console call.
	reportCtx, cancel := context.WithTimeout(ctx, OrganizationReportTimeout)
	defer cancel()

	records, err := s.store.ListOrganizationRecords(reportCtx)
	if err != nil {
		return err
	}

	payload := BuildOrganizationInventory(records, time.Now().UTC())
	response, err := s.client.ReportOrganizations(reportCtx, payload)
	if err != nil {
		return fmt.Errorf("report %d organizations: %w", len(payload.Organizations), err)
	}

	// A signed inventory that console accepted proves the link is up again: the sync client
	// signs even while stale on purpose, so this is a legitimate recovery signal and it
	// clears the official-channel gate for the whole process.
	s.noteSuccess(time.Now().UTC())
	s.leaveStale()

	upserted := int64(0)
	if response != nil {
		upserted = response.Upserted
	}
	log.Printf("[consolesync] reported %d organizations to console (%d upserted)", len(payload.Organizations), upserted)
	return nil
}

// runOrganizationReporter reports the inventory once on sync start and then every
// OrganizationReportInterval until ctx is cancelled. It is meant to run in its own
// goroutine; nothing here blocks the heartbeat loop.
func (s *Service) runOrganizationReporter(ctx context.Context) {
	if s.cfg.OrganizationReportInterval <= 0 {
		log.Printf("[consolesync] organization inventory reporting disabled")
		return
	}
	log.Printf("[consolesync] organization inventory reporting every %s", s.cfg.OrganizationReportInterval)

	reporter := &organizationReporter{service: s}
	reporter.start(ctx)

	ticker := time.NewTicker(s.cfg.OrganizationReportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Let the in-flight report finish (its context is already cancelled, so the
			// console call aborts) before the loop is considered stopped.
			reporter.wait()
			return
		case <-ticker.C:
			reporter.start(ctx)
		}
	}
}

// organizationReporter owns one report schedule and guarantees at most one report is in
// flight: a tick that fires while the previous report is still running is skipped instead
// of queueing another call.
type organizationReporter struct {
	service *Service

	mu      sync.Mutex
	running bool
	wg      sync.WaitGroup
}

func (r *organizationReporter) start(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		log.Printf("[consolesync] organization inventory report still running; skipping this tick")
		return
	}
	r.running = true
	r.wg.Add(1)
	r.mu.Unlock()

	go func() {
		defer func() {
			r.mu.Lock()
			r.running = false
			r.mu.Unlock()
			r.wg.Done()
		}()
		if err := r.service.reportOrganizationInventory(ctx); err != nil {
			// Best effort: a failing inventory must never disturb sync or heartbeat.
			log.Printf("[consolesync] organization inventory report failed: %v", err)
		}
	}()
}

// wait blocks until no report is in flight.
func (r *organizationReporter) wait() { r.wg.Wait() }
