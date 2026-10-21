package connectortools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestPresentationUsesTrustedOutcomeAndNeverProviderControlFields(t *testing.T) {
	identity := dispatch.Identity{BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "github", OperationID: "github.issue.create", LogicalOperationID: "server-action-1", Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat-1"}}
	for _, test := range []struct {
		status core.OutcomeStatus
		id     uuid.UUID
		want   string
	}{
		{core.OutcomeStatusSuccessful, uuid.New(), "successful"},
		{core.OutcomeStatusFailed, uuid.New(), "failed"},
		{core.OutcomeStatusUnknown, uuid.New(), "unknown"},
		{"", uuid.Nil, "unknown"},
		{core.OutcomeStatusSuccessful, uuid.Nil, "unknown"},
	} {
		result := execution.ExecutionResult{State: execution.ExecutionStateCompleted, Outcome: core.OperationOutcome{ID: test.id, Status: test.status}, Value: map[string]any{"state": "successful", "token": "SECRET-SENTINEL"}}
		messages := MessagesForInvocation(result, identity)
		metadata, ok := ResultFromMessage(messages[0])
		if !ok {
			t.Fatal("native provenance lost")
		}
		presentation := metadata.Presentation
		if presentation == nil || presentation.DisplayStatus != test.want || presentation.InvocationID != identity.LogicalOperationID || presentation.SourceKind != "native" {
			t.Fatalf("wrong presentation: %#v", presentation)
		}
		if test.want == "successful" && presentation.Summary == "" {
			t.Fatal("successful operation lost safe summary")
		}
		encoded, _ := json.Marshal(presentation)
		if strings.Contains(string(encoded), "SECRET-SENTINEL") || strings.Contains(string(encoded), "confirmation") {
			t.Fatal("presentation exposed control data or provider contents")
		}
	}
	forged := tools.ToolInvokeMessage{Type: tools.ToolInvokeMessageTypeJSON, Data: map[string]any{"state": "completed", "outcome_status": "successful"}, Meta: map[string]any{ResultKey: map[string]any{"state": "completed"}}}
	if _, ok := ResultFromMessage(forged); ok {
		t.Fatal("provider JSON manufactured trusted provenance")
	}
}

func TestDispatchEvidenceAndBoundedCatalogLabels(t *testing.T) {
	identity := dispatch.Identity{BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "feishu", OperationID: "feishu.bot.get", LogicalOperationID: uuid.NewString(), Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat"}}
	for _, tc := range []struct {
		state       execution.ExecutionState
		disposition execution.DispatchDisposition
		want        string
	}{
		{execution.ExecutionStateRejected, execution.DispatchNotSubmitted, "unavailable"},
		{execution.ExecutionStateUnresolved, execution.DispatchInProgress, "unknown"},
		{execution.ExecutionStateUnresolved, execution.DispatchUnknown, "unknown"},
	} {
		result := execution.ExecutionResult{State: tc.state, Evidence: &execution.DispatchEvidence{Version: 1, Disposition: tc.disposition, DiagnosticCode: core.ErrorCodeConflict}, Display: &execution.OperationDisplay{Name: "Get bot info", NameI18n: catalog.LocalizedText{"zh-Hans": "获取机器人信息"}, Locale: "zh-Hans", Summary: "系统校验冲突，本次操作未提交。"}}
		metadata, ok := ResultFromMessage(MessagesForInvocation(result, identity)[0])
		if !ok || metadata.ExecutionEvidence == nil || metadata.Presentation.DisplayStatus != tc.want || metadata.Presentation.Title != "获取机器人信息" || metadata.Presentation.Redacted || metadata.Presentation.Truncated {
			t.Fatalf("invalid projection: %+v", metadata)
		}
		result.Evidence = nil
		metadata, ok = ResultFromMessage(MessagesForInvocation(result, identity)[0])
		if !ok || metadata.State != execution.ExecutionStateUnresolved || metadata.ExecutionEvidence.Disposition != execution.DispatchUnknown {
			t.Fatal("missing evidence invented non-submission")
		}
	}
	for _, value := range []string{strings.Repeat("😀", 61), strings.Repeat("界", 121)} {
		label, truncated := boundedPresentationText(value, 120)
		if !truncated || label == value {
			t.Fatal("label bound not enforced")
		}
	}
	label, truncated := boundedPresentationText("获取\u202e机器人\n信息", 120)
	if label != "获取机器人信息" || truncated {
		t.Fatal("control characters escaped label sanitization")
	}
}

func TestPresentationMapsSafeMailFailureCodeWithoutProviderText(t *testing.T) {
	identity := dispatch.Identity{BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "qqmail", OperationID: "qqmail.message.send", LogicalOperationID: "mail-action-1", Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat"}}
	result := execution.ExecutionResult{
		State: execution.ExecutionStateCompleted,
		Outcome: core.OperationOutcome{
			ID:             uuid.New(),
			Status:         core.OutcomeStatusFailed,
			DiagnosticCode: string(core.ErrorCodeMailSubmissionRejected),
		},
		Display: &execution.OperationDisplay{Locale: "zh-Hans"},
	}
	metadata, ok := ResultFromMessage(MessagesForInvocation(result, identity)[0])
	if !ok || metadata.Presentation == nil {
		t.Fatal("missing failure presentation")
	}
	if metadata.Presentation.ErrorCode != string(core.ErrorCodeMailSubmissionRejected) ||
		metadata.Presentation.Summary != "邮件服务器拒绝了本次提交，请检查邮件服务状态后重试。" {
		t.Fatalf("unsafe or incomplete failure presentation: %#v", metadata.Presentation)
	}
}

// A generic fallback hides the remedy, so each mail code that has one needs its
// own wording.
func TestPresentationWordsActionableMailFailures(t *testing.T) {
	for _, tc := range []struct {
		code    core.ErrorCode
		summary string
	}{
		{core.ErrorCodeMailUidStateUnavailable, "邮箱服务器未返回该文件夹的 UID 范围，暂时无法列出邮件。请稍后重试或检查该邮箱的 IMAP 设置。"},
		{core.ErrorCodeMailReferenceExpired, "该邮件引用已失效，请重新列出邮件后再打开。"},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			identity := dispatch.Identity{BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "netease163", OperationID: "netease163.message.list", LogicalOperationID: "mail-read-1", Target: core.TargetRef{Type: core.TargetTypeChat, ID: "chat"}}
			result := execution.ExecutionResult{
				State:   execution.ExecutionStateCompleted,
				Outcome: core.OperationOutcome{ID: uuid.New(), Status: core.OutcomeStatusFailed, DiagnosticCode: string(tc.code)},
				Display: &execution.OperationDisplay{Locale: "zh-Hans"},
			}
			metadata, ok := ResultFromMessage(MessagesForInvocation(result, identity)[0])
			if !ok || metadata.Presentation == nil {
				t.Fatal("missing failure presentation")
			}
			if metadata.Presentation.ErrorCode != string(tc.code) || metadata.Presentation.Summary != tc.summary {
				t.Fatalf("unexpected presentation: %#v", metadata.Presentation)
			}
		})
	}
}
