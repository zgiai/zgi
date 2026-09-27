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

// departmentStatusActive mirrors workspace/model.DepartmentStatusActive.
//
// pre's own department list (departmentRepository.GetByOrganizationID, which the contacts
// page reads through GET /organizations/:id/departments) filters on this status, so archived
// departments are not part of the inventory console mirrors.
const departmentStatusActive = "active"

// OrganizationMemberRecord is one organization member read from the local database.
//
// Name is already resolved the way pre's contacts page renders it (`member_name ||
// account_name`), so the wire mapping is a plain copy.
type OrganizationMemberRecord struct {
	AccountID    string
	Name         string
	Email        string
	Role         string
	Status       string
	LastLoginAt  *time.Time
	DepartmentID *string
}

// OrganizationDepartmentRecord is one department of an organization, together with the
// member count pre's own department tree shows.
type OrganizationDepartmentRecord struct {
	ID          string
	Name        string
	MemberCount int64
	ParentID    *string
}

// OrganizationRecord is one organization read from the local database, before it is shaped
// for the wire.
type OrganizationRecord struct {
	ID          string
	Name        string
	ShortName   string
	Status      string
	MemberCount int64
	UpdatedAt   time.Time
	// Departments is pre's department list for the organization; Members is pre's member
	// list (the rows the contacts page shows), both best-effort.
	Departments []OrganizationDepartmentRecord
	Members     []OrganizationMemberRecord
}

// OrganizationDepartmentInventory is one department in the console report body.
type OrganizationDepartmentInventory struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	MemberCount int64   `json:"member_count"`
	ParentID    *string `json:"parent_id,omitempty"`
}

// OrganizationMemberInventory is one member in the console report body.
//
// LastLoginAt is RFC3339 in UTC, or null when the account never logged in. DepartmentID is
// null when the member has no department in this organization.
type OrganizationMemberInventory struct {
	AccountID    string  `json:"account_id"`
	Name         string  `json:"name"`
	Email        string  `json:"email"`
	Role         string  `json:"role"`
	Status       string  `json:"status"`
	LastLoginAt  *string `json:"last_login_at"`
	DepartmentID *string `json:"department_id"`
}

// OrganizationInventory is one organization in the console report body.
type OrganizationInventory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ShortName   string `json:"short_name"`
	Status      string `json:"status"`
	MemberCount int64  `json:"member_count"`
	// UpdatedAt is RFC3339 in UTC, the format console stores and compares.
	UpdatedAt   string                            `json:"updated_at"`
	Departments []OrganizationDepartmentInventory `json:"departments"`
	Members     []OrganizationMemberInventory     `json:"members"`
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
			Departments: buildDepartmentInventory(record.Departments),
			Members:     buildMemberInventory(record.Members),
		})
	}
	return OrganizationInventoryRequest{
		Organizations: organizations,
		SyncedAt:      formatConsoleTime(syncedAt),
	}
}

// buildDepartmentInventory maps department rows onto the wire, always yielding a JSON array
// (never null) so console can iterate the field unconditionally.
func buildDepartmentInventory(records []OrganizationDepartmentRecord) []OrganizationDepartmentInventory {
	departments := make([]OrganizationDepartmentInventory, 0, len(records))
	for _, record := range records {
		departments = append(departments, OrganizationDepartmentInventory{
			ID:          record.ID,
			Name:        record.Name,
			MemberCount: record.MemberCount,
			ParentID:    record.ParentID,
		})
	}
	return departments
}

// buildMemberInventory maps member rows onto the wire, rendering last_login_at as an
// RFC3339 UTC string or null.
func buildMemberInventory(records []OrganizationMemberRecord) []OrganizationMemberInventory {
	members := make([]OrganizationMemberInventory, 0, len(records))
	for _, record := range records {
		member := OrganizationMemberInventory{
			AccountID:    record.AccountID,
			Name:         record.Name,
			Email:        record.Email,
			Role:         record.Role,
			Status:       record.Status,
			DepartmentID: record.DepartmentID,
		}
		if record.LastLoginAt != nil {
			lastLoginAt := formatConsoleTime(*record.LastLoginAt)
			member.LastLoginAt = &lastLoginAt
		}
		members = append(members, member)
	}
	return members
}

// formatConsoleTime renders a timestamp the way console expects it: RFC3339, UTC.
func formatConsoleTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

// ListOrganizationRecords reads every organization together with its active member count,
// its member list and its department list.
//
// The extra queries are plain GORM over the shared table names, so postgres, sqlite and
// mysql all work without dialect-specific SQL. Organizations without members or
// departments are still reported, with empty lists.
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
	members, err := s.organizationMembers(ctx)
	if err != nil {
		return nil, err
	}
	departments, err := s.organizationDepartments(ctx)
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
			Members:     members[row.ID],
			Departments: departments[row.ID],
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

