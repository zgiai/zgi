package providers

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

type registration struct {
	definition catalog.Definition
	driver     Driver
}
type Runtime struct{ drivers map[string]registration }

// ReadConfirmationTarget reads a scoped local conversation without credentials
// or an external request. It is deliberately separate from provider execution.
func (runtime *Runtime) ReadConfirmationTarget(ctx context.Context, request execution.ProviderInvocation) (string, error) {
	if runtime == nil {
		return "", Invalid()
	}
	entry, ok := runtime.drivers[string(request.ConnectorID)]
	if !ok {
		return "", Invalid()
	}
	reader, ok := entry.driver.(interface {
		ReadConfirmationTarget(context.Context, ResourceScope, string) (string, error)
	})
	input, valid := request.Input.(map[string]any)
	if !ok || !valid {
		return "", Invalid()
	}
	return reader.ReadConfirmationTarget(ctx, ResourceScope{
		OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ConnectionID: request.ConnectionID, CredentialVersion: request.CredentialVersion,
		BindingID: request.BindingID, BindingRevision: request.BindingRevision, Target: request.Target,
	}, Text(input, "context_handle"))
}

var _ connections.CandidateVerifier = (*Runtime)(nil)
var _ execution.NativeProviderInvoker = (*Runtime)(nil)

