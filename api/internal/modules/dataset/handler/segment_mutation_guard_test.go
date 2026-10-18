package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestDisabledSegmentMutationsNeverReachDependencies(t *testing.T) {
	handler := &SegmentHandler{} // Nil dependencies must never be consulted.
	for _, test := range []struct {
		name string
		call gin.HandlerFunc
	}{
		{"delete segments", handler.DeleteDocumentSegments},
		{"create segment", handler.CreateDocumentSegment},
		{"update segment", handler.UpdateDocumentSegment},
		{"delete segment", handler.DeleteDocumentSegment},
		{"update status", handler.UpdateDocumentSegmentStatus},
		{"create child", handler.CreateChildChunk},
		{"update child", handler.UpdateChildChunk},
		{"delete child", handler.DeleteChildChunk},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			test.call(ctx)
			var result response.Response
			if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Code != "211004" || result.Message != "dataset segments must be edited from file management" {
				t.Fatalf("mutation response = %#v, want existing file-management guard", result)
			}
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("HTTP status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}