// organizationMembers reads the organization member list pre's contacts page shows, keyed
// by organization id.
//
// This is the same read path as departmentRepository.ListMembers: `members` joined to
// `accounts`, with the member's department resolved through `department_members` scoped to
// a department of the same organization. Accounts soft-deleted in pre are excluded -- they
// are no longer part of the member list. The display name follows the contacts page
// (`member_name || account_name`).
//
// The department is a correlated subquery rather than a join so one member is always
// exactly one report row: pre keeps a member in a single department per organization, and a
// stray extra row must not make console render the member twice. The oldest membership wins
// so the result is stable.
func (s *Store) organizationMembers(ctx context.Context) (map[string][]OrganizationMemberRecord, error) {
	var rows []struct {
		OrganizationID string     `gorm:"column:organization_id"`
		AccountID      string     `gorm:"column:account_id"`
		AccountName    string     `gorm:"column:account_name"`
		MemberName     *string    `gorm:"column:member_name"`
		Email          string     `gorm:"column:email"`
		Role           string     `gorm:"column:role"`
		Status         string     `gorm:"column:status"`
		LastLoginAt    *time.Time `gorm:"column:last_login_at"`
		DepartmentID   *string    `gorm:"column:department_id"`
	}
	if err := s.db.WithContext(ctx).
		Table("members AS m").
		Select(`m.organization_id, m.account_id, m.name AS member_name, m.role, m.status,
			a.name AS account_name, a.email, a.last_login_at,
			(SELECT d.id FROM department_members AS dm
				JOIN departments AS d ON d.id = dm.department_id
				WHERE dm.account_id = m.account_id AND d.group_id = m.organization_id
				ORDER BY dm.created_at ASC
				LIMIT 1) AS department_id`).
		Joins("JOIN accounts AS a ON a.id = m.account_id").
		Where("a.deleted_at IS NULL").
		Order("m.organization_id ASC, m.created_at ASC, m.account_id ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read organization members: %w", err)
	}

	members := make(map[string][]OrganizationMemberRecord, len(rows))
	for _, row := range rows {
		member := OrganizationMemberRecord{
			AccountID:    row.AccountID,
			Name:         row.AccountName,
			Email:        row.Email,
			Role:         row.Role,
			Status:       row.Status,
			LastLoginAt:  row.LastLoginAt,
			DepartmentID: row.DepartmentID,
		}
		if row.MemberName != nil && *row.MemberName != "" {
			member.Name = *row.MemberName
		}
		members[row.OrganizationID] = append(members[row.OrganizationID], member)
	}
	return members, nil
}

// organizationDepartments reads pre's department list per organization, keyed by
// organization id.
//
// It mirrors departmentRepository.GetByOrganizationID (active departments only, ordered by
// sort_order then created_at) together with the per-department member counts its tree
// builds, and no postgres-only SQL is used so sqlite and mysql behave identically.
func (s *Store) organizationDepartments(ctx context.Context) (map[string][]OrganizationDepartmentRecord, error) {
	var rows []struct {
		OrganizationID string  `gorm:"column:group_id"`
		ID             string  `gorm:"column:id"`
		Name           string  `gorm:"column:name"`
		ParentID       *string `gorm:"column:parent_id"`
	}
	if err := s.db.WithContext(ctx).
		Table("departments").
		Select("group_id, id, name, parent_id").
		Where("status = ?", departmentStatusActive).
		Order("group_id ASC, sort_order ASC, created_at ASC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read organization departments: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	counts, err := s.departmentMemberCounts(ctx)
	if err != nil {
		return nil, err
	}

	departments := make(map[string][]OrganizationDepartmentRecord, len(rows))
	for _, row := range rows {
		departments[row.OrganizationID] = append(departments[row.OrganizationID], OrganizationDepartmentRecord{
			ID:          row.ID,
			Name:        row.Name,
			MemberCount: counts[row.ID],
			ParentID:    row.ParentID,
		})
	}
	return departments, nil
}

// departmentMemberCounts returns the number of member rows per department, which is the
// count pre's own department tree shows.
func (s *Store) departmentMemberCounts(ctx context.Context) (map[string]int64, error) {
	var rows []struct {
		DepartmentID string `gorm:"column:department_id"`
		MemberCount  int64  `gorm:"column:member_count"`
	}
	if err := s.db.WithContext(ctx).
		Table("department_members").
		Select("department_id, COUNT(*) AS member_count").
		Group("department_id").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("count department members: %w", err)
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.DepartmentID] = row.MemberCount
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
		case <-s.reportNow:
			// A command just changed the organization membership. Unlike a tick, this
			// signal must not be dropped by the in-flight guard: a report that started
			// before the change would leave console stale until the next tick. So wait
			// for any running report and then report the state that now exists. The
			// command path only ever leaves one signal pending, so this is still a
			// single extra report per membership change.
			reporter.wait()
			reporter.start(ctx)
		}
	}
}

// requestOrganizationReport asks the inventory reporter for one immediate report.
//
// It is called from the command apply path, so it never blocks: the buffered signal
// channel holds at most one pending request and every further request is dropped, which
// coalesces a burst of membership changes into a single extra report. When the reporter is
// not running (reporting disabled, or a service built without NewService) this is a no-op.
func (s *Service) requestOrganizationReport() {
	if s.reportNow == nil {
		return
	}
	select {
	case s.reportNow <- struct{}{}:
	default:
		// A report is already pending; one is enough.
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
