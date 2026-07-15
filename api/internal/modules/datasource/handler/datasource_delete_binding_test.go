package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	datasourceservice "github.com/zgiai/zgi/api/internal/modules/datasource/service"
	"github.com/zgiai/zgi/api/internal/util"
)

type dataSourceDeleteBindingService struct {
	datasourceservice.DataSourceService
	databaseAction string
	databaseToken  string
	tableAction    string
	tableToken     string
	err            error
	previewImpact  *agentbindings.Impact
}

func (s *dataSourceDeleteBindingService) PreviewDataSourceDeleteImpact(context.Context, string, string, string) (*agentbindings.Impact, error) {
	return s.previewImpact, nil
}

func (s *dataSourceDeleteBindingService) PreviewTableDeleteImpact(context.Context, string, string, string, string) (*agentbindings.Impact, error) {
	return s.previewImpact, nil
}

func (s *dataSourceDeleteBindingService) DeleteDataSourceByID(_ context.Context, _, _ string, _, action, token string) error {
	s.databaseAction = action
	s.databaseToken = token
	return s.err
}

func TestDatabaseDeleteImpactPreviewHandlersReturnAffectedAgents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &dataSourceDeleteBindingService{previewImpact: &agentbindings.Impact{
		Code:        agentbindings.ConflictCodeResourceBound,
		ImpactToken: "preview-token",
		Agents: []agentbindings.ImpactAgent{{
			AgentID:     uuid.NewString(),
			Name:        "Sales assistant",
			Description: "Uses sales data",
		}},
	}}
	tests := []struct {
		name   string
		params gin.Params
		call   func(*DataSourceHandler, *gin.Context)
	}{
		{name: "database", params: gin.Params{{Key: "id", Value: "database-1"}}, call: func(h *DataSourceHandler, c *gin.Context) { h.PreviewDataSourceDeleteImpact(c) }},
		{name: "table", params: gin.Params{{Key: "id", Value: "database-1"}, {Key: "table_id", Value: "table-1"}}, call: func(h *DataSourceHandler, c *gin.Context) { h.PreviewTableDeleteImpact(c) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/delete-impact", nil)
			ctx.Params = test.params
			ctx.Set("account_id", uuid.NewString())
			util.SetOrganizationScopeCompat(ctx, uuid.NewString())

			test.call(&DataSourceHandler{service: service}, ctx)

			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Sales assistant") {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func (s *dataSourceDeleteBindingService) DeleteTable(_ context.Context, _, _, _, _, action, token string) error {
	s.tableAction = action
	s.tableToken = token
	return s.err
}

func TestDatabaseDeleteHandlersPassBindingConfirmationAndReturnConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		target     string
		params     gin.Params
		call       func(*DataSourceHandler, *gin.Context)
		assertArgs func(*testing.T, *dataSourceDeleteBindingService)
		binding    agentbindings.BindingType
	}{
		{
			name:   "database",
			target: "/data-dbs/database-1?agent_binding_action=unbind&impact_token=stale-token",
			params: gin.Params{{Key: "id", Value: "database-1"}},
			call:   func(handler *DataSourceHandler, ctx *gin.Context) { handler.DeleteDataSourceByID(ctx) },
			assertArgs: func(t *testing.T, service *dataSourceDeleteBindingService) {
				if service.databaseAction != "unbind" || service.databaseToken != "stale-token" {
					t.Fatalf("database confirmation = (%q, %q)", service.databaseAction, service.databaseToken)
				}
			},
			binding: agentbindings.BindingTypeDatabase,
		},
		{
			name:   "table",
			target: "/data-dbs/database-1/tables/table-1?agent_binding_action=unbind&impact_token=stale-token",
			params: gin.Params{{Key: "id", Value: "database-1"}, {Key: "table_id", Value: "table-1"}},
			call:   func(handler *DataSourceHandler, ctx *gin.Context) { handler.DeleteTable(ctx) },
			assertArgs: func(t *testing.T, service *dataSourceDeleteBindingService) {
				if service.tableAction != "unbind" || service.tableToken != "stale-token" {
					t.Fatalf("table confirmation = (%q, %q)", service.tableAction, service.tableToken)
				}
			},
			binding: agentbindings.BindingTypeDatabaseTable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &dataSourceDeleteBindingService{err: &agentbindings.ConflictError{Impact: agentbindings.Impact{
				Code:        agentbindings.ConflictCodeResourceBound,
				Operation:   "delete_" + test.name,
				BindingType: test.binding,
				ResourceID:  test.name + "-1",
				ImpactToken: "fresh-token",
			}}}
			handler := &DataSourceHandler{service: service}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodDelete, test.target, nil)
			ctx.Params = test.params
			ctx.Set("account_id", uuid.NewString())
			util.SetOrganizationScopeCompat(ctx, uuid.NewString())

			test.call(handler, ctx)

			test.assertArgs(t, service)
			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
