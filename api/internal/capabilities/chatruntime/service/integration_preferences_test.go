package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestRefreshAIChatIntegrationRunConfigUsesAuthoritativeCurrentSnapshot(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	connectionOne := uuid.New().String()
	connectionTwo := uuid.New().String()
	callCount := 0
	svc := &service{integrationPrefs: AIChatIntegrationPreferenceResolverFunc(func(_ context.Context, scope Scope) (AIChatIntegrationRuntimePreferences, error) {
		callCount++
		if scope.OrganizationID != organizationID || scope.AccountID != accountID || scope.WorkspaceID == nil || *scope.WorkspaceID != workspaceID {
			t.Fatalf("resolver scope = %#v, want current organization/account/workspace", scope)
		}
		if callCount == 1 {
			return AIChatIntegrationRuntimePreferences{
				SelectedConnectionIDs: map[string][]string{
					" GitHub ": {connectionOne, connectionOne, connectionTwo},
				},
				PreferredConnectionIDs: map[string]string{"github": connectionTwo},
			}, nil
		}
		return AIChatIntegrationRuntimePreferences{}, nil
	})}
	scope := Scope{OrganizationID: organizationID, AccountID: accountID, WorkspaceID: &workspaceID}
	stale := RunConfig{
		IntegrationSelectedConnectionIDs: map[string][]string{"stale": {uuid.New().String()}},
		IntegrationConnectionIDs:         map[string]string{"stale": uuid.New().String()},
	}

	got, err := svc.refreshAIChatIntegrationRunConfig(context.Background(), scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, stale)
	if err != nil {
		t.Fatalf("refreshAIChatIntegrationRunConfig() error = %v", err)
	}
	if want := map[string][]string{"github": {connectionOne, connectionTwo}}; !reflect.DeepEqual(got.IntegrationSelectedConnectionIDs, want) {
		t.Fatalf("selected connections = %#v, want %#v", got.IntegrationSelectedConnectionIDs, want)
	}
	if want := map[string]string{"github": connectionTwo}; !reflect.DeepEqual(got.IntegrationConnectionIDs, want) {
		t.Fatalf("preferred connections = %#v, want %#v", got.IntegrationConnectionIDs, want)
	}

	got, err = svc.refreshAIChatIntegrationRunConfig(context.Background(), scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, got)
	if err != nil {
		t.Fatalf("refresh after preference removal error = %v", err)
	}
	if got.IntegrationSelectedConnectionIDs != nil || got.IntegrationConnectionIDs != nil {
		t.Fatalf("removed preferences left stale runtime authorization: selected=%#v preferred=%#v", got.IntegrationSelectedConnectionIDs, got.IntegrationConnectionIDs)
	}
}

func TestWorkspaceScopedPreferenceEnablesExternalAppsRuntime(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	connectionID := uuid.New().String()
	resolverWorkspaceID := uuid.Nil
	svc := &service{integrationPrefs: AIChatIntegrationPreferenceResolverFunc(func(_ context.Context, scope Scope) (AIChatIntegrationRuntimePreferences, error) {
		if scope.WorkspaceID != nil {
			resolverWorkspaceID = *scope.WorkspaceID
		}
		return AIChatIntegrationRuntimePreferences{
			SelectedConnectionIDs:  map[string][]string{"github": {connectionID}},
			PreferredConnectionIDs: map[string]string{"github": connectionID},
		}, nil
	})}
	scope := Scope{OrganizationID: organizationID, AccountID: accountID, WorkspaceID: &workspaceID}
	config, err := svc.refreshAIChatIntegrationRunConfig(
		context.Background(),
		scope,
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		RunConfig{},
	)
	if err != nil {
		t.Fatalf("refresh integration config: %v", err)
	}
	if resolverWorkspaceID != workspaceID {
		t.Fatalf("resolver workspace = %s, want %s", resolverWorkspaceID, workspaceID)
	}

	catalog := []skills.SkillDiscoveryMetadata{{
		ID: skills.SkillExternalApps, Status: skills.SkillStatusActive,
		SupportedCallers: []string{runtimemodel.ConversationCallerAIChat},
		Exposure:         skills.SystemSkillExposureProfile(skills.SkillExternalApps),
	}}
	enabled := addAIChatExternalAppsSkillID(nil, catalog, &config)
	if !reflect.DeepEqual(enabled, []string{skills.SkillExternalApps}) {
		t.Fatalf("enabled skills = %#v, want external-apps", enabled)
	}
	params := skillRuntimeParameters(scope, config)
	if params["workspace_id"] != workspaceID.String() {
		t.Fatalf("runtime workspace = %#v, want %s", params["workspace_id"], workspaceID)
	}
	selected, ok := params["integration_selected_connection_ids"].(map[string][]string)
	if !ok || !reflect.DeepEqual(selected["github"], []string{connectionID}) {
		t.Fatalf("runtime selected connections = %#v", params["integration_selected_connection_ids"])
	}
}

