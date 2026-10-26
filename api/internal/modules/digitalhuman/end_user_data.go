package digitalhuman

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/runtimeworkspace"
	"gorm.io/gorm"
)

// endUserExportPage is how many records of a kind one export page reads.
const endUserExportPage = 500

// EndUserData erases and exports what DigitalHumans keep for a Runtime
// Workspace end user: instances and their memory, runs, tasks with their
// reports, notifications, and artifacts (ADR 0010).
type EndUserData struct{}

// NewEndUserData returns the DigitalHuman end-user data owner.
func NewEndUserData() *EndUserData { return endUserData }

var endUserData = &EndUserData{}

var _ runtimeworkspace.EndUserDataOwner = (*EndUserData)(nil)

// EraseEndUser deletes an end user's DigitalHuman data in tx. Instances stay
// as erased tombstones unless purge is set.
func (*EndUserData) EraseEndUser(ctx context.Context, tx *gorm.DB, subjectID uuid.UUID, purge bool) (runtimeworkspace.EndUserErasure, error) {
	var erasure runtimeworkspace.EndUserErasure
	db := tx.WithContext(ctx)
	// The memory event guard accepts this end user's erasure for the rest of
	// the transaction.
	if err := db.Exec("SELECT set_config('zgi.digital_human_end_user_erase', ?, true)", subjectID.String()).Error; err != nil {
		return erasure, fmt.Errorf("open DigitalHuman end-user erasure: %w", err)
	}
	var runIDs []uuid.UUID
	if err := db.Table("digital_human_runs").Where("user_subject_id = ?", subjectID).Pluck("id", &runIDs).Error; err != nil {
		return erasure, fmt.Errorf("list end-user runs: %w", err)
	}

	if len(runIDs) > 0 {
		for _, statement := range []string{
			// Events of the DigitalHuman's own memory keep their history but
			// lose the end user's run.
			"UPDATE digital_human_memory_events SET run_id = NULL WHERE run_id IN ?",
			"DELETE FROM digital_human_action_ledger WHERE run_id IN ?",
			"DELETE FROM digital_human_memory_jobs WHERE run_id IN ?",
		} {
			if err := db.Exec(statement, runIDs).Error; err != nil {
				return erasure, fmt.Errorf("detach end-user runs: %w", err)
			}
		}
	}

	var toolFileIDs []string
	if err := db.Raw(`
		SELECT file ->> 'id' FROM digital_human_task_reports report
		JOIN automation_tasks task ON task.id = report.task_id,
		LATERAL jsonb_array_elements(COALESCE(report.files, '[]'::jsonb)) file
		WHERE task.owner_subject_id = ?
		UNION
		SELECT version.file_id FROM artifact_versions version
		JOIN artifacts artifact ON artifact.id = version.artifact_id
		WHERE artifact.owner_kind = 'api_end_user' AND artifact.owner_id = ? AND COALESCE(version.file_id, '') <> ''
	`, subjectID, subjectID).Scan(&toolFileIDs).Error; err != nil {
		return erasure, fmt.Errorf("list end-user report and artifact files: %w", err)
	}
	if fileIDs := validUUIDs(toolFileIDs); len(fileIDs) > 0 {
		if err := db.Table("tool_files").Where("id IN ? AND file_key <> ''", fileIDs).Pluck("file_key", &erasure.ObjectKeys).Error; err != nil {
			return erasure, fmt.Errorf("list end-user report and artifact objects: %w", err)
		}
		if err := db.Exec("DELETE FROM tool_files WHERE id IN ?", fileIDs).Error; err != nil {
			return erasure, fmt.Errorf("delete end-user report and artifact files: %w", err)
		}
	}
	for _, statement := range []string{
		"DELETE FROM automation_tasks WHERE owner_subject_id = ?",
		"DELETE FROM inbox_notifications WHERE recipient_kind = 'api_end_user' AND recipient_id = ?",
		"DELETE FROM artifacts WHERE owner_kind = 'api_end_user' AND owner_id = ?",
		"DELETE FROM digital_human_runs WHERE user_subject_id = ?",
	} {
		if err := db.Exec(statement, subjectID).Error; err != nil {
			return erasure, fmt.Errorf("erase end-user DigitalHuman data: %w", err)
		}
	}
	if err := eraseEndUserConnections(db, subjectID, time.Now().UTC()); err != nil {
		return erasure, err
	}
	var erased []EndUserInstance
	if err := db.Where("subject_id = ? AND status <> ?", subjectID, EndUserInstanceErased).Find(&erased).Error; err != nil {
		return erasure, fmt.Errorf("load end-user instances: %w", err)
	}
	for _, instance := range erased {
		if err := recordInstanceEvent(ctx, db, instance, instanceAuditErased, accessAuditSourceExternalAccess, nil, map[string]any{"purge": purge}); err != nil {
			return erasure, err
		}
	}
	if purge {
		err := db.Exec("DELETE FROM digital_human_end_user_instances WHERE subject_id = ?", subjectID).Error
		if err != nil {
			return erasure, fmt.Errorf("delete end-user instances: %w", err)
		}
	} else if err := db.Exec("UPDATE digital_human_end_user_instances SET status = 'erased', label = '', quota_policy = '{}', updated_at = ? WHERE subject_id = ?", time.Now().UTC(), subjectID).Error; err != nil {
		return erasure, fmt.Errorf("mark end-user instances erased: %w", err)
	}
	return erasure, nil
}

