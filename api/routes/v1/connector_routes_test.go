package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	connectorcatalog "github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorconnections "github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/util"
)

const connectorRouteSecretSentinel = "connector-route-KEY-SECRET-SENTINEL"

func TestConnectorRoutesCatalogContractAndOwnerScope(t *testing.T) {
	definitionCatalog := newConnectorRouteTestCatalog(t)
	organizationID, ownerID, otherOwnerID := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, time.September, 1, 10, 0, 0, 0, time.UTC)
	ownerDisplay, otherDisplay := "octocat", "must-not-disclose-other-owner"
	ownerConnection := connectorcore.PersonalConnection{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: "github", ProviderID: "github", DriverID: "github-rest", AuthMethodID: "personal_access_token",
		Name: "octocat", ExternalDisplayName: &ownerDisplay,
		EncryptedCredentials: connectorRouteSecretSentinel, CredentialVersion: 3, State: connectorcore.ReadinessReady,
		UsableOperationIDs: []connectorcore.Identifier{"github.list_repositories"}, UnavailableOperations: map[connectorcore.Identifier]string{},
		Revision: 2, LastVerifiedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	otherConnection := ownerConnection
	otherConnection.ID = uuid.New()
	otherConnection.OwnerAccountID = otherOwnerID
	otherConnection.Name = otherDisplay
	otherConnection.ExternalDisplayName = &otherDisplay
	secondConnection := ownerConnection
	secondConnection.ID = uuid.New()
	secondConnection.Name = "octocat read-only"
	externalAccountID := "583231"
	ownerConnection.ExternalAccountID = &externalAccountID
	secondConnection.ExternalAccountID = &externalAccountID
	service := &connectorRouteRecordingService{connections: []connectorcore.PersonalConnection{ownerConnection, secondConnection, otherConnection}}
	router := newConnectorRouteTestRouter(definitionCatalog, service, organizationID, ownerID)

	recorder := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connectors/catalog?classification=connector&status=connected", "", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("catalog status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, expected := range []string{`"id":"github"`, `"plugin_id":"zgi.github"`, `"classification":"connector"`, ownerConnection.ID.String()} {
		if !strings.Contains(body, expected) {
			t.Fatalf("catalog body missing %q: %s", expected, body)
		}
	}
	if !strings.Contains(body, `"category":"developer"`) || !strings.Contains(body, `"tags":["code","repository","issues","collaboration"]`) {
		t.Fatalf("catalog presentation metadata missing: %s", body)
	}
	for _, privateField := range []string{"source_urls", "publisher", "reviewed_at", "rationale"} {
		if strings.Contains(body, `"`+privateField+`"`) {
			t.Fatalf("catalog leaked private presentation evidence %q: %s", privateField, body)
		}
	}
	if strings.Contains(body, `"id":"reference-data"`) || strings.Contains(body, otherDisplay) {
		t.Fatalf("catalog classification/owner filter leaked data: %s", body)
	}
	assertConnectorRouteBodyIsSecretFree(t, body)
	assertConnectorRouteLastScope(t, service, "list", organizationID, ownerID)
	var catalogEnvelope struct {
		Data connectorCatalogResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &catalogEnvelope); err != nil {
		t.Fatal(err)
	}
	if len(catalogEnvelope.Data.Items) != 1 {
		t.Fatalf("catalog items = %#v", catalogEnvelope.Data.Items)
	}
	item := catalogEnvelope.Data.Items[0]
	if item.ConnectionSummary.Count != 2 || item.Name != "GitHub" || item.NameI18n["zh-Hans"] != "GitHub" ||
		item.DescriptionI18n["zh-Hans"] != "GitHub 仓库和议题" ||
		item.KeyFields[0].LabelI18n["zh-Hans"] != "个人访问令牌" || item.KeyFields[0].PlaceholderI18n["zh-Hans"] != "请输入访问令牌" ||
		item.Capabilities[0].NameI18n["zh-Hans"] != "列出仓库" || item.Capabilities[0].DescriptionI18n["zh-Hans"] != "列出可用仓库" {
		t.Fatalf("catalog localized metadata or multi-connection summary = %#v", item)
	}

	detail := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connectors/catalog/github", "", nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), ownerConnection.ID.String()) {
		t.Fatalf("detail status/body = %d %s", detail.Code, detail.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, detail.Body.String())
	if got := service.calls[len(service.calls)-1].filter.ConnectorID; got != "github" {
		t.Fatalf("detail connector filter = %q", got)
	}

	listed := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-connections?connector_id=github", "", nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("connection list = %d %s", listed.Code, listed.Body.String())
	}
	var connectionEnvelope struct {
		Data connectorPersonalConnectionListResponse `json:"data"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &connectionEnvelope); err != nil {
		t.Fatal(err)
	}
	if len(connectionEnvelope.Data.Items) != 2 || connectionEnvelope.Data.Items[0].ID != ownerConnection.ID ||
		connectionEnvelope.Data.Items[1].ID != secondConnection.ID || strings.Contains(listed.Body.String(), otherDisplay) {
		t.Fatalf("same-account connections were collapsed or leaked: %s", listed.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, listed.Body.String())
}

func TestConnectorRoutesCatalogCountsEveryConnectionNeedingAttention(t *testing.T) {
	definitionCatalog := newConnectorRouteTestCatalog(t)
	organizationID, ownerID := uuid.New(), uuid.New()
	readyID := uuid.New()
	readyDisplay := "primary-ready-account"
	ready := connectorcore.PersonalConnection{
		ID: readyID, OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: "github", State: connectorcore.ReadinessReady,
		ExternalDisplayName: &readyDisplay, EncryptedCredentials: connectorRouteSecretSentinel,
		UsableOperationIDs: []connectorcore.Identifier{"github.list_repositories"},
	}
	reconnect := ready
	reconnect.ID, reconnect.State = uuid.New(), connectorcore.ReadinessReconnectRequired
	unavailable := ready
	unavailable.ID, unavailable.State = uuid.New(), connectorcore.ReadinessUnavailable
	otherOwner := unavailable
	otherOwner.ID, otherOwner.OwnerAccountID = uuid.New(), uuid.New()

	testCases := []struct {
		name        string
		connections []connectorcore.PersonalConnection
		wantCount   int
		wantReady   bool
	}{
		{name: "ready-reconnect-unavailable", connections: []connectorcore.PersonalConnection{ready, reconnect, unavailable}, wantCount: 2, wantReady: true},
		{name: "ready-unavailable-reconnect", connections: []connectorcore.PersonalConnection{ready, unavailable, reconnect}, wantCount: 2, wantReady: true},
		{name: "reconnect-ready-unavailable", connections: []connectorcore.PersonalConnection{reconnect, ready, unavailable}, wantCount: 2, wantReady: true},
		{name: "reconnect-unavailable-ready", connections: []connectorcore.PersonalConnection{reconnect, unavailable, ready}, wantCount: 2, wantReady: true},
		{name: "unavailable-ready-reconnect", connections: []connectorcore.PersonalConnection{unavailable, ready, reconnect}, wantCount: 2, wantReady: true},
		{name: "unavailable-reconnect-ready", connections: []connectorcore.PersonalConnection{unavailable, reconnect, ready}, wantCount: 2, wantReady: true},
		{name: "only-needing-attention", connections: []connectorcore.PersonalConnection{unavailable, reconnect}, wantCount: 2},
		{name: "all-ready", connections: []connectorcore.PersonalConnection{ready}, wantReady: true},
		{name: "no-connections"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			connections := append([]connectorcore.PersonalConnection{}, testCase.connections...)
			service := &connectorRouteRecordingService{connections: append(connections, otherOwner)}
			router := newConnectorRouteTestRouter(definitionCatalog, service, organizationID, ownerID)
			type catalogItem struct {
				ID                string `json:"id"`
				ConnectionSummary struct {
					Count               int                     `json:"count"`
					NeedsAttentionCount int                     `json:"needs_attention_count"`
					PrimaryConnectionID *uuid.UUID              `json:"primary_connection_id"`
					State               connectorcore.Readiness `json:"state"`
					ExternalDisplayName string                  `json:"external_display_name"`
				} `json:"connection_summary"`
			}
			assertSummary := func(t *testing.T, item catalogItem) {
				t.Helper()
				summary := item.ConnectionSummary
				if item.ID != "github" || summary.Count != len(testCase.connections) ||
					summary.NeedsAttentionCount != testCase.wantCount || summary.NeedsAttentionCount < 0 {
					t.Fatalf("catalog summary = %#v, want total=%d attention=%d", item, len(testCase.connections), testCase.wantCount)
				}
				if testCase.wantReady && (summary.PrimaryConnectionID == nil || *summary.PrimaryConnectionID != readyID ||
					summary.State != connectorcore.ReadinessReady || summary.ExternalDisplayName != readyDisplay) {
					t.Fatalf("ready primary compatibility changed: %#v", summary)
				}
			}
			for _, status := range []string{"all", "connected", "needs_attention"} {
				t.Run(status, func(t *testing.T) {
					recorder := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connectors/catalog?classification=connector&status="+status, "", nil)
					if recorder.Code != http.StatusOK {
						t.Fatalf("catalog status/body = %d %s", recorder.Code, recorder.Body.String())
					}
					assertConnectorRouteBodyIsSecretFree(t, recorder.Body.String())
					var envelope struct {
						Data struct {
							Items []catalogItem `json:"items"`
						} `json:"data"`
					}
					if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
						t.Fatal(err)
					}
					wantItem := status == "all" || (status == "connected" && testCase.wantReady) ||
						(status == "needs_attention" && testCase.wantCount > 0)
					if !wantItem {
						if len(envelope.Data.Items) != 0 {
							t.Fatalf("unexpected %s match: %#v", status, envelope.Data.Items)
						}
						return
					}
					if len(envelope.Data.Items) != 1 {
						t.Fatalf("catalog %s items = %#v, want GitHub", status, envelope.Data.Items)
					}
					assertSummary(t, envelope.Data.Items[0])
				})
			}
			t.Run("detail", func(t *testing.T) {
				recorder := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connectors/catalog/github", "", nil)
				if recorder.Code != http.StatusOK {
					t.Fatalf("catalog detail status/body = %d %s", recorder.Code, recorder.Body.String())
				}
				assertConnectorRouteBodyIsSecretFree(t, recorder.Body.String())
				var envelope struct {
					Data catalogItem `json:"data"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				assertSummary(t, envelope.Data)
			})
		})
	}
}

