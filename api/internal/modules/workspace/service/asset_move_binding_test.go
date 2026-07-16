package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/dto"
)

func TestMoveImpactRequestFromPreviewBuildsBatch(t *testing.T) {
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	sourceWorkspaceID := uuid.NewString()
	targetWorkspaceID := uuid.NewString()
	agentID := uuid.NewString()
	datasetID := uuid.NewString()
	databaseID := uuid.NewString()
	request, ok, err := moveImpactRequestFromPreview(organizationID, accountID, dto.WorkspaceAssetMovePreviewResponse{
		Movable: true,
		Items: []dto.WorkspaceAssetMovePreviewItem{{
			Type:              AssetMoveTypeAgent,
			ID:                agentID,
			ResolvedAgentID:   agentID,
			ResolvedAgentType: "WORKFLOW",
			FromWorkspaceID:   sourceWorkspaceID,
			TargetWorkspaceID: targetWorkspaceID,
			Movable:           true,
		}, {
			Type:              AssetMoveTypeDataset,
			ID:                datasetID,
			FromWorkspaceID:   sourceWorkspaceID,
			TargetWorkspaceID: targetWorkspaceID,
			Movable:           true,
		}, {
			Type:              AssetMoveTypeDatabase,
			ID:                databaseID,
			FromWorkspaceID:   sourceWorkspaceID,
			TargetWorkspaceID: targetWorkspaceID,
			Movable:           true,
		}, {
			Type:              AssetMoveTypeFile,
			ID:                uuid.NewString(),
			FromWorkspaceID:   sourceWorkspaceID,
			TargetWorkspaceID: targetWorkspaceID,
			Movable:           true,
		}},
	})
	if err != nil || !ok {
		t.Fatalf("moveImpactRequestFromPreview() ok = %v, error = %v", ok, err)
	}
	if len(request.MovingAgentIDs) != 1 || request.MovingAgentIDs[0].String() != agentID {
		t.Fatalf("moving agent IDs = %v, want %s", request.MovingAgentIDs, agentID)
	}
	if len(request.ResourceRefs) != 3 {
		t.Fatalf("resource refs = %#v, want workflow, dataset, database", request.ResourceRefs)
	}
	want := map[agentbindings.BindingType]string{
		agentbindings.BindingTypeWorkflow:         agentID,
		agentbindings.BindingTypeKnowledgeDataset: datasetID,
		agentbindings.BindingTypeDatabase:         databaseID,
	}
	for _, ref := range request.ResourceRefs {
		if want[ref.BindingType] != ref.ResourceID {
			t.Fatalf("resource ref = %#v, want %s", ref, want[ref.BindingType])
		}
	}
}