func validUUIDs(values []string) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if id, err := uuid.Parse(value); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// EndUserDataExport is what DigitalHumans keep for an end user.
type EndUserDataExport struct {
	Instances     []map[string]any `json:"instances"`
	Runs          []map[string]any `json:"runs"`
	Tasks         []map[string]any `json:"tasks"`
	Reports       []map[string]any `json:"reports"`
	Notifications []map[string]any `json:"notifications"`
	Artifacts     []map[string]any `json:"artifacts"`
}

// endUserExportSections are the kinds of DigitalHuman records an end user
// has, each ordered so pages are stable.
var endUserExportSections = []struct {
	name string
	sql  string
}{
	{"instances", `SELECT id, digital_human_id, status, first_seen_at, last_active_at FROM digital_human_end_user_instances WHERE subject_id = ? AND status <> 'erased' ORDER BY first_seen_at, id`},
	{"runs", `SELECT id, digital_human_id, conversation_id, query, status, result_summary, created_at, completed_at FROM digital_human_runs WHERE user_subject_id = ? ORDER BY created_at, id`},
	{"tasks", `SELECT id, subject_id AS digital_human_id, name, goal, status, work_status, schedule_type, schedule_config, created_at FROM automation_tasks WHERE owner_subject_id = ? ORDER BY created_at, id`},
	{"reports", `SELECT report.id, report.task_id, report.outcome, report.summary, report.body, report.issues, report.files, report.created_at
		FROM digital_human_task_reports report JOIN automation_tasks task ON task.id = report.task_id WHERE task.owner_subject_id = ? ORDER BY report.created_at, report.id`},
	{"notifications", `SELECT id, event, title, summary, read_at, created_at FROM inbox_notifications WHERE recipient_kind = 'api_end_user' AND recipient_id = ? ORDER BY created_at, id`},
	{"artifacts", `SELECT artifact.id, artifact.application_id AS digital_human_id, artifact.kind, artifact.title, artifact.current_version, version.content, version.file_name, artifact.created_at
		FROM artifacts artifact LEFT JOIN artifact_versions version ON version.artifact_id = artifact.id AND version.number = artifact.current_version
		WHERE artifact.owner_kind = 'api_end_user' AND artifact.owner_id = ? AND artifact.deleted_at IS NULL ORDER BY artifact.created_at, artifact.id`},
}

// EndUserExportSections names the DigitalHuman part of an export and its
// kinds of records.
func (*EndUserData) EndUserExportSections() (string, []string) {
	names := make([]string, 0, len(endUserExportSections))
	for _, section := range endUserExportSections {
		names = append(names, section.name)
	}
	return "digital_humans", names
}

// ExportEndUserPage reads one page of one kind of an end user's
// DigitalHuman records.
func (*EndUserData) ExportEndUserPage(ctx context.Context, db *gorm.DB, subjectID uuid.UUID, section string, offset, limit int) ([]map[string]any, error) {
	for _, candidate := range endUserExportSections {
		if candidate.name != section {
			continue
		}
		rows := []map[string]any{}
		if err := db.WithContext(ctx).Raw(candidate.sql+" LIMIT ? OFFSET ?", subjectID, limit, offset).Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("export end-user DigitalHuman %s: %w", section, err)
		}
		return rows, nil
	}
	return nil, fmt.Errorf("unknown end-user DigitalHuman export section %q", section)
}

// ExportEndUser collects all of an end user's DigitalHuman data at once.
func (data *EndUserData) ExportEndUser(ctx context.Context, db *gorm.DB, subjectID uuid.UUID) (string, any, error) {
	export := &EndUserDataExport{}
	targets := map[string]*[]map[string]any{"instances": &export.Instances, "runs": &export.Runs,
		"tasks": &export.Tasks, "reports": &export.Reports, "notifications": &export.Notifications, "artifacts": &export.Artifacts}
	for _, section := range endUserExportSections {
		all := []map[string]any{}
		for offset := 0; ; offset += endUserExportPage {
			rows, err := data.ExportEndUserPage(ctx, db, subjectID, section.name, offset, endUserExportPage)
			if err != nil {
				return "", nil, err
			}
			all = append(all, rows...)
			if len(rows) < endUserExportPage {
				break
			}
		}
		*targets[section.name] = all
	}
	return "digital_humans", export, nil
}
