package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

type datasetDeleteBindingService struct {
	datasetservice.DatasetService
	dataset        *datasetmodel.Dataset
	action         string
	impactToken    string
	deleteResponse error
	previewImpact  *agentbindings.Impact
}

func (s *datasetDeleteBindingService) PreviewDatasetDeleteImpact(context.Context, string, string) (*agentbindings.Impact, error) {
	return s.previewImpact, nil
}

func (s *datasetDeleteBindingService) GetDatasetByID(context.Context, string) (*datasetmodel.Dataset, error) {
	return s.dataset, nil
}

func (s *datasetDeleteBindingService) DeleteDataset(_ context.Context, _, _, _, action, impactToken string) error {
	s.action = action
	s.impactToken = impactToken
	return s.deleteResponse
}

func TestDeleteDatasetPassesBindingConfirmationQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	datasetID := uuid.NewString()
	service := &datasetDeleteBindingService{dataset: &datasetmodel.Dataset{ID: datasetID, WorkspaceID: uuid.NewString()}}
	handler := &DatasetHandler{datasetService: service}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/datasets/"+datasetID+"?agent_binding_action=unbind&impact_token=confirmed-token", nil)
	ctx.Params = gin.Params{{Key: "dataset_id", Value: datasetID}}
	ctx.Set("account_id", uuid.NewString())
	ctx.Set("tenant_id", uuid.NewString())

	handler.DeleteDataset(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.action != "unbind" || service.impactToken != "confirmed-token" {
		t.Fatalf("binding confirmation = (%q, %q), want (unbind, confirmed-token)", service.action, service.impactToken)
	}
}

func TestDeleteDatasetReturnsStructuredBindingConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	datasetID := uuid.NewString()
	service := &datasetDeleteBindingService{
		dataset: &datasetmodel.Dataset{ID: datasetID, WorkspaceID: uuid.NewString()},
		deleteResponse: &agentbindings.ConflictError{Impact: agentbindings.Impact{
			Code:        agentbindings.ConflictCodeResourceBound,
			Operation:   "delete_dataset",
			BindingType: agentbindings.BindingTypeKnowledgeDataset,
			ResourceID:  datasetID,
			ImpactToken: "impact-token",
		}},
	}
	handler := &DatasetHandler{datasetService: service}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/datasets/"+datasetID, nil)
	ctx.Params = gin.Params{{Key: "dataset_id", Value: datasetID}}
	ctx.Set("account_id", uuid.NewString())
	ctx.Set("tenant_id", uuid.NewString())

	handler.DeleteDataset(ctx)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != agentbindings.ConflictCodeResourceBound {
		t.Fatalf("code = %q, want %q", payload.Code, agentbindings.ConflictCodeResourceBound)
	}
}

func TestPreviewDatasetDeleteImpactReturnsAgentPresentation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	datasetID := uuid.NewString()
	service := &datasetDeleteBindingService{previewImpact: &agentbindings.Impact{
		Code:        agentbindings.ConflictCodeResourceBound,
		Operation:   "delete_dataset",
		BindingType: agentbindings.BindingTypeKnowledgeDataset,
		ResourceID:  datasetID,
		ImpactToken: "impact-token",
		Agents: []agentbindings.ImpactAgent{{
			AgentID:     uuid.NewString(),
			Name:        "Customer assistant",
			Description: "Answers customer questions",
			IconType:    "text",
			Icon:        `{"icon":"CA"}`,
		}},
	}}
	handler := &DatasetHandler{datasetService: service}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/datasets/"+datasetID+"/delete-impact", nil)
	ctx.Params = gin.Params{{Key: "dataset_id", Value: datasetID}}
	ctx.Set("account_id", uuid.NewString())

	handler.PreviewDatasetDeleteImpact(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{"Customer assistant", "Answers customer questions", "impact-token"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %s, want %q", body, want)
		}
	}
}
