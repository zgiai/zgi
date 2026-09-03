package v1

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/onboarding"
	"github.com/zgiai/zgi/api/internal/util"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"net/http"
	"strings"
	"testing"
)

type onboardingRouteFixture struct {
	starts, gets, cancels int
	command               onboarding.StartCommand
	org, owner            uuid.UUID
	revision              int64
	err                   error
}

func (s *onboardingRouteFixture) Start(_ context.Context, c onboarding.StartCommand) (onboarding.Snapshot, error) {
	s.starts++
	s.command = c
	return onboarding.Snapshot{ID: uuid.New(), State: onboarding.Starting, Region: c.Region}, s.err
}
func (s *onboardingRouteFixture) Get(_ context.Context, org, owner, id uuid.UUID) (onboarding.Snapshot, error) {
	s.gets++
	s.org, s.owner = org, owner
	return onboarding.Snapshot{ID: id, State: onboarding.Awaiting, VerificationURL: "https://accounts.feishu.cn/fixture"}, s.err
}
func (s *onboardingRouteFixture) Cancel(_ context.Context, org, owner, id uuid.UUID, revision int64) (onboarding.Snapshot, error) {
	s.cancels++
	s.org, s.owner = org, owner
	s.revision = revision
	return onboarding.Snapshot{ID: id, State: onboarding.Cancelled}, s.err
}
func onboardingRouteRouter(t *testing.T, s *onboardingRouteFixture, org, owner uuid.UUID) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/console/api")
	group.Use(func(c *gin.Context) {
		if org != uuid.Nil {
			util.SetOrganizationID(c, org.String())
		}
		if owner != uuid.Nil {
			c.Set("account_id", owner.String())
		}
		c.Next()
	})
	defs, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, append(appcatalog.DefaultDefinitions(), core.CatalogDefinitions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(defs)
	if err != nil {
		t.Fatal(err)
	}
	(&connectorHandler{onboarding: s, errorProjector: projector}).registerOnboarding(group)
	return router
}
func TestConnectorOnboardingRoutesUseServerOwnerAndReadOnlyGET(t *testing.T) {
	s := &onboardingRouteFixture{}
	org, owner, id := uuid.New(), uuid.New(), uuid.New()
	r := onboardingRouteRouter(t, s, org, owner)
	res := connectorRouteRequest(t, r, "POST", "/console/api/connectors/feishu/onboarding-flows", `{"intent":"create_app","region":"feishu","template_id":"feishu-app-template-v2"}`, map[string]string{"Idempotency-Key": "fixture-key"})
	if res.Code != 202 || s.command.OrganizationID != org || s.command.OwnerAccountID != owner || s.command.IdempotencyKey != "fixture-key" {
		t.Fatalf("start %d %s", res.Code, res.Body)
	}
	for range 3 {
		res = connectorRouteRequest(t, r, "GET", "/console/api/connectors/onboarding-flows/"+id.String(), "", nil)
		if res.Code != 200 || res.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unsafe cached GET")
		}
	}
	if s.starts != 1 || s.gets != 3 || s.org != org || s.owner != owner {
		t.Fatal("GET created an app or used external identity")
	}
	res = connectorRouteRequest(t, r, "DELETE", "/console/api/connectors/onboarding-flows/"+id.String(), `{"expected_revision":7}`, nil)
	if res.Code != 200 || s.revision != 7 || s.cancels != 1 {
		t.Fatal("cancel CAS")
	}
}
func TestConnectorOnboardingRoutesRejectSpoofedFieldsAndWrongIntent(t *testing.T) {
	s := &onboardingRouteFixture{}
	r := onboardingRouteRouter(t, s, uuid.New(), uuid.New())
	for _, body := range []string{`{"intent":"update_app","region":"feishu"}`, `{"intent":"create_app","region":"feishu","owner_account_id":"fake"}`, `{"intent":"create_app","region":"feishu","app_secret":"sentinel"}`, `{"intent":"create_app","region":"feishu","scope":["all"]}`, `{"intent":"create_app","region":"` + strings.Repeat("x", 9000) + `"}`} {
		res := connectorRouteRequest(t, r, "POST", "/console/api/connectors/feishu/onboarding-flows", body, nil)
		if res.Code != http.StatusBadRequest || s.starts != 0 || strings.Contains(res.Body.String(), "sentinel") {
			t.Fatalf("bad body %d", res.Code)
		}
	}
	res := connectorRouteRequest(t, r, "POST", "/console/api/connector-connections/"+uuid.NewString()+"/onboarding-flows", `{"intent":"update_app","region":"lark","expected_revision":1}`, nil)
	if res.Code != 400 || s.starts != 0 {
		t.Fatal("client changed update region")
	}
	for _, owner := range []uuid.UUID{uuid.Nil} {
		unauth := onboardingRouteRouter(t, s, uuid.New(), owner)
		res := connectorRouteRequest(t, unauth, "GET", "/console/api/connectors/onboarding-flows/"+uuid.NewString(), "", nil)
		if res.Code != 401 || s.gets != 0 {
			t.Fatal("unauthenticated GET")
		}
	}
}
func TestConnectorAppErrorsUseLocalizedCatalogWithoutProviderCause(t *testing.T) {
	for _, tc := range []struct {
		code   core.ErrorCode
		status int
	}{{core.ErrorCodeInvalidAppCredentials, 422}, {core.ErrorCodePermissionMissing, 403}, {core.ErrorCodeResourceAccessDenied, 403}, {core.ErrorCodeApplicationNotReady, 409}, {core.ErrorCodeRateLimited, 429}, {core.ErrorCodeFlowExpired, 410}} {
		t.Run(string(tc.code), func(t *testing.T) {
			s := &onboardingRouteFixture{err: core.NewError(tc.code, false, errors.New("APP-SECRET-SENTINEL"))}
			r := onboardingRouteRouter(t, s, uuid.New(), uuid.New())
			en := connectorRouteRequest(t, r, "GET", "/console/api/connectors/onboarding-flows/"+uuid.NewString(), "", map[string]string{"Accept-Language": "en-US"})
			zh := connectorRouteRequest(t, r, "GET", "/console/api/connectors/onboarding-flows/"+uuid.NewString(), "", map[string]string{"Accept-Language": "zh-CN"})
			if en.Code != tc.status || zh.Code != tc.status || en.Body.String() == zh.Body.String() || strings.Contains(en.Body.String()+zh.Body.String(), "SENTINEL") || en.Header().Get(apptransport.HeaderApplicationErrorCode) == "" {
				t.Fatalf("projection %d %s / %d %s", en.Code, en.Body, zh.Code, zh.Body)
			}
		})
	}
}
func TestConnectorFeishuCatalogIsNotVerifiedWithoutPersonalConnection(t *testing.T) {
	d, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	defs, err := catalog.New(d)
	if err != nil {
		t.Fatal(err)
	}
	r := newConnectorRouteTestRouter(defs, &connectorRouteRecordingService{}, uuid.New(), uuid.New())
	res := connectorRouteRequest(t, r, "GET", "/console/api/connectors/catalog/feishu", "", nil)
	if res.Code != 200 {
		t.Fatalf("catalog %s", res.Body)
	}
	body := res.Body.String()
	if !strings.Contains(body, `"credential_fields"`) || !strings.Contains(body, `"permission_template"`) || strings.Contains(body, `"usable":true`) {
		t.Fatal("unverified app or missing descriptor")
	}
	if strings.Contains(body, `cardkit:card:read`) || !strings.Contains(body, `"profile_id":"feishu-app-template-v2"`) || !strings.Contains(body, `"user_event_dependencies"`) {
		t.Fatal("catalog must expose the reviewed template and informational event dependencies")
	}
}
