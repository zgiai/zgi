package digitalhuman

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	toolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

// TaskReportFileSource resolves and reads the files a task report carries.
// Callers authorize the reader for the report first; a report's files are
// then readable by whoever may read the report (ADR 0009, phase 0).
type TaskReportFileSource interface {
	TaskReportFiles(ctx context.Context, scope Scope, report *TaskReport) ([]TaskReportFile, error)
	ReadTaskReportFile(ctx context.Context, scope Scope, report *TaskReport, fileID string) (*toolfile.ToolFile, []byte, error)
}

// reportRunFiles lists the files of the run behind report that it carries:
// the ones the DigitalHuman chose, or every file the run produced. A file
// qualifies only while its run still owns it.
func (s *RuntimeService) reportRunFiles(ctx context.Context, scope Scope, report *TaskReport) (*Run, []*toolfile.ToolFile, error) {
	if s == nil || report == nil || report.DigitalHumanRunID == nil || toolfile.GlobalToolFileManager == nil {
		return nil, nil, nil
	}
	run, err := s.store.RunByID(ctx, Scope{OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID}, report.DigitalHumanID, *report.DigitalHumanRunID)
	if err != nil || run == nil {
		return nil, nil, nil
	}
	ids := make([]string, 0, MaxTaskReportFiles)
	for _, artifact := range channelDeliveryArtifacts(run.ExecutionTrace) {
		id, err := uuid.Parse(artifact.ToolFileID)
		if err != nil || (len(report.FileIDs) > 0 && !slices.Contains(report.FileIDs, id)) {
			continue
		}
		ids = append(ids, id.String())
	}
	if len(ids) == 0 {
		return run, nil, nil
	}
	stored, err := toolfile.GetToolFilesByIDsGlobal(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	files := make([]*toolfile.ToolFile, 0, len(ids))
	for _, id := range ids {
		file := stored[id]
		if file == nil || file.TenantID != run.OrganizationID.String() || file.DeletedAt != nil || file.IsExpired(now) {
			continue
		}
		if protectedToolFile(file) {
			if _, err := backgroundExecutionFileAccess(run, file); err != nil {
				continue
			}
		}
		files = append(files, file)
	}
	return run, files, nil
}

// TaskReportFiles describes the files a report carries.
func (s *RuntimeService) TaskReportFiles(ctx context.Context, scope Scope, report *TaskReport) ([]TaskReportFile, error) {
	_, files, err := s.reportRunFiles(ctx, scope, report)
	if err != nil {
		return nil, err
	}
	result := make([]TaskReportFile, 0, len(files))
	for _, file := range files {
		name := safeChannelArtifactFilename(file.Name)
		if name == "" {
			name = "generated-file" + file.GetFileExtension()
		}
		result = append(result, TaskReportFile{ID: file.ID, Name: name, MimeType: file.MimeType, Size: file.Size})
	}
	return result, nil
}

// ReadTaskReportFile reads one file the report carries.
func (s *RuntimeService) ReadTaskReportFile(ctx context.Context, scope Scope, report *TaskReport, fileID string) (*toolfile.ToolFile, []byte, error) {
	run, files, err := s.reportRunFiles(ctx, scope, report)
	if err != nil {
		return nil, nil, err
	}
	for _, file := range files {
		if file.ID != fileID {
			continue
		}
		manager := toolfile.GlobalToolFileManager
		if protectedToolFile(file) {
			access, err := backgroundExecutionFileAccess(run, file)
			if err != nil {
				return nil, nil, err
			}
			data, current, err := manager.GetExecutionFileBinary(ctx, access, file.ID)
			return current, data, err
		}
		data, _, err := manager.GetFileBinary(ctx, file.ID)
		return file, data, err
	}
	return nil, nil, ErrNotFound
}

// protectedToolFile reports whether a file belongs to a sandboxed execution
// and must be read through its execution identity.
func protectedToolFile(file *toolfile.ToolFile) bool {
	return file.ExecutionIdentity != nil || file.ExecutionMessageID != nil
}

var _ TaskReportFileSource = (*RuntimeService)(nil)
