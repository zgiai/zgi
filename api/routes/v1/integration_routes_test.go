package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/util"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type integrationRouteAdapter struct{ driverID string }

func TestIntegrationRouteErrorReturnsSafeProviderDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	integrationRouteError(context, integrations.NewProviderError(
		integrations.ErrorCodeProviderRejected,
		"provider rejected validation",
		nil,
		integrations.ProviderDiagnostics{ErrorCode: "60020", RequestID: "request-1", HTTPStatus: 200},
	))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Code string `json:"code"`
		Data struct {
			ProviderErrorCode  string `json:"provider_error_code"`
			ProviderRequestID  string `json:"provider_request_id"`
			ProviderHTTPStatus int    `json:"provider_http_status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != integrations.ErrorCodeProviderRejected || payload.Data.ProviderErrorCode != "60020" || payload.Data.ProviderRequestID != "request-1" || payload.Data.ProviderHTTPStatus != 200 {
		t.Fatalf("payload = %#v", payload)
	}
}

type integrationCatalogAccountService struct {
	interfaces.AccountService
	admin bool
}

func (service integrationCatalogAccountService) IsOrganizationAdminOrOwner(context.Context, string, string) (bool, error) {
	return service.admin, nil
}

func (adapter *integrationRouteAdapter) DriverID() string { return adapter.driverID }

func (*integrationRouteAdapter) Execute(context.Context, integrations.ActionRequest) (*integrations.ActionResult, error) {
	return &integrations.ActionResult{Output: map[string]interface{}{"ok": true}, AttemptCount: 1}, nil
}

type integrationRouteConnectionRepository struct {
	items map[uuid.UUID]*integrations.IntegrationConnection
}

func (repository *integrationRouteConnectionRepository) Create(_ context.Context, connection *integrations.IntegrationConnection) error {
	if repository.items == nil {
		repository.items = map[uuid.UUID]*integrations.IntegrationConnection{}
	}
	copyValue := *connection
	repository.items[connection.ID] = &copyValue
	return nil
}

func (repository *integrationRouteConnectionRepository) GetByID(_ context.Context, organizationID, connectionID uuid.UUID) (*integrations.IntegrationConnection, error) {
	connection := repository.items[connectionID]
	if connection == nil || connection.OrganizationID != organizationID {
		return nil, integrations.ErrConnectionNotFound
	}
	copyValue := *connection
	return &copyValue, nil
}

func (repository *integrationRouteConnectionRepository) List(_ context.Context, organizationID uuid.UUID, filter integrations.ConnectionListFilter) ([]*integrations.IntegrationConnection, error) {
	items := make([]*integrations.IntegrationConnection, 0, len(repository.items))
	for _, connection := range repository.items {
		if connection.OrganizationID != organizationID || (filter.IntegrationID != "" && connection.IntegrationID != filter.IntegrationID) || (filter.DriverID != "" && connection.DriverID != filter.DriverID) {
			continue
		}
		if len(filter.CredentialSources) > 0 {
			matched := false
			for _, source := range filter.CredentialSources {
				matched = matched || connection.CredentialSource == source
			}
			if !matched {
				continue
			}
		}
		if filter.OwnerAccountID != nil && (connection.OwnerAccountID == nil || *connection.OwnerAccountID != *filter.OwnerAccountID) {
			continue
		}
		if len(filter.Statuses) > 0 {
			matched := false
			for _, status := range filter.Statuses {
				matched = matched || connection.Status == status
			}
			if !matched {
				continue
			}
		}
		copyValue := *connection
		items = append(items, &copyValue)
	}
	return items, nil
}

func (repository *integrationRouteConnectionRepository) Count(ctx context.Context, organizationID uuid.UUID, filter integrations.ConnectionListFilter) (int64, error) {
	items, err := repository.List(ctx, organizationID, filter)
	return int64(len(items)), err
}

func (*integrationRouteConnectionRepository) GetDefault(context.Context, uuid.UUID, string, string) (*integrations.IntegrationConnection, error) {
	return nil, integrations.ErrConnectionNotFound
}

func (repository *integrationRouteConnectionRepository) Update(_ context.Context, connection *integrations.IntegrationConnection) error {
	if repository.items[connection.ID] == nil {
		return integrations.ErrConnectionNotFound
	}
	copyValue := *connection
	repository.items[connection.ID] = &copyValue
	return nil
}

func (*integrationRouteConnectionRepository) SetDefault(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (repository *integrationRouteConnectionRepository) Delete(_ context.Context, organizationID, connectionID uuid.UUID) error {
	connection := repository.items[connectionID]
	if connection == nil || connection.OrganizationID != organizationID {
		return integrations.ErrConnectionNotFound
	}
	delete(repository.items, connectionID)
	return nil
}

type integrationRouteGrantRepository struct {
	grants        []integrations.IntegrationConnectionGrant
	saved         *integrations.IntegrationConnectionGrant
	saveExpected  int
	deleted       uuid.UUID
	deleteConnID  uuid.UUID
	applicableErr error
}

func (repository *integrationRouteGrantRepository) ListApplicable(_ context.Context, organizationID, connectionID, accountID uuid.UUID, workspaceID *uuid.UUID) ([]integrations.IntegrationConnectionGrant, error) {
	if repository.applicableErr != nil {
		return nil, repository.applicableErr
	}
	items := make([]integrations.IntegrationConnectionGrant, 0)
	for _, grant := range repository.grants {
		if grant.OrganizationID != organizationID || grant.ConnectionID != connectionID {
			continue
		}
		switch grant.PrincipalType {
		case integrations.ConnectionGrantPrincipalOrganization:
			if grant.PrincipalID == nil {
				items = append(items, grant)
			}
		case integrations.ConnectionGrantPrincipalAccount:
			if grant.PrincipalID != nil && *grant.PrincipalID == accountID {
				items = append(items, grant)
			}
		case integrations.ConnectionGrantPrincipalWorkspace:
			if workspaceID != nil && grant.PrincipalID != nil && *grant.PrincipalID == *workspaceID {
				items = append(items, grant)
			}
		}
	}
	return items, nil
}

func (repository *integrationRouteGrantRepository) List(_ context.Context, organizationID, connectionID uuid.UUID) ([]integrations.IntegrationConnectionGrant, error) {
	items := make([]integrations.IntegrationConnectionGrant, 0)
	for _, grant := range repository.grants {
		if grant.OrganizationID == organizationID && grant.ConnectionID == connectionID {
			items = append(items, grant)
		}
	}
	return items, nil
}

func (repository *integrationRouteGrantRepository) Save(_ context.Context, grant *integrations.IntegrationConnectionGrant, expectedRevision int) error {
	copyValue := *grant
	if copyValue.ID == uuid.Nil {
		copyValue.ID = uuid.New()
		grant.ID = copyValue.ID
	}
	if expectedRevision < 1 {
		copyValue.Revision = 1
		grant.Revision = 1
	} else {
		copyValue.Revision = expectedRevision + 1
		grant.Revision = copyValue.Revision
	}
	repository.saved = &copyValue
	repository.saveExpected = expectedRevision
	return nil
}

func (repository *integrationRouteGrantRepository) Delete(_ context.Context, _ uuid.UUID, connectionID, grantID uuid.UUID) error {
	repository.deleted = grantID
	repository.deleteConnID = connectionID
	return nil
}

type integrationRoutePreferenceRepository struct {
	items          []integrations.AIChatIntegrationPreference
	organizationID uuid.UUID
	accountID      uuid.UUID
	workspaceID    *uuid.UUID
}

func (repository *integrationRoutePreferenceRepository) List(_ context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID) ([]integrations.AIChatIntegrationPreference, error) {
	if repository.organizationID != organizationID || repository.accountID != accountID || !sameRouteUUIDPointer(repository.workspaceID, workspaceID) {
		return []integrations.AIChatIntegrationPreference{}, nil
	}
	return append([]integrations.AIChatIntegrationPreference(nil), repository.items...), nil
}

func (repository *integrationRoutePreferenceRepository) Replace(_ context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID, preferences []integrations.AIChatIntegrationPreference) error {
	repository.organizationID = organizationID
	repository.accountID = accountID
	repository.workspaceID = cloneRouteUUIDPointer(workspaceID)
	repository.items = append([]integrations.AIChatIntegrationPreference(nil), preferences...)
	for index := range repository.items {
		repository.items[index].OrganizationID = organizationID
		repository.items[index].AccountID = accountID
		repository.items[index].WorkspaceID = cloneRouteUUIDPointer(workspaceID)
	}
	return nil
}

type integrationRouteHealthRepository struct {
	items        []integrations.ConnectionHealthEvent
	total        int64
	organization uuid.UUID
	connection   uuid.UUID
	page         int
	pageSize     int
}

func (*integrationRouteHealthRepository) Record(context.Context, integrations.ConnectionHealthObservation) (integrations.ConnectionHealthEvent, error) {
	return integrations.ConnectionHealthEvent{}, nil
}

func (repository *integrationRouteHealthRepository) List(_ context.Context, organizationID, connectionID uuid.UUID, page, pageSize int) ([]integrations.ConnectionHealthEvent, int64, error) {
	repository.organization = organizationID
	repository.connection = connectionID
	repository.page = page
	repository.pageSize = pageSize
	return append([]integrations.ConnectionHealthEvent(nil), repository.items...), repository.total, nil
}

func cloneRouteUUIDPointer(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func sameRouteUUIDPointer(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func TestIntegrationCatalogComesOnlyFromRegisteredProviderDefinition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	accountID := uuid.New()
	workspaceID := uuid.New()
	registry := integrations.NewRegistry()
	if err := registry.Register(integrations.Registration{
		Definition: integrations.ProviderDefinition{
			ID: "github", DriverID: "github-rest", Name: "GitHub Enterprise", Description: "Organization source control.", Icon: "github",
			NameI18n: integrationRouteLocalizedText("GitHub Enterprise", "GitHub 企业版"),
			DescriptionI18n: integrationRouteLocalizedText(
				"Organization source control.",
				"组织级源代码管理。",
			),
			AuthMethods: []integrations.AuthMethodDefinition{{
				ID: "pat", Type: integrations.AuthMethodTypeAPIKey, CredentialSource: integrations.ConnectionCredentialSourceOrganization,
				Label: "PAT", LabelI18n: integrationRouteLocalizedText("PAT", "个人访问令牌"), Available: true,
				Fields: []integrations.CredentialFieldDefinition{{
					Key: "token", Label: "Token", LabelI18n: integrationRouteLocalizedText("Token", "令牌"),
					Input: integrations.CredentialFieldInputPassword, Required: true, Secret: true,
				}},
			}},
			HealthProbe: integrations.HealthProbeDefinition{Supported: true, MayIncurCost: false},
			Actions: []integrations.ActionDefinition{{
				ID: "github.issue.list", ToolName: "list_github_issues", Name: "List issues", Description: "List repository issues.",
				NameI18n:        integrationRouteLocalizedText("List issues", "列出议题"),
				DescriptionI18n: integrationRouteLocalizedText("List repository issues.", "列出仓库议题。"),
				InputSchema:     map[string]interface{}{"type": "object", "additionalProperties": false},
				OutputSchema: map[string]interface{}{
					"type": "object", "properties": map[string]interface{}{"ok": map[string]interface{}{"type": "boolean"}}, "required": []string{"ok"},
				},
				Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
				SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
			}},
		},
		Adapter: &integrationRouteAdapter{driverID: "github-rest"},
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	connectionRepository := &integrationRouteConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{}}
	grantRepository := &integrationRouteGrantRepository{}
	handler := &integrationHandler{deps: IntegrationRouteDeps{
		Registry: registry, Connections: &integrationRouteConnectionService{},
		Access: integrations.NewConnectionAccessService(connectionRepository, grantRepository),
	}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/console/api/integrations/catalog", nil)
	util.SetOrganizationID(ctx, organizationID.String())
	util.SetWorkspaceID(ctx, workspaceID.String())
	ctx.Set("account_id", accountID.String())
	attachIntegrationWorkspaceScopeForTest(handler, organizationID, accountID, workspaceID)

	handler.catalog(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{"GitHub Enterprise", "github.issue.list", "catalog_revision", "schema_hash"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("catalog body missing %q: %s", expected, body)
		}
	}
	if strings.Contains(body, "input_schema") || strings.Contains(body, "Search and read public webpages with Exa") {
		t.Fatalf("catalog used a hard-coded or oversized action contract: %s", body)
	}
	if strings.Contains(body, `"credential_source":"platform"`) ||
		strings.Contains(body, `"type":"platform"`) ||
		strings.Contains(body, "platform_credentials_configured") {
		t.Fatalf("catalog exposed deployment-owned credentials: %s", body)
	}
	if !strings.Contains(body, `"health_state":"setup_required"`) || !strings.Contains(body, `"connection_summary":{"total":0`) {
		t.Fatalf("catalog omitted authoritative health summary: %s", body)
	}

	detailRecorder := httptest.NewRecorder()
	detailContext, _ := gin.CreateTestContext(detailRecorder)
	detailContext.Params = gin.Params{{Key: "integration_id", Value: "github"}, {Key: "action_id", Value: "github.issue.list"}}
	handler.actionDetail(detailContext)
	if detailRecorder.Code != http.StatusOK || !strings.Contains(detailRecorder.Body.String(), "input_schema") {
		t.Fatalf("action detail status=%d body=%s", detailRecorder.Code, detailRecorder.Body.String())
	}

	searchRecorder := httptest.NewRecorder()
	searchContext, _ := gin.CreateTestContext(searchRecorder)
	searchContext.Request = httptest.NewRequest(http.MethodGet, "/console/api/integrations/providers/github/actions?query=issues&caller=aichat", nil)
	searchContext.Params = gin.Params{{Key: "integration_id", Value: "github"}}
	handler.searchProviderActions(searchContext)
	if searchRecorder.Code != http.StatusOK || !strings.Contains(searchRecorder.Body.String(), "github.issue.list") || strings.Contains(searchRecorder.Body.String(), "input_schema") {
		t.Fatalf("action search status=%d body=%s", searchRecorder.Code, searchRecorder.Body.String())
	}
}

func TestIntegrationCatalogHealthIncludesOnlyConnectionsVisibleToCurrentActor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	workspaceID := uuid.New()
	registry := integrations.NewRegistry()
	registerCatalogProvider := func(id string) {
		t.Helper()
		name, chineseName := integrationRouteProviderNames(id)
		err := registry.Register(integrations.Registration{
			Definition: integrations.ProviderDefinition{
				ID: id, DriverID: id + "-rest", Name: name,
				NameI18n:    integrationRouteLocalizedText(name, chineseName),
				Description: "External application used by catalog health tests.",
				DescriptionI18n: integrationRouteLocalizedText(
					"External application used by catalog health tests.",
					"目录健康状态测试使用的外部应用。",
				),
				AuthMethods: []integrations.AuthMethodDefinition{{
					ID: "personal_access_token", Type: integrations.AuthMethodTypeAPIKey,
					CredentialSource: integrations.ConnectionCredentialSourceAccount,
					Label:            "Personal access token", LabelI18n: integrationRouteLocalizedText("Personal access token", "个人访问令牌"), Available: true,
					Fields: []integrations.CredentialFieldDefinition{{
						Key: "token", Label: "Token", LabelI18n: integrationRouteLocalizedText("Token", "令牌"),
						Input: integrations.CredentialFieldInputPassword, Required: true, Secret: true,
					}},
				}},
				HealthProbe: integrations.HealthProbeDefinition{Supported: true},
				Actions: []integrations.ActionDefinition{{
					ID: id + ".item.list", ToolName: "list_" + id + "_items", Name: "List items", Description: "List available items.",
					NameI18n:        integrationRouteLocalizedText("List items", "列出项目"),
					DescriptionI18n: integrationRouteLocalizedText("List available items.", "列出可用项目。"),
					InputSchema:     map[string]interface{}{"type": "object", "additionalProperties": false},
					OutputSchema:    map[string]interface{}{"type": "object", "additionalProperties": true},
					Effect:          toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
					SupportedCallers: []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
				}},
			},
			Adapter: &integrationRouteAdapter{driverID: id + "-rest"},
		})
		if err != nil {
			t.Fatalf("register %s provider: %v", id, err)
		}
	}
	registerCatalogProvider("github")
	registerCatalogProvider("notion")
	registerCatalogProvider("slack")

	ownUnknownID := uuid.New()
	otherHealthyID := uuid.New()
	sharedDegradedID := uuid.New()
	otherOnlyHealthyID := uuid.New()
	repository := &integrationRouteConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{
		ownUnknownID: {
			ID: ownUnknownID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "mine",
			CredentialSource: integrations.ConnectionCredentialSourceAccount, OwnerAccountID: &accountID,
			Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthUnknown, AuthStatus: integrations.ConnectionAuthUnknown,
		},
		otherHealthyID: {
			ID: otherHealthyID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "other secret",
			CredentialSource: integrations.ConnectionCredentialSourceAccount, OwnerAccountID: &otherAccountID,
			Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid, ScopeStatus: integrations.ConnectionScopeVerified,
		},
		sharedDegradedID: {
			ID: sharedDegradedID, OrganizationID: organizationID, IntegrationID: "notion", DriverID: "notion-rest", Name: "shared",
			CredentialSource: integrations.ConnectionCredentialSourceOrganization,
			Status:           integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthDegraded, AuthStatus: integrations.ConnectionAuthValid,
		},
		otherOnlyHealthyID: {
			ID: otherOnlyHealthyID, OrganizationID: organizationID, IntegrationID: "slack", DriverID: "slack-rest", Name: "other only",
			CredentialSource: integrations.ConnectionCredentialSourceAccount, OwnerAccountID: &otherAccountID,
			Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid, ScopeStatus: integrations.ConnectionScopeVerified,
		},
	}}
	grantRepository := &integrationRouteGrantRepository{grants: []integrations.IntegrationConnectionGrant{{
		ID: uuid.New(), OrganizationID: organizationID, ConnectionID: sharedDegradedID,
		PrincipalType: integrations.ConnectionGrantPrincipalOrganization, AccessMode: integrations.ConnectionGrantAccessRead,
		AllowedActionIDs: []string{"notion.item.list"},
	}}}
	service := &integrationRouteConnectionService{listItems: []integrations.ConnectionView{
		{ID: ownUnknownID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "mine", CredentialSource: integrations.ConnectionCredentialSourceAccount, OwnerAccountID: &accountID, Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthUnknown, AuthStatus: integrations.ConnectionAuthUnknown},
		{ID: otherHealthyID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "other secret", CredentialSource: integrations.ConnectionCredentialSourceAccount, OwnerAccountID: &otherAccountID, Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid, ScopeStatus: integrations.ConnectionScopeVerified},
		{ID: sharedDegradedID, OrganizationID: organizationID, IntegrationID: "notion", DriverID: "notion-rest", Name: "shared", CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthDegraded, AuthStatus: integrations.ConnectionAuthValid},
		{ID: otherOnlyHealthyID, OrganizationID: organizationID, IntegrationID: "slack", DriverID: "slack-rest", Name: "other only", CredentialSource: integrations.ConnectionCredentialSourceAccount, OwnerAccountID: &otherAccountID, Status: integrations.ConnectionStatusActive, HealthStatus: integrations.ConnectionHealthHealthy, AuthStatus: integrations.ConnectionAuthValid, ScopeStatus: integrations.ConnectionScopeVerified},
	}}
	handler := &integrationHandler{deps: IntegrationRouteDeps{
		Registry: registry, Connections: service,
		Access: integrations.NewConnectionAccessService(repository, grantRepository),
	}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/console/api/integrations/catalog", nil)
	util.SetOrganizationID(ctx, organizationID.String())
	util.SetWorkspaceID(ctx, workspaceID.String())
	ctx.Set("account_id", accountID.String())
	attachIntegrationWorkspaceScopeForTest(handler, organizationID, accountID, workspaceID)

	handler.catalog(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data integrationCatalogResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode catalog response: %v; body=%s", err, recorder.Body.String())
	}
	byID := make(map[string]integrations.ProviderCatalogItem, len(payload.Data.Items))
	for _, item := range payload.Data.Items {
		byID[item.IntegrationID] = item
	}
	if item := byID["github"]; item.HealthState != integrations.ProviderHealthStateConfigured || item.ConnectionSummary == nil || item.ConnectionSummary.Total != 1 {
		t.Fatalf("other account changed GitHub aggregate: %#v", item)
	}
	if item := byID["notion"]; item.HealthState != integrations.ProviderHealthStateDegraded || item.ConnectionSummary == nil || item.ConnectionSummary.Total != 1 {
		t.Fatalf("shared degraded aggregate = %#v", item)
	}
	if item := byID["slack"]; item.HealthState != integrations.ProviderHealthStateSetupRequired || item.ConnectionSummary == nil || item.ConnectionSummary.Total != 0 {
		t.Fatalf("other account connection leaked into Slack aggregate: %#v", item)
	}
	if strings.Contains(recorder.Body.String(), "other secret") || strings.Contains(recorder.Body.String(), otherHealthyID.String()) || strings.Contains(recorder.Body.String(), otherOnlyHealthyID.String()) {
		t.Fatalf("catalog leaked another account's connection metadata: %s", recorder.Body.String())
	}

	managementRecorder := httptest.NewRecorder()
	managementContext, _ := gin.CreateTestContext(managementRecorder)
	managementContext.Request = httptest.NewRequest(http.MethodGet, "/console/api/integrations/catalog?audience=organization", nil)
	util.SetOrganizationID(managementContext, organizationID.String())
	util.SetWorkspaceID(managementContext, workspaceID.String())
	managementContext.Set("account_id", accountID.String())
	managementContext.Set("account_service", integrationCatalogAccountService{admin: true})
	handler.catalog(managementContext)
	if managementRecorder.Code != http.StatusOK {
		t.Fatalf("organization catalog status = %d, body = %s", managementRecorder.Code, managementRecorder.Body.String())
	}
	var managementPayload struct {
		Data integrationCatalogResponse `json:"data"`
	}
	if err := json.Unmarshal(managementRecorder.Body.Bytes(), &managementPayload); err != nil {
		t.Fatalf("decode organization catalog: %v", err)
	}
	managementByID := make(map[string]integrations.ProviderCatalogItem, len(managementPayload.Data.Items))
	for _, item := range managementPayload.Data.Items {
		managementByID[item.IntegrationID] = item
	}
	if item := managementByID["github"]; item.HealthState != integrations.ProviderHealthStateSetupRequired || item.ConnectionSummary == nil || item.ConnectionSummary.Total != 0 {
		t.Fatalf("organization catalog aggregated a personal connection: %#v", item)
	}
	if item := managementByID["notion"]; item.HealthState != integrations.ProviderHealthStateDegraded || item.ConnectionSummary == nil || item.ConnectionSummary.Total != 1 {
		t.Fatalf("organization catalog did not aggregate managed shared connection: %#v", item)
	}

	forbiddenRecorder := httptest.NewRecorder()
	forbiddenContext, _ := gin.CreateTestContext(forbiddenRecorder)
	forbiddenContext.Request = httptest.NewRequest(http.MethodGet, "/console/api/integrations/catalog?audience=organization", nil)
	util.SetOrganizationID(forbiddenContext, organizationID.String())
	util.SetWorkspaceID(forbiddenContext, workspaceID.String())
	forbiddenContext.Set("account_id", accountID.String())
	forbiddenContext.Set("account_service", integrationCatalogAccountService{admin: false})
	handler.catalog(forbiddenContext)
	if forbiddenRecorder.Code != http.StatusForbidden {
		t.Fatalf("non-admin organization catalog status = %d, body = %s", forbiddenRecorder.Code, forbiddenRecorder.Body.String())
	}
}

type integrationRouteConnectionService struct {
	created     integrations.CreateConnectionInput
	updated     integrations.UpdateConnectionInput
	updateCalls int
	testCalls   int
	deleted     uuid.UUID
	getItem     integrations.ConnectionView
	listItems   []integrations.ConnectionView
	listFilter  integrations.ConnectionListFilter
}

func (service *integrationRouteConnectionService) Create(_ context.Context, input integrations.CreateConnectionInput) (integrations.ConnectionView, error) {
	service.created = input
	return integrations.ConnectionView{
		ID: input.OrganizationID, OrganizationID: input.OrganizationID, IntegrationID: input.IntegrationID,
		DriverID: input.DriverID, Name: input.Name, CredentialSource: input.CredentialSource,
		AuthType: input.AuthType, CredentialConfigured: true, Status: integrations.ConnectionStatusPending,
	}, nil
}

func (service *integrationRouteConnectionService) Get(context.Context, uuid.UUID, uuid.UUID) (integrations.ConnectionView, error) {
	return service.getItem, nil
}

func (service *integrationRouteConnectionService) List(_ context.Context, _ uuid.UUID, filter integrations.ConnectionListFilter) ([]integrations.ConnectionView, error) {
	service.listFilter = filter
	items := make([]integrations.ConnectionView, 0, len(service.listItems))
	for _, item := range service.listItems {
		if integrationRouteConnectionViewMatchesFilter(item, filter) {
			items = append(items, item)
		}
	}
	return items, nil
}

func (service *integrationRouteConnectionService) ListPage(ctx context.Context, organizationID uuid.UUID, filter integrations.ConnectionListFilter) (integrations.ConnectionListPage, error) {
	items, err := service.List(ctx, organizationID, filter)
	if err != nil {
		return integrations.ConnectionListPage{}, err
	}
	return integrations.ConnectionListPage{
		Items: items, Page: filter.Page, PageSize: filter.PageSize, Total: int64(len(items)),
		HasMore: false,
	}, nil
}

func (service *integrationRouteConnectionService) Update(_ context.Context, input integrations.UpdateConnectionInput) (integrations.ConnectionView, error) {
	service.updated = input
	service.updateCalls++
	return service.getItem, nil
}

func (service *integrationRouteConnectionService) Test(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) (integrations.ConnectionView, *integrations.ConnectionProfile, error) {
	service.testCalls++
	return service.getItem, nil, nil
}

func (*integrationRouteConnectionService) SetDefault(context.Context, uuid.UUID, uuid.UUID) (integrations.ConnectionView, error) {
	return integrations.ConnectionView{}, nil
}

func (*integrationRouteConnectionService) SetDefaultAs(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID) (integrations.ConnectionView, error) {
	return integrations.ConnectionView{}, nil
}

func (service *integrationRouteConnectionService) Delete(_ context.Context, _ uuid.UUID, connectionID uuid.UUID) error {
	service.deleted = connectionID
	return nil
}

func (service *integrationRouteConnectionService) DeleteAs(ctx context.Context, organizationID, connectionID uuid.UUID, _ *uuid.UUID) error {
	return service.Delete(ctx, organizationID, connectionID)
}

func integrationRouteConnectionViewMatchesFilter(item integrations.ConnectionView, filter integrations.ConnectionListFilter) bool {
	if filter.IntegrationID != "" && item.IntegrationID != filter.IntegrationID {
		return false
	}
	if filter.DriverID != "" && item.DriverID != filter.DriverID {
		return false
	}
	if len(filter.CredentialSources) > 0 {
		matched := false
		for _, source := range filter.CredentialSources {
			matched = matched || item.CredentialSource == source
		}
		if !matched {
			return false
		}
	}
	if filter.OwnerAccountID != nil && (item.OwnerAccountID == nil || *item.OwnerAccountID != *filter.OwnerAccountID) {
		return false
	}
	return true
}

func TestIntegrationCreateConnectionUsesAuthenticatedOrganizationAndNeverEchoesCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	actorID := uuid.New()
	service := &integrationRouteConnectionService{}
	handler := &integrationHandler{deps: IntegrationRouteDeps{Connections: service}}
	body := `{"organization_id":"` + uuid.NewString() + `","integration_id":"web-search","driver_id":"exa","name":"Team Exa","credential_source":"organization","auth_type":"api_key","auth_method_id":"organization_api_key","credentials":{"api_key":"top-secret-value"}}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/console/api/integrations/connections", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	util.SetOrganizationID(ctx, organizationID.String())
	ctx.Set("account_id", actorID.String())

	handler.createConnection(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.created.OrganizationID != organizationID || service.created.ActorID == nil || *service.created.ActorID != actorID {
		t.Fatalf("server scope was not authoritative: %#v", service.created)
	}
	if service.created.AuthMethodID != "organization_api_key" || service.created.OwnerAccountID != nil {
		t.Fatalf("authentication method/owner = %q/%v", service.created.AuthMethodID, service.created.OwnerAccountID)
	}
	if strings.Contains(recorder.Body.String(), "top-secret-value") || strings.Contains(recorder.Body.String(), "credentials") {
		t.Fatalf("credential leaked in response: %s", recorder.Body.String())
	}
	if len(service.created.Credentials) != 0 {
		t.Fatalf("handler retained credential map after request: %#v", service.created.Credentials)
	}
}

func TestIntegrationCreateMyConnectionUsesAuthenticatedActorAsOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID := uuid.New()
	actorID := uuid.New()
	service := &integrationRouteConnectionService{}
	handler := &integrationHandler{deps: IntegrationRouteDeps{Connections: service}}
	body := `{"integration_id":"github","driver_id":"github-rest","name":"My GitHub","credential_source":"account","auth_type":"api_key","auth_method_id":"personal_access_token","credentials":{"token":"github_pat_secret"}}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/console/api/integrations/connections", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	util.SetOrganizationID(ctx, organizationID.String())
	ctx.Set("account_id", actorID.String())

	handler.createMyConnection(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.created.OwnerAccountID == nil || *service.created.OwnerAccountID != actorID || service.created.AuthMethodID != "personal_access_token" {
		t.Fatalf("personal connection ownership = %#v", service.created)
	}
	if strings.Contains(recorder.Body.String(), "github_pat_secret") || len(service.created.Credentials) != 0 {
		t.Fatalf("credential was retained or returned: input=%#v body=%s", service.created, recorder.Body.String())
	}
}

func TestIntegrationDeleteRefusesConnectionStillBoundToAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	organizationID := uuid.New()
	connectionID := uuid.New()
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT\("agent_id"\)\) FROM "agent_resource_bindings"`).
		WithArgs(organizationID, "integration_connection", connectionID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	service := &integrationRouteConnectionService{getItem: integrations.ConnectionView{
		ID: connectionID, OrganizationID: organizationID,
		CredentialSource: integrations.ConnectionCredentialSourceOrganization,
	}}
	handler := &integrationHandler{deps: IntegrationRouteDeps{DB: db, Connections: service}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/console/api/integrations/connections/"+connectionID.String(), nil)
	ctx.Params = gin.Params{{Key: "id", Value: connectionID.String()}}
	util.SetOrganizationID(ctx, organizationID.String())
	ctx.Set("account_id", uuid.NewString())

	handler.deleteConnection(ctx)

	if recorder.Code != http.StatusConflict || service.deleted != uuid.Nil {
		t.Fatalf("delete status=%d deleted=%s body=%s", recorder.Code, service.deleted, recorder.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil || payload["code"] != "integration_connection_in_use" {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}

func TestIntegrationRouteErrorMapsConnectionConflictsToHTTPConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, testCase := range []struct {
		name string
		code string
	}{
		{name: "concurrent update or duplicate name", code: integrations.ErrorCodeConnectionConflict},
		{name: "bound connection", code: integrations.ErrorCodeConnectionInUse},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			integrationRouteError(ctx, integrations.NewError(testCase.code, "connection conflict", nil))
			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil || payload["code"] != testCase.code {
				t.Fatalf("unexpected response: %s", recorder.Body.String())
			}
		})
	}
}

func newIntegrationRouteRegistry(t *testing.T) *integrations.Registry {
	return newIntegrationRouteRegistryWithWriteAction(t, false)
}

func newIntegrationRouteRegistryWithWriteAction(t *testing.T, includeWriteAction bool) *integrations.Registry {
	t.Helper()
	registry := integrations.NewRegistry()
	actions := []integrations.ActionDefinition{{
		ID: "github.issue.list", ToolName: "list_github_issues", Name: "List issues", Description: "List issues.",
		NameI18n:        integrationRouteLocalizedText("List issues", "列出议题"),
		DescriptionI18n: integrationRouteLocalizedText("List issues.", "列出议题。"),
		InputSchema:     map[string]interface{}{"type": "object", "additionalProperties": false},
		OutputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{"ok": map[string]interface{}{"type": "boolean"}}, "required": []string{"ok"},
		},
		Effect: toolgovernance.EffectRead, RiskLevel: toolgovernance.RiskLevelLow,
		SupportedAuthMethodIDs: []string{"pat"},
		SupportedCallers:       []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
	}}
	if includeWriteAction {
		actions = append(actions, integrations.ActionDefinition{
			ID: "github.issue.create", ToolName: "create_github_issue", Name: "Create issue", Description: "Create an issue.",
			NameI18n:        integrationRouteLocalizedText("Create issue", "创建议题"),
			DescriptionI18n: integrationRouteLocalizedText("Create an issue.", "创建一个议题。"),
			InputSchema:     map[string]interface{}{"type": "object", "additionalProperties": false},
			OutputSchema: map[string]interface{}{
				"type": "object", "properties": map[string]interface{}{"ok": map[string]interface{}{"type": "boolean"}}, "required": []string{"ok"},
			},
			Effect: toolgovernance.EffectCreate, RiskLevel: toolgovernance.RiskLevelMedium,
			SupportedAuthMethodIDs: []string{"pat"},
			SupportedCallers:       []tools.ToolInvokeFrom{tools.ToolInvokeFromAIChat},
		})
	}
	err := registry.Register(integrations.Registration{
		Definition: integrations.ProviderDefinition{
			ID: "github", DriverID: "github-rest", Name: "GitHub", Description: "GitHub provider.", Icon: "github",
			NameI18n:        integrationRouteLocalizedText("GitHub", "GitHub"),
			DescriptionI18n: integrationRouteLocalizedText("GitHub provider.", "GitHub 提供方。"),
			AuthMethods: []integrations.AuthMethodDefinition{{
				ID: "pat", Type: integrations.AuthMethodTypeAPIKey,
				CredentialSource: integrations.ConnectionCredentialSourceOrganization,
				Label:            "PAT", LabelI18n: integrationRouteLocalizedText("PAT", "个人访问令牌"), Available: true,
				Fields: []integrations.CredentialFieldDefinition{{
					Key: "token", Label: "Token", LabelI18n: integrationRouteLocalizedText("Token", "令牌"),
					Input: integrations.CredentialFieldInputPassword, Required: true, Secret: true,
				}},
			}},
			Actions: actions,
		},
		Adapter: &integrationRouteAdapter{driverID: "github-rest"},
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	return registry
}

func integrationRouteLocalizedText(english, simplifiedChinese string) integrations.LocalizedText {
	return integrations.LocalizedText{
		integrations.LocaleEnglishUS:         english,
		integrations.LocaleSimplifiedChinese: simplifiedChinese,
	}
}

func integrationRouteProviderNames(id string) (string, string) {
	switch id {
	case "github":
		return "GitHub", "GitHub"
	case "notion":
		return "Notion", "Notion"
	case "slack":
		return "Slack", "Slack"
	default:
		return "Test external application", "测试外部应用"
	}
}

func TestIntegrationReplaceAIChatPreferencesConvertsRouteDTOAndUsesAuthenticatedScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID, accountID, workspaceID, connectionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	connectionRepo := &integrationRouteConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{
		connectionID: {
			ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest",
			CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive,
		},
	}}
	grantRepo := &integrationRouteGrantRepository{grants: []integrations.IntegrationConnectionGrant{{
		OrganizationID: organizationID, ConnectionID: connectionID,
		PrincipalType: integrations.ConnectionGrantPrincipalAccount, PrincipalID: &accountID,
		AccessMode: integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"},
	}}}
	preferenceRepo := &integrationRoutePreferenceRepository{}
	access := integrations.NewConnectionAccessService(connectionRepo, grantRepo)
	handler := &integrationHandler{deps: IntegrationRouteDeps{
		Preferences: integrations.NewAIChatIntegrationPreferenceService(preferenceRepo, connectionRepo, access),
	}}
	body := `{"items":[{"integration_id":"github","selected_connection_ids":["` + connectionID.String() + `"],"preferred_connection_id":"` + connectionID.String() + `"}]}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/console/api/integrations/aichat/preferences", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	util.SetOrganizationID(ctx, organizationID.String())
	util.SetWorkspaceID(ctx, workspaceID.String())
	ctx.Set("account_id", accountID.String())
	attachIntegrationWorkspaceScopeForTest(handler, organizationID, accountID, workspaceID)

	handler.replaceAIChatPreferences(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if preferenceRepo.organizationID != organizationID || preferenceRepo.accountID != accountID || preferenceRepo.workspaceID == nil || *preferenceRepo.workspaceID != workspaceID {
		t.Fatalf("preference scope = %s/%s/%v", preferenceRepo.organizationID, preferenceRepo.accountID, preferenceRepo.workspaceID)
	}
	if len(preferenceRepo.items) != 1 || preferenceRepo.items[0].IntegrationID != "github" || len(preferenceRepo.items[0].SelectedConnectionIDs) != 1 || preferenceRepo.items[0].SelectedConnectionIDs[0] != connectionID.String() || preferenceRepo.items[0].PreferredConnectionID == nil || *preferenceRepo.items[0].PreferredConnectionID != connectionID {
		t.Fatalf("converted preferences = %#v", preferenceRepo.items)
	}
}

func TestIntegrationReplaceAIChatPreferencesAllowsExplicitEmptySelection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID, accountID, workspaceID := uuid.New(), uuid.New(), uuid.New()
	preferenceRepo := &integrationRoutePreferenceRepository{items: []integrations.AIChatIntegrationPreference{{
		OrganizationID: organizationID,
		AccountID:      accountID,
		WorkspaceID:    &workspaceID,
		IntegrationID:  "github",
	}}}
	handler := &integrationHandler{deps: IntegrationRouteDeps{
		Preferences: integrations.NewAIChatIntegrationPreferenceService(
			preferenceRepo,
			&integrationRouteConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{}},
			integrations.NewConnectionAccessService(
				&integrationRouteConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{}},
				&integrationRouteGrantRepository{},
			),
		),
	}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/console/api/integrations/aichat/preferences", bytes.NewBufferString(`{"items":[]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	util.SetOrganizationID(ctx, organizationID.String())
	util.SetWorkspaceID(ctx, workspaceID.String())
	ctx.Set("account_id", accountID.String())
	attachIntegrationWorkspaceScopeForTest(handler, organizationID, accountID, workspaceID)

	handler.replaceAIChatPreferences(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if preferenceRepo.organizationID != organizationID || preferenceRepo.accountID != accountID || len(preferenceRepo.items) != 0 {
		t.Fatalf("preferences = %#v, want explicit empty replacement", preferenceRepo.items)
	}
}

func TestIntegrationListAIChatPreferencesReturnsOnlyCurrentAuthorizedSelections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID, accountID, workspaceID := uuid.New(), uuid.New(), uuid.New()
	allowedID, revokedID, deletedID := uuid.New(), uuid.New(), uuid.New()
	connectionRepo := &integrationRouteConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{
		allowedID: {
			ID: allowedID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "allowed",
			CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive,
		},
		revokedID: {
			ID: revokedID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "must-not-leak",
			CredentialSource: integrations.ConnectionCredentialSourceOrganization, Status: integrations.ConnectionStatusActive,
		},
	}}
	grantRepo := &integrationRouteGrantRepository{grants: []integrations.IntegrationConnectionGrant{{
		OrganizationID: organizationID, ConnectionID: allowedID,
		PrincipalType: integrations.ConnectionGrantPrincipalAccount, PrincipalID: &accountID,
		AccessMode: integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"},
	}}}
	preferenceRepo := &integrationRoutePreferenceRepository{
		organizationID: organizationID,
		accountID:      accountID,
		workspaceID:    &workspaceID,
		items: []integrations.AIChatIntegrationPreference{{
			ID: uuid.New(), OrganizationID: organizationID, AccountID: accountID, WorkspaceID: &workspaceID,
			IntegrationID: "github", SelectedConnectionIDs: []string{allowedID.String(), revokedID.String(), deletedID.String()},
			PreferredConnectionID: &revokedID, Revision: 1,
		}},
	}
	handler := &integrationHandler{deps: IntegrationRouteDeps{
		Preferences: integrations.NewAIChatIntegrationPreferenceService(
			preferenceRepo,
			connectionRepo,
			integrations.NewConnectionAccessService(connectionRepo, grantRepo),
		),
	}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/console/api/integrations/aichat/preferences", nil)
	util.SetOrganizationID(ctx, organizationID.String())
	util.SetWorkspaceID(ctx, workspaceID.String())
	ctx.Set("account_id", accountID.String())
	attachIntegrationWorkspaceScopeForTest(handler, organizationID, accountID, workspaceID)

	handler.listAIChatPreferences(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, allowedID.String()) {
		t.Fatalf("authorized connection missing from response: %s", body)
	}
	for _, forbidden := range []string{revokedID.String(), deletedID.String(), "must-not-leak"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("inaccessible connection metadata %q leaked in response: %s", forbidden, body)
		}
	}
	var payload struct {
		Data struct {
			Items []integrations.AIChatIntegrationPreference `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Data.Items) != 1 || payload.Data.Items[0].PreferredConnectionID == nil || *payload.Data.Items[0].PreferredConnectionID != allowedID {
		t.Fatalf("sanitized route preferences = %#v", payload.Data.Items)
	}
}

func TestIntegrationAvailableConnectionsFiltersByConnectionACL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID, accountID, workspaceID := uuid.New(), uuid.New(), uuid.New()
	allowedID, deniedID, ownedID, otherOwnedID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	otherAccountID := uuid.New()
	connectionRepo := &integrationRouteConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{}}
	service := &integrationRouteConnectionService{}
	for _, item := range []struct {
		id     uuid.UUID
		source integrations.ConnectionCredentialSource
		owner  *uuid.UUID
		name   string
	}{
		{id: allowedID, source: integrations.ConnectionCredentialSourceOrganization, name: "granted"},
		{id: deniedID, source: integrations.ConnectionCredentialSourceOrganization, name: "not-granted"},
		{id: ownedID, source: integrations.ConnectionCredentialSourceAccount, owner: &accountID, name: "mine"},
		{id: otherOwnedID, source: integrations.ConnectionCredentialSourceAccount, owner: &otherAccountID, name: "someone-else"},
	} {
		connectionRepo.items[item.id] = &integrations.IntegrationConnection{
			ID: item.id, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: item.name,
			CredentialSource: item.source, OwnerAccountID: item.owner, Status: integrations.ConnectionStatusActive,
		}
		service.listItems = append(service.listItems, integrations.ConnectionView{
			ID: item.id, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: item.name,
			CredentialSource: item.source, OwnerAccountID: item.owner, Status: integrations.ConnectionStatusActive,
		})
	}
	grantRepo := &integrationRouteGrantRepository{grants: []integrations.IntegrationConnectionGrant{{
		OrganizationID: organizationID, ConnectionID: allowedID,
		PrincipalType: integrations.ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID,
		AccessMode: integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"},
	}}}
	handler := &integrationHandler{deps: IntegrationRouteDeps{
		Connections: service, Access: integrations.NewConnectionAccessService(connectionRepo, grantRepo),
	}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/console/api/integrations/available-connections?integration_id=github", nil)
	util.SetOrganizationID(ctx, organizationID.String())
	util.SetWorkspaceID(ctx, workspaceID.String())
	ctx.Set("account_id", accountID.String())
	attachIntegrationWorkspaceScopeForTest(handler, organizationID, accountID, workspaceID)

	handler.listAvailableConnections(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, allowedID.String()) || !strings.Contains(body, ownedID.String()) || strings.Contains(body, deniedID.String()) || strings.Contains(body, otherOwnedID.String()) || !strings.Contains(body, `"total":2`) {
		t.Fatalf("ACL-filtered response = %s", body)
	}
}

func TestIntegrationConnectionGrantCRUDAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID, actorID, connectionID := uuid.New(), uuid.New(), uuid.New()
	service := &integrationRouteConnectionService{getItem: integrations.ConnectionView{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest",
		AuthMethodID:     "pat",
		CredentialSource: integrations.ConnectionCredentialSourceOrganization,
	}}
	newContext := func(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "id", Value: connectionID.String()}}
		util.SetOrganizationID(ctx, organizationID.String())
		ctx.Set("account_id", actorID.String())
		return ctx, recorder
	}

	t.Run("create update and delete", func(t *testing.T) {
		grantRepo := &integrationRouteGrantRepository{}
		handler := &integrationHandler{deps: IntegrationRouteDeps{Registry: newIntegrationRouteRegistry(t), Connections: service, Grants: grantRepo}}
		ctx, recorder := newContext(http.MethodPost, "/connections/"+connectionID.String()+"/grants", `{"principal_type":"organization","access_mode":"read","allowed_action_ids":["github.issue.list"]}`)
		handler.createConnectionGrant(ctx)
		if recorder.Code != http.StatusOK || grantRepo.saved == nil || grantRepo.saveExpected != 0 || len(grantRepo.saved.ResourceConstraints) != 0 {
			t.Fatalf("create status=%d saved=%#v body=%s", recorder.Code, grantRepo.saved, recorder.Body.String())
		}

		grantID := uuid.New()
		grantRepo.grants = append(grantRepo.grants, integrations.IntegrationConnectionGrant{
			ID: grantID, OrganizationID: organizationID, ConnectionID: connectionID,
			PrincipalType: integrations.ConnectionGrantPrincipalOrganization,
			AccessMode:    integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"},
			ResourceConstraints: map[string]interface{}{}, Revision: 3,
		})
		ctx, recorder = newContext(http.MethodPut, "/connections/"+connectionID.String()+"/grants/"+grantID.String(), `{"revision":3,"principal_type":"organization","access_mode":"read","allowed_action_ids":["github.issue.list"]}`)
		ctx.Params = append(ctx.Params, gin.Param{Key: "grant_id", Value: grantID.String()})
		handler.updateConnectionGrant(ctx)
		if recorder.Code != http.StatusOK || grantRepo.saved == nil || grantRepo.saved.ID != grantID || grantRepo.saveExpected != 3 || len(grantRepo.saved.ResourceConstraints) != 0 {
			t.Fatalf("update status=%d saved=%#v body=%s", recorder.Code, grantRepo.saved, recorder.Body.String())
		}

		ctx, recorder = newContext(http.MethodDelete, "/connections/"+connectionID.String()+"/grants/"+grantID.String(), "")
		ctx.Params = append(ctx.Params, gin.Param{Key: "grant_id", Value: grantID.String()})
		handler.deleteConnectionGrant(ctx)
		if recorder.Code != http.StatusOK || grantRepo.deleted != grantID || grantRepo.deleteConnID != connectionID {
			t.Fatalf("delete status=%d deleted=%s connection=%s body=%s", recorder.Code, grantRepo.deleted, grantRepo.deleteConnID, recorder.Body.String())
		}
	})

	t.Run("update refuses to erase an existing resource constraint", func(t *testing.T) {
		grantID := uuid.New()
		grantRepo := &integrationRouteGrantRepository{grants: []integrations.IntegrationConnectionGrant{{
			ID: grantID, OrganizationID: organizationID, ConnectionID: connectionID,
			PrincipalType: integrations.ConnectionGrantPrincipalOrganization,
			AccessMode:    integrations.ConnectionGrantAccessRead, AllowedActionIDs: []string{"github.issue.list"},
			ResourceConstraints: map[string]interface{}{"resource_ids": []string{"repo-private"}}, Revision: 2,
		}}}
		handler := &integrationHandler{deps: IntegrationRouteDeps{Registry: newIntegrationRouteRegistry(t), Connections: service, Grants: grantRepo}}
		ctx, recorder := newContext(http.MethodPut, "/connections/"+connectionID.String()+"/grants/"+grantID.String(), `{"revision":2,"principal_type":"organization","access_mode":"read","allowed_action_ids":["github.issue.list"]}`)
		ctx.Params = append(ctx.Params, gin.Param{Key: "grant_id", Value: grantID.String()})
		handler.updateConnectionGrant(ctx)
		if recorder.Code != http.StatusBadRequest || grantRepo.saved != nil {
			t.Fatalf("status=%d saved=%#v body=%s", recorder.Code, grantRepo.saved, recorder.Body.String())
		}
		if got := grantRepo.grants[0].ResourceConstraints["resource_ids"]; got == nil {
			t.Fatalf("existing resource constraints were cleared: %#v", grantRepo.grants[0])
		}
	})

	t.Run("read access rejects non-read actions", func(t *testing.T) {
		grantRepo := &integrationRouteGrantRepository{}
		handler := &integrationHandler{deps: IntegrationRouteDeps{
			Registry: newIntegrationRouteRegistryWithWriteAction(t, true), Connections: service, Grants: grantRepo,
		}}
		ctx, recorder := newContext(http.MethodPost, "/connections/"+connectionID.String()+"/grants", `{"principal_type":"organization","access_mode":"read","allowed_action_ids":["github.issue.create"]}`)
		handler.createConnectionGrant(ctx)
		if recorder.Code != http.StatusBadRequest || grantRepo.saved != nil || !strings.Contains(recorder.Body.String(), integrations.ErrorCodeInvalidInput) {
			t.Fatalf("status=%d saved=%#v body=%s", recorder.Code, grantRepo.saved, recorder.Body.String())
		}
	})

	t.Run("write access remains compatible with non-read actions", func(t *testing.T) {
		grantRepo := &integrationRouteGrantRepository{}
		handler := &integrationHandler{deps: IntegrationRouteDeps{
			Registry: newIntegrationRouteRegistryWithWriteAction(t, true), Connections: service, Grants: grantRepo,
		}}
		ctx, recorder := newContext(http.MethodPost, "/connections/"+connectionID.String()+"/grants", `{"principal_type":"organization","access_mode":"write","allowed_action_ids":["github.issue.create"]}`)
		handler.createConnectionGrant(ctx)
		if recorder.Code != http.StatusOK || grantRepo.saved == nil || grantRepo.saved.AccessMode != integrations.ConnectionGrantAccessWrite {
			t.Fatalf("status=%d saved=%#v body=%s", recorder.Code, grantRepo.saved, recorder.Body.String())
		}
	})

	t.Run("authentication-incompatible actions are rejected", func(t *testing.T) {
		grantRepo := &integrationRouteGrantRepository{}
		previousAuthMethodID := service.getItem.AuthMethodID
		service.getItem.AuthMethodID = "tenant_app"
		t.Cleanup(func() { service.getItem.AuthMethodID = previousAuthMethodID })
		handler := &integrationHandler{deps: IntegrationRouteDeps{
			Registry: newIntegrationRouteRegistry(t), Connections: service, Grants: grantRepo,
		}}
		ctx, recorder := newContext(http.MethodPost, "/connections/"+connectionID.String()+"/grants", `{"principal_type":"organization","access_mode":"read","allowed_action_ids":["github.issue.list"]}`)
		handler.createConnectionGrant(ctx)
		if recorder.Code != http.StatusBadRequest || grantRepo.saved != nil ||
			!strings.Contains(recorder.Body.String(), integrations.ErrorCodeInvalidInput) {
			t.Fatalf("status=%d saved=%#v body=%s", recorder.Code, grantRepo.saved, recorder.Body.String())
		}
	})

	for _, testCase := range []struct {
		name string
		body string
	}{
		{name: "unknown provider action", body: `{"principal_type":"organization","access_mode":"read","allowed_action_ids":["github.issue.delete"]}`},
		{name: "organization principal cannot carry an id", body: `{"principal_type":"organization","principal_id":"` + uuid.NewString() + `","access_mode":"read","allowed_action_ids":["github.issue.list"]}`},
		{name: "resource constraints are not accepted before provider extraction exists", body: `{"principal_type":"organization","access_mode":"read","allowed_action_ids":["github.issue.list"],"resource_constraints":{"allow_all":true}}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			grantRepo := &integrationRouteGrantRepository{}
			handler := &integrationHandler{deps: IntegrationRouteDeps{Registry: newIntegrationRouteRegistry(t), Connections: service, Grants: grantRepo}}
			ctx, recorder := newContext(http.MethodPost, "/connections/"+connectionID.String()+"/grants", testCase.body)
			handler.createConnectionGrant(ctx)
			if recorder.Code != http.StatusBadRequest || grantRepo.saved != nil {
				t.Fatalf("status=%d saved=%#v body=%s", recorder.Code, grantRepo.saved, recorder.Body.String())
			}
		})
	}

	for _, testCase := range []struct {
		name       string
		rowCount   int
		wantStatus int
		wantSaved  bool
	}{
		{name: "archived workspace principal is rejected", rowCount: 0, wantStatus: http.StatusBadRequest},
		{name: "normal workspace principal is accepted", rowCount: 1, wantStatus: http.StatusOK, wantSaved: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New: %v", err)
			}
			defer sqlDB.Close()
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
			if err != nil {
				t.Fatalf("gorm.Open: %v", err)
			}
			workspaceID := uuid.New()
			mock.ExpectQuery(`SELECT count\(\*\) FROM "workspaces"`).
				WithArgs(workspaceID.String(), organizationID, "normal").
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(testCase.rowCount))
			grantRepo := &integrationRouteGrantRepository{}
			handler := &integrationHandler{deps: IntegrationRouteDeps{DB: db, Registry: newIntegrationRouteRegistry(t), Connections: service, Grants: grantRepo}}
			ctx, recorder := newContext(http.MethodPost, "/connections/"+connectionID.String()+"/grants", `{"principal_type":"workspace","principal_id":"`+workspaceID.String()+`","access_mode":"read","allowed_action_ids":["github.issue.list"]}`)
			handler.createConnectionGrant(ctx)
			if recorder.Code != testCase.wantStatus || (grantRepo.saved != nil) != testCase.wantSaved {
				t.Fatalf("status=%d saved=%#v body=%s", recorder.Code, grantRepo.saved, recorder.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("SQL expectations: %v", err)
			}
		})
	}
}

