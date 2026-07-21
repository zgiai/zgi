package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/image/registry"
	imageservice "github.com/zgiai/zgi/api/internal/modules/image/service"
	"github.com/zgiai/zgi/api/internal/util"
)

type routeBoundaryImageService struct{}

func (routeBoundaryImageService) ListModels(context.Context, imageservice.Scope) ([]registry.ImageModel, error) {
	return []registry.ImageModel{}, nil
}

func (routeBoundaryImageService) Generate(context.Context, imageservice.Scope, imageservice.GenerateRequest) (*imageservice.GenerateResult, error) {
	return nil, nil
}

func TestRegisterRoutesAppliesWorkspaceMiddlewareOnlyToGenerate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	organizationID := uuid.NewString()
	accountID := uuid.NewString()
	router.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, organizationID)
		c.Set("account_id", accountID)
		c.Next()
	})

	workspaceMiddlewareCalls := 0
	workspaceRequired := func(c *gin.Context) {
		workspaceMiddlewareCalls++
		c.AbortWithStatus(http.StatusTeapot)
	}
	NewHandler(routeBoundaryImageService{}).RegisterRoutes(router.Group(""), workspaceRequired)

	modelsRecorder := httptest.NewRecorder()
	modelsRequest := httptest.NewRequest(http.MethodGet, "/image-runtime/models", nil)
	router.ServeHTTP(modelsRecorder, modelsRequest)
	if modelsRecorder.Code != http.StatusOK {
		t.Fatalf("models status = %d, want %d", modelsRecorder.Code, http.StatusOK)
	}
	if workspaceMiddlewareCalls != 0 {
		t.Fatalf("workspace middleware calls after models = %d, want 0", workspaceMiddlewareCalls)
	}

	generateRecorder := httptest.NewRecorder()
	generateRequest := httptest.NewRequest(http.MethodPost, "/image-runtime/generate", strings.NewReader(`{}`))
	generateRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(generateRecorder, generateRequest)
	if generateRecorder.Code != http.StatusTeapot {
		t.Fatalf("generate status = %d, want %d", generateRecorder.Code, http.StatusTeapot)
	}
	if workspaceMiddlewareCalls != 1 {
		t.Fatalf("workspace middleware calls after generate = %d, want 1", workspaceMiddlewareCalls)
	}
}
