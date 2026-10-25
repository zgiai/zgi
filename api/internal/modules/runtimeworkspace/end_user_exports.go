package runtimeworkspace

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// End-user exports run as jobs (ADR 0012): a request queues one, a worker
// writes every conversation and message page by page, and the compressed
// result is kept for a few days.
const (
	EndUserExportPending   = "pending"
	EndUserExportRunning   = "running"
	EndUserExportSucceeded = "succeeded"
	EndUserExportFailed    = "failed"

	endUserExportMaxCompressedBytes = 64 << 20
	endUserExportRetention          = 7 * 24 * time.Hour
	endUserExportLease              = 10 * time.Minute
	endUserExportMaxAttempts        = 3
	endUserExportPageSize           = 200
)

var (
	// ErrExportNotReady reports a download of an export that has not
	// succeeded.
	ErrExportNotReady = errors.New("runtime workspace end-user export is not ready")
	// ErrExportNotFound reports an unknown or expired export.
	ErrExportNotFound = errors.New("runtime workspace end-user export not found")
	errExportTooLarge = errors.New("runtime workspace end-user export is too large")
)

// EndUserExportJob is a queued, running, or finished export.
type EndUserExportJob struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey"`
	RuntimeWorkspaceID uuid.UUID  `gorm:"type:uuid;not null"`
	SubjectID          uuid.UUID  `gorm:"type:uuid;not null"`
	ActorKind          string     `gorm:"type:varchar(16);not null"`
	ActorID            *uuid.UUID `gorm:"type:uuid"`
	Status             string     `gorm:"type:varchar(16);not null"`
	Attempts           int        `gorm:"not null"`
	LeaseUntil         *time.Time
	Content            []byte
	SizeBytes          int64  `gorm:"not null"`
	Error              string `gorm:"type:varchar(500);not null"`
	CreatedAt          time.Time
	FinishedAt         *time.Time
	ExpiresAt          time.Time
}

func (EndUserExportJob) TableName() string { return "runtime_workspace_end_user_exports" }