func NewRuntime(drivers ...Driver) (*Runtime, error) {
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{drivers: map[string]registration{}}
	for _, driver := range drivers {
		if driver == nil {
			return nil, Invalid()
		}
		id := driver.ConnectorID()
		if _, exists := runtime.drivers[id]; exists {
			return nil, Invalid()
		}
		for _, definition := range definitions {
			if definition.ID == id {
				if definition.UsesCredentials() {
					if _, ok := driver.(CredentialDriver); !ok {
						return nil, Invalid()
					}
				} else if _, ok := driver.(KeyDriver); !ok {
					return nil, Invalid()
				}
				runtime.drivers[id] = registration{definition, driver}
			}
		}
		if _, exists := runtime.drivers[id]; !exists {
			return nil, Invalid()
		}
	}
	for _, definition := range definitions {
		if _, registered := runtime.drivers[definition.ID]; !registered && !definition.IsMessagingCredentials() && !definition.IsMailCredentials() && !catalog.IsSocialMCPConnector(definition.ID) {
			return nil, Invalid()
		}
	}
	return runtime, nil
}
func (runtime *Runtime) lookup(id, driver, auth string, credentials map[string]string) (registration, CredentialInput, error) {
	if runtime == nil {
		return registration{}, CredentialInput{}, Invalid()
	}
	entry, ok := runtime.drivers[id]
	if !ok || entry.definition.DriverID != driver || entry.definition.AuthMethodID != auth || catalog.ValidateCredentialValues(entry.definition, credentials) != nil {
		return registration{}, CredentialInput{}, Invalid()
	}
	if !entry.definition.UsesCredentials() {
		if len(credentials) != len(entry.definition.KeyFields) || len(entry.definition.KeyFields) != 1 {
			return registration{}, CredentialInput{}, Invalid()
		}
		key := strings.TrimSpace(credentials[entry.definition.KeyFields[0].Name])
		if key == "" || len(key) > 16384 || strings.ContainsAny(key, "\r\n") {
			return registration{}, CredentialInput{}, Invalid()
		}
	}
	return entry, CredentialInput{Values: credentials}, nil
}
func (runtime *Runtime) Verify(ctx context.Context, request connections.CandidateVerificationRequest) (connections.CandidateVerificationResult, error) {
	if request.OrganizationID == uuid.Nil || request.OwnerAccountID == uuid.Nil || request.ConnectionID == uuid.Nil {
		return connections.CandidateVerificationResult{}, connections.ErrCandidateContract
	}
	entry, credentials, err := runtime.lookup(request.ConnectorID, request.DriverID, request.AuthMethodID, request.Credentials)
	if err != nil {
		return connections.CandidateVerificationResult{}, connections.ErrCandidateContract
	}
	allowed := map[string]bool{}
	for _, op := range entry.definition.Operations {
		allowed[op.ID] = true
	}
	seen := map[string]bool{}
	for _, id := range request.OperationIDs {
		if !allowed[id] || seen[id] {
			return connections.CandidateVerificationResult{}, connections.ErrCandidateContract
		}
		seen[id] = true
	}
	credentials.ResourceScope = ResourceScope{OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID, ConnectionID: request.ConnectionID}
	if entry.definition.IsMailCredentials() {
		operations := catalog.MailOperationsFor(entry.definition.ID, request.MailUsage)
		if len(operations) == 0 {
			return connections.CandidateVerificationResult{}, connections.ErrCandidateContract
		}
		if len(seen) > 0 {
			for _, id := range operations {
				if !seen[id] {
					return connections.CandidateVerificationResult{}, connections.ErrCandidateContract
				}
			}
			if len(seen) != len(operations) {
				return connections.CandidateVerificationResult{}, connections.ErrCandidateContract
			}
		}
		seen = map[string]bool{}
		for _, id := range operations {
			seen[id] = true
		}
		credentials.MailUsage = request.MailUsage
	} else if request.MailUsage != "" {
		return connections.CandidateVerificationResult{}, connections.ErrCandidateContract
	}
	verified, err := entry.verify(ctx, credentials)
	if err != nil {
		failure := FailureOf(err)
		code := failure.Code
		if failure.Unknown {
			code = core.ErrorCodeProviderUnavailable
			if entry.definition.IsMessagingCredentials() {
				code = core.ErrorCodeVerificationUncertain
			}
		}
		result := connections.CandidateVerificationResult{}
		if entry.definition.IsMailCredentials() && verified.Evidence != nil && verified.Evidence.Mail != nil {
			m := verified.Evidence.Mail
			if m.Usage == request.MailUsage {
				result.MailVerification = (&core.MailVerification{Usage: m.Usage, IMAPStatus: m.IMAPStatus, SMTPStatus: m.SMTPStatus}).SafeCopy()
			}
		}
		return result, core.NewError(code, failure.Retryable, err)
	}
	usable := []string{}
	evidence := entry.safeEvidence(verified.Evidence, credentials)
	if entry.definition.IsMailCredentials() && (evidence == nil || evidence.Mail == nil || !evidence.Mail.CredentialVerified || verified.AccountID != credentials.Values["email"]) {
		return connections.CandidateVerificationResult{}, core.NewError(core.ErrorCodeMailAuthenticationRejected, false, nil)
	}
	eligible := map[string]bool{}
	if entry.definition.IsMessagingCredentials() {
		if evidence == nil || evidence.Messaging == nil || verified.AccountID != catalog.CredentialIdentity(entry.definition, credentials.Values) {
			return connections.CandidateVerificationResult{}, core.NewError(core.ErrorCodeInvalidAppCredentials, false, nil)
		}
		for _, op := range evidence.Messaging.Operations {
			eligible[op.ID] = op.Availability != "denied"
		}
	}
	for _, id := range verified.Operations {
		if allowed[id] && (len(seen) == 0 || seen[id]) && (!entry.definition.IsMessagingCredentials() || eligible[id]) {
			usable = append(usable, id)
		}
	}
	if len(usable) == 0 {
		return connections.CandidateVerificationResult{}, core.NewError(core.ErrorCodeInsufficientKey, false, nil)
	}
	if entry.definition.IsMailCredentials() && len(usable) != len(seen) {
		return connections.CandidateVerificationResult{}, core.NewError(core.ErrorCodeInsufficientKey, false, nil)
	}
	return connections.CandidateVerificationResult{ExternalAccountID: entry.redactText(verified.AccountID, credentials), ExternalDisplayName: entry.redactText(verified.DisplayName, credentials), UsableOperationIDs: usable, ProviderRequestID: entry.safeRequestID(verified.RequestID, credentials), CostUSD: verified.CostUSD, Evidence: evidence}, nil
}
func (runtime *Runtime) Invoke(ctx context.Context, request execution.ProviderInvocation) (execution.ProviderInvocationResult, error) {
	entry, credentials, err := runtime.lookup(request.ConnectorID.String(), request.DriverID.String(), request.AuthMethodID.String(), request.Credentials)
	if err != nil {
		return rejected(core.ErrorCodeInvalidInput)
	}
	if request.OrganizationID == uuid.Nil || request.ConnectionID == uuid.Nil || request.OwnerAccountID == uuid.Nil || request.ActingPrincipalID != request.OwnerAccountID || request.Target.Validate() != nil || request.CredentialVersion < 1 || strings.TrimSpace(request.LogicalOperationID) == "" {
		return rejected(core.ErrorCodeAccessDenied)
	}
	if request.ProviderID.String() != entry.definition.ProviderID {
		return rejected(core.ErrorCodeConnectorUnavailable)
	}
	found := false
	for _, operation := range entry.definition.Operations {
		if operation.ID == request.OperationID.String() {
			found = true
			break
		}
	}
	if !found {
		return rejected(core.ErrorCodeInvalidInput)
	}
	input, ok := request.Input.(map[string]any)
	if request.Input != nil && !ok {
		return rejected(core.ErrorCodeInvalidInput)
	}
	credentials.ResourceScope = ResourceScope{OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID, ConnectionID: request.ConnectionID, CredentialVersion: request.CredentialVersion, BindingID: request.BindingID, BindingRevision: request.BindingRevision, Target: request.Target, LogicalOperationID: request.LogicalOperationID}
	response, err := entry.invoke(ctx, request.OperationID.String(), credentials, input)
	if err != nil {
		failure := FailureOf(err)
		status := core.OutcomeStatusFailed
		if failure.Unknown {
			status = core.OutcomeStatusUnknown
		}
		code := failure.Code
		if code == core.ErrorCodeInvalidKey || code == core.ErrorCodeInvalidAppCredentials {
			code = core.ErrorCodeConnectionNotReady
		}
		if code == core.ErrorCodeInsufficientKey {
			code = core.ErrorCodeAccessDenied
		}
		return execution.ProviderInvocationResult{Status: status, FailureCode: code, DiagnosticCode: string(code), Retryable: failure.Retryable, ProviderRequestID: entry.safeRequestID(failure.RequestID, credentials)}, core.NewError(code, failure.Retryable, err)
	}
	for _, secret := range entry.secrets(credentials) {
		Redact(response.Value, secret)
	}
	return execution.ProviderInvocationResult{Status: core.OutcomeStatusSuccessful, Value: response.Value, ProviderRequestID: entry.safeRequestID(response.RequestID, credentials)}, nil
}

