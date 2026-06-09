package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestFailDatasetFolderReadMapsAccessDeniedToPermissionError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	failDatasetFolderRead(c, service.ErrDatasetAccessDenied)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	var body response.Response
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != strconv.Itoa(response.ErrDatasetPermissionDenied.Code) {
		t.Fatalf("code = %s, want %d", body.Code, response.ErrDatasetPermissionDenied.Code)
	}
}
