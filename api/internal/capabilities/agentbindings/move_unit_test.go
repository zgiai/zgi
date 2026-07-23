package agentbindings

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestClassifyMoveImpactBindingsPreservesOnlyCoMovedPairs(t *testing.T) {
	movingAgentID := uuid.New()
	stationaryAgentID := uuid.New()
	request := MoveImpactRequest{
		OrganizationID:    uuid.New(),
		TargetWorkspaceID: uuid.New(),
		ResourceRefs: []ResourceRef{{
			BindingType: BindingTypeKnowledgeDataset,
			ResourceID:  "dataset-moving",
		}, {
			BindingType: BindingTypeDatabase,
			ResourceID:  "database-moving",
		}, {
			BindingType: BindingTypeWorkflow,
			ResourceID:  "workflow-agent-moving",
		}},
		MovingAgentIDs: []uuid.UUID{movingAgentID},
		ActorID:        uuid.New(),
	}
	resourceBindings := []Binding{
		{ID: uuid.New(), AgentID: movingAgentID, BindingType: BindingTypeKnowledgeDataset, ResourceID: "dataset-moving"},
		{ID: uuid.New(), AgentID: stationaryAgentID, BindingType: BindingTypeKnowledgeDataset, ResourceID: "dataset-moving"},
		{ID: uuid.New(), AgentID: movingAgentID, BindingType: BindingTypeDatabaseTable, ParentResourceID: "database-moving", ResourceID: "table-moving"},
		{ID: uuid.New(), AgentID: stationaryAgentID, BindingType: BindingTypeDatabaseTable, ParentResourceID: "database-moving", ResourceID: "table-moving"},
		{ID: uuid.New(), AgentID: movingAgentID, BindingType: BindingTypeWorkflow, ParentResourceID: "workflow-agent-moving", ResourceID: "workflow-binding-moving"},
		{ID: uuid.New(), AgentID: stationaryAgentID, BindingType: BindingTypeWorkflow, ParentResourceID: "workflow-agent-moving", ResourceID: "workflow-binding-moving"},
	}
	agentBindings := []Binding{
		resourceBindings[0],
		resourceBindings[2],
		resourceBindings[4],
		{ID: uuid.New(), AgentID: movingAgentID, BindingType: BindingTypeKnowledgeDataset, ResourceID: "dataset-left"},
		{ID: uuid.New(), AgentID: movingAgentID, BindingType: BindingTypeSkill, ResourceID: "calculator"},
	}

	got := classifyMoveImpactBindings(request, resourceBindings, agentBindings)
	if len(got) != 4 {
		t.Fatalf("impacted bindings = %#v, want stationary dataset/table/workflow and moving agent's left dataset", got)
	}
	want := map[string]bool{
		stationaryAgentID.String() + "|knowledge_dataset|dataset-moving": false,
		stationaryAgentID.String() + "|database_table|table-moving":      false,
		stationaryAgentID.String() + "|workflow|workflow-binding-moving": false,
		movingAgentID.String() + "|knowledge_dataset|dataset-left":       false,
	}
	for _, binding := range got {
		key := binding.AgentID.String() + "|" + string(binding.BindingType) + "|" + binding.ResourceID
		if _, ok := want[key]; !ok {
			t.Fatalf("unexpected impacted binding %s", key)
		}
		want[key] = true
	}
	for key, found := range want {
		if !found {
			t.Fatalf("missing impacted binding %s", key)
		}
	}
}

func TestMoveImpactTokenBindsEntireBatch(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	request := MoveImpactRequest{
		OrganizationID:    uuid.New(),
		TargetWorkspaceID: uuid.New(),
		ResourceRefs: []ResourceRef{{
			BindingType: BindingTypeKnowledgeDataset,
			ResourceID:  "dataset-1",
		}},
		MovingAgentIDs: []uuid.UUID{uuid.New()},
		ActorID:        uuid.New(),
	}
	normalized, err := normalizeMoveImpactRequest(request)
	if err != nil {
		t.Fatalf("normalizeMoveImpactRequest() error = %v", err)
	}
	payload := moveImpactPayload(normalized, []Binding{{
		ID:          uuid.New(),
		AgentID:     normalized.MovingAgentIDs[0],
		BindingType: BindingTypeKnowledgeDataset,
		ResourceID:  "dataset-left",
	}}, now.Add(ImpactTokenTTL))
	token, err := encodeMoveImpactToken([]byte("shared-secret"), payload)
	if err != nil {
		t.Fatalf("encodeMoveImpactToken() error = %v", err)
	}
	decoded, err := decodeMoveImpactToken([]byte("shared-secret"), token)
	if err != nil {
		t.Fatalf("decodeMoveImpactToken() error = %v", err)
	}
	if !moveImpactPayloadEqual(decoded, payload) {
		t.Fatalf("decoded payload = %#v, want %#v", decoded, payload)
	}
	if _, err := decodeMoveImpactToken([]byte("another-secret"), token); !errors.Is(err, ErrImpactTokenInvalid) {
		t.Fatalf("decode with another secret error = %v, want ErrImpactTokenInvalid", err)
	}
	changed := payload
	changed.TargetWorkspaceID = uuid.NewString()
	if moveImpactPayloadEqual(decoded, changed) {
		t.Fatal("move impact payload accepted changed target workspace")
	}
	changed = payload
	changed.MovingAgentIDs = append([]string{}, payload.MovingAgentIDs...)
	changed.MovingAgentIDs = append(changed.MovingAgentIDs, uuid.NewString())
	if moveImpactPayloadEqual(decoded, changed) {
		t.Fatal("move impact payload accepted changed moving agent set")
	}
}

func TestNormalizeMoveDependenciesDoesNotRequireTargetOrActor(t *testing.T) {
	organizationID := uuid.New()
	agentID := uuid.New()
	request, err := normalizeMoveDependencyRequest(MoveDependencyRequest{
		OrganizationID: organizationID,
		ResourceRefs: []ResourceRef{{
			BindingType: BindingTypeKnowledgeDataset,
			ResourceID:  " dataset-1 ",
		}, {
			BindingType: BindingTypeKnowledgeDataset,
			ResourceID:  "dataset-1",
		}},
		MovingAgentIDs: []uuid.UUID{agentID, agentID},
	})
	if err != nil {
		t.Fatalf("normalizeMoveDependencyRequest() error = %v", err)
	}
	if len(request.ResourceRefs) != 1 || request.ResourceRefs[0].ResourceID != "dataset-1" {
		t.Fatalf("resource refs = %#v, want one normalized dataset", request.ResourceRefs)
	}
	if len(request.MovingAgentIDs) != 1 || request.MovingAgentIDs[0] != agentID {
		t.Fatalf("moving Agent IDs = %#v, want %s", request.MovingAgentIDs, agentID)
	}
}
