package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datasource/indexing"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"go.uber.org/zap"
)

func TestSchemaIndexHandlerRejectsUnauthorizedAccessWithoutTouchingStorage(t *testing.T) {
	productCatalog, err := catalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	h := NewSchemaIndexHandler(indexing.NewService(nil, nil, nil, nil, nil, nil, nil, zap.NewNop()), productCatalog)
	for _, endpoint := range []struct {
		method string
		handle func(*gin.Context)
	}{{http.MethodGet, h.Status}, {http.MethodPost, h.Start}, {http.MethodGet, h.History}} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(endpoint.method, "/", nil)
		c.Request.Header.Set("Accept-Language", "zh-Hans")
		c.Set("organization_id", uuid.NewString())
		c.Params = gin.Params{{Key: "id", Value: uuid.NewString()}}
		endpoint.handle(c)
		if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "auth.unauthenticated") {
			t.Fatalf("unsafe or incorrect response: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestSchemaIndexHandlerProjectsWorkspacePermissionDenial(t *testing.T) {
	productCatalog, err := catalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	h := NewSchemaIndexHandler(nil, productCatalog)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Accept-Language", "zh-Hans")

	h.fail(c, indexing.ErrAccessDenied)

	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "auth.forbidden") || strings.Contains(w.Body.String(), indexing.ErrAccessDenied.Error()) {
		t.Fatalf("unsafe or incorrect response: %d %s", w.Code, w.Body.String())
	}
}
