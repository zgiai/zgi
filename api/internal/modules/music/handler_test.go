package music

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestMusicScopeRejectsZeroIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope := testScope()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	util.SetOrganizationID(context, scope.OrganizationID.String())
	util.SetWorkspaceID(context, scope.WorkspaceID.String())
	context.Set("account_id", uuid.Nil.String())

	if _, ok := musicScope(context); ok {
		t.Fatal("musicScope() ok = true, want false")
	}
	if got, want := recorder.Code, http.StatusUnauthorized; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestHandlerCreatesTaskAsAcceptedAndReturnsScopedTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope := testScope()
	repo := newMemoryRepository()
	service := NewService(repo, &dispatcherStub{}, availableMusicModelStub(), &assetStoreStub{})
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, scope.OrganizationID.String())
		util.SetWorkspaceID(c, scope.WorkspaceID.String())
		c.Set("account_id", scope.AccountID.String())
		c.Next()
	})
	handler.RegisterRoutes(router.Group(""))

	request := httptest.NewRequest(http.MethodPost, "/music/tasks", bytes.NewBufferString(`{
		"request_id":"11111111-1111-1111-1111-111111111111",
		"model":"music-3.0",
		"mode":"instrumental",
		"prompt":"warm piano"
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if got, want := recorder.Code, http.StatusAccepted; got != want {
		t.Fatalf("POST status = %d, want %d; body=%s", got, want, recorder.Body.String())
	}
	var createBody struct {
		Data TaskView `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &createBody); err != nil {
		t.Fatal(err)
	}
	if createBody.Data.ID.String() == "" || createBody.Data.Status != StatusQueued {
		t.Fatalf("create response = %#v", createBody.Data)
	}
	if got, requestID := createBody.Data.ID.String(), "11111111-1111-1111-1111-111111111111"; got == requestID {
		t.Fatalf("task id = request id %q; internal billing identity must be server-generated", got)
	}

	getRequest := httptest.NewRequest(http.MethodGet, "/music/tasks/"+createBody.Data.ID.String(), nil)
	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, getRequest)
	if got, want := getRecorder.Code, http.StatusOK; got != want {
		t.Fatalf("GET status = %d, want %d; body=%s", got, want, getRecorder.Body.String())
	}
}

