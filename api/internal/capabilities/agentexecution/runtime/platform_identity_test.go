package executionruntime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeplatform"
)

type identityCheckingGateway struct {
	*fakePlatformGateway
	identity execution.Identity
	canceled bool
}

func (g *identityCheckingGateway) check(scope agentExecutionScope) error {
	if scope.Identity == nil || *scope.Identity != g.identity {
		return errors.New("execution identity was lost")
	}
	return nil
}

func (g *identityCheckingGateway) GetBinding(ctx context.Context, scope agentExecutionScope, id string) (agentExecutionBinding, error) {
	if err := g.check(scope); err != nil {
		return agentExecutionBinding{}, err
	}
	return g.fakePlatformGateway.GetBinding(ctx, scope, id)
}

func (g *identityCheckingGateway) GetJob(ctx context.Context, scope agentExecutionScope, id string) (agentExecutionJob, error) {
	if err := g.check(scope); err != nil {
		return agentExecutionJob{}, err
	}
	return g.fakePlatformGateway.GetJob(ctx, scope, id)
}

func (g *identityCheckingGateway) Capabilities(ctx context.Context, scope agentExecutionScope, id string) (agentExecutionCapabilities, error) {
	if err := g.check(scope); err != nil {
		return agentExecutionCapabilities{}, err
	}
	return g.fakePlatformGateway.Capabilities(ctx, scope, id)
}

func (g *identityCheckingGateway) CancelJob(ctx context.Context, scope agentExecutionScope, id string, generation int64) error {
	if err := g.check(scope); err != nil {
		return err
	}
	g.canceled = true
	return g.fakePlatformGateway.CancelJob(ctx, scope, id, generation)
}

func TestPlatformRelayAndFileOperationsRetainExecutionIdentity(t *testing.T) {
	relay, original := newTestPlatformRelay(t)
	identity := execution.Identity{Version: 1, WorkspaceID: "resource-workspace", ApplicationType: "digital_human",
		ApplicationID: "digital-human", Entry: "webapp", SubjectKind: "account", ConversationID: relay.scope.ConversationID, PublicationRef: "published-v1"}
	gateway := &identityCheckingGateway{fakePlatformGateway: original, identity: identity}
	relay.agentExecutionGateway, relay.platform = gateway, gateway
	relay.scope.AgentID, relay.scope.ExecutionIdentity = "", identity
	scope := agentExecutionScope{OrganizationID: relay.scope.OrganizationID, SubjectID: relay.scope.SubjectID, Identity: &identity}
	if _, err := relay.SubmitJob(t.Context(), scope, "binding-1", "tool:message:call", "/bin/sh", nil, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := relay.authorizeRequest(t.Context(), relay.scope, runtimeplatform.ReadFile); err != nil {
		t.Fatal(err)
	}
	original.capabilities.FileTransfer, original.capabilities.MaxDownloadBytes = true, 1024
	original.downloads = map[string][]byte{"result.txt": []byte("result")}
	files := &agentPlatformFiles{gateway: gateway, persister: &fakeAgentExecutionArtifactPersister{}}
	if _, err := files.Handle(t.Context(), relay.scope, runtimeplatform.Request{ID: "file", Action: runtimeplatform.ExportFile, Arguments: json.RawMessage(`{"path":"result.txt"}`)}); err != nil {
		t.Fatal(err)
	}
	relay.Close(t.Context())
	if !gateway.canceled {
		t.Fatal("identity-scoped Job was not canceled when grant closed")
	}
}