func TestIntegrationConnectionGrantListResolvesScopedPrincipalNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID, connectionID := uuid.New(), uuid.New()
	workspaceID, missingWorkspaceID := uuid.New(), uuid.New()
	accountID, missingAccountID := uuid.New(), uuid.New()
	service := &integrationRouteConnectionService{getItem: integrations.ConnectionView{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest",
		CredentialSource: integrations.ConnectionCredentialSourceOrganization,
	}}
	grants := []integrations.IntegrationConnectionGrant{
		{ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID, PrincipalType: integrations.ConnectionGrantPrincipalOrganization, AccessMode: integrations.ConnectionGrantAccessRead},
		{ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID, PrincipalType: integrations.ConnectionGrantPrincipalWorkspace, PrincipalID: &workspaceID, AccessMode: integrations.ConnectionGrantAccessRead, ResourceConstraints: map[string]interface{}{"resource_ids": []string{"repo-a"}}},
		{ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID, PrincipalType: integrations.ConnectionGrantPrincipalWorkspace, PrincipalID: &missingWorkspaceID, AccessMode: integrations.ConnectionGrantAccessRead},
		{ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID, PrincipalType: integrations.ConnectionGrantPrincipalAccount, PrincipalID: &accountID, AccessMode: integrations.ConnectionGrantAccessRead},
		{ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID, PrincipalType: integrations.ConnectionGrantPrincipalAccount, PrincipalID: &missingAccountID, AccessMode: integrations.ConnectionGrantAccessRead},
	}
	grantRepo := &integrationRouteGrantRepository{grants: grants}

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	mock.ExpectQuery(`FROM "organizations"`).
		WithArgs(organizationID.String(), "active").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(organizationID.String(), "Default Group"))
	mock.ExpectQuery(`FROM "workspaces"`).
		WithArgs(organizationID.String(), "normal", workspaceID.String(), missingWorkspaceID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(workspaceID.String(), "Engineering"))
	mock.ExpectQuery(`FROM members AS member`).
		WithArgs(organizationID.String(), "active", accountID.String(), missingAccountID.String()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(accountID.String(), "Alice"))

	handler := &integrationHandler{deps: IntegrationRouteDeps{DB: db, Connections: service, Grants: grantRepo}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/connections/"+connectionID.String()+"/grants", nil)
	ctx.Params = gin.Params{{Key: "id", Value: connectionID.String()}}
	util.SetOrganizationID(ctx, organizationID.String())
	handler.listConnectionGrants(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data struct {
			Items []integrationConnectionGrantResponse `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, recorder.Body.String())
	}
	if len(payload.Data.Items) != len(grants) {
		t.Fatalf("items=%d want=%d body=%s", len(payload.Data.Items), len(grants), recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), `"resource_constraints"`) {
		t.Fatalf("grant list leaked non-editable resource constraints: %s", recorder.Body.String())
	}
	byID := make(map[uuid.UUID]integrationConnectionGrantResponse, len(payload.Data.Items))
	for _, item := range payload.Data.Items {
		byID[item.ID] = item
	}
	for _, expected := range []struct {
		grantID                uuid.UUID
		name                   string
		state                  integrationGrantPrincipalState
		hasResourceConstraints bool
		editable               bool
	}{
		{grants[0].ID, "Default Group", integrationGrantPrincipalStateActive, false, true},
		{grants[1].ID, "Engineering", integrationGrantPrincipalStateActive, true, false},
		{grants[2].ID, "", integrationGrantPrincipalStateMissing, false, true},
		{grants[3].ID, "Alice", integrationGrantPrincipalStateActive, false, true},
		{grants[4].ID, "", integrationGrantPrincipalStateMissing, false, true},
	} {
		item, found := byID[expected.grantID]
		if !found || item.PrincipalDisplayName != expected.name || item.PrincipalState != expected.state || item.HasResourceConstraints != expected.hasResourceConstraints || item.Editable != expected.editable {
			t.Fatalf("grant %s = %#v, want name=%q state=%s constrained=%t editable=%t", expected.grantID, item, expected.name, expected.state, expected.hasResourceConstraints, expected.editable)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}

func TestIntegrationConnectionHealthHistoryPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID, connectionID := uuid.New(), uuid.New()
	service := &integrationRouteConnectionService{getItem: integrations.ConnectionView{
		ID: connectionID, OrganizationID: organizationID,
		CredentialSource: integrations.ConnectionCredentialSourceOrganization,
	}}
	healthRepo := &integrationRouteHealthRepository{
		items: []integrations.ConnectionHealthEvent{{
			ID: uuid.New(), OrganizationID: organizationID, ConnectionID: connectionID,
			Classification: integrations.ConnectionHealthClassificationSuccess, ObservedAt: time.Now().UTC(),
		}},
		total: 25,
	}
	handler := &integrationHandler{deps: IntegrationRouteDeps{Connections: service, HealthEvents: healthRepo}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/connections/"+connectionID.String()+"/health-events?page=2&page_size=10", nil)
	ctx.Params = gin.Params{{Key: "id", Value: connectionID.String()}}
	util.SetOrganizationID(ctx, organizationID.String())

	handler.listConnectionHealthEvents(ctx)

	if recorder.Code != http.StatusOK || healthRepo.organization != organizationID || healthRepo.connection != connectionID || healthRepo.page != 2 || healthRepo.pageSize != 10 {
		t.Fatalf("status=%d query=%s/%s page=%d size=%d body=%s", recorder.Code, healthRepo.organization, healthRepo.connection, healthRepo.page, healthRepo.pageSize, recorder.Body.String())
	}
	for _, expected := range []string{`"page":2`, `"page_size":10`, `"total":25`, `"has_more":true`} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("response missing %s: %s", expected, recorder.Body.String())
		}
	}
}

func TestIntegrationPersonalConnectionUpdateDeniesNonOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID, actorID, otherAccountID, connectionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	connectionRepo := &integrationRouteConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{
		connectionID: {
			ID: connectionID, OrganizationID: organizationID, IntegrationID: "github",
			CredentialSource: integrations.ConnectionCredentialSourceAccount, OwnerAccountID: &otherAccountID,
		},
	}}
	service := &integrationRouteConnectionService{}
	handler := &integrationHandler{deps: IntegrationRouteDeps{ConnectionRepo: connectionRepo, Connections: service}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/my-connections/"+connectionID.String(), bytes.NewBufferString(`{"revision":1,"name":"stolen"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: connectionID.String()}}
	util.SetOrganizationID(ctx, organizationID.String())
	ctx.Set("account_id", actorID.String())

	handler.updateMyConnection(ctx)

	if recorder.Code != http.StatusForbidden || service.updateCalls != 0 || !strings.Contains(recorder.Body.String(), integrations.ErrorCodeAccessDenied) {
		t.Fatalf("status=%d update_calls=%d body=%s", recorder.Code, service.updateCalls, recorder.Body.String())
	}
}

func TestIntegrationManagementRoutesHidePersonalConnectionsAndOwnerUsesMyConnections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organizationID, ownerID, adminID := uuid.New(), uuid.New(), uuid.New()
	personalID, organizationConnectionID := uuid.New(), uuid.New()
	privateAccountID := "github-user-private"
	privateDisplayName := "Private GitHub account"
	personalRecord := &integrations.IntegrationConnection{
		ID: personalID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest",
		Name: "Owner personal GitHub", CredentialSource: integrations.ConnectionCredentialSourceAccount,
		OwnerAccountID: &ownerID, AccountID: &privateAccountID, DisplayName: &privateDisplayName,
		Config: map[string]interface{}{"private_metadata": "owner-only"}, Status: integrations.ConnectionStatusActive,
	}
	organizationRecord := &integrations.IntegrationConnection{
		ID: organizationConnectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest",
		Name: "Organization GitHub", CredentialSource: integrations.ConnectionCredentialSourceOrganization,
		Status: integrations.ConnectionStatusActive,
	}
	connectionRepo := &integrationRouteConnectionRepository{items: map[uuid.UUID]*integrations.IntegrationConnection{
		personalID: personalRecord, organizationConnectionID: organizationRecord,
	}}
	service := &integrationRouteConnectionService{
		getItem: integrations.ConnectionView{
			ID: personalID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest",
			Name: personalRecord.Name, CredentialSource: integrations.ConnectionCredentialSourceAccount,
			OwnerAccountID: &ownerID, AccountID: &privateAccountID, DisplayName: &privateDisplayName,
			Config: map[string]interface{}{"private_metadata": "owner-only"}, Status: integrations.ConnectionStatusActive,
			Revision: 1,
		},
		listItems: []integrations.ConnectionView{
			{
				ID: personalID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest",
				Name: personalRecord.Name, CredentialSource: integrations.ConnectionCredentialSourceAccount,
				OwnerAccountID: &ownerID, AccountID: &privateAccountID, DisplayName: &privateDisplayName,
				Config: map[string]interface{}{"private_metadata": "owner-only"}, Status: integrations.ConnectionStatusActive,
			},
			{
				ID: organizationConnectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest",
				Name: organizationRecord.Name, CredentialSource: integrations.ConnectionCredentialSourceOrganization,
				Status: integrations.ConnectionStatusActive,
			},
		},
	}
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	handler := &integrationHandler{deps: IntegrationRouteDeps{
		DB: db, Registry: newIntegrationRouteRegistry(t), Connections: service, ConnectionRepo: connectionRepo,
	}}
	newContext := func(method, path, body string, actorID uuid.UUID) (*gin.Context, *httptest.ResponseRecorder) {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "id", Value: personalID.String()}}
		util.SetOrganizationID(ctx, organizationID.String())
		ctx.Set("account_id", actorID.String())
		return ctx, recorder
	}

	t.Run("management list excludes account-owned metadata", func(t *testing.T) {
		ctx, recorder := newContext(http.MethodGet, "/connections", "", adminID)
		handler.listConnections(ctx)
		body := recorder.Body.String()
		if recorder.Code != http.StatusOK || !strings.Contains(body, organizationConnectionID.String()) || strings.Contains(body, personalID.String()) || strings.Contains(body, privateAccountID) || strings.Contains(body, "owner-only") {
			t.Fatalf("status=%d body=%s", recorder.Code, body)
		}
		if len(service.listFilter.CredentialSources) != 1 || service.listFilter.CredentialSources[0] != integrations.ConnectionCredentialSourceOrganization || service.listFilter.OwnerAccountID != nil {
			t.Fatalf("management filter = %#v", service.listFilter)
		}
	})

	for _, testCase := range []struct {
		name   string
		method string
		body   string
		call   func(*gin.Context)
	}{
		{name: "read", method: http.MethodGet, call: handler.getConnection},
		{name: "update credentials", method: http.MethodPatch, body: `{"revision":1,"credentials":{"token":"must-not-be-read"}}`, call: handler.updateConnection},
		{name: "test", method: http.MethodPost, call: handler.testConnection},
		{name: "set default", method: http.MethodPost, call: handler.setDefaultConnection},
		{name: "list grants", method: http.MethodGet, call: handler.listConnectionGrants},
		{name: "create grant", method: http.MethodPost, body: `{"principal_type":"organization","access_mode":"read","allowed_action_ids":["github.issue.list"]}`, call: handler.createConnectionGrant},
		{name: "update grant", method: http.MethodPut, body: `{"revision":1,"principal_type":"organization","access_mode":"read","allowed_action_ids":["github.issue.list"]}`, call: handler.updateConnectionGrant},
		{name: "delete grant", method: http.MethodDelete, call: handler.deleteConnectionGrant},
		{name: "read health events", method: http.MethodGet, call: handler.listConnectionHealthEvents},
		{name: "read delete impact", method: http.MethodGet, call: handler.connectionDeleteImpact},
		{name: "delete", method: http.MethodDelete, call: handler.deleteConnection},
	} {
		t.Run("management cannot "+testCase.name, func(t *testing.T) {
			beforeUpdates, beforeTests, beforeDeleted := service.updateCalls, service.testCalls, service.deleted
			ctx, recorder := newContext(testCase.method, "/connections/"+personalID.String(), testCase.body, adminID)
			testCase.call(ctx)
			body := recorder.Body.String()
			if recorder.Code != http.StatusNotFound || service.updateCalls != beforeUpdates || service.testCalls != beforeTests || service.deleted != beforeDeleted {
				t.Fatalf("status=%d updates=%d tests=%d deleted=%s body=%s", recorder.Code, service.updateCalls, service.testCalls, service.deleted, body)
			}
			for _, privateValue := range []string{personalRecord.Name, privateAccountID, privateDisplayName, "owner-only", "must-not-be-read"} {
				if strings.Contains(body, privateValue) {
					t.Fatalf("management error leaked %q: %s", privateValue, body)
				}
			}
		})
	}

	t.Run("management cannot create an account-owned connection", func(t *testing.T) {
		ctx, recorder := newContext(http.MethodPost, "/connections", `{"integration_id":"github","driver_id":"github-rest","name":"Admin-created personal","credential_source":"account","auth_type":"api_key","auth_method_id":"personal_access_token","credentials":{"token":"must-be-cleared"}}`, adminID)
		handler.createConnection(ctx)
		if recorder.Code != http.StatusForbidden || service.created.IntegrationID != "" || strings.Contains(recorder.Body.String(), "must-be-cleared") {
			t.Fatalf("status=%d created=%#v body=%s", recorder.Code, service.created, recorder.Body.String())
		}
	})

	t.Run("management cannot create a legacy platform connection", func(t *testing.T) {
		ctx, recorder := newContext(http.MethodPost, "/connections", `{"integration_id":"github","driver_id":"github-rest","name":"Platform","credential_source":"platform","auth_type":"platform","auth_method_id":"platform"}`, adminID)
		handler.createConnection(ctx)
		if recorder.Code != http.StatusBadRequest || service.created.IntegrationID != "" {
			t.Fatalf("status=%d created=%#v body=%s", recorder.Code, service.created, recorder.Body.String())
		}
	})

	t.Run("owner lists updates tests and deletes through my-connections", func(t *testing.T) {
		ctx, recorder := newContext(http.MethodGet, "/my-connections", "", ownerID)
		handler.listMyConnections(ctx)
		body := recorder.Body.String()
		if recorder.Code != http.StatusOK || !strings.Contains(body, personalID.String()) || strings.Contains(body, organizationConnectionID.String()) {
			t.Fatalf("list status=%d body=%s", recorder.Code, body)
		}
		if len(service.listFilter.CredentialSources) != 1 || service.listFilter.CredentialSources[0] != integrations.ConnectionCredentialSourceAccount || service.listFilter.OwnerAccountID == nil || *service.listFilter.OwnerAccountID != ownerID {
			t.Fatalf("personal filter = %#v", service.listFilter)
		}

		ctx, recorder = newContext(http.MethodPatch, "/my-connections/"+personalID.String(), `{"revision":1,"name":"Renamed","credentials":{"token":"owner-secret"}}`, ownerID)
		handler.updateMyConnection(ctx)
		if recorder.Code != http.StatusOK || service.updateCalls != 1 || service.updated.ConnectionID != personalID || len(service.updated.Credentials) != 0 {
			t.Fatalf("update status=%d input=%#v body=%s", recorder.Code, service.updated, recorder.Body.String())
		}

		ctx, recorder = newContext(http.MethodPost, "/my-connections/"+personalID.String()+"/test", "", ownerID)
		handler.testMyConnection(ctx)
		if recorder.Code != http.StatusOK || service.testCalls != 1 {
			t.Fatalf("test status=%d calls=%d body=%s", recorder.Code, service.testCalls, recorder.Body.String())
		}

		mock.ExpectQuery(`SELECT COUNT\(DISTINCT\("agent_id"\)\) FROM "agent_resource_bindings"`).
			WithArgs(organizationID, "integration_connection", personalID.String()).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		ctx, recorder = newContext(http.MethodDelete, "/my-connections/"+personalID.String(), "", ownerID)
		handler.deleteMyConnection(ctx)
		if recorder.Code != http.StatusOK || service.deleted != personalID {
			t.Fatalf("delete status=%d deleted=%s body=%s", recorder.Code, service.deleted, recorder.Body.String())
		}
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("SQL expectations: %v", err)
	}
}
