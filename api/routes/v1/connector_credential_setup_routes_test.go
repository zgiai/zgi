package v1

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/util"
)

const credentialSetupSecretSentinel = "credential-setup-PRIVATE-KEY-SENTINEL"

// credentialSetupRouteCatalog holds the two dialects the credential-setup
// surface admits: the published messaging contract and the inert Server
// Operations SSH contract. The SSH definition is only handed to a focused test
// handler; neither definition is registered with a runtime route catalog or a
// provider/driver.
func credentialSetupRouteCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	dingtalk, ok := catalog.MessagingDefinition("dingtalk")
	if !ok {
		t.Fatal("dingtalk messaging definition missing")
	}
	definitions, err := catalog.New(dingtalk, catalog.ServerOpsSSHDefinition())
	if err != nil {
		t.Fatal(err)
	}
	return definitions
}

func credentialSetupRouteRouter(t *testing.T, service *connectorRouteRecordingService, organizationID, ownerID uuid.UUID) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api")
	group.Use(func(c *gin.Context) {
		if organizationID != uuid.Nil {
			util.SetOrganizationID(c, organizationID.String())
		}
		if ownerID != uuid.Nil {
			c.Set("account_id", ownerID.String())
		}
		c.Next()
	})
	(&connectorHandler{catalog: credentialSetupRouteCatalog(t), connections: service}).register(group)
	return router
}

