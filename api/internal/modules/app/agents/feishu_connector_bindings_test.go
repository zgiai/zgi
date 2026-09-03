package agents

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
)

func TestFeishuAgentUsesExistingPersonalBindingAndSnapshotLifecycle(t *testing.T) {
	org, owner, agentID, first, second := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo := newMemoryAgentConnectorBindingRepository()
	repo.connections[first] = memoryAgentConnectorConnection{organizationID: org, ownerID: owner, connectorID: "feishu"}
	repo.connections[second] = memoryAgentConnectorConnection{organizationID: org, ownerID: owner, connectorID: "feishu"}
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: agentID.String()}
	before := syncAgentConnectorBindingsWithRepository(t, repo, org, owner, target, []dto.AgentConnectorBinding{{ConnectorID: "feishu", ConnectionID: first.String()}})
	after := syncAgentConnectorBindingsWithRepository(t, repo, org, owner, target, []dto.AgentConnectorBinding{{ConnectorID: "feishu", ConnectionID: second.String()}})
	if len(before) != 1 || len(after) != 1 || after[0].ID != before[0].ID || after[0].Revision != before[0].Revision+1 || after[0].ConnectionID != second {
		t.Fatal("app switch did not use stable personal binding")
	}
	config := &AgentsConfig{}
	if _, err := applyAgentConfigRequestToDraft(config, dto.AgentConfigRequest{ModelParameters: map[string]interface{}{}, ConnectorBindings: []dto.AgentConnectorBinding{{ConnectorID: "feishu", ConnectionID: second.String()}}}, owner.String()); err != nil {
		t.Fatal(err)
	}
	snapshot := agentConfigSnapshot(agentID.String(), config)
	restored := agentConfigResponseFromSnapshot(agentID.String(), snapshot)
	if len(restored.ConnectorBindings) != 1 || restored.ConnectorBindings[0].ConnectionID != second.String() {
		t.Fatal("published/history identity lost")
	}
}
