package digitalhuman

import (
	"context"
	"fmt"
	"path"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	executionruntime "github.com/zgiai/zgi/api/internal/capabilities/agentexecution/runtime"
	"github.com/zgiai/zgi/api/internal/capabilities/artifacts"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// exportArtifactPersister keeps every file a run exports from its sandbox in
// the artifact center (ADR 0009), besides the conversation file the export
// creates, so later conversations can find and revise it. A file exported
// again under the same name in the conversation, or by the same task,
// becomes a new version. Keeping it never fails the export.
type exportArtifactPersister struct {
	next      executionruntime.ArtifactPersister
	artifacts *artifacts.Service
	run       Run
	event     map[string]any
}

// withExportArtifacts wraps next when the run has an owner to keep
// artifacts for; otherwise exports stay conversation files only.
func withExportArtifacts(next executionruntime.ArtifactPersister, service *artifacts.Service, run Run, event map[string]any) executionruntime.ArtifactPersister {
	if next == nil || service == nil {
		return next
	}
	return &exportArtifactPersister{next: next, artifacts: service, run: run, event: event}
}

func (p *exportArtifactPersister) Persist(ctx context.Context, scope execution.Scope, conversationID, filePath string, data []byte) (map[string]interface{}, error) {
	result, err := p.next.Persist(ctx, scope, conversationID, filePath, data)
	if err != nil || len(result) == 0 || !ArtifactsEnabled() {
		return result, err
	}
	owner, visibility, _, ok := runArtifactOwner(&p.run, p.event)
	conversation, convErr := uuid.Parse(conversationID)
	if !ok || convErr != nil || conversation == uuid.Nil {
		return result, nil
	}
	name := safeChannelArtifactFilename(path.Base(filePath))
	if name == "" {
		name = "generated-file" + path.Ext(filePath)
	}
	title, _ := truncateArtifactText(name, artifacts.MaxTitleRunes)
	mimeType, _ := result["mime_type"].(string)
	runID := p.run.ID
	published, err := p.artifacts.Publish(ctx, artifacts.PublishRequest{
		Scope: artifactScope(p.run.OrganizationID, p.run.WorkspaceID, p.run.DigitalHumanID), Owner: owner, Visibility: visibility,
		Kind: artifactKindFor(mimeType, name), Title: title, File: &artifacts.Blob{Name: name, MimeType: mimeType, Data: data},
		Source: artifacts.SourceRun, SourceTaskID: artifactContextUUID(p.event, "task_id"), SourceConversationID: &conversation, SourceRunID: &runID,
		Actor: artifacts.Actor{Kind: "digital_human_run", ID: runID.String()},
	})
	if err != nil {
		logger.WarnContext(ctx, "DigitalHuman exported file could not be kept in the artifact center", "run_id", runID.String(), err)
		return result, nil
	}
	result["kept_artifact_id"] = published.Artifact.ID.String()
	result["kept_artifact_version"] = published.Version.Number
	result["kept_artifact_note"] = fmt.Sprintf("Kept in the artifact center as %q, version %d.", published.Artifact.Title, published.Version.Number)
	return result, nil
}

var _ executionruntime.ArtifactPersister = (*exportArtifactPersister)(nil)
