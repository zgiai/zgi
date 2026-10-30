package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/response"
)

func hitTestingErrorProjector(t *testing.T) *apptransport.Projector {
	t.Helper()
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal,
		append(appcatalog.DefaultDefinitions(), datasetservice.CatalogDefinitions()...)...)
	if err != nil {
		t.Fatalf("compose catalog: %v", err)
	}
	projector, err := apptransport.NewProjector(catalog)
	if err != nil {
		t.Fatalf("projector: %v", err)
	}
	return projector
}

// An unrouted embedding model used to surface as HTTP 500 / 399001 with the raw
// nested error chain. It is now a cataloged, localized configuration error
// naming the dataset's model, and the upstream text is not returned.
func TestHitTestingUnroutedEmbeddingModelReturnsCatalogedError(t *testing.T) {
	handler := &DatasetHandler{}
	handler.SetErrorProjector(hitTestingErrorProjector(t))
	modelName := "text-embedding-v4"
	dataset := &dataset_model.Dataset{EmbeddingModel: &modelName}
	cause := fmt.Errorf("all retrieval methods failed: %w", errors.Join(
		fmt.Errorf("failed to embed query: failed to select provider: %w", llmerrors.NewModelNotFoundErrorWithName(modelName)),
	))

	c, recorder := newDatasetAccessTestContext("account-1", "org-1")
	c.Request.Header.Set("Accept-Language", "zh-CN")
	handler.respondHitTestingError(c, dataset, cause)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "dataset.embedding_model.unavailable" {
		t.Fatalf("code = %#v, want dataset.embedding_model.unavailable", body["code"])
	}
	message, _ := body["message"].(string)
	if !strings.Contains(message, modelName) || !strings.Contains(message, "重新索引") {
		t.Fatalf("message = %q, want the zh-Hans copy naming the model", message)
	}
	if strings.Contains(message, "failed to select provider") || strings.Contains(message, "all retrieval methods failed") {
		t.Fatalf("message leaks the diagnostic chain: %q", message)
	}
	if params, _ := body["params"].(map[string]interface{}); params["model"] != modelName {
		t.Fatalf("params = %#v, want model %s", body["params"], modelName)
	}
}

func TestHitTestingUnhandledErrorDoesNotLeakDiagnosticText(t *testing.T) {
	c, recorder := newDatasetAccessTestContext("account-1", "org-1")
	respondHitTestingError(c, &dataset_model.Dataset{}, errors.New("weaviate: dial tcp 10.0.0.5:8080: connection refused"))

	if strings.Contains(recorder.Body.String(), "10.0.0.5") {
		t.Fatalf("response leaks the cause: %s", recorder.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != strconv.Itoa(response.ErrSystemError.Code) {
		t.Fatalf("code = %#v, want legacy %d", body["code"], response.ErrSystemError.Code)
	}
}