func TestConnectorRoutesBuiltInExaIsConnectorWithoutChangingStableIdentity(t *testing.T) {
	definitions, err := connectorcatalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	definitionCatalog, err := connectorcatalog.New(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	router := newConnectorRouteTestRouter(definitionCatalog, &connectorRouteRecordingService{}, uuid.New(), uuid.New())
	for _, classification := range []string{"connector", "data_source"} {
		t.Run(classification, func(t *testing.T) {
			recorder := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connectors/catalog?classification="+classification, "", nil)
			if recorder.Code != http.StatusOK {
				t.Fatalf("catalog status/body = %d %s", recorder.Code, recorder.Body.String())
			}
			var envelope struct {
				Data connectorCatalogResponse `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range envelope.Data.Items {
				if item.ID == "web-search" {
					found = true
					if item.PluginID != "zgi.exa-search" || item.Classification != connectorcatalog.ClassificationConnector ||
						len(item.Capabilities) != 2 || item.Capabilities[0].ID != "web.search" || item.Capabilities[1].ID != "web.fetch" {
						t.Fatalf("Exa HTTP identity changed: %#v", item)
					}
				}
			}
			if found != (classification == "connector") {
				t.Fatalf("Exa presence under classification %s = %v", classification, found)
			}
			assertConnectorRouteBodyIsSecretFree(t, recorder.Body.String())
		})
	}
	detail := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connectors/catalog/web-search", "", nil)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), `"classification":"connector"`) {
		t.Fatalf("Exa detail classification = %d %s", detail.Code, detail.Body.String())
	}
}

func TestConnectorRoutesCatalogTaxonomyFilters(t *testing.T) {
	definitions, err := connectorcatalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	definitionCatalog, err := connectorcatalog.New(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	router := newConnectorRouteTestRouter(definitionCatalog, &connectorRouteRecordingService{}, uuid.New(), uuid.New())
	for _, test := range []struct {
		query, want string
	}{
		{"category=information", "web-search"},
		{"tag=code", "github"},
	} {
		recorder := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connectors/catalog?"+test.query, "", nil)
		if recorder.Code != http.StatusOK {
			t.Fatalf("taxonomy filter %s status/body = %d %s", test.query, recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), `"id":"`+test.want+`"`) {
			t.Fatalf("taxonomy filter %s missing %s: %s", test.query, test.want, recorder.Body.String())
		}
		var envelope struct {
			Data connectorCatalogResponse `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if len(envelope.Data.Items) != 1 || envelope.Data.Items[0].ID != test.want {
			t.Fatalf("taxonomy filter %s = %#v", test.query, envelope.Data.Items)
		}
	}
	bad := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connectors/catalog?category=unknown", "", nil)
	if bad.Code < 400 || bad.Code >= 500 {
		t.Fatalf("unknown taxonomy filter status = %d", bad.Code)
	}
}

func TestConnectorRoutesKeySetupContractDestroysSecretAndUsesActor(t *testing.T) {
	definitionCatalog := newConnectorRouteTestCatalog(t)
	organizationID, ownerID := uuid.New(), uuid.New()
	now := time.Date(2026, time.September, 1, 10, 30, 0, 0, time.UTC)
	connection := connectorcore.PersonalConnection{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: "github", ProviderID: "github", DriverID: "github-rest", AuthMethodID: "personal_access_token",
		Name: "octocat", EncryptedCredentials: "sealed-" + connectorRouteSecretSentinel,
		CredentialVersion: 1, State: connectorcore.ReadinessReady,
		UsableOperationIDs:    []connectorcore.Identifier{"github.list_repositories"},
		UnavailableOperations: map[connectorcore.Identifier]string{"github.create_issue": connectorRouteSecretSentinel},
		Revision:              1, LastVerifiedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	attempt := connectorcore.KeySetupAttempt{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectorID: "github",
		Target:             &connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: "agent-1"},
		IdempotencyKeyHash: strings.Repeat("a", 64), RequestFingerprint: strings.Repeat("b", 64),
		State: connectorcore.SetupStateReady, ResultConnectionID: connection.ID, ExpiresAt: now.Add(time.Minute),
	}
	service := &connectorRouteRecordingService{
		startResult:    connectorconnections.KeySetupResult{Attempt: attempt, Connection: &connection},
		getSetupResult: connectorconnections.KeySetupResult{Attempt: attempt, Connection: &connection},
	}
	binding := connectorcore.TargetBinding{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectionID: connection.ID, ConnectorID: "github", Target: *attempt.Target,
		Status:   connectorcore.BindingStatusActive,
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	bindingService := &connectorRouteRecordingBindingService{
		upsertResult: connectorbindings.UpsertBindingResult{Binding: binding, Created: true},
	}
	router := newConnectorRouteTestRouter(definitionCatalog, service, organizationID, ownerID, bindingService)
	body := `{"values":{"token":"` + connectorRouteSecretSentinel + `"},"target":{"type":"agent","id":"agent-1"}}`
	recorder := connectorRouteRequest(t, router, http.MethodPost, "/console/api/connectors/github/key-setups", body, map[string]string{
		"Idempotency-Key": "connector-route-idempotency-0001",
	})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("setup status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	responseBody := recorder.Body.String()
	for _, expected := range []string{attempt.ID.String(), connection.ID.String(), binding.ID.String(), `"state":"ready"`, `"credential_configured":true`} {
		if !strings.Contains(responseBody, expected) {
			t.Fatalf("setup body missing %q: %s", expected, responseBody)
		}
	}
	assertConnectorRouteBodyIsSecretFree(t, responseBody)
	assertConnectorRouteLastScope(t, service, "start", organizationID, ownerID)
	if service.lastCommand.IdempotencyKey != "connector-route-idempotency-0001" || service.lastCommand.Target == nil || service.lastCommand.Target.Type != connectorcore.TargetTypeAgent || service.lastCommand.Target.ID != "agent-1" {
		t.Fatalf("setup command = %#v", service.lastCommand)
	}
	if len(service.capturedValues) != 0 {
		t.Fatalf("route retained candidate values after service returned: %#v", service.capturedValues)
	}
	if len(bindingService.upsertCalls) != 1 {
		t.Fatalf("contextual setup binding calls = %#v", bindingService.upsertCalls)
	}
	attachmentCommand := bindingService.upsertCalls[0]
	if attachmentCommand.OrganizationID != organizationID || attachmentCommand.ActingPrincipalID != ownerID ||
		attachmentCommand.ConnectionID != connection.ID || attachmentCommand.Target != *attempt.Target ||
		attachmentCommand.IdempotencyKey != "key-setup:"+attempt.ID.String() || attachmentCommand.ExpectedRevision != 0 {
		t.Fatalf("contextual setup binding command = %#v", attachmentCommand)
	}

	// Polling a completed attempt replays the same deterministic attachment
	// command and returns the same stable Binding UUID.
	poll := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connectors/key-setups/"+attempt.ID.String(), "", nil)
	if poll.Code != http.StatusOK || !strings.Contains(poll.Body.String(), binding.ID.String()) || len(bindingService.upsertCalls) != 2 {
		t.Fatalf("ready poll status/body/calls = %d %s / %#v", poll.Code, poll.Body.String(), bindingService.upsertCalls)
	}
	if bindingService.upsertCalls[1].IdempotencyKey != attachmentCommand.IdempotencyKey {
		t.Fatalf("poll attachment idempotency = %q, want %q", bindingService.upsertCalls[1].IdempotencyKey, attachmentCommand.IdempotencyKey)
	}

	// Losing edit authority does not invalidate a verified Personal Connection.
	// The response remains ready and exposes only a stable warning code.
	bindingService.upsertErr = connectorcore.NewError(connectorcore.ErrorCodeAccessDenied, false, nil)
	warning := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connectors/key-setups/"+attempt.ID.String(), "", nil)
	if warning.Code != http.StatusOK || !strings.Contains(warning.Body.String(), `"state":"ready"`) ||
		!strings.Contains(warning.Body.String(), connection.ID.String()) ||
		!strings.Contains(warning.Body.String(), `"binding_warning":"connector_access_denied"`) ||
		strings.Contains(warning.Body.String(), `"attachment"`) {
		t.Fatalf("binding warning status/body = %d %s", warning.Code, warning.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, warning.Body.String())

	// Creating another connection never supplies replacement authority for an
	// already occupied target slot. A conflict preserves the verified result.
	bindingService.upsertErr = connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	conflict := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connectors/key-setups/"+attempt.ID.String(), "", nil)
	if conflict.Code != http.StatusOK || !strings.Contains(conflict.Body.String(), connection.ID.String()) ||
		!strings.Contains(conflict.Body.String(), `"binding_warning":"connector_conflict"`) || strings.Contains(conflict.Body.String(), `"attachment"`) ||
		bindingService.upsertCalls[len(bindingService.upsertCalls)-1].ExpectedRevision != 0 {
		t.Fatalf("occupied-slot setup response = %d %s", conflict.Code, conflict.Body.String())
	}

	unknownFieldBody := `{"values":{"token":"` + connectorRouteSecretSentinel + `"},"credential_source":"organization"}`
	invalid := connectorRouteRequest(t, router, http.MethodPost, "/console/api/connectors/github/key-setups", unknownFieldBody, map[string]string{
		"Idempotency-Key": "connector-route-idempotency-0002",
	})
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), string(connectorcore.ErrorCodeInvalidInput)) {
		t.Fatalf("strict request status/body = %d %s", invalid.Code, invalid.Body.String())
	}
	startCalls := 0
	for _, call := range service.calls {
		if call.operation == "start" {
			startCalls++
		}
	}
	if startCalls != 1 {
		t.Fatalf("invalid request reached setup service; calls=%#v", service.calls)
	}
	assertConnectorRouteBodyIsSecretFree(t, invalid.Body.String())
}

func TestConnectorRoutesSetupAndConnectionReadsPassOrganizationAndOwner(t *testing.T) {
	definitionCatalog := newConnectorRouteTestCatalog(t)
	organizationID, ownerID, otherOwnerID := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, time.September, 1, 11, 0, 0, 0, time.UTC)
	attempt := connectorcore.KeySetupAttempt{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectorID: "github",
		IdempotencyKeyHash: strings.Repeat("c", 64), RequestFingerprint: strings.Repeat("d", 64),
		State: connectorcore.SetupStateVerifying, ExpiresAt: now.Add(time.Minute),
	}
	ownerConnection := connectorcore.PersonalConnection{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: "github", Name: "mine", State: connectorcore.ReadinessReady,
		CredentialVersion: 1, Revision: 1, LastVerifiedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	otherConnection := ownerConnection
	otherConnection.ID = uuid.New()
	otherConnection.OwnerAccountID = otherOwnerID
	otherConnection.Name = "other-owner-private-name"
	service := &connectorRouteRecordingService{
		attempt: attempt, connections: []connectorcore.PersonalConnection{ownerConnection, otherConnection},
	}
	binding := connectorcore.TargetBinding{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectionID: ownerConnection.ID, ConnectorID: "github",
		Target:   connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: "agent-1"},
		Status:   connectorcore.BindingStatusActive,
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	bindingService := &connectorRouteRecordingBindingService{
		impactResult: connectorbindings.RemovalImpact{
			ConnectionID: ownerConnection.ID, CanRemove: false,
			BlockingBindingCount: 1, AffectedBindings: []connectorcore.TargetBinding{binding},
		},
	}
	router := newConnectorRouteTestRouter(definitionCatalog, service, organizationID, ownerID, bindingService)

	poll := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connectors/key-setups/"+attempt.ID.String(), "", nil)
	if poll.Code != http.StatusOK || !strings.Contains(poll.Body.String(), `"state":"verifying"`) {
		t.Fatalf("poll status/body = %d %s", poll.Code, poll.Body.String())
	}
	assertConnectorRouteLastScope(t, service, "get_setup", organizationID, ownerID)

	cancel := connectorRouteRequest(t, router, http.MethodDelete, "/console/api/connectors/key-setups/"+attempt.ID.String(), "", nil)
	if cancel.Code != http.StatusNoContent || cancel.Body.Len() != 0 {
		t.Fatalf("cancel status/body = %d %q", cancel.Code, cancel.Body.String())
	}
	assertConnectorRouteLastScope(t, service, "cancel_setup", organizationID, ownerID)

	list := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-connections?connector_id=github&target_type=agent&target_id=agent-1", "", nil)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), ownerConnection.ID.String()) || strings.Contains(list.Body.String(), otherConnection.ID.String()) || strings.Contains(list.Body.String(), otherConnection.Name) {
		t.Fatalf("owner list status/body = %d %s", list.Code, list.Body.String())
	}
	assertConnectorRouteLastScope(t, service, "list", organizationID, ownerID)
	assertConnectorRouteBodyIsSecretFree(t, list.Body.String())

	otherDetail := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-connections/"+otherConnection.ID.String(), "", nil)
	if otherDetail.Code != http.StatusNotFound || !strings.Contains(otherDetail.Body.String(), string(connectorcore.ErrorCodeNotFound)) || strings.Contains(otherDetail.Body.String(), otherConnection.Name) {
		t.Fatalf("cross-owner detail status/body = %d %s", otherDetail.Code, otherDetail.Body.String())
	}
	assertConnectorRouteLastScope(t, service, "get_connection", organizationID, ownerID)

	ownerDetail := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-connections/"+ownerConnection.ID.String(), "", nil)
	if ownerDetail.Code != http.StatusOK || !strings.Contains(ownerDetail.Body.String(), `"capabilities"`) || !strings.Contains(ownerDetail.Body.String(), binding.ID.String()) {
		t.Fatalf("owner detail contract status/body = %d %s", ownerDetail.Code, ownerDetail.Body.String())
	}
	if len(bindingService.impactCalls) != 1 || bindingService.impactCalls[0] != (connectorRouteImpactCall{organizationID: organizationID, actingPrincipalID: ownerID, connectionID: ownerConnection.ID}) {
		t.Fatalf("connection detail impact scope = %#v", bindingService.impactCalls)
	}
	assertConnectorRouteBodyIsSecretFree(t, ownerDetail.Body.String())
}

func TestConnectorTargetBindingRoutesUseActorScopeAndStrictContract(t *testing.T) {
	definitionCatalog := newConnectorRouteTestCatalog(t)
	organizationID, ownerID := uuid.New(), uuid.New()
	connectionID := uuid.New()
	now := time.Date(2026, time.September, 1, 11, 30, 0, 0, time.UTC)
	native := connectorcore.TargetBinding{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectionID: connectionID, ConnectorID: "github",
		Target:   connectorcore.TargetRef{Type: connectorcore.TargetTypeWorkflow, ID: "workflow-1", BindingInstanceID: "node-1"},
		Status:   connectorcore.BindingStatusActive,
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	unavailable := native
	unavailable.ID = uuid.New()
	unavailable.Target.BindingInstanceID = "node-unavailable"
	unavailable.Status = connectorcore.BindingStatusNeedsReplacement
	unavailable.Constraint = connectorcore.OwnerConstraint(ownerID, []connectorcore.Identifier{})
	unavailable.Constraint.SourceDigest = connectorRouteSecretSentinel
	bindingService := &connectorRouteRecordingBindingService{
		listResult: []connectorcore.TargetBinding{native, unavailable},
		upsertResult: connectorbindings.UpsertBindingResult{
			Binding: native,
			Created: true,
		},
	}
	router := newConnectorRouteTestRouter(definitionCatalog, &connectorRouteRecordingService{}, organizationID, ownerID, bindingService)

	list := connectorRouteRequest(
		t, router, http.MethodGet,
		"/console/api/connector-target-bindings?target_type=workflow&target_id=workflow-1", "", nil,
	)
	if list.Code != http.StatusOK {
		t.Fatalf("binding list status/body = %d %s", list.Code, list.Body.String())
	}
	for _, expected := range []string{
		native.ID.String(), unavailable.ID.String(), `"binding_instance_id":"node-1"`,
		`"scope":"draft"`,
	} {
		if !strings.Contains(list.Body.String(), expected) {
			t.Fatalf("binding list missing %q: %s", expected, list.Body.String())
		}
	}
	assertConnectorRouteBodyIsSecretFree(t, list.Body.String())
	if len(bindingService.listCalls) != 1 {
		t.Fatalf("binding list calls = %#v", bindingService.listCalls)
	}
	listCall := bindingService.listCalls[0]
	if listCall.organizationID != organizationID || listCall.actingPrincipalID != ownerID || listCall.target != (connectorbindings.TargetIdentity{Type: connectorcore.TargetTypeWorkflow, ID: "workflow-1"}) {
		t.Fatalf("binding list call = %#v", listCall)
	}

	createBody := `{"connection_id":"` + connectionID.String() + `","target":{"type":"workflow","id":"workflow-1","binding_instance_id":"node-1"}}`
	created := connectorRouteRequest(
		t, router, http.MethodPost, "/console/api/connector-target-bindings", createBody,
		map[string]string{"Idempotency-Key": "binding-route-idempotency-0001"},
	)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), native.ID.String()) || !strings.Contains(created.Body.String(), `"status":"active"`) {
		t.Fatalf("binding create status/body = %d %s", created.Code, created.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, created.Body.String())
	if len(bindingService.upsertCalls) != 1 {
		t.Fatalf("binding upsert calls = %#v", bindingService.upsertCalls)
	}
	command := bindingService.upsertCalls[0]
	if command.OrganizationID != organizationID || command.ActingPrincipalID != ownerID || command.ConnectionID != connectionID ||
		command.IdempotencyKey != "binding-route-idempotency-0001" || command.Target != native.Target || command.ExpectedRevision != 0 {
		t.Fatalf("binding upsert command = %#v", command)
	}

	detached := native
	detached.Status = connectorcore.BindingStatusDisabled
	detached.Revision = native.Revision + 1
	bindingService.detachResult = detached
	detachResponse := connectorRouteRequest(
		t, router, http.MethodDelete, "/console/api/connector-target-bindings/"+native.ID.String(),
		`{"revision":1}`, nil,
	)
	if detachResponse.Code != http.StatusOK || !strings.Contains(detachResponse.Body.String(), `"status":"disabled"`) ||
		!strings.Contains(detachResponse.Body.String(), `"revision":2`) {
		t.Fatalf("binding detach status/body = %d %s", detachResponse.Code, detachResponse.Body.String())
	}
	if len(bindingService.detachCalls) != 1 {
		t.Fatalf("binding detach calls = %#v", bindingService.detachCalls)
	}
	detachCommand := bindingService.detachCalls[0]
	if detachCommand.OrganizationID != organizationID || detachCommand.ActingPrincipalID != ownerID ||
		detachCommand.BindingID != native.ID || detachCommand.ExpectedRevision != native.Revision {
		t.Fatalf("binding detach command = %#v", detachCommand)
	}
	assertConnectorRouteBodyIsSecretFree(t, detachResponse.Body.String())

	invalid := connectorRouteRequest(
		t, router, http.MethodPost, "/console/api/connector-target-bindings",
		`{"connection_id":"`+connectionID.String()+`","target":{"type":"agent","id":"agent-1"},"grants":[]}`,
		map[string]string{"Idempotency-Key": "binding-route-idempotency-0002"},
	)
	if invalid.Code != http.StatusBadRequest || len(bindingService.upsertCalls) != 1 {
		t.Fatalf("strict binding request status/calls = %d/%d body=%s", invalid.Code, len(bindingService.upsertCalls), invalid.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, invalid.Body.String())

	missingTarget := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-target-bindings?target_type=agent", "", nil)
	if missingTarget.Code != http.StatusBadRequest || len(bindingService.listCalls) != 1 {
		t.Fatalf("missing target status/calls = %d/%d body=%s", missingTarget.Code, len(bindingService.listCalls), missingTarget.Body.String())
	}
}

func TestConnectorTargetBindingRoutesProjectAuthorityDenial(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	bindingService := &connectorRouteRecordingBindingService{
		listErr: connectorcore.NewError(connectorcore.ErrorCodeAccessDenied, false, nil),
	}
	router := newConnectorRouteTestRouter(newConnectorRouteTestCatalog(t), &connectorRouteRecordingService{}, organizationID, ownerID, bindingService)
	recorder := connectorRouteRequest(
		t, router, http.MethodGet,
		"/console/api/connector-target-bindings?target_type=chat&target_id=chat-1", "", nil,
	)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), string(connectorcore.ErrorCodeAccessDenied)) {
		t.Fatalf("authority denial status/body = %d %s", recorder.Code, recorder.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, recorder.Body.String())
}

func TestConnectorConnectionTargetFilterRequiresCompleteEditableTarget(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	service := &connectorRouteRecordingService{}
	bindingService := &connectorRouteRecordingBindingService{
		listErr: connectorcore.NewError(connectorcore.ErrorCodeAccessDenied, false, nil),
	}
	router := newConnectorRouteTestRouter(newConnectorRouteTestCatalog(t), service, organizationID, ownerID, bindingService)

	for _, path := range []string{
		"/console/api/connector-connections?target_type=agent",
		"/console/api/connector-connections?target_id=agent-1",
	} {
		recorder := connectorRouteRequest(t, router, http.MethodGet, path, "", nil)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("incomplete target %q status/body = %d %s", path, recorder.Code, recorder.Body.String())
		}
	}
	if len(service.calls) != 0 || len(bindingService.listCalls) != 0 {
		t.Fatalf("incomplete target reached dependencies: connections=%#v bindings=%#v", service.calls, bindingService.listCalls)
	}

	denied := connectorRouteRequest(
		t, router, http.MethodGet,
		"/console/api/connector-connections?target_type=agent&target_id=agent-1", "", nil,
	)
	if denied.Code != http.StatusForbidden || len(bindingService.listCalls) != 1 || len(service.calls) != 0 {
		t.Fatalf("denied target status/calls = %d / %#v / %#v body=%s", denied.Code, bindingService.listCalls, service.calls, denied.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, denied.Body.String())
}

func TestConnectorConnectionMaintenanceRoutesUseOwnerScopeAndRemainSecretFree(t *testing.T) {
	catalog := newConnectorRouteTestCatalog(t)
	organizationID, ownerID, connectionID := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, time.September, 1, 14, 0, 0, 0, time.UTC)
	connection := connectorcore.PersonalConnection{
		ID: connectionID, OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: "github", ProviderID: "github", DriverID: "github-rest", AuthMethodID: "personal_access_token",
		Name: "Renamed account", EncryptedCredentials: "sealed-" + connectorRouteSecretSentinel,
		CredentialVersion: 2, State: connectorcore.ReadinessReady,
		UsableOperationIDs: []connectorcore.Identifier{"github.list_repositories"},
		Revision:           4, LastVerifiedAt: now, CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
	}
	binding := connectorcore.TargetBinding{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectionID: connectionID, ConnectorID: "github",
		Target:   connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: "agent-1"},
		Status:   connectorcore.BindingStatusActive,
		Revision: 2, CreatedAt: now, UpdatedAt: now,
	}
	attempt := connectorcore.KeySetupAttempt{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: "github", ConnectionID: connectionID, ResultConnectionID: connectionID,
		State: connectorcore.SetupStateReady, ExpiresAt: now.Add(time.Minute),
	}
	maintenance := &connectorRouteRecordingMaintenanceService{
		renameResult: connection,
		reconnectResult: connectorconnections.ReconnectResult{
			KeySetupResult:   connectorconnections.KeySetupResult{Attempt: attempt, Connection: &connection},
			ReplacedExisting: true,
		},
		diagnoseResult: connection,
		impactResult: connectorconnections.ConnectionRemovalImpact{
			ConnectionID: connectionID, CanRemove: false, ImpactToken: "signed-impact-token",
			ExpiresAt: now.Add(10 * time.Minute), AffectedBindings: []connectorcore.TargetBinding{binding},
		},
	}
	connections := &connectorRouteRecordingService{connections: []connectorcore.PersonalConnection{connection}}
	bindings := &connectorRouteRecordingBindingService{impactResult: connectorbindings.RemovalImpact{
		ConnectionID: connectionID, CanRemove: false, BlockingBindingCount: 1,
		AffectedBindings: []connectorcore.TargetBinding{binding},
	}}
	router := newConnectorRouteTestRouterWithMaintenance(catalog, connections, maintenance, organizationID, ownerID, bindings)

	rename := connectorRouteRequest(t, router, http.MethodPatch, "/console/api/connector-connections/"+connectionID.String(),
		`{"revision":3,"name":"Renamed account"}`, nil)
	if rename.Code != http.StatusOK || !strings.Contains(rename.Body.String(), `"name":"Renamed account"`) {
		t.Fatalf("rename status/body = %d %s", rename.Code, rename.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, rename.Body.String())

	reconnect := connectorRouteRequest(t, router, http.MethodPost, "/console/api/connector-connections/"+connectionID.String()+"/reconnect",
		`{"values":{"token":"`+connectorRouteSecretSentinel+`"}}`, map[string]string{
			"Idempotency-Key": "connector-reconnect-idempotency-0001",
		})
	if reconnect.Code != http.StatusOK || !strings.Contains(reconnect.Body.String(), `"state":"ready"`) {
		t.Fatalf("reconnect status/body = %d %s", reconnect.Code, reconnect.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, reconnect.Body.String())
	if len(maintenance.capturedValues) != 0 {
		t.Fatalf("reconnect route retained candidate values: %#v", maintenance.capturedValues)
	}

	diagnostics := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-connections/"+connectionID.String()+"/diagnostics", "", nil)
	if diagnostics.Code != http.StatusOK || !strings.Contains(diagnostics.Body.String(), `"capabilities"`) || !strings.Contains(diagnostics.Body.String(), binding.ID.String()) {
		t.Fatalf("diagnostics status/body = %d %s", diagnostics.Code, diagnostics.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, diagnostics.Body.String())

	impact := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-connections/"+connectionID.String()+"/removal-impact", "", nil)
	if impact.Code != http.StatusOK || !strings.Contains(impact.Body.String(), `"impact_token":"signed-impact-token"`) ||
		!strings.Contains(impact.Body.String(), binding.ID.String()) || !strings.Contains(impact.Body.String(), `"can_remove":false`) {
		t.Fatalf("impact status/body = %d %s", impact.Code, impact.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, impact.Body.String())

	remove := connectorRouteRequest(t, router, http.MethodDelete, "/console/api/connector-connections/"+connectionID.String()+"?force=true", "", map[string]string{
		"X-Impact-Token": "signed-impact-token",
	})
	if remove.Code != http.StatusNoContent || remove.Body.Len() != 0 {
		t.Fatalf("remove status/body = %d %q", remove.Code, remove.Body.String())
	}

	wantOperations := []string{"rename", "reconnect", "diagnose", "impact", "remove"}
	if len(maintenance.calls) != len(wantOperations) {
		t.Fatalf("maintenance calls = %#v", maintenance.calls)
	}
	for index, operation := range wantOperations {
		call := maintenance.calls[index]
		if call.operation != operation || call.organizationID != organizationID || call.ownerID != ownerID || call.connectionID != connectionID {
			t.Fatalf("maintenance call %d = %#v", index, call)
		}
	}
	if maintenance.calls[0].revision != 3 || maintenance.calls[0].name != "Renamed account" {
		t.Fatalf("rename call = %#v", maintenance.calls[0])
	}
	if maintenance.calls[1].idempotencyKey != "connector-reconnect-idempotency-0001" {
		t.Fatalf("reconnect call = %#v", maintenance.calls[1])
	}
	if !maintenance.calls[4].force || maintenance.calls[4].impactToken != "signed-impact-token" {
		t.Fatalf("remove call = %#v", maintenance.calls[4])
	}
}

func TestConnectorRouteStableNonSecretErrorProjection(t *testing.T) {
	definitionCatalog := newConnectorRouteTestCatalog(t)
	organizationID, ownerID := uuid.New(), uuid.New()
	connectionID := uuid.New()
	testCases := []struct {
		name       string
		err        error
		code       connectorcore.ErrorCode
		httpStatus int
	}{
		{name: "invalid input", err: connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil), code: connectorcore.ErrorCodeInvalidInput, httpStatus: http.StatusBadRequest},
		{name: "not found", err: connectorcore.NewError(connectorcore.ErrorCodeNotFound, false, nil), code: connectorcore.ErrorCodeNotFound, httpStatus: http.StatusNotFound},
		{name: "conflict", err: connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil), code: connectorcore.ErrorCodeConflict, httpStatus: http.StatusConflict},
		{name: "invalid key", err: connectorcore.NewError(connectorcore.ErrorCodeInvalidKey, false, nil), code: connectorcore.ErrorCodeInvalidKey, httpStatus: http.StatusUnprocessableEntity},
		{name: "insufficient key", err: connectorcore.NewError(connectorcore.ErrorCodeInsufficientKey, false, nil), code: connectorcore.ErrorCodeInsufficientKey, httpStatus: http.StatusUnprocessableEntity},
		{name: "provider unavailable", err: connectorcore.NewError(connectorcore.ErrorCodeProviderUnavailable, true, nil), code: connectorcore.ErrorCodeProviderUnavailable, httpStatus: http.StatusServiceUnavailable},
		{name: "unstructured internal", err: fmt.Errorf("provider echoed %s", connectorRouteSecretSentinel), code: connectorcore.ErrorCodeInternal, httpStatus: http.StatusInternalServerError},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			service := &connectorRouteRecordingService{getConnectionErr: testCase.err}
			router := newConnectorRouteTestRouter(definitionCatalog, service, organizationID, ownerID)
			recorder := connectorRouteRequest(t, router, http.MethodGet, "/console/api/connector-connections/"+connectionID.String(), "", nil)
			if recorder.Code != testCase.httpStatus || !strings.Contains(recorder.Body.String(), `"code":"`+string(testCase.code)+`"`) {
				t.Fatalf("status/body = %d %s", recorder.Code, recorder.Body.String())
			}
			assertConnectorRouteBodyIsSecretFree(t, recorder.Body.String())
		})
	}
}

func newConnectorRouteTestCatalog(t *testing.T) *connectorcatalog.Catalog {
	t.Helper()
	base := connectorcatalog.Definition{
		ID: "github", PluginID: "zgi.github", PluginVersion: "1.0.0",
		SourceDialect: connectorcatalog.SourceDialectAgentPluginsV1, SourceSchemaVersion: connectorcatalog.SourceSchemaVersion1,
		ProviderID: "github", DriverID: "github-rest", AuthMethodID: "personal_access_token",
		Name: "GitHub", Description: "GitHub repositories and issues", Classification: connectorcatalog.ClassificationConnector,
		NameI18n:        connectorcatalog.LocalizedText{"en-US": "GitHub", "zh-Hans": "GitHub"},
		DescriptionI18n: connectorcatalog.LocalizedText{"en-US": "GitHub repositories and issues", "zh-Hans": "GitHub 仓库和议题"},
		CapabilityTypes: []connectorcatalog.CapabilityType{connectorcatalog.CapabilityTypeAction},
		SetupMethod:     connectorcatalog.SetupMethodKey, Ownership: connectorcatalog.OwnershipPersonal,
		Surfaces: connectorcatalog.FirstReleaseSurfaces(),
		KeyFields: []connectorcatalog.KeyField{{
			Name: "token", Label: "Personal access token", Placeholder: "Enter an access token", Required: true, Secret: true,
			LabelI18n:       connectorcatalog.LocalizedText{"en-US": "Personal access token", "zh-Hans": "个人访问令牌"},
			PlaceholderI18n: connectorcatalog.LocalizedText{"en-US": "Enter an access token", "zh-Hans": "请输入访问令牌"},
		}},
		Operations: []connectorcatalog.Operation{
			{
				ID: "github.list_repositories", ToolName: "github.list_repositories", Name: "List repositories", Description: "List repositories",
				NameI18n:        connectorcatalog.LocalizedText{"en-US": "List repositories", "zh-Hans": "列出仓库"},
				DescriptionI18n: connectorcatalog.LocalizedText{"en-US": "List repositories", "zh-Hans": "列出可用仓库"},
			},
			{ID: "github.create_issue", ToolName: "github.create_issue", Name: "Create issue", Description: "Create an issue"},
		},
	}
	dataSource := base
	dataSource.ID, dataSource.PluginID, dataSource.ProviderID, dataSource.DriverID, dataSource.AuthMethodID = "reference-data", "test.reference-data", "reference-data", "reference-data", "api_key"
	dataSource.Name, dataSource.Description, dataSource.Classification = "Reference data", "Look up reference data", connectorcatalog.ClassificationDataSource
	dataSource.CapabilityTypes = []connectorcatalog.CapabilityType{connectorcatalog.CapabilityTypeData}
	dataSource.KeyFields = []connectorcatalog.KeyField{{Name: "api_key", Label: "API Key", Required: true, Secret: true}}
	dataSource.Operations = []connectorcatalog.Operation{{ID: "reference.lookup", ToolName: "reference_lookup", Name: "Lookup", Description: "Look up reference data"}}
	result, err := connectorcatalog.New(base, dataSource)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type connectorRouteServiceCall struct {
	operation      string
	organizationID uuid.UUID
	ownerID        uuid.UUID
	resourceID     uuid.UUID
	filter         connectorconnections.ConnectionListFilter
}

type connectorRouteRecordingService struct {
	calls            []connectorRouteServiceCall
	connections      []connectorcore.PersonalConnection
	attempt          connectorcore.KeySetupAttempt
	startResult      connectorconnections.KeySetupResult
	getSetupResult   connectorconnections.KeySetupResult
	startErr         error
	getSetupErr      error
	cancelErr        error
	listErr          error
	getConnectionErr error
	lastCommand      connectorconnections.KeySetupCommand
	capturedValues   map[string]string
}

func (service *connectorRouteRecordingService) StartKeySetup(_ context.Context, command connectorconnections.KeySetupCommand) (connectorconnections.KeySetupResult, error) {
	service.calls = append(service.calls, connectorRouteServiceCall{operation: "start", organizationID: command.OrganizationID, ownerID: command.OwnerAccountID})
	service.lastCommand = command
	service.capturedValues = command.Values
	return service.startResult, service.startErr
}

func (service *connectorRouteRecordingService) GetKeySetup(_ context.Context, organizationID, ownerID, attemptID uuid.UUID) (connectorconnections.KeySetupResult, error) {
	service.calls = append(service.calls, connectorRouteServiceCall{operation: "get_setup", organizationID: organizationID, ownerID: ownerID, resourceID: attemptID})
	if service.getSetupErr != nil {
		return connectorconnections.KeySetupResult{}, service.getSetupErr
	}
	if service.getSetupResult.Attempt.ID != uuid.Nil {
		result := service.getSetupResult
		if result.Attempt.ID != attemptID || result.Attempt.OrganizationID != organizationID || result.Attempt.OwnerAccountID != ownerID {
			return connectorconnections.KeySetupResult{}, connectorcore.NewError(connectorcore.ErrorCodeNotFound, false, nil)
		}
		return result, nil
	}
	if service.attempt.ID == uuid.Nil || service.attempt.ID != attemptID || service.attempt.OrganizationID != organizationID || service.attempt.OwnerAccountID != ownerID {
		return connectorconnections.KeySetupResult{}, connectorcore.NewError(connectorcore.ErrorCodeNotFound, false, nil)
	}
	return connectorconnections.KeySetupResult{Attempt: service.attempt}, nil
}

func (service *connectorRouteRecordingService) CancelKeySetup(_ context.Context, organizationID, ownerID, attemptID uuid.UUID) (connectorcore.KeySetupAttempt, error) {
	service.calls = append(service.calls, connectorRouteServiceCall{operation: "cancel_setup", organizationID: organizationID, ownerID: ownerID, resourceID: attemptID})
	if service.cancelErr != nil {
		return connectorcore.KeySetupAttempt{}, service.cancelErr
	}
	if service.attempt.ID == uuid.Nil || service.attempt.ID != attemptID || service.attempt.OrganizationID != organizationID || service.attempt.OwnerAccountID != ownerID {
		return connectorcore.KeySetupAttempt{}, connectorcore.NewError(connectorcore.ErrorCodeNotFound, false, nil)
	}
	cancelled := service.attempt
	cancelled.State = connectorcore.SetupStateCancelled
	return cancelled, nil
}

func (service *connectorRouteRecordingService) ListConnections(_ context.Context, organizationID, ownerID uuid.UUID, filter connectorconnections.ConnectionListFilter) ([]connectorcore.PersonalConnection, error) {
	service.calls = append(service.calls, connectorRouteServiceCall{operation: "list", organizationID: organizationID, ownerID: ownerID, filter: filter})
	if service.listErr != nil {
		return nil, service.listErr
	}
	items := make([]connectorcore.PersonalConnection, 0)
	for _, item := range service.connections {
		if item.OrganizationID != organizationID || item.OwnerAccountID != ownerID {
			continue
		}
		if filter.ConnectorID != "" && item.ConnectorID != filter.ConnectorID {
			continue
		}
		if len(filter.States) > 0 && !connectorRouteContainsState(filter.States, item.State) {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (service *connectorRouteRecordingService) GetConnection(_ context.Context, organizationID, ownerID, connectionID uuid.UUID) (connectorcore.PersonalConnection, error) {
	service.calls = append(service.calls, connectorRouteServiceCall{operation: "get_connection", organizationID: organizationID, ownerID: ownerID, resourceID: connectionID})
	if service.getConnectionErr != nil {
		return connectorcore.PersonalConnection{}, service.getConnectionErr
	}
	for _, item := range service.connections {
		if item.ID == connectionID && item.OrganizationID == organizationID && item.OwnerAccountID == ownerID {
			return item, nil
		}
	}
	return connectorcore.PersonalConnection{}, connectorcore.NewError(connectorcore.ErrorCodeNotFound, false, nil)
}

type connectorRouteMaintenanceCall struct {
	operation      string
	organizationID uuid.UUID
	ownerID        uuid.UUID
	connectionID   uuid.UUID
	revision       int
	name           string
	idempotencyKey string
	force          bool
	impactToken    string
}

type connectorRouteRecordingMaintenanceService struct {
	calls           []connectorRouteMaintenanceCall
	renameResult    connectorcore.PersonalConnection
	reconnectResult connectorconnections.ReconnectResult
	diagnoseResult  connectorcore.PersonalConnection
	impactResult    connectorconnections.ConnectionRemovalImpact
	renameErr       error
	reconnectErr    error
	diagnoseErr     error
	impactErr       error
	removeErr       error
	capturedValues  map[string]string
}

func (service *connectorRouteRecordingMaintenanceService) Rename(
	_ context.Context,
	organizationID, ownerID, connectionID uuid.UUID,
	revision int,
	name string,
) (connectorcore.PersonalConnection, error) {
	service.calls = append(service.calls, connectorRouteMaintenanceCall{
		operation: "rename", organizationID: organizationID, ownerID: ownerID,
		connectionID: connectionID, revision: revision, name: name,
	})
	return service.renameResult, service.renameErr
}

func (service *connectorRouteRecordingMaintenanceService) Reconnect(
	_ context.Context,
	command connectorconnections.ReconnectCommand,
) (connectorconnections.ReconnectResult, error) {
	service.calls = append(service.calls, connectorRouteMaintenanceCall{
		operation: "reconnect", organizationID: command.OrganizationID, ownerID: command.OwnerAccountID,
		connectionID: command.ConnectionID, idempotencyKey: command.IdempotencyKey,
	})
	service.capturedValues = command.Values
	return service.reconnectResult, service.reconnectErr
}

func (service *connectorRouteRecordingMaintenanceService) Diagnose(
	_ context.Context,
	organizationID, ownerID, connectionID uuid.UUID,
) (connectorcore.PersonalConnection, error) {
	service.calls = append(service.calls, connectorRouteMaintenanceCall{
		operation: "diagnose", organizationID: organizationID, ownerID: ownerID, connectionID: connectionID,
	})
	return service.diagnoseResult, service.diagnoseErr
}

func (service *connectorRouteRecordingMaintenanceService) GetRemovalImpact(
	_ context.Context,
	organizationID, ownerID, connectionID uuid.UUID,
) (connectorconnections.ConnectionRemovalImpact, error) {
	service.calls = append(service.calls, connectorRouteMaintenanceCall{
		operation: "impact", organizationID: organizationID, ownerID: ownerID, connectionID: connectionID,
	})
	return service.impactResult, service.impactErr
}

func (service *connectorRouteRecordingMaintenanceService) Remove(
	_ context.Context,
	command connectorconnections.RemoveConnectionCommand,
) error {
	service.calls = append(service.calls, connectorRouteMaintenanceCall{
		operation: "remove", organizationID: command.OrganizationID, ownerID: command.OwnerAccountID,
		connectionID: command.ConnectionID, force: command.Force, impactToken: command.ImpactToken,
	})
	return service.removeErr
}

type connectorRouteBindingListCall struct {
	organizationID    uuid.UUID
	actingPrincipalID uuid.UUID
	target            connectorbindings.TargetIdentity
}

type connectorRouteRecordingBindingService struct {
	listCalls    []connectorRouteBindingListCall
	upsertCalls  []connectorbindings.UpsertBindingCommand
	detachCalls  []connectorbindings.DetachBindingCommand
	impactCalls  []connectorRouteImpactCall
	listResult   []connectorcore.TargetBinding
	upsertResult connectorbindings.UpsertBindingResult
	detachResult connectorcore.TargetBinding
	impactResult connectorbindings.RemovalImpact
	listErr      error
	upsertErr    error
	detachErr    error
	impactErr    error
}

type connectorRouteImpactCall struct {
	organizationID    uuid.UUID
	actingPrincipalID uuid.UUID
	connectionID      uuid.UUID
}

func (service *connectorRouteRecordingBindingService) ListTargetBindings(
	_ context.Context,
	organizationID, actingPrincipalID uuid.UUID,
	target connectorbindings.TargetIdentity,
) ([]connectorcore.TargetBinding, error) {
	service.listCalls = append(service.listCalls, connectorRouteBindingListCall{
		organizationID: organizationID, actingPrincipalID: actingPrincipalID, target: target,
	})
	if service.listErr != nil {
		return nil, service.listErr
	}
	return service.listResult, nil
}

func (service *connectorRouteRecordingBindingService) UpsertBinding(
	_ context.Context,
	command connectorbindings.UpsertBindingCommand,
) (connectorbindings.UpsertBindingResult, error) {
	service.upsertCalls = append(service.upsertCalls, command)
	if service.upsertErr != nil {
		return connectorbindings.UpsertBindingResult{}, service.upsertErr
	}
	return service.upsertResult, nil
}

func (service *connectorRouteRecordingBindingService) DetachBinding(
	_ context.Context,
	command connectorbindings.DetachBindingCommand,
) (connectorcore.TargetBinding, error) {
	service.detachCalls = append(service.detachCalls, command)
	if service.detachErr != nil {
		return connectorcore.TargetBinding{}, service.detachErr
	}
	return service.detachResult, nil
}

func (service *connectorRouteRecordingBindingService) GetRemovalImpact(
	_ context.Context,
	organizationID, actingPrincipalID, connectionID uuid.UUID,
) (connectorbindings.RemovalImpact, error) {
	service.impactCalls = append(service.impactCalls, connectorRouteImpactCall{
		organizationID: organizationID, actingPrincipalID: actingPrincipalID, connectionID: connectionID,
	})
	if service.impactErr != nil {
		return connectorbindings.RemovalImpact{}, service.impactErr
	}
	if service.impactResult.ConnectionID == uuid.Nil {
		return connectorbindings.RemovalImpact{ConnectionID: connectionID, CanRemove: true, AffectedBindings: []connectorcore.TargetBinding{}}, nil
	}
	return service.impactResult, nil
}

func newConnectorRouteTestRouter(
	catalog connectorDefinitionCatalog,
	service connectorConnectionService,
	organizationID, ownerID uuid.UUID,
	bindingServices ...connectorBindingService,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api")
	group.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, organizationID.String())
		c.Set("account_id", ownerID.String())
		c.Next()
	})
	bindingService := connectorBindingService(&connectorRouteRecordingBindingService{})
	if len(bindingServices) > 0 && bindingServices[0] != nil {
		bindingService = bindingServices[0]
	}
	(&connectorHandler{
		catalog: catalog, connections: service, bindings: bindingService,
		maintenance: &connectorRouteRecordingMaintenanceService{},
	}).register(group)
	return router
}

func newConnectorRouteTestRouterWithMaintenance(
	catalog connectorDefinitionCatalog,
	service connectorConnectionService,
	maintenance connectorMaintenanceService,
	organizationID, ownerID uuid.UUID,
	bindingService connectorBindingService,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api")
	group.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, organizationID.String())
		c.Set("account_id", ownerID.String())
		c.Next()
	})
	(&connectorHandler{
		catalog: catalog, connections: service, bindings: bindingService, maintenance: maintenance,
	}).register(group)
	return router
}

func connectorRouteRequest(t *testing.T, router http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func assertConnectorRouteLastScope(t *testing.T, service *connectorRouteRecordingService, operation string, organizationID, ownerID uuid.UUID) {
	t.Helper()
	if len(service.calls) == 0 {
		t.Fatal("service was not called")
	}
	call := service.calls[len(service.calls)-1]
	if call.operation != operation || call.organizationID != organizationID || call.ownerID != ownerID {
		t.Fatalf("last service call = %#v", call)
	}
}

func assertConnectorRouteBodyIsSecretFree(t *testing.T, body string) {
	t.Helper()
	for _, forbidden := range []string{
		connectorRouteSecretSentinel,
		`"provider_id"`,
		`"driver_id"`,
		`"auth_method_id"`,
		`"encrypted_credentials"`,
		`"idempotency_key_hash"`,
		`"request_fingerprint"`,
		`"organization_id"`,
		`"owner_account_id"`,
		"provider echoed",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response exposed forbidden value %q: %s", forbidden, body)
		}
	}
}

func connectorRouteContainsState(states []connectorcore.Readiness, candidate connectorcore.Readiness) bool {
	for _, state := range states {
		if state == candidate {
			return true
		}
	}
	return false
}
