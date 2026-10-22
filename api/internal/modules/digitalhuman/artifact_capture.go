package digitalhuman

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/artifacts"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// ArtifactReportCapture keeps the files a final task report carries in the
// artifact center (ADR 0009), owned by the report's requester. A recurring
// task's file of the same name becomes a new version of one artifact.
type ArtifactReportCapture struct {
	runtime   *RuntimeService
	artifacts *artifacts.Service
}

// NewArtifactReportCapture builds the capture of report files.
func NewArtifactReportCapture(runtime *RuntimeService, service *artifacts.Service) *ArtifactReportCapture {
	return &ArtifactReportCapture{runtime: runtime, artifacts: service}
}

// NotifyTaskReport captures the report's files. A file that cannot be kept
// is logged and skipped; it never fails the report.
func (c *ArtifactReportCapture) NotifyTaskReport(ctx context.Context, notice TaskReportNotice) error {
	if c == nil || c.runtime == nil || c.artifacts == nil || !ArtifactsEnabled() || notice.RunID == nil || notice.RecipientID == uuid.Nil {
		return nil
	}
	owner := artifacts.Owner{ID: notice.RecipientID}
	visibility := artifacts.VisibilityOwner
	switch notice.RecipientKind {
	case "account":
		owner.Kind = artifacts.OwnerAccount
		if notice.Shared {
			visibility = artifacts.VisibilityTeam
		}
	case "api_end_user":
		owner.Kind = artifacts.OwnerAPIEndUser
	default:
		return nil
	}
	scope := Scope{OrganizationID: notice.OrganizationID, WorkspaceID: notice.WorkspaceID}
	report := &TaskReport{ID: notice.ReportID, DigitalHumanID: notice.DigitalHumanID, DigitalHumanRunID: notice.RunID, FileIDs: notice.FileIDs}
	files, err := c.runtime.TaskReportFiles(ctx, scope, report)
	if err != nil {
		logger.WarnContext(ctx, "DigitalHuman report files could not be listed for the artifact center", "report_id", notice.ReportID.String(), err)
		return nil
	}
	taskID, reportID, runID := notice.TaskID, notice.ReportID, *notice.RunID
	for _, file := range files {
		stored, data, err := c.runtime.ReadTaskReportFile(ctx, scope, report, file.ID)
		if err != nil || stored == nil {
			logger.WarnContext(ctx, "DigitalHuman report file could not be read for the artifact center", "report_id", notice.ReportID.String(), "file_id", file.ID, err)
			continue
		}
		_, err = c.artifacts.Publish(ctx, artifacts.PublishRequest{
			Scope: artifactScope(notice.OrganizationID, notice.WorkspaceID, notice.DigitalHumanID), Owner: owner, Visibility: visibility,
			Kind: artifactKindFor(file.MimeType, file.Name), Title: file.Name, File: &artifacts.Blob{Name: file.Name, MimeType: file.MimeType, Data: data},
			Source: artifacts.SourceTaskReport, SourceTaskID: &taskID, SourceReportID: &reportID, SourceRunID: &runID,
			Actor: artifacts.Actor{Kind: "digital_human_run", ID: runID.String()},
		})
		if err != nil {
			logger.WarnContext(ctx, "DigitalHuman report file could not be kept in the artifact center", "report_id", notice.ReportID.String(), "file_id", file.ID, err)
		}
	}
	return nil
}

var _ TaskReportNotifier = (*ArtifactReportCapture)(nil)
