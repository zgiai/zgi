package gateway

import (
	"testing"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	providermodel "github.com/zgiai/ginext/internal/modules/llm/provider/model"
)

func TestCheckModelAuthorization_WorkspaceSubject_IgnoresAPIKeyLimits(t *testing.T) {
	svc := &llmGatewayServiceImpl{}
	limits := `["gpt-4o"]`
	workspaceID := "ws-1"

	apiKey := &apikeymodel.TenantAPIKey{
		ModelLimitsEnabled: true,
		ModelLimits:        &limits,
	}
	appCtx := &AppContext{WorkspaceID: &workspaceID}

	if err := svc.checkModelAuthorization(apiKey, appCtx, "claude-3-5-sonnet"); err != nil {
		t.Fatalf("checkModelAuthorization err = %v, want nil", err)
	}
}

func TestCheckModelAuthorization_APIKeySubject_EnforcesModelLimits(t *testing.T) {
	svc := &llmGatewayServiceImpl{}
	limits := `["gpt-4o"]`
	apiKey := &apikeymodel.TenantAPIKey{
		ModelLimitsEnabled: true,
		ModelLimits:        &limits,
	}

	err := svc.checkModelAuthorization(apiKey, nil, "claude-3-5-sonnet")
	if err != ErrModelNotAuthorized {
		t.Fatalf("checkModelAuthorization err = %v, want %v", err, ErrModelNotAuthorized)
	}
}

func TestCheckModelAuthorization_NoWorkspaceAndNilAPIKey_ReturnsInvalidAPIKey(t *testing.T) {
	svc := &llmGatewayServiceImpl{}
	err := svc.checkModelAuthorization(nil, nil, "gpt-4o")
	if err != ErrInvalidAPIKey {
		t.Fatalf("checkModelAuthorization err = %v, want %v", err, ErrInvalidAPIKey)
	}
}

func TestCheckModelAuthorization_OrganizationSubject_IgnoresAPIKeyLimits(t *testing.T) {
	svc := &llmGatewayServiceImpl{}
	limits := `["gpt-4o"]`
	workspaceID := "ws-1"
	subjectType := quotaSubjectTypeOrganization

	apiKey := &apikeymodel.TenantAPIKey{
		ModelLimitsEnabled: true,
		ModelLimits:        &limits,
	}
	appCtx := &AppContext{
		WorkspaceID:        &workspaceID,
		BillingSubjectType: &subjectType,
	}

	if err := svc.checkModelAuthorization(apiKey, appCtx, "claude-3-5-sonnet"); err != nil {
		t.Fatalf("checkModelAuthorization err = %v, want nil", err)
	}
}

func TestCreateBillingContext_OrganizationSubjectDoesNotUseWorkspaceQuota(t *testing.T) {
	svc := &llmGatewayServiceImpl{}
	workspaceID := "ws-1"
	subjectType := quotaSubjectTypeOrganization
	shadowOrgID := uuid.New()
	channelID := uuid.New()

	apiKey := &apikeymodel.TenantAPIKey{ID: "key-1"}
	appCtx := &AppContext{
		AppType:            stringPtr("agent"),
		WorkspaceID:        &workspaceID,
		BillingSubjectType: &subjectType,
	}
	providerSelection := &ProviderSelection{
		Model:             llmmodel.LLMModel{ID: uuid.New(), Model: "gpt-4o"},
		Provider:          providermodel.LLMProvider{ID: uuid.New(), Provider: "openai"},
		UseSystemProvider: true,
	}

	billingCtx := svc.createBillingContext(
		apiKey,
		appCtx,
		providerSelection,
		&channelID,
		shadowOrgID,
		123,
		false,
		time.Now(),
		"req-1",
		"attempt-1",
	)

	if billingCtx.QuotaSubjectType != quotaSubjectTypeOrganization {
		t.Fatalf("quotaSubjectType = %q, want %q", billingCtx.QuotaSubjectType, quotaSubjectTypeOrganization)
	}
	if billingCtx.QuotaSubjectID != shadowOrgID.String() {
		t.Fatalf("quotaSubjectID = %q, want %q", billingCtx.QuotaSubjectID, shadowOrgID.String())
	}
	if billingCtx.WorkspaceID != "" {
		t.Fatalf("workspaceID = %q, want empty for organization billing subject", billingCtx.WorkspaceID)
	}
}

func stringPtr(v string) *string {
	return &v
}
