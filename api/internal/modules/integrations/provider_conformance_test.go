package integrations_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/exa"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/feishu"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/github"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/gmail"
	xadapter "github.com/zgiai/zgi/api/internal/modules/integrations/adapters/x"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestBuiltInProviderActionContracts(t *testing.T) {
	providers := map[string][]integrations.ActionDefinition{
		integrations.IntegrationWebSearch: exa.Actions(),
		feishu.IntegrationID:              feishu.Actions(),
		github.IntegrationID:              github.Actions(),
		gmail.IntegrationID:               gmail.Actions(),
		xadapter.IntegrationID:            xadapter.Actions(),
	}
	for integrationID, actions := range providers {
		integrationID, actions := integrationID, actions
		t.Run(integrationID, func(t *testing.T) {
			if len(actions) == 0 {
				t.Fatal("provider has no actions")
			}
			byID := make(map[string]integrations.ActionDefinition, len(actions))
			for _, action := range actions {
				byID[action.ID] = action
				if action.Effect != toolgovernance.EffectRead && supportsCaller(action, tools.ToolInvokeFromAgent) {
					t.Fatalf("non-read action %q advertises Agent execution", action.ID)
				}

				err := integrations.ValidateActionInput(integrationID, action, map[string]interface{}{
					"__unexpected_argument__": "sensitive-value-must-not-be-reflected",
				})
				if integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
					t.Fatalf("action %q accepted an unknown argument: %v", action.ID, err)
				}
				feedback := integrations.ActionInputValidationFeedback(err)
				if feedback["failure_stage"] != integrations.ActionValidationStagePreflight || feedback["provider_request_sent"] != false {
					t.Fatalf("action %q feedback = %#v", action.ID, feedback)
				}
				if encoded := fmt.Sprint(feedback); strings.Contains(encoded, "sensitive-value-must-not-be-reflected") {
					t.Fatalf("action %q reflected a rejected argument value: %s", action.ID, encoded)
				}
				if recoveryProvider, ok := err.(interface{ PublicErrorRecovery() map[string]interface{} }); ok {
					recovery := recoveryProvider.PublicErrorRecovery()
					if recovery["recovery_kind"] != "action_schema" || recovery["recovery_action"] != nil || recovery["retry_action"] != nil {
						t.Fatalf("action %q recovery leaks a tool-surface contract: %#v", action.ID, recovery)
					}
				}
			}

			for _, action := range actions {
				for _, hint := range action.PreparationHints {
					preparation, ok := byID[hint.ActionID]
					if !ok {
						t.Fatalf("action %q references unknown preparation action %q", action.ID, hint.ActionID)
					}
					if preparation.Effect != toolgovernance.EffectRead {
						t.Fatalf("action %q preparation action %q is not read-only", action.ID, hint.ActionID)
					}
				}
			}
		})
	}
}

func TestBuiltInProviderPreparationChains(t *testing.T) {
	providers := map[string][]integrations.ActionDefinition{
		integrations.IntegrationWebSearch: exa.Actions(),
		feishu.IntegrationID:              feishu.Actions(),
		github.IntegrationID:              github.Actions(),
		gmail.IntegrationID:               gmail.Actions(),
		xadapter.IntegrationID:            xadapter.Actions(),
	}
	testCases := []struct {
		integrationID string
		targetAction  string
		prepareAction string
		targetArg     string
		resultPath    string
	}{
		{integrations.IntegrationWebSearch, integrations.ActionWebFetch, integrations.ActionWebSearch, "urls", "results[].url"},
		{gmail.IntegrationID, gmail.ActionGetMail, gmail.ActionSearchMail, "message_id", "messages[].id"},
		{gmail.IntegrationID, gmail.ActionReplyMail, gmail.ActionSearchMail, "message_id", "messages[].id"},
		{gmail.IntegrationID, gmail.ActionReplyMail, gmail.ActionGetMail, "message_id", "message.id"},
		{feishu.IntegrationID, feishu.ActionReadDocument, feishu.ActionListDriveFiles, "document_id", "files[].token"},
		{feishu.IntegrationID, feishu.ActionListMessages, feishu.ActionListChats, "chat_id", "chats[].chat_id"},
		{feishu.IntegrationID, feishu.ActionListEvents, feishu.ActionListCalendars, "calendar_id", "calendars[].calendar_id"},
		{feishu.IntegrationID, feishu.ActionCreateEvent, feishu.ActionListCalendars, "calendar_id", "calendars[].calendar_id"},
		{feishu.IntegrationID, feishu.ActionSendUserMessage, feishu.ActionSearchContacts, "recipient_id", "users[].open_id"},
		{feishu.IntegrationID, feishu.ActionSendBotMessage, feishu.ActionListChats, "recipient_id", "chats[].chat_id"},
		{github.IntegrationID, github.ActionGetIssue, github.ActionListIssues, "issue_number", "issues[].number"},
		{xadapter.IntegrationID, xadapter.ActionListPostsByUser, xadapter.ActionGetUserByUsername, "user_id", "user.id"},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.integrationID+"/"+testCase.targetAction+"/"+testCase.prepareAction, func(t *testing.T) {
			action, ok := actionByID(providers[testCase.integrationID], testCase.targetAction)
			if !ok {
				t.Fatalf("target action %q is missing", testCase.targetAction)
			}
			for _, hint := range action.PreparationHints {
				if hint.ActionID == testCase.prepareAction && containsString(hint.TargetArguments, testCase.targetArg) && containsString(hint.ResultPaths, testCase.resultPath) {
					return
				}
			}
			t.Fatalf("action %q has no preparation hint %q mapping %q from %q: %#v", testCase.targetAction, testCase.prepareAction, testCase.targetArg, testCase.resultPath, action.PreparationHints)
		})
	}
}

func actionByID(actions []integrations.ActionDefinition, actionID string) (integrations.ActionDefinition, bool) {
	for _, action := range actions {
		if action.ID == actionID {
			return action, true
		}
	}
	return integrations.ActionDefinition{}, false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func supportsCaller(action integrations.ActionDefinition, caller tools.ToolInvokeFrom) bool {
	for _, supported := range action.SupportedCallers {
		if supported == caller {
			return true
		}
	}
	return false
}
