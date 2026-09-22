package connectortools

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"strings"
	"unicode"
)

// OperationRef is resolved by the host. A presentation never creates authority
// and is not accepted as dispatcher or confirmation input.
type OperationRef struct {
	ConnectorID        core.Identifier `json:"connector_id"`
	OperationID        core.Identifier `json:"operation_id"`
	BindingID          uuid.UUID       `json:"binding_id"`
	ConnectionID       uuid.UUID       `json:"connection_id"`
	Target             core.TargetRef  `json:"target"`
	LogicalOperationID string          `json:"logical_operation_id"`
	ActionStepID       string          `json:"action_step_id,omitempty"`
	PlanPhaseID        string          `json:"plan_phase_id,omitempty"`
}

func (ref OperationRef) Valid() bool {
	return ref.ConnectorID.Valid() && ref.OperationID.Valid() && ref.BindingID != uuid.Nil && ref.ConnectionID != uuid.Nil && ref.Target.Validate() == nil && strings.TrimSpace(ref.LogicalOperationID) != ""
}

// OperationPresentation is a versioned, read-only projection of server facts.
// Summaries deliberately exclude provider content, arguments and receipts.
type OperationPresentation struct {
	SchemaVersion int             `json:"schema_version"`
	SourceKind    string          `json:"source_kind"`
	InvocationID  string          `json:"invocation_id"`
	ConnectorID   core.Identifier `json:"connector_id,omitempty"`
	OperationID   core.Identifier `json:"operation_id,omitempty"`
	DisplayStatus string          `json:"display_status"`
	Redacted      bool            `json:"redacted"`
	Truncated     bool            `json:"truncated"`
	Title         string          `json:"title,omitempty"`
	Summary       string          `json:"summary,omitempty"`
	ErrorCode     string          `json:"error_code,omitempty"`
}

// PresentationForExecution returns the same safe, read-only presentation
// stored in a native invocation. Runtime confirmation and Workflow adapters
// use this helper so every surface applies the same redaction and fallback
// rules instead of inventing its own terminal card payload.
func PresentationForExecution(result execution.ExecutionResult, identity dispatch.Identity) *OperationPresentation {
	messages := MessagesForInvocation(result, identity)
	if len(messages) == 0 {
		return nil
	}
	metadata, ok := ResultFromMessage(messages[0])
	if !ok || metadata.Presentation == nil {
		return nil
	}
	presentation := *metadata.Presentation
	return &presentation
}

func MessagesForInvocation(result execution.ExecutionResult, identity dispatch.Identity, causes ...error) []tools.ToolInvokeMessage {
	if result.State == execution.ExecutionStateCompleted && result.Outcome.ID == uuid.Nil {
		display := result.Display
		result = execution.UnresolvedResult(core.ErrorCodeOutcomeUnknown)
		result.Display = display
	}
	messages := Messages(result)
	for i := range messages {
		metadata, ok := ResultFromMessage(messages[i])
		if !ok {
			// Dispatch was attempted, but did not return a valid terminal or
			// confirmation shape. Preserve its action identity as unknown so
			// the host cannot send it through generic argument-error retries.
			code := core.ErrorCodeOutcomeUnknown
			if len(causes) > 0 && causes[0] != nil {
				code = core.ErrorCodeOf(causes[0])
			}
			unknown := Messages(execution.UnresolvedResult(code))[0]
			metadata, _ = ResultFromMessage(unknown)
			messages[i].Data = unknown.Data
		}
		metadata.Operation = &OperationRef{ConnectorID: identity.ConnectorID, OperationID: identity.OperationID, BindingID: identity.BindingID, ConnectionID: identity.ConnectionID, Target: identity.Target, LogicalOperationID: identity.LogicalOperationID}
		metadata.Operation.ActionStepID, metadata.Operation.PlanPhaseID = identity.ActionStepID, identity.PlanPhaseID
		metadata.Presentation = &OperationPresentation{SchemaVersion: 1, SourceKind: "native", InvocationID: identity.LogicalOperationID, ConnectorID: identity.ConnectorID, OperationID: identity.OperationID, DisplayStatus: presentationStatus(metadata)}
		if result.Display != nil {
			name := result.Display.Name
			if localized := result.Display.NameI18n[result.Display.Locale]; strings.TrimSpace(localized) != "" {
				name = localized
			}
			metadata.Presentation.Title, metadata.Presentation.Truncated = boundedPresentationText(name, 120)
			var summaryTruncated bool
			metadata.Presentation.Summary, summaryTruncated = boundedPresentationText(result.Display.Summary, 800)
			metadata.Presentation.Truncated = metadata.Presentation.Truncated || summaryTruncated
		}
		code := strings.TrimSpace(result.Outcome.DiagnosticCode)
		if code == "" {
			for _, cause := range causes {
				if cause == nil {
					continue
				}
				if candidate := string(core.ErrorCodeOf(cause)); candidate != "" {
					code = candidate
					break
				}
			}
		}
		if code != "" {
			metadata.Presentation.ErrorCode = code
			if strings.TrimSpace(metadata.Presentation.Summary) == "" {
				metadata.Presentation.Summary = safePresentationSummary(code, result.Display)
			}
		} else if strings.TrimSpace(metadata.Presentation.Summary) == "" && metadata.Presentation.DisplayStatus == "successful" {
			metadata.Presentation.Summary = safeSuccessSummary(result.Display)
		}
		messages[i].Meta[ResultKey] = metadata
	}
	return messages
}