// credentialSetupRequestBody renders the JSON envelope for one SSH credential
// submission. The fixture values live only in the local map, which the shared
// destroy helper clears afterwards.
func credentialSetupRequestBody(t *testing.T, values map[string]string, extra map[string]any) string {
	t.Helper()
	body := map[string]any{"values": values}
	for key, value := range extra {
		body[key] = value
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func sshFixtureValues(privateKey string) map[string]string {
	return map[string]string{
		"host":            "server.example.internal",
		"port":            "22",
		"username":        "deploy",
		"private_key":     privateKey,
		"passphrase":      "fixture-passphrase",
		"host_key_sha256": "SHA256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU",
	}
}

func sshFixturePrivateKey() string {
	return strings.Join([]string{
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		credentialSetupSecretSentinel,
		"-----END OPENSSH PRIVATE KEY-----",
	}, "\n") + "\n"
}

func TestCredentialSetupRouteAdmitsSSHContractIntoExistingSetupHandler(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	attempt := core.KeySetupAttempt{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: catalog.ServerOpsSSHConnectorID, State: core.SetupStateVerifying,
	}
	service := &connectorRouteRecordingService{
		startResult: connections.KeySetupResult{Attempt: attempt},
	}
	router := credentialSetupRouteRouter(t, service, organizationID, ownerID)

	response := connectorRouteRequest(
		t, router, http.MethodPost, "/console/api/connectors/"+catalog.ServerOpsSSHConnectorID+"/credential-setups",
		credentialSetupRequestBody(t, sshFixtureValues(sshFixturePrivateKey()), nil),
		map[string]string{"Idempotency-Key": "credential-setup-ssh-idempotency-0001"},
	)
	if response.Code != http.StatusAccepted {
		t.Fatalf("ssh setup status/body = %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("ssh credential setup response is cacheable")
	}
	if !strings.Contains(response.Body.String(), attempt.ID.String()) || !strings.Contains(response.Body.String(), `"state":"verifying"`) {
		t.Fatalf("ssh setup body = %s", response.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, response.Body.String())
	if strings.Contains(response.Body.String(), credentialSetupSecretSentinel) {
		t.Fatal("ssh setup response echoed a submitted secret")
	}
	if len(service.calls) != 1 {
		t.Fatalf("ssh setup service calls = %#v", service.calls)
	}
	call := service.calls[0]
	if call.operation != "start" || call.organizationID != organizationID || call.ownerID != ownerID {
		t.Fatalf("ssh setup call scope = %#v", call)
	}
	if service.lastCommand.ConnectorID != catalog.ServerOpsSSHConnectorID ||
		service.lastCommand.IdempotencyKey != "credential-setup-ssh-idempotency-0001" {
		t.Fatalf("ssh setup command = %#v", service.lastCommand)
	}
	if service.lastCommand.Target != nil {
		t.Fatalf("ssh setup forwarded a target: %#v", service.lastCommand.Target)
	}
	if service.lastCommand.MailUsage != "" {
		t.Fatalf("ssh setup forwarded mail usage %q", service.lastCommand.MailUsage)
	}
	if len(service.capturedValues) != 0 {
		t.Fatalf("route retained candidate ssh values after service returned: %#v", service.capturedValues)
	}
}

// TestCredentialSetupRouteSSHChecksTargetAndMailUsagePresence pins exact
// presence detection on the SSH dialect: omitting both keys is accepted, while
// every supplied form — populated, null, empty string, or whitespace — is
// rejected regardless of the decoded value.
func TestCredentialSetupRouteSSHChecksTargetAndMailUsagePresence(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	path := "/console/api/connectors/" + catalog.ServerOpsSSHConnectorID + "/credential-setups"
	headers := map[string]string{"Idempotency-Key": "credential-setup-ssh-idempotency-0002"}

	for name, testCase := range map[string]struct {
		extra  map[string]any
		status int
	}{
		"both keys missing":     {extra: nil, status: http.StatusAccepted},
		"target populated":      {extra: map[string]any{"target": map[string]any{"type": "agent", "id": "agent-1"}}, status: http.StatusBadRequest},
		"target empty object":   {extra: map[string]any{"target": map[string]any{}}, status: http.StatusBadRequest},
		"target null":           {extra: map[string]any{"target": nil}, status: http.StatusBadRequest},
		"target blank":          {extra: map[string]any{"target": ""}, status: http.StatusBadRequest},
		"target whitespace":     {extra: map[string]any{"target": " \t\n"}, status: http.StatusBadRequest},
		"mail_usage populated":  {extra: map[string]any{"mail_usage": "read_send"}, status: http.StatusBadRequest},
		"mail_usage null":       {extra: map[string]any{"mail_usage": nil}, status: http.StatusBadRequest},
		"mail_usage blank":      {extra: map[string]any{"mail_usage": ""}, status: http.StatusBadRequest},
		"mail_usage whitespace": {extra: map[string]any{"mail_usage": " \t\n"}, status: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			attempt := core.KeySetupAttempt{
				ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
				ConnectorID: catalog.ServerOpsSSHConnectorID, State: core.SetupStateVerifying,
			}
			service := &connectorRouteRecordingService{startResult: connections.KeySetupResult{Attempt: attempt}}
			router := credentialSetupRouteRouter(t, service, organizationID, ownerID)

			response := connectorRouteRequest(t, router, http.MethodPost, path,
				credentialSetupRequestBody(t, sshFixtureValues(sshFixturePrivateKey()), testCase.extra), headers)
			if response.Code != testCase.status {
				t.Fatalf("ssh %s status/body = %d %s", name, response.Code, response.Body.String())
			}
			assertConnectorRouteBodyIsSecretFree(t, response.Body.String())
			if strings.Contains(response.Body.String(), credentialSetupSecretSentinel) {
				t.Fatalf("ssh %s echoed a submitted secret", name)
			}
			if testCase.status == http.StatusAccepted {
				if len(service.calls) != 1 || service.calls[0].operation != "start" {
					t.Fatalf("ssh %s service calls = %#v", name, service.calls)
				}
				if service.lastCommand.Target != nil || service.lastCommand.MailUsage != "" {
					t.Fatalf("ssh %s forwarded target/mail usage: %#v", name, service.lastCommand)
				}
				return
			}
			if len(service.calls) != 0 {
				t.Fatalf("ssh %s reached the setup service: %#v", name, service.calls)
			}
		})
	}
}

func TestCredentialSetupRouteRejectsOtherDialectsAndOversizedBody(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	service := &connectorRouteRecordingService{}
	router := credentialSetupRouteRouter(t, service, organizationID, ownerID)
	oversizedBody := `{"values":{"private_key":"` + strings.Repeat("A", 600*1024) + `"}}`

	for name, testCase := range map[string]struct {
		path string
		body string
	}{
		"key dialect":       {path: "/console/api/connectors/github/credential-setups", body: `{"values":{"token":"x"}}`},
		"app dialect":       {path: "/console/api/connectors/feishu/credential-setups", body: `{"values":{"app_id":"x"}}`},
		"mail dialect":      {path: "/console/api/connectors/gmail/credential-setups", body: `{"values":{"email":"x"}}`},
		"unknown connector": {path: "/console/api/connectors/unknown/credential-setups", body: `{"values":{}}`},
		"oversized body":    {path: "/console/api/connectors/" + catalog.ServerOpsSSHConnectorID + "/credential-setups", body: oversizedBody},
		"two json objects":  {path: "/console/api/connectors/" + catalog.ServerOpsSSHConnectorID + "/credential-setups", body: `{"values":{}}{"values":{}}`},
		"unknown ssh field": {path: "/console/api/connectors/" + catalog.ServerOpsSSHConnectorID + "/credential-setups", body: `{"values":{},"unexpected":true}`},
	} {
		t.Run(name, func(t *testing.T) {
			response := connectorRouteRequest(t, router, http.MethodPost, testCase.path, testCase.body,
				map[string]string{"Idempotency-Key": "credential-setup-reject-idempotency-0003"})
			if response.Code != http.StatusBadRequest {
				t.Fatalf("%s status/body = %d %s", name, response.Code, response.Body.String())
			}
			assertConnectorRouteBodyIsSecretFree(t, response.Body.String())
			if len(service.calls) != 0 {
				t.Fatalf("%s reached the setup service: %#v", name, service.calls)
			}
		})
	}
}

func TestCredentialSetupRouteMessagingRegressionStaysUnchanged(t *testing.T) {
	organizationID, ownerID := uuid.New(), uuid.New()
	attempt := core.KeySetupAttempt{
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: "dingtalk", State: core.SetupStateVerifying,
	}
	service := &connectorRouteRecordingService{startResult: connections.KeySetupResult{Attempt: attempt}}
	router := credentialSetupRouteRouter(t, service, organizationID, ownerID)

	response := connectorRouteRequest(
		t, router, http.MethodPost, "/console/api/connectors/dingtalk/credential-setups",
		`{"values":{"client_id":"fixture-client","client_secret":"fixture-client-secret"}}`,
		map[string]string{"Idempotency-Key": "credential-setup-messaging-idempotency-0004"},
	)
	if response.Code != http.StatusAccepted || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("messaging setup status/header/body = %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), attempt.ID.String()) {
		t.Fatalf("messaging setup body = %s", response.Body.String())
	}
	assertConnectorRouteBodyIsSecretFree(t, response.Body.String())
	if len(service.calls) != 1 || service.calls[0].operation != "start" || service.calls[0].organizationID != organizationID || service.calls[0].ownerID != ownerID {
		t.Fatalf("messaging setup call = %#v", service.calls)
	}
	if service.lastCommand.ConnectorID != "dingtalk" || service.lastCommand.IdempotencyKey != "credential-setup-messaging-idempotency-0004" {
		t.Fatalf("messaging setup command = %#v", service.lastCommand)
	}
}

func TestCredentialSetupRouteRemainsOwnerScoped(t *testing.T) {
	service := &connectorRouteRecordingService{}
	path := "/console/api/connectors/" + catalog.ServerOpsSSHConnectorID + "/credential-setups"
	for name, router := range map[string]*gin.Engine{
		"missing actor": credentialSetupRouteRouter(t, service, uuid.Nil, uuid.Nil),
		"missing owner": credentialSetupRouteRouter(t, service, uuid.New(), uuid.Nil),
	} {
		t.Run(name, func(t *testing.T) {
			response := connectorRouteRequest(t, router, http.MethodPost, path,
				credentialSetupRequestBody(t, sshFixtureValues(sshFixturePrivateKey()), nil),
				map[string]string{"Idempotency-Key": "credential-setup-unauthenticated-0005"})
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated ssh setup status/body = %d %s", response.Code, response.Body.String())
			}
			assertConnectorRouteBodyIsSecretFree(t, response.Body.String())
			if len(service.calls) != 0 {
				t.Fatalf("unauthenticated ssh setup reached the service: %#v", service.calls)
			}
		})
	}
}