func TestRefreshAIChatIntegrationRunConfigDoesNotResolveForAgent(t *testing.T) {
	called := false
	svc := &service{integrationPrefs: AIChatIntegrationPreferenceResolverFunc(func(context.Context, Scope) (AIChatIntegrationRuntimePreferences, error) {
		called = true
		return AIChatIntegrationRuntimePreferences{}, nil
	})}
	config := RunConfig{IntegrationConnectionIDs: map[string]string{"github": "agent-binding"}}
	got, err := svc.refreshAIChatIntegrationRunConfig(context.Background(), Scope{}, Caller{Type: runtimemodel.ConversationCallerAgent}, config)
	if err != nil {
		t.Fatalf("refreshAIChatIntegrationRunConfig() error = %v", err)
	}
	if called || !reflect.DeepEqual(got.IntegrationConnectionIDs, config.IntegrationConnectionIDs) {
		t.Fatalf("Agent config was resolved or changed: called=%v got=%#v", called, got.IntegrationConnectionIDs)
	}
}

func TestRefreshAIChatIntegrationRunConfigRejectsPreferredConnectionOutsideSelection(t *testing.T) {
	svc := &service{integrationPrefs: AIChatIntegrationPreferenceResolverFunc(func(context.Context, Scope) (AIChatIntegrationRuntimePreferences, error) {
		return AIChatIntegrationRuntimePreferences{
			SelectedConnectionIDs:  map[string][]string{"github": {uuid.New().String()}},
			PreferredConnectionIDs: map[string]string{"github": uuid.New().String()},
		}, nil
	})}
	if _, err := svc.refreshAIChatIntegrationRunConfig(context.Background(), Scope{}, Caller{Type: runtimemodel.ConversationCallerAIChat}, RunConfig{}); err == nil {
		t.Fatal("refreshAIChatIntegrationRunConfig() error = nil, want preferred-outside-selection failure")
	}
}

func TestAIChatExecutionBoundariesReloadIntegrationPreferences(t *testing.T) {
	sentinel := errors.New("current preference resolver failed")
	workspaceID := uuid.New()
	resolverCalls := 0
	resolver := AIChatIntegrationPreferenceResolverFunc(func(_ context.Context, resolvedScope Scope) (AIChatIntegrationRuntimePreferences, error) {
		resolverCalls++
		if resolvedScope.WorkspaceID == nil || *resolvedScope.WorkspaceID != workspaceID {
			t.Fatalf("resolver workspace scope = %v, want %s", resolvedScope.WorkspaceID, workspaceID)
		}
		return AIChatIntegrationRuntimePreferences{}, sentinel
	})
	svc := &service{integrationPrefs: resolver}
	scope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New(), WorkspaceID: &workspaceID, SkipAccessCheck: true}
	caller := Caller{Type: runtimemodel.ConversationCallerAIChat}
	conversation := &runtimemodel.Conversation{ID: uuid.New(), WorkspaceID: &workspaceID}
	message := &runtimemodel.Message{ID: uuid.New(), ConversationID: conversation.ID}

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "new chat",
			call: func() error {
				_, err := svc.PrepareConfiguredChat(context.Background(), scope, caller, RunConfig{}, runtimedto.ChatRequest{})
				return err
			},
		},
		{
			name: "regenerate",
			call: func() error {
				_, err := svc.prepareRootRegeneration(context.Background(), scope, caller, RunConfig{}, message.ID, runtimedto.RegenerateMessageRequest{}, false)
				return err
			},
		},
		{
			name: "tool governance approval continuation",
			call: func() error {
				_, err := svc.prepareToolGovernanceContinuationChat(context.Background(), scope, &ToolGovernanceContinuation{Conversation: conversation, Message: message})
				return err
			},
		},
		{
			name: "client action continuation",
			call: func() error {
				_, err := svc.prepareClientActionContinuationChat(context.Background(), scope, &ClientActionContinuation{Conversation: conversation, Message: message}, runtimedto.ClientActionResultRequest{})
				return err
			},
		},
		{
			name: "user input continuation",
			call: func() error {
				_, err := svc.prepareUserInputContinuationChat(context.Background(), scope, caller, RunConfig{}, &UserInputContinuation{Conversation: conversation, Message: message}, runtimedto.UserInputContinuationRequest{})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, sentinel) {
				t.Fatalf("execution boundary error = %v, want resolver error %v", err, sentinel)
			}
		})
	}
	if resolverCalls != len(tests) {
		t.Fatalf("resolver calls = %d, want %d execution boundaries", resolverCalls, len(tests))
	}
}
