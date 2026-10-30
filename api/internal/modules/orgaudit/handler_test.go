package orgaudit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

type fakeLister struct {
	filter Filter
	rows   []Row
	total  int64
	calls  int
}

func (f *fakeLister) List(_ context.Context, filter Filter) ([]Row, int64, error) {
	f.calls++
	f.filter = filter
	return f.rows, f.total, nil
}

type fakeAdmins struct {
	allowed bool
	err     error
}

func (f fakeAdmins) IsOrganizationAdminOrOwner(context.Context, string, string) (bool, error) {
	return f.allowed, f.err
}

func newTestHandler(t *testing.T, lister *fakeLister, admins fakeAdmins) *gin.Engine {
	t.Helper()
	definitions := append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	group := engine.Group("/organizations", func(c *gin.Context) {
		if account := c.GetHeader("X-Test-Account"); account != "" {
			c.Set("account_id", account)
		}
	})
	NewHandler(NewService(lister, admins), projector).RegisterRoutes(group)
	return engine
}

func get(engine *gin.Engine, target, account string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if account != "" {
		request.Header.Set("X-Test-Account", account)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

func TestListAuditLogsReturnsPageWithFilters(t *testing.T) {
	actor := uuid.MustParse(testActorID)
	lister := &fakeLister{total: 45, rows: []Row{{Log: Log{ID: uuid.New(), Action: ActionMemberRemove, Result: ResultSuccess, Before: []byte("{}"), After: []byte("{}")}, ActorName: "Ada"}}}
	engine := newTestHandler(t, lister, fakeAdmins{allowed: true})

	response := get(engine, "/organizations/"+testOrganizationID+"/audit-logs?page=2&limit=20&actor_account_id="+testActorID+
		"&target_account_id="+testTargetID+"&action=member.remove&result=success&from=2026-10-01T00:00:00Z&to=1792800000", testActorID)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Code string `json:"code"`
		Data Page   `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "0" || body.Data.Total != 45 || body.Data.Page != 2 || !body.Data.HasMore || len(body.Data.Items) != 1 || body.Data.Items[0].ActorName != "Ada" {
		t.Fatalf("body = %+v", body)
	}
	got := lister.filter
	if got.OrganizationID.String() != testOrganizationID || got.ActorAccountID == nil || *got.ActorAccountID != actor ||
		got.TargetAccountID == nil || got.TargetAccountID.String() != testTargetID || got.Action != ActionMemberRemove || got.Result != ResultSuccess {
		t.Fatalf("filter = %+v", got)
	}
	if got.From == nil || !got.From.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) || got.To == nil || got.To.Unix() != 1792800000 {
		t.Fatalf("time range = %v..%v", got.From, got.To)
	}
}

func TestListAuditLogsClampsLimit(t *testing.T) {
	lister := &fakeLister{}
	engine := newTestHandler(t, lister, fakeAdmins{allowed: true})
	if response := get(engine, "/organizations/"+testOrganizationID+"/audit-logs?limit=5000", testActorID); response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if lister.filter.Limit != maxLimit || lister.filter.Page != 1 {
		t.Fatalf("page/limit = %d/%d", lister.filter.Page, lister.filter.Limit)
	}
}

func TestListAuditLogsRejectsNonAdmins(t *testing.T) {
	lister := &fakeLister{}
	engine := newTestHandler(t, lister, fakeAdmins{allowed: false})
	response := get(engine, "/organizations/"+testOrganizationID+"/audit-logs", testActorID)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	if body["code"] != AppCodeAccessDenied.String() {
		t.Fatalf("code = %v", body["code"])
	}
	if lister.calls != 0 {
		t.Fatal("non-admin reached the store")
	}
}

func TestListAuditLogsErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		target  string
		account string
		admins  fakeAdmins
		status  int
		code    string
	}{
		{"unauthenticated", "/organizations/" + testOrganizationID + "/audit-logs", "", fakeAdmins{allowed: true}, http.StatusUnauthorized, appcatalog.CodeAuthenticationRequired.String()},
		{"bad organization", "/organizations/current/audit-logs", testActorID, fakeAdmins{allowed: true}, http.StatusBadRequest, AppCodeFilterInvalid.String()},
		{"bad actor", "/organizations/" + testOrganizationID + "/audit-logs?actor_account_id=x", testActorID, fakeAdmins{allowed: true}, http.StatusBadRequest, AppCodeFilterInvalid.String()},
		{"bad result", "/organizations/" + testOrganizationID + "/audit-logs?result=maybe", testActorID, fakeAdmins{allowed: true}, http.StatusBadRequest, AppCodeFilterInvalid.String()},
		{"reversed range", "/organizations/" + testOrganizationID + "/audit-logs?from=1792800000&to=1792700000", testActorID, fakeAdmins{allowed: true}, http.StatusBadRequest, AppCodeFilterInvalid.String()},
		{"missing organization", "/organizations/" + testOrganizationID + "/audit-logs", testActorID, fakeAdmins{err: errors.New("organization not found")}, http.StatusNotFound, appcatalog.CodeResourceNotFound.String()},
		{"store failure", "/organizations/" + testOrganizationID + "/audit-logs", testActorID, fakeAdmins{err: errors.New("db down")}, http.StatusInternalServerError, appcatalog.CodeInternal.String()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := get(newTestHandler(t, &fakeLister{}, tc.admins), tc.target, tc.account)
			var body map[string]any
			_ = json.Unmarshal(response.Body.Bytes(), &body)
			if response.Code != tc.status || body["code"] != tc.code {
				t.Fatalf("status = %d code = %v, want %d %s", response.Code, body["code"], tc.status, tc.code)
			}
		})
	}
}

func TestCatalogDefinitionsHaveBothLocales(t *testing.T) {
	for _, definition := range CatalogDefinitions() {
		for _, locale := range []appcatalog.Locale{appcatalog.LocaleEnglishUS, appcatalog.LocaleChineseSimplified} {
			if definition.Messages[locale] == "" {
				t.Fatalf("%s has no %s message", definition.Code, locale)
			}
		}
	}
}