func (entry registration) verify(ctx context.Context, input CredentialInput) (Verification, error) {
	if entry.definition.UsesCredentials() {
		return entry.driver.(CredentialDriver).VerifyCredentials(ctx, input)
	}
	return entry.driver.(KeyDriver).Verify(ctx, strings.TrimSpace(input.Values[entry.definition.KeyFields[0].Name]))
}
func (entry registration) invoke(ctx context.Context, operation string, input CredentialInput, args map[string]any) (Response, error) {
	if entry.definition.UsesCredentials() {
		return entry.driver.(CredentialDriver).InvokeCredentials(ctx, operation, input, args)
	}
	return entry.driver.(KeyDriver).Invoke(ctx, operation, strings.TrimSpace(input.Values[entry.definition.KeyFields[0].Name]), args)
}
func (entry registration) secrets(input CredentialInput) []string {
	var values []string
	if entry.definition.UsesCredentials() {
		for _, field := range entry.definition.CredentialFields {
			if field.Secret {
				values = append(values, input.Values[field.Name])
			}
		}
	} else {
		for _, field := range entry.definition.KeyFields {
			values = append(values, input.Values[field.Name])
		}
	}
	// Also cover normalized values used by legacy drivers without ever joining credentials.
	for _, value := range append([]string(nil), values...) {
		if trimmed := strings.TrimSpace(value); trimmed != value {
			values = append(values, trimmed)
		}
	}
	return values
}
func (entry registration) redactText(value string, input CredentialInput) string {
	for _, secret := range entry.secrets(input) {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	return value
}
func (entry registration) safeRequestID(value string, input CredentialInput) string {
	for _, secret := range entry.secrets(input) {
		value = SafeRequestID(value, secret)
	}
	return SafeRequestID(value, "")
}

func (entry registration) safeEvidence(value *core.ProviderEvidence, input CredentialInput) *core.ProviderEvidence {
	if entry.definition.IsMailCredentials() {
		return entry.safeMailEvidence(value, input)
	}
	if value != nil && entry.definition.IsMessagingCredentials() {
		return entry.safeMessagingEvidence(value, input)
	}
	if value == nil || !entry.definition.IsAppCredentials() {
		return nil
	}
	result := *value
	result.Region = input.Values["region"]
	result.AppID = entry.redactText(input.Values["app_id"], input)
	result.AppName = SafeText(entry.redactText(value.AppName, input), 128)
	result.TemplateID = entry.definition.Setup.TemplateID
	// This driver supports app credentials, not user authorization or events.
	result.UserAuthorizationEnabled, result.EventsEnabled = false, false
	result.EventConfiguration = "unknown"
	result.ScopeStatus = "unknown"
	result.GrantedScopes = []string{}
	if value.ScopeStatus == "verified" {
		result.ScopeStatus = "verified"
		known := map[string]bool{}
		if entry.definition.ID == "feishu" {
			profile := catalog.FeishuPermissionTemplate()
			addons, _ := profile["addons"].(map[string]any)
			scopes, _ := addons["scopes"].(map[string]any)
			tenant, _ := scopes["tenant"].([]any)
			for _, raw := range tenant {
				if scope, ok := raw.(string); ok {
					known[scope] = true
				}
			}
		}
		for _, op := range entry.definition.Operations {
			for _, group := range op.RequiredScopes {
				for _, scope := range group {
					known[scope] = true
				}
			}
		}
		for _, scope := range value.GrantedScopes {
			if known[scope] {
				result.GrantedScopes = append(result.GrantedScopes, scope)
				delete(known, scope)
			}
		}
	}
	return &result
}
func rejected(code core.ErrorCode) (execution.ProviderInvocationResult, error) {
	return execution.ProviderInvocationResult{Status: core.OutcomeStatusFailed, FailureCode: code, DiagnosticCode: string(code)}, core.NewError(code, false, nil)
}
