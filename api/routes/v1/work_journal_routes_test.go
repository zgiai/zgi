package v1

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/workjournal"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"net/http"
	"testing"
)

type journalRouteStub struct {
	service *workjournal.Service
	scope   digitalhuman.Scope
}

func (s *journalRouteStub) ForEmployee(_ context.Context, scope digitalhuman.Scope, id uuid.UUID) (*workjournal.Service, workjournal.Scope, error) {
	s.scope = scope
	return s.service, workjournal.Scope{OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, OwnerID: scope.ActorID, EmployeeID: id}, nil
}
func TestWorkJournalReadRoutesUseAuthenticatedOwnerAndValidatePagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	defer sql.Close()
	if err = db.AutoMigrate(&workjournal.Record{}, &workjournal.RecordVersion{}, &workjournal.Operation{}); err != nil {
		t.Fatal(err)
	}
	service, _ := workjournal.New(db, nil)
	org, ws, owner, employee, otherOwner := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	own := workjournal.Scope{OrganizationID: org, WorkspaceID: ws, OwnerID: owner, EmployeeID: employee}
	foreign := own
	foreign.OwnerID = otherOwner
	record, err := service.SaveRecord(t.Context(), own, workjournal.SaveRecordRequest{Kind: "ticket", Key: "mail-1", State: "waiting", Content: datatypes.JSON(`{"question":"ours"}`)})
	if err != nil {
		t.Fatal(err)
	}
	secret, err := service.SaveRecord(t.Context(), foreign, workjournal.SaveRecordRequest{Kind: "ticket", Key: "mail-1", State: "waiting", Content: datatypes.JSON(`{"question":"foreign"}`)})
	if err != nil {
		t.Fatal(err)
	}
	stub := &journalRouteStub{service: service}
	router := gin.New()
	group := router.Group("/console/api")
	group.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, org.String())
		util.SetWorkspaceScopeCompat(c, ws.String())
		c.Set("account_id", owner.String())
	})
	registerWorkJournalRoutes(group, stub)
	base := "/console/api/digital-humans/" + employee.String() + "/work-journal"
	for _, suffix := range []string{"/records?limit=1&owner_id=" + otherOwner.String(), "/records/" + record.ID.String(), "/records/" + record.ID.String() + "/history", "/operations"} {
		r := connectorRouteRequest(t, router, http.MethodGet, base+suffix, "", nil)
		if r.Code != 200 {
			t.Fatal(r.Code, r.Body.String())
		}
	}
	if stub.scope.ActorID != owner {
		t.Fatal("query supplied owner")
	}
	for _, suffix := range []string{"/records/" + secret.ID.String(), "/records/" + secret.ID.String() + "/history", "/records?limit=101", "/records?after_id=not-uuid", "/records?limit=-1", "/operations?state=made-up", "/records/" + record.ID.String() + "/history?after_revision=bad"} {
		r := connectorRouteRequest(t, router, http.MethodGet, base+suffix, "", nil)
		if r.Code == 200 {
			t.Fatal("invalid or foreign read accepted", suffix, r.Body.String())
		}
	}
	for _, path := range []string{"/digital-humans/:id/work-journal/records", "/digital-humans/:id/work-journal/operations/:operationID"} {
		if p := digitalHumanPermissionForRequest(http.MethodGet, path); p != workspacemodel.WorkspacePermissionDigitalHumanLogsView {
			t.Fatal(p)
		}
	}
	r := connectorRouteRequest(t, router, http.MethodPost, base+"/operations/"+uuid.NewString()+"/retry", "{}", nil)
	if r.Code != 404 {
		t.Fatal("HTTP operation reset route exposed", r.Code)
	}
}