// SafeTerminalSummary returns a locale-appropriate safe summary for a terminal
// presentation that has no provider-derived summary. It reuses the same stable
// error-code and success copy as the native projector and never includes raw
// provider content. An empty result means the frontend should localize the
// display status itself (for example cancelled terminals).
func SafeTerminalSummary(displayStatus string, errorCode string, locale string) string {
	display := &execution.OperationDisplay{Locale: locale}
	if code := strings.TrimSpace(errorCode); code != "" {
		return safePresentationSummary(code, display)
	}
	if displayStatus == "successful" || displayStatus == "partial" {
		return safeSuccessSummary(display)
	}
	return ""
}

func safeSuccessSummary(display *execution.OperationDisplay) string {
	locale := ""
	if display != nil {
		locale = strings.ToLower(strings.TrimSpace(display.Locale))
	}
	if strings.HasPrefix(locale, "zh") {
		return "连接器操作已成功完成。"
	}
	return "The Connector operation completed successfully."
}

func safePresentationSummary(code string, display *execution.OperationDisplay) string {
	locale := ""
	if display != nil {
		locale = strings.ToLower(strings.TrimSpace(display.Locale))
	}
	chinese := strings.HasPrefix(locale, "zh")
	switch core.ErrorCode(code) {
	case core.ErrorCodeMailSubmissionRejected:
		if chinese {
			return "邮件服务器拒绝了本次提交，请检查邮件服务状态后重试。"
		}
		return "The mail server rejected this submission. Check the mail service and try again."
	case core.ErrorCodeMailRecipientRejected:
		if chinese {
			return "邮件服务器拒绝了收件人，请检查收件人后重试。"
		}
		return "The mail server rejected a recipient. Check the recipients and try again."
	case core.ErrorCodeMailAuthenticationRejected:
		if chinese {
			return "邮箱认证失败，请检查连接凭据和已启用的邮件服务。"
		}
		return "Email authentication failed. Check the connection credentials and enabled mail services."
	case core.ErrorCodeOutcomeUnknown:
		if chinese {
			return "暂时无法确认操作结果，请先核查服务商侧状态，避免重复执行。"
		}
		return "The operation result cannot be confirmed yet. Check the provider before retrying."
	default:
		if chinese {
			return "连接器操作未能完成，请检查连接状态后重试。"
		}
		return "The Connector operation did not complete. Check the connection and try again."
	}
}

func presentationStatus(result ResultMetadata) string {
	switch result.State {
	case execution.ExecutionStateRejected:
		return "unavailable"
	case execution.ExecutionStateUnresolved:
		return "unknown"
	case execution.ExecutionStateRequiresConfirmation:
		return "pending"
	case execution.ExecutionStateCancelled:
		return "cancelled"
	case execution.ExecutionStateBlocked:
		return "unavailable"
	case execution.ExecutionStateCompleted:
		if result.OutcomeID != uuid.Nil && result.OutcomeStatus.Valid() {
			return string(result.OutcomeStatus)
		}
	}
	return "unknown"
}

func boundedPresentationText(value string, limit int) (string, bool) {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	units := 0
	for offset, r := range value {
		units++
		if r > 0xffff {
			units++
		}
		if units > limit {
			return value[:offset], true
		}
	}
	return value, false
}
