package v1

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/util"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

func TestMailSetupRouteCarriesUsageAndSafePartialStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	definitions, err := catalog.New(catalog.GmailDefinition())
	if err != nil {
		t.Fatal(err)
	}
	errorDefinitions, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, append(appcatalog.DefaultDefinitions(), core.CatalogDefinitions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(errorDefinitions)
	if err != nil {
		t.Fatal(err)
	}
	service := &connectorRouteRecordingService{startErr: core.NewError(core.ErrorCodeMailAuthenticationRejected, false, nil), startResult: connections.KeySetupResult{MailVerification: &core.MailVerification{Usage: "read_send", IMAPStatus: "verified", SMTPStatus: "failed"}}}
	org, owner := uuid.New(), uuid.New()
	router := gin.New()
	group := router.Group("/console/api")
	group.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, org.String())
		c.Set("account_id", owner.String())
		c.Next()
	})
	handler := &connectorHandler{catalog: definitions, connections: service, errorProjector: projector}
	group.POST("/connectors/:connectorId/key-setups", handler.startKeySetup)
	for _, locale := range []string{"en-US", "zh-Hans"} {
		response := connectorRouteRequest(t, router, "POST", "/console/api/connectors/gmail/key-setups", `{"values":{"email":"private@example.com","app_password":"Abcd efgh ijkl MNOP"},"mail_usage":"read_send"}`, map[string]string{"Idempotency-Key": uuid.NewString(), "Accept-Language": locale})
		if response.Code < 400 || response.Header().Get("Cache-Control") != "no-store" || service.lastCommand.MailUsage != "read_send" || service.lastCommand.OrganizationID != org || service.lastCommand.OwnerAccountID != owner || len(service.capturedValues) != 0 {
			t.Fatal("unsafe mail setup contract")
		}
		body := response.Body.String()
		for _, secret := range []string{"private@example.com", "Abcd", "auth_method_id", "driver_id"} {
			if strings.Contains(body, secret) {
				t.Fatal("failure response disclosed private fields")
			}
		}
		var decoded struct {
			Message          string                `json:"message"`
			MailVerification core.MailVerification `json:"mail_verification"`
		}
		if json.Unmarshal(response.Body.Bytes(), &decoded) != nil || decoded.MailVerification.IMAPStatus != "verified" || decoded.MailVerification.SMTPStatus != "failed" || decoded.Message == "" {
			t.Fatal("missing localized safe partial status")
		}
	}
	service.startErr = core.NewError(core.ErrorCodeProviderUnavailable, true, nil)
	response := connectorRouteRequest(t, router, "POST", "/console/api/connectors/gmail/key-setups", `{"values":{"email":"private@example.com","app_password":"Abcd efgh ijkl MNOP"},"mail_usage":"read_send"}`, map[string]string{"Idempotency-Key": uuid.NewString()})
	if response.Code != 503 || !strings.Contains(response.Body.String(), `"mail_verification"`) || !strings.Contains(response.Body.String(), `"retryable":true`) {
		t.Fatal("legacy network failure lost safe protocol status")
	}
}
func TestMailCatalogShowsSMTPConditionalWithoutGrantingRead(t *testing.T) {
	d := catalog.GmailDefinition()
	connection := core.PersonalConnection{State: core.ReadinessReady, UsableOperationIDs: []core.Identifier{"gmail.message.send"}, ProviderEvidence: &core.ProviderEvidence{Mail: &core.MailEvidence{Usage: "send", Operations: []core.OperationEvidence{{ID: "gmail.message.send", Availability: "conditional"}}}}}
	item := projectCatalogItem(d, []core.PersonalConnection{connection})
	if item.SetupMethod != catalog.SetupMethodMailCredentials || item.Setup.Handler != "gmail-app-password-v1" {
		t.Fatal("mail panel routing contract missing")
	}
	for _, capability := range item.Capabilities {
		if capability.ID == "gmail.message.send" {
			if !capability.Usable || capability.Availability != "conditional" || capability.Permission != "not_scoped" {
				t.Fatal("SMTP evidence exaggerated")
			}
		} else if capability.Usable {
			t.Fatal("send usage exposed read capability")
		}
	}
}