func TestHandlerListsOwnedTasksWithPaginationAndSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope := testScope()
	owned := queuedTask()
	owned.OrganizationID = scope.OrganizationID
	owned.WorkspaceID = scope.WorkspaceID
	owned.AccountID = scope.AccountID
	owned.Prompt = "warm piano"
	other := queuedTask()
	other.OrganizationID = scope.OrganizationID
	other.WorkspaceID = scope.WorkspaceID
	other.Prompt = "warm piano"
	service := NewService(newMemoryRepository(owned, other), &dispatcherStub{}, availableMusicModelStub(), &assetStoreStub{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, scope.OrganizationID.String())
		util.SetWorkspaceID(c, scope.WorkspaceID.String())
		c.Set("account_id", scope.AccountID.String())
		c.Next()
	})
	NewHandler(service).RegisterRoutes(router.Group(""))

	request := httptest.NewRequest(http.MethodGet, "/music/tasks?page=1&page_size=10&search=piano", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("GET list status = %d, want %d; body=%s", got, want, recorder.Body.String())
	}
	var response struct {
		Data TaskList `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.Total != 1 || len(response.Data.Items) != 1 || response.Data.Items[0].ID != owned.ID {
		t.Fatalf("GET list response = %#v", response.Data)
	}
}

func TestHandlerRejectsInvalidTaskListPagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope := testScope()
	service := NewService(newMemoryRepository(), &dispatcherStub{}, availableMusicModelStub(), &assetStoreStub{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, scope.OrganizationID.String())
		util.SetWorkspaceID(c, scope.WorkspaceID.String())
		c.Set("account_id", scope.AccountID.String())
		c.Next()
	})
	NewHandler(service).RegisterRoutes(router.Group(""))

	request := httptest.NewRequest(http.MethodGet, "/music/tasks?page=1&page_size=101", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if got, want := recorder.Code, http.StatusBadRequest; got != want {
		t.Fatalf("GET list status = %d, want %d; body=%s", got, want, recorder.Body.String())
	}
}

func TestHandlerRejectsRequestIDReusedWithDifferentPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope := testScope()
	service := NewService(newMemoryRepository(), &dispatcherStub{}, availableMusicModelStub(), &assetStoreStub{})
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, scope.OrganizationID.String())
		util.SetWorkspaceID(c, scope.WorkspaceID.String())
		c.Set("account_id", scope.AccountID.String())
		c.Next()
	})
	handler.RegisterRoutes(router.Group(""))

	const requestID = "22222222-2222-2222-2222-222222222222"
	first := httptest.NewRequest(http.MethodPost, "/music/tasks", bytes.NewBufferString(`{
		"request_id":"`+requestID+`",
		"model":"music-3.0",
		"mode":"instrumental",
		"prompt":"warm piano"
	}`))
	first.Header.Set("Content-Type", "application/json")
	firstRecorder := httptest.NewRecorder()
	router.ServeHTTP(firstRecorder, first)
	if got, want := firstRecorder.Code, http.StatusAccepted; got != want {
		t.Fatalf("first POST status = %d, want %d; body=%s", got, want, firstRecorder.Body.String())
	}

	second := httptest.NewRequest(http.MethodPost, "/music/tasks", bytes.NewBufferString(`{
		"request_id":"`+requestID+`",
		"model":"music-3.0",
		"mode":"instrumental",
		"prompt":"cold piano"
	}`))
	second.Header.Set("Content-Type", "application/json")
	secondRecorder := httptest.NewRecorder()
	router.ServeHTTP(secondRecorder, second)
	if got, want := secondRecorder.Code, http.StatusConflict; got != want {
		t.Fatalf("second POST status = %d, want %d; body=%s", got, want, secondRecorder.Body.String())
	}
	var response struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(secondRecorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if got, want := response.Code, "TASK_CONFLICT"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
	if got, want := response.Message, "Music request ID is already in use"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestHandlerRejectsOversizedCreateBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope := testScope()
	service := NewService(newMemoryRepository(), &dispatcherStub{}, availableMusicModelStub(), &assetStoreStub{})
	handler := NewHandler(service)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, scope.OrganizationID.String())
		util.SetWorkspaceID(c, scope.WorkspaceID.String())
		c.Set("account_id", scope.AccountID.String())
		c.Next()
	})
	handler.RegisterRoutes(router.Group(""))

	body := bytes.Repeat([]byte("x"), maxCreateRequestBytes+1)
	request := httptest.NewRequest(http.MethodPost, "/music/tasks", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if got, want := recorder.Code, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	var response struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if got, want := response.Message, "Invalid music generation request"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestHandlerRejectsDeletingActiveTask(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scope := testScope()
	task := queuedTask()
	task.OrganizationID = scope.OrganizationID
	task.WorkspaceID = scope.WorkspaceID
	task.AccountID = scope.AccountID
	service := NewService(newMemoryRepository(task), &dispatcherStub{}, availableMusicModelStub(), &assetStoreStub{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		util.SetOrganizationID(c, scope.OrganizationID.String())
		util.SetWorkspaceID(c, scope.WorkspaceID.String())
		c.Set("account_id", scope.AccountID.String())
		c.Next()
	})
	NewHandler(service).RegisterRoutes(router.Group(""))

	request := httptest.NewRequest(http.MethodDelete, "/music/tasks/"+task.ID.String(), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if got, want := recorder.Code, http.StatusConflict; got != want {
		t.Fatalf("DELETE status = %d, want %d; body=%s", got, want, recorder.Body.String())
	}
	var response struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != "TASK_NOT_DELETABLE" {
		t.Fatalf("DELETE code = %q, want TASK_NOT_DELETABLE", response.Code)
	}
}