// EndUserExportView is an export job as the APIs show it. SizeBytes is the
// compressed size.
type EndUserExportView struct {
	ID         uuid.UUID  `json:"id"`
	Status     string     `json:"status"`
	SizeBytes  int64      `json:"size_bytes"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
}

func endUserExportView(job EndUserExportJob) *EndUserExportView {
	return &EndUserExportView{ID: job.ID, Status: job.Status, SizeBytes: job.SizeBytes, Error: job.Error,
		CreatedAt: job.CreatedAt, FinishedAt: job.FinishedAt, ExpiresAt: job.ExpiresAt}
}

// RequestEndUserExport queues an export of an end user, or returns the one
// already queued or running.
func (s *Service) RequestEndUserExport(ctx context.Context, workspaceID, subjectID uuid.UUID, actor EndUserActor) (*EndUserExportView, error) {
	runtimeWorkspace, user, err := s.loadEndUser(ctx, s.db, workspaceID, subjectID, false)
	if err != nil {
		return nil, err
	}
	if user.Status == EndUserErased {
		return nil, ErrEndUserNotFound
	}
	now := s.now()
	job := EndUserExportJob{ID: uuid.New(), RuntimeWorkspaceID: runtimeWorkspace.LogicalID(), SubjectID: subjectID,
		ActorKind: actor.Kind, Status: EndUserExportPending, CreatedAt: now, ExpiresAt: now.Add(endUserExportRetention)}
	if actor.ID != uuid.Nil {
		id := actor.ID
		job.ActorID = &id
	}
	result := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:     []clause.Column{{Name: "subject_id"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "status IN ('pending', 'running')"}}},
		DoNothing:   true,
	}).Create(&job)
	if result.Error != nil {
		return nil, fmt.Errorf("queue end-user export: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var queued EndUserExportJob
		if err := s.db.WithContext(ctx).Omit("content").Where("subject_id = ? AND status IN ?", subjectID, []string{EndUserExportPending, EndUserExportRunning}).
			Take(&queued).Error; err != nil {
			return nil, fmt.Errorf("load queued end-user export: %w", err)
		}
		job = queued
	}
	return endUserExportView(job), nil
}

func (s *Service) loadEndUserExport(ctx context.Context, workspaceID, subjectID, exportID uuid.UUID, withContent bool) (*EndUserExportJob, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, s.endUserLoadError(err)
	}
	query := s.db.WithContext(ctx)
	if !withContent {
		query = query.Omit("content")
	}
	var job EndUserExportJob
	if err := query.Where("id = ? AND subject_id = ? AND runtime_workspace_id = ? AND expires_at > ?",
		exportID, subjectID, runtimeWorkspace.LogicalID(), s.now()).Take(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrExportNotFound
		}
		return nil, fmt.Errorf("load end-user export: %w", err)
	}
	return &job, nil
}

// GetEndUserExport reads an export job.
func (s *Service) GetEndUserExport(ctx context.Context, workspaceID, subjectID, exportID uuid.UUID) (*EndUserExportView, error) {
	job, err := s.loadEndUserExport(ctx, workspaceID, subjectID, exportID, false)
	if err != nil {
		return nil, err
	}
	return endUserExportView(*job), nil
}

// OpenEndUserExport returns the JSON of a succeeded export.
func (s *Service) OpenEndUserExport(ctx context.Context, workspaceID, subjectID, exportID uuid.UUID) (io.ReadCloser, error) {
	job, err := s.loadEndUserExport(ctx, workspaceID, subjectID, exportID, true)
	if err != nil {
		return nil, err
	}
	if job.Status != EndUserExportSucceeded {
		return nil, ErrExportNotReady
	}
	reader, err := gzip.NewReader(bytes.NewReader(job.Content))
	if err != nil {
		return nil, fmt.Errorf("open end-user export: %w", err)
	}
	return reader, nil
}

// ExportEndUser collects an end user's data in every bound app at once and
// records the export. Requests go through RequestEndUserExport; this is the
// same document, for callers inside the process.
func (s *Service) ExportEndUser(ctx context.Context, workspaceID, subjectID uuid.UUID, actor EndUserActor) (*EndUserExport, error) {
	var buffer bytes.Buffer
	if err := s.writeEndUserExport(ctx, &buffer, workspaceID, subjectID, actor); err != nil {
		return nil, err
	}
	var export EndUserExport
	if err := json.Unmarshal(buffer.Bytes(), &export); err != nil {
		return nil, fmt.Errorf("decode end-user export: %w", err)
	}
	return &export, nil
}

// exportWriter writes JSON pieces and keeps the first error.
type exportWriter struct {
	w   io.Writer
	err error
}

func (e *exportWriter) raw(text string) {
	if e.err == nil {
		_, e.err = io.WriteString(e.w, text)
	}
}

func (e *exportWriter) value(value any) {
	if e.err != nil {
		return
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		e.err = err
		return
	}
	_, e.err = e.w.Write(encoded)
}

// writeEndUserExport writes the EndUserExport document, conversations and
// messages page by page so no history is cut off.
func (s *Service) writeEndUserExport(ctx context.Context, w io.Writer, workspaceID, subjectID uuid.UUID, actor EndUserActor) error {
	view, err := s.GetEndUser(ctx, workspaceID, subjectID)
	if err != nil {
		return err
	}
	if view.Status == EndUserErased {
		return ErrEndUserNotFound
	}
	runtimeWorkspace, _, err := s.loadEndUser(ctx, s.db, workspaceID, subjectID, false)
	if err != nil {
		return err
	}
	db := s.db.WithContext(ctx)
	out := &exportWriter{w: w}
	out.raw(`{"exported_at":`)
	out.value(s.now())
	out.raw(`,"end_user":`)
	out.value(view)
	out.raw(`,"conversations":[`)
	if err := s.writeExportedConversations(db, out, subjectID); err != nil {
		return err
	}
	out.raw(`]`)

	var relationshipIDs []uuid.UUID
	if err := db.Table("user_memory_relationships").Where("subject_id = ?", subjectID).Pluck("id", &relationshipIDs).Error; err != nil {
		return fmt.Errorf("export end-user memory: %w", err)
	}
	memory := relationshipmemory.NewService(s.db)
	exportedMemory := make([]any, 0, len(relationshipIDs))
	for _, relationshipID := range relationshipIDs {
		exported, err := memory.Export(ctx, relationshipID)
		if err != nil {
			return fmt.Errorf("export end-user memory: %w", err)
		}
		exportedMemory = append(exportedMemory, exported)
	}
	out.raw(`,"memory":`)
	out.value(exportedMemory)

	files := []ExportedFile{}
	if err := db.Table("upload_files AS files").Select("files.id, files.name, files.size, files.mime_type, files.created_at").
		Joins("JOIN runtime_external_files AS bindings ON bindings.file_id = files.id").
		Where("bindings.subject_id = ?", subjectID).Order("files.created_at").Scan(&files).Error; err != nil {
		return fmt.Errorf("export end-user files: %w", err)
	}
	out.raw(`,"files":`)
	out.value(files)

	if err := s.writeExportedApps(ctx, out, subjectID); err != nil {
		return err
	}

	if err := recordEndUserEvent(db, runtimeWorkspace.LogicalID(), subjectID, EndUserEventExported, actor, s.now()); err != nil {
		return err
	}
	history := []EndUserEvent{}
	for page := 1; ; page++ {
		events, err := s.ListEndUserEvents(ctx, workspaceID, subjectID, page, 200)
		if err != nil {
			return err
		}
		history = append(history, events.Items...)
		if !events.HasMore {
			break
		}
	}
	out.raw(`,"history":`)
	out.value(history)
	out.raw(`}`)
	return out.err
}

// exportedConversationRow is an ExportedConversation without its messages.
type exportedConversationRow struct {
	ID         uuid.UUID  `json:"id"`
	CallerType string     `json:"caller_type"`
	CallerID   *uuid.UUID `json:"caller_id,omitempty"`
	Title      string     `json:"title"`
	CreatedAt  time.Time  `json:"created_at"`
}

// writeExportedApps writes each application domain's data under "apps",
// page by page for owners that can page it.
func (s *Service) writeExportedApps(ctx context.Context, out *exportWriter, subjectID uuid.UUID) error {
	first := true
	open := func(key string) {
		if first {
			out.raw(`,"apps":{`)
			first = false
		} else {
			out.raw(",")
		}
		out.value(key)
		out.raw(":")
	}
	for _, owner := range registeredEndUserDataOwners() {
		if pager, ok := owner.(EndUserExportPager); ok {
			key, sections := pager.EndUserExportSections()
			if key == "" {
				continue
			}
			open(key)
			out.raw("{")
			for index, section := range sections {
				if index > 0 {
					out.raw(",")
				}
				out.value(section)
				out.raw(":[")
				for offset := 0; ; offset += endUserExportPageSize {
					rows, err := pager.ExportEndUserPage(ctx, s.db, subjectID, section, offset, endUserExportPageSize)
					if err != nil {
						return err
					}
					for position, row := range rows {
						if offset > 0 || position > 0 {
							out.raw(",")
						}
						out.value(row)
					}
					if out.err != nil {
						return out.err
					}
					if len(rows) < endUserExportPageSize {
						break
					}
				}
				out.raw("]")
			}
			out.raw("}")
			continue
		}
		key, data, err := owner.ExportEndUser(ctx, s.db, subjectID)
		if err != nil {
			return err
		}
		if key != "" && data != nil {
			open(key)
			out.value(data)
		}
	}
	if !first {
		out.raw("}")
	}
	return out.err
}

type exportCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

func (s *Service) writeExportedConversations(db *gorm.DB, out *exportWriter, subjectID uuid.UUID) error {
	var after *exportCursor
	first := true
	for {
		var conversations []exportedConversationRow
		query := db.Table("chat_runtime_conversations").Select("id, caller_type, caller_id, title, created_at").
			Where("account_id = ? AND deleted_at IS NULL", subjectID)
		if after != nil {
			query = query.Where("(created_at, id) > (?, ?)", after.CreatedAt, after.ID)
		}
		if err := query.Order("created_at, id").Limit(endUserExportPageSize).Scan(&conversations).Error; err != nil {
			return fmt.Errorf("export end-user conversations: %w", err)
		}
		for _, conversation := range conversations {
			if !first {
				out.raw(",")
			}
			first = false
			// The conversation without its messages, which follow page by page.
			header, err := json.Marshal(conversation)
			if err != nil {
				return err
			}
			out.raw(string(header[:len(header)-1]) + `,"messages":[`)
			if err := writeExportedMessages(db, out, conversation.ID); err != nil {
				return err
			}
			out.raw(`]}`)
			if out.err != nil {
				return out.err
			}
		}
		if len(conversations) < endUserExportPageSize {
			return out.err
		}
		last := conversations[len(conversations)-1]
		after = &exportCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
}

func writeExportedMessages(db *gorm.DB, out *exportWriter, conversationID uuid.UUID) error {
	var after *exportCursor
	first := true
	for {
		var messages []ExportedMessage
		query := db.Table("chat_runtime_messages").Select("id, query, answer, status, created_at").
			Where("conversation_id = ? AND deleted_at IS NULL", conversationID)
		if after != nil {
			query = query.Where("(created_at, id) > (?, ?)", after.CreatedAt, after.ID)
		}
		if err := query.Order("created_at, id").Limit(endUserExportPageSize).Scan(&messages).Error; err != nil {
			return fmt.Errorf("export end-user messages: %w", err)
		}
		for _, message := range messages {
			if !first {
				out.raw(",")
			}
			first = false
			out.value(message)
		}
		if out.err != nil || len(messages) < endUserExportPageSize {
			return out.err
		}
		last := messages[len(messages)-1]
		after = &exportCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
}

// cappedBuffer refuses writes past a size.
type cappedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errExportTooLarge
	}
	return b.Buffer.Write(p)
}

// runEndUserExport writes one export, compressed.
func (s *Service) runEndUserExport(ctx context.Context, job EndUserExportJob) ([]byte, error) {
	buffer := &cappedBuffer{limit: endUserExportMaxCompressedBytes}
	compressed := gzip.NewWriter(buffer)
	actor := EndUserActor{Kind: job.ActorKind}
	if job.ActorID != nil {
		actor.ID = *job.ActorID
	}
	if err := s.writeEndUserExport(ctx, compressed, job.RuntimeWorkspaceID, job.SubjectID, actor); err != nil {
		return nil, err
	}
	if err := compressed.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// claimEndUserExport leases the oldest queued export, or one whose lease
// ran out.
func claimEndUserExport(ctx context.Context, db *gorm.DB, now time.Time) (*EndUserExportJob, error) {
	var claimed *EndUserExportJob
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var jobs []EndUserExportJob
		if err := tx.Omit("content").Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? OR status = ? AND lease_until < ?", EndUserExportPending, EndUserExportRunning, now).
			Order("created_at").Limit(1).Find(&jobs).Error; err != nil {
			return fmt.Errorf("claim end-user export: %w", err)
		}
		if len(jobs) == 0 {
			return nil
		}
		job := jobs[0]
		job.Attempts++
		lease := now.Add(endUserExportLease)
		job.Status, job.LeaseUntil = EndUserExportRunning, &lease
		if err := tx.Model(&EndUserExportJob{}).Where("id = ?", job.ID).
			Updates(map[string]any{"status": job.Status, "attempts": job.Attempts, "lease_until": lease}).Error; err != nil {
			return err
		}
		claimed = &job
		return nil
	})
	return claimed, err
}

// finishEndUserExport stores the result, retries a failure a few times, or
// gives up with a reason.
func finishEndUserExport(ctx context.Context, db *gorm.DB, job EndUserExportJob, content []byte, runErr error, now time.Time) error {
	updates := map[string]any{"lease_until": nil}
	switch {
	case runErr == nil:
		updates["status"], updates["content"], updates["size_bytes"] = EndUserExportSucceeded, content, int64(len(content))
		updates["finished_at"], updates["expires_at"], updates["error"] = now, now.Add(endUserExportRetention), ""
	case errors.Is(runErr, errExportTooLarge):
		updates["status"], updates["finished_at"], updates["error"] = EndUserExportFailed, now, "The export is larger than 64 MB compressed."
	case errors.Is(runErr, ErrEndUserNotFound):
		updates["status"], updates["finished_at"], updates["error"] = EndUserExportFailed, now, "The end user no longer exists."
	case job.Attempts < endUserExportMaxAttempts:
		updates["status"] = EndUserExportPending
	default:
		updates["status"], updates["finished_at"], updates["error"] = EndUserExportFailed, now, "The export failed. Request a new one."
	}
	return db.WithContext(ctx).Model(&EndUserExportJob{}).Where("id = ?", job.ID).Updates(updates).Error
}
