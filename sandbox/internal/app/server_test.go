package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/executor"
	"github.com/zgiai/zgi-sandbox/internal/lifecycle"
	"github.com/zgiai/zgi-sandbox/internal/observer"
	"github.com/zgiai/zgi-sandbox/internal/policy"
	"github.com/zgiai/zgi-sandbox/internal/runner"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
	"github.com/zgiai/zgi-sandbox/internal/testutil"
)

func TestHealthEndpoint(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "req_health")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("X-Request-ID") != "req_health" {
		t.Fatalf("expected request ID response header, got %q", rr.Header().Get("X-Request-ID"))
	}
	if !strings.Contains(rr.Body.String(), `"runtime_backend":"preview-process"`) {
		t.Fatalf("expected normalized runtime backend in health, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"environment":"local"`) {
		t.Fatalf("expected environment in health, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"network_policy_enforced":false`) {
		t.Fatalf("expected network enforcement flag in health, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"shutdown_timeout_secs":10`) {
		t.Fatalf("expected shutdown timeout in health, got %s", rr.Body.String())
	}
}

func TestReadyEndpoint(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"ready"`) {
		t.Fatalf("expected ready status, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"ready":true`) {
		t.Fatalf("expected ready flag, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"postgres":"ok"`) {
		t.Fatalf("expected postgres check, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"runtime":"ok"`) {
		t.Fatalf("expected runtime check, got %s", rr.Body.String())
	}
}

func TestReadyEndpointReportsStoreFailure(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}
	if err := server.store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"not_ready"`) {
		t.Fatalf("expected not ready status, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"ready":false`) {
		t.Fatalf("expected not ready flag, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"postgres":"error"`) {
		t.Fatalf("expected postgres error check, got %s", rr.Body.String())
	}
}

func TestReadyEndpointRequiresConfiguredDependencyProfiles(t *testing.T) {
	cfg := testConfig(t)
	cfg.RequiredDependencyProfiles = []string{"skill-office"}
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"dependency_profile:skill-office":"error"`) {
		t.Fatalf("expected dependency profile error check, got %s", rr.Body.String())
	}
}

func TestReadyEndpointPassesWithRequiredDependencyProfileArtifact(t *testing.T) {
	cfg := testConfig(t)
	cfg.DependencyRootFSDir = t.TempDir()
	cfg.RequiredDependencyProfiles = []string{"skill-office"}
	writeServerDependencyProfileArtifact(t, cfg.DependencyRootFSDir, "skill-office")
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"dependency_profile:skill-office":"ok"`) {
		t.Fatalf("expected dependency profile ok check, got %s", rr.Body.String())
	}
}

func TestReadyEndpointRejectsNonGet(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/ready", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRequestIDMiddlewareGeneratesRequestID(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	requestID := rr.Header().Get("X-Request-ID")
	if !strings.HasPrefix(requestID, "req_") {
		t.Fatalf("expected generated request ID header, got %q", requestID)
	}
}

func TestRequestIDMiddlewareSanitizesRequestID(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", " req_bad\nvalue\t ")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Header().Get("X-Request-ID") != "req_badvalue" {
		t.Fatalf("expected sanitized request ID, got %q", rr.Header().Get("X-Request-ID"))
	}
}

func TestRunEndpoint(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/run", strings.NewReader(`{"language":"python3","code":"print('ok')"}`))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid json, got %v", err)
	}

	if payload["message"] != "success" {
		t.Fatalf("expected success message, got %#v", payload["message"])
	}
}

func TestRunEndpointRejectsPreviewNetworkRequest(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/run", strings.NewReader(`{"language":"python3","code":"print('blocked')","enable_network":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req_policy_network_test")

	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "does not enforce network policy") {
		t.Fatalf("expected network enforcement error, got %s", rr.Body.String())
	}

	events := server.observer.Query(observer.Query{Type: "policy.denied", RequestID: "req_policy_network_test", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected one policy deny event, got %d", len(events))
	}
	if events[0].Metadata["code"] != "network_policy_not_enforced" {
		t.Fatalf("expected network policy deny code, got %+v", events[0].Metadata)
	}
	if events[0].Metadata["runtime_backend"] != "preview-process" {
		t.Fatalf("expected runtime backend metadata, got %+v", events[0].Metadata)
	}
}

func TestEgressCheckDeniesSandboxNetworkDisabledAndRecordsDecision(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{
		"runtime_profile":"session",
		"ttl_seconds":60,
		"organization_id":"organization-egress",
		"workspace_id":"workspace-egress",
		"network_enabled":false,
		"network_policy":"workflow-safe"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox create to return 200, got %d body=%s", createRes.Code, createRes.Body.String())
	}

	var createPayload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("expected sandbox create payload, got %v", err)
	}

	checkReq := httptest.NewRequest(http.MethodPost, "/v1/network/egress/check", strings.NewReader(fmt.Sprintf(`{
		"sandbox_id":%q,
		"organization_id":"organization-egress",
		"destination":"https://example.com/resource"
	}`, createPayload.Data.ID)))
	checkReq.Header.Set("Content-Type", "application/json")
	checkReq.Header.Set("X-Request-ID", "req_egress_disabled_test")
	checkRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(checkRes, checkReq)
	if checkRes.Code != http.StatusOK {
		t.Fatalf("expected egress check to return 200, got %d body=%s", checkRes.Code, checkRes.Body.String())
	}
	for _, expected := range []string{
		`"allowed":false`,
		`"code":"egress_denied_sandbox_network_disabled"`,
		`"policy":"workflow-safe"`,
		`"destination":"https://example.com/resource"`,
	} {
		if !strings.Contains(checkRes.Body.String(), expected) {
			t.Fatalf("expected egress check response to include %s, got %s", expected, checkRes.Body.String())
		}
	}

	events := server.observer.Query(observer.Query{SandboxID: createPayload.Data.ID, Type: "network.egress.decision", RequestID: "req_egress_disabled_test", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected one egress decision event, got %d", len(events))
	}
	if fmt.Sprint(events[0].Metadata["allowed"]) != "false" || events[0].Metadata["code"] != "egress_denied_sandbox_network_disabled" {
		t.Fatalf("expected sandbox network disabled decision metadata, got %+v", events[0].Metadata)
	}
	if events[0].Metadata["organization_id"] != "organization-egress" || events[0].Metadata["workspace_id"] != "workspace-egress" {
		t.Fatalf("expected ownership metadata on egress decision, got %+v", events[0].Metadata)
	}
}

func TestEgressCheckRejectsMissingDestination(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/network/egress/check", strings.NewReader(`{"sandbox_id":"sbx_missing","destination":" "}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "destination is required") {
		t.Fatalf("expected destination validation error, got %s", rr.Body.String())
	}
}

func TestExecCodeRejectsOversizedShortCodeRequestBeforeFullDecode(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	body := `{"language":"python3","profile":"code-short","code":"` + strings.Repeat("x", 140*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/exec/code", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "request body exceeds max size of 131072 bytes") {
		t.Fatalf("expected profile-specific request limit message, got %s", rr.Body.String())
	}
}

func TestServerRejectsProductionPreviewBackend(t *testing.T) {
	cfg := testConfig(t)
	cfg.Environment = "production"
	cfg.RuntimeBackend = "preview"

	if _, err := NewServer(cfg); err == nil {
		t.Fatal("expected production preview backend to be rejected")
	}
}

func TestSandboxListEndpoint(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60,"organization_id":"organization-api","workspace_id":"workspace-api","app_id":"app-api","workflow_run_id":"run-api","user_id":"user-api"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)

	if createRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox create to return 200, got %d", createRes.Code)
	}
	if !strings.Contains(createRes.Body.String(), `"effective_limits"`) {
		t.Fatalf("expected sandbox create response to include effective limits, got %s", createRes.Body.String())
	}
	for _, expected := range []string{
		`"organization_id":"organization-api"`,
		`"workspace_id":"workspace-api"`,
		`"app_id":"app-api"`,
		`"workflow_run_id":"run-api"`,
		`"user_id":"user-api"`,
	} {
		if !strings.Contains(createRes.Body.String(), expected) {
			t.Fatalf("expected create response to include %s, got %s", expected, createRes.Body.String())
		}
	}
	var createPayload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("expected sandbox create payload, got %v", err)
	}
	createdEvents := server.observer.Query(observer.Query{SandboxID: createPayload.Data.ID, Type: "sandbox.created", Limit: 1})
	if len(createdEvents) != 1 {
		t.Fatalf("expected sandbox created event, got %#v", createdEvents)
	}
	if createdEvents[0].Metadata["runtime_backend"] != "preview-process" || createdEvents[0].Metadata["runtime_profile"] != "session" {
		t.Fatalf("expected runtime metadata on sandbox created event, got %+v", createdEvents[0].Metadata)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/sandboxes", nil)
	listRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRes, listReq)

	if listRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox list to return 200, got %d", listRes.Code)
	}
	if !strings.Contains(listRes.Body.String(), `"organization_id":"organization-api"`) {
		t.Fatalf("expected sandbox list to include ownership fields, got %s", listRes.Body.String())
	}
}

func TestSandboxCreateRejectsInvalidOwnershipField(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","organization_id":"organization api"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)

	if createRes.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid ownership field to return 400, got %d body=%s", createRes.Code, createRes.Body.String())
	}
	if !strings.Contains(createRes.Body.String(), "organization_id contains invalid characters") {
		t.Fatalf("expected ownership validation error, got %s", createRes.Body.String())
	}
}

func TestSandboxOperationsRejectCrossOrganizationAccess(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60,"organization_id":"organization-one"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox create to return 200, got %d body=%s", createRes.Code, createRes.Body.String())
	}
	var createBody struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createBody); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createBody.Data.ID == "" {
		t.Fatalf("expected sandbox id, got %s", createRes.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/sandboxes/"+createBody.Data.ID+"?organization_id=organization-two", nil)
	getReq.Header.Set("X-Request-ID", "req_cross_organization_get_test")
	getRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRes, getReq)
	assertCrossOrganizationAccessDenied(t, getRes)

	codeReq := httptest.NewRequest(http.MethodPost, "/v1/exec/code", strings.NewReader(fmt.Sprintf(`{"sandbox_id":%q,"organization_id":"organization-two","language":"python3","code":"print('blocked')"}`, createBody.Data.ID)))
	codeReq.Header.Set("Content-Type", "application/json")
	codeRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(codeRes, codeReq)
	assertCrossOrganizationAccessDenied(t, codeRes)

	fileReq := httptest.NewRequest(http.MethodGet, "/v1/files/tree?sandbox_id="+url.QueryEscape(createBody.Data.ID), nil)
	fileReq.Header.Set("X-ZGI-Organization-ID", "organization-two")
	fileRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(fileRes, fileReq)
	assertCrossOrganizationAccessDenied(t, fileRes)

	listOtherReq := httptest.NewRequest(http.MethodGet, "/v1/sandboxes?organization_id=organization-two", nil)
	listOtherRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(listOtherRes, listOtherReq)
	if listOtherRes.Code != http.StatusOK {
		t.Fatalf("expected cross organization sandbox list to return 200, got %d body=%s", listOtherRes.Code, listOtherRes.Body.String())
	}
	if strings.Contains(listOtherRes.Body.String(), createBody.Data.ID) {
		t.Fatalf("expected cross organization sandbox list to hide sandbox, got %s", listOtherRes.Body.String())
	}

	allowedReq := httptest.NewRequest(http.MethodGet, "/v1/sandboxes/"+createBody.Data.ID+"?organization_id=organization-one", nil)
	allowedRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(allowedRes, allowedReq)
	if allowedRes.Code != http.StatusOK {
		t.Fatalf("expected matching organization access to return 200, got %d body=%s", allowedRes.Code, allowedRes.Body.String())
	}

	events := server.observer.Query(observer.Query{SandboxID: createBody.Data.ID, Type: "policy.denied", RequestID: "req_cross_organization_get_test", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected one cross organization policy deny event, got %d", len(events))
	}
	if events[0].Metadata["code"] != "cross_organization_sandbox_access_denied" {
		t.Fatalf("expected cross organization policy deny code, got %+v", events[0].Metadata)
	}
	if events[0].Metadata["organization_id"] != "organization-one" || events[0].Metadata["requested_organization_id"] != "organization-two" {
		t.Fatalf("expected owner and requested organization metadata, got %+v", events[0].Metadata)
	}
}

func assertCrossOrganizationAccessDenied(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected cross organization access to return 403, got %d body=%s", rr.Code, rr.Body.String())
	}
	for _, expected := range []string{
		`"code":"cross_organization_sandbox_access_denied"`,
		`"organization_id":"organization-two"`,
		"sandbox does not belong to organization",
	} {
		if !strings.Contains(rr.Body.String(), expected) {
			t.Fatalf("expected cross organization response to include %s, got %s", expected, rr.Body.String())
		}
	}
}

func TestTemplateEndpointRendersAndRejectsUnsafeHelpers(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/exec/template", strings.NewReader(`{"template":"Hello {{ upper .name }}","variables":{"name":"zgi"}}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected template render to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"content":"Hello ZGI"`) {
		t.Fatalf("expected rendered content, got %s", rr.Body.String())
	}

	rejectReq := httptest.NewRequest(http.MethodPost, "/v1/exec/template", strings.NewReader(`{"template":"{{ env \"HOME\" }}","variables":{}}`))
	rejectReq.Header.Set("Content-Type", "application/json")
	rejectRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(rejectRes, rejectReq)
	if rejectRes.Code != http.StatusBadRequest {
		t.Fatalf("expected unsafe helper to return 400, got %d body=%s", rejectRes.Code, rejectRes.Body.String())
	}
}

func TestSkillEndpointRunsManifestPackage(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox create to return 200, got %d body=%s", createRes.Code, createRes.Body.String())
	}
	var createBody struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createBody); err != nil {
		t.Fatalf("decode sandbox create: %v", err)
	}

	archive := testZipBase64(t, map[string]string{
		"SKILL.md":       "skill",
		"scripts/run.py": "import json, os, sys\npayload = json.loads(sys.stdin.read() or '{}')\nos.makedirs('artifacts', exist_ok=True)\nopen('artifacts/report.txt', 'w').write('api artifact\\n')\nprint(json.dumps({'ok': True, 'input': payload.get('input')}))\n",
		"skill.manifest.json": `{
  "entrypoint": "scripts/run.py",
  "language": "python3",
  "timeout_ms": 30000,
  "allowed_artifact_paths": ["artifacts"],
  "max_artifact_count": 10,
  "max_artifact_bytes": 32768,
  "result_mode": "mixed"
}`,
	})
	uploadBody := fmt.Sprintf(`{"sandbox_id":%q,"path":"skills/api","archive_base64":%q,"format":"zip","validate_skill_manifest":true}`, createBody.Data.ID, archive)
	uploadReq := httptest.NewRequest(http.MethodPost, "/v1/files/upload-archive", strings.NewReader(uploadBody))
	uploadReq.Header.Set("Content-Type", "application/json")
	uploadRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(uploadRes, uploadReq)
	if uploadRes.Code != http.StatusOK {
		t.Fatalf("expected archive upload to return 200, got %d body=%s", uploadRes.Code, uploadRes.Body.String())
	}

	runReq := httptest.NewRequest(http.MethodPost, "/v1/exec/skill", strings.NewReader(fmt.Sprintf(`{"sandbox_id":%q,"path":"skills/api","input_json":{"input":"api"}}`, createBody.Data.ID)))
	runReq.Header.Set("Content-Type", "application/json")
	runRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(runRes, runReq)
	if runRes.Code != http.StatusOK {
		t.Fatalf("expected skill execution to return 200, got %d body=%s", runRes.Code, runRes.Body.String())
	}
	for _, expected := range []string{`"entrypoint":"scripts/run.py"`, `"artifact_manifests"`, `"result_json":{"input":"api","ok":true}`} {
		if !strings.Contains(runRes.Body.String(), expected) {
			t.Fatalf("expected skill response to include %s, got %s", expected, runRes.Body.String())
		}
	}
}

func TestEgressCheckRecordsPolicyDecision(t *testing.T) {
	recorder := observer.NewRecorder(100)
	cfg := config.FromEnv()
	cfg.RuntimeBackend = "linux-secure"
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManager(recorder, policyService)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	server := &Server{
		config:    cfg,
		lifecycle: manager,
		observer:  recorder,
		policy:    policyService,
	}

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile:    string(sandbox.RuntimeSession),
		NetworkEnabled:    true,
		NetworkPolicy:     "workflow-safe",
		DependencyProfile: "stdlib",
		OrganizationID:    "organization-egress",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/network/egress/check", strings.NewReader(fmt.Sprintf(`{"sandbox_id":%q,"organization_id":"organization-egress","destination":"https://127.0.0.1"}`, box.ID)))
	req = req.WithContext(observer.ContextWithRequestID(req.Context(), "req_egress_check"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req_egress_check")
	rr := httptest.NewRecorder()
	server.handleEgressCheck(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected egress check to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"allowed":false`) || !strings.Contains(rr.Body.String(), `"code":"egress_denied_cidr"`) || !strings.Contains(rr.Body.String(), `"denied_cidr":"127.0.0.0/8"`) {
		t.Fatalf("expected denied CIDR decision, got %s", rr.Body.String())
	}

	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "network.egress.decision", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected egress decision event, got %d", len(events))
	}
	if fmt.Sprint(events[0].Metadata["allowed"]) != "false" || events[0].Metadata["code"] != "egress_denied_cidr" || events[0].Metadata["denied_cidr"] != "127.0.0.0/8" {
		t.Fatalf("unexpected egress event metadata: %#v", events[0].Metadata)
	}
	if events[0].Metadata["organization_id"] != "organization-egress" || events[0].Metadata["request_id"] != "req_egress_check" {
		t.Fatalf("expected ownership and request metadata, got %#v", events[0].Metadata)
	}
}

func TestEgressProxyDeniesSandboxNetworkDisabledAndRecordsDecision(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{
		"runtime_profile":"session",
		"ttl_seconds":60,
		"organization_id":"organization-egress-proxy",
		"network_enabled":false,
		"network_policy":"workflow-safe"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox create to return 200, got %d body=%s", createRes.Code, createRes.Body.String())
	}

	var createPayload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("expected sandbox create payload, got %v", err)
	}

	proxyReq := httptest.NewRequest(http.MethodPost, "/v1/network/egress/proxy", strings.NewReader(fmt.Sprintf(`{
		"sandbox_id":%q,
		"organization_id":"organization-egress-proxy",
		"destination":"https://example.com/resource"
	}`, createPayload.Data.ID)))
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("X-Request-ID", "req_egress_proxy_disabled_test")
	proxyRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(proxyRes, proxyReq)
	if proxyRes.Code != http.StatusForbidden {
		t.Fatalf("expected egress proxy to return 403, got %d body=%s", proxyRes.Code, proxyRes.Body.String())
	}
	if !strings.Contains(proxyRes.Body.String(), `"code":"egress_denied_sandbox_network_disabled"`) {
		t.Fatalf("expected network disabled decision in response, got %s", proxyRes.Body.String())
	}

	events := server.observer.Query(observer.Query{SandboxID: createPayload.Data.ID, Type: "network.egress.decision", RequestID: "req_egress_proxy_disabled_test", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected one egress decision event, got %d", len(events))
	}
	if fmt.Sprint(events[0].Metadata["allowed"]) != "false" || events[0].Metadata["code"] != "egress_denied_sandbox_network_disabled" {
		t.Fatalf("expected denied egress proxy decision metadata, got %+v", events[0].Metadata)
	}
}

func TestExecuteEgressProxyRoutesApprovedRequestAndCapsResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "text/plain" {
			t.Fatalf("expected proxied content type, got %q", r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read upstream request body: %v", err)
		}
		if string(body) != "payload!" {
			t.Fatalf("expected proxied body, got %q", string(body))
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Upstream", "ok")
		_, _ = w.Write([]byte("response-over-limit"))
	}))
	defer upstream.Close()

	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse upstream port: %v", err)
	}
	cfg := testConfig(t)
	cfg.EgressProxyMaxBodyBytes = 8
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	box := &sandbox.Sandbox{ID: "sbx_proxy_unit"}
	decision := policy.EgressDecision{
		Allowed:              true,
		Code:                 "egress_allowed",
		Reason:               "network policy allows destination",
		Policy:               "workflow-safe",
		Destination:          upstream.URL + "/resource",
		Protocol:             "http",
		Host:                 parsed.Hostname(),
		Port:                 port,
		ResolvedIPs:          []string{parsed.Hostname()},
		MaxRequestDurationMS: 1000,
	}
	result, err := server.executeEgressProxy(context.Background(), box, egressProxyRequest{
		Method:      "POST",
		Destination: decision.Destination,
		Headers:     map[string]string{"Content-Type": "text/plain", "Connection": "blocked"},
		Body:        "payload!",
	}, decision)
	if err != nil {
		t.Fatalf("execute egress proxy: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected upstream status 200, got %d", result.StatusCode)
	}
	if result.Body != "response" || result.BodyEncoding != "text" || !result.Truncated || result.BodyBytes != 8 {
		t.Fatalf("expected capped text response, got %+v", result)
	}
	if result.Headers["Content-Type"] != "text/plain" || result.ContentType != "text/plain" {
		t.Fatalf("expected safe response headers, got %+v content_type=%s", result.Headers, result.ContentType)
	}
	if _, ok := result.Headers["X-Upstream"]; ok {
		t.Fatalf("unexpected non-allowlisted response header: %+v", result.Headers)
	}
}

func TestEgressProxyEnforcesOrganizationNetworkRequestRate(t *testing.T) {
	cfg := config.FromEnv()
	cfg.MaxNetworkRequestsPerMinutePerOrganization = 1
	recorder := observer.NewRecorder(100)
	server := &Server{
		config:   cfg,
		observer: recorder,
		policy:   policy.NewService(cfg),
	}
	box := &sandbox.Sandbox{
		ID:             "sbx_network_rate",
		OrganizationID: "organization-network-rate",
	}
	if err := server.enforceOrganizationNetworkRequestRate(box); err != nil {
		t.Fatalf("expected first network request to pass, got %v", err)
	}
	recorder.Record("network.egress.proxy", box.ID, "network egress proxied", map[string]any{
		"organization_id": box.OrganizationID,
		"status":          "success",
	})

	err := server.enforceOrganizationNetworkRequestRate(box)
	var limitErr *policy.LimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("expected network request rate limit error, got %T %v", err, err)
	}
	if limitErr.Code != "organization_network_request_rate_limit_exceeded" || limitErr.Limit != "max_network_requests_per_minute_per_organization" {
		t.Fatalf("unexpected network request rate limit error: %+v", limitErr)
	}
	if limitErr.Details["organization_id"] != box.OrganizationID || limitErr.Details["recent_network_requests"] != 1 {
		t.Fatalf("unexpected network request rate details: %+v", limitErr.Details)
	}
	if err := server.enforceOrganizationNetworkRequestRate(&sandbox.Sandbox{ID: "sbx_other", OrganizationID: "organization-other"}); err != nil {
		t.Fatalf("expected other organization to have separate network request quota, got %v", err)
	}
}

func TestEgressProxyEndpointRejectsOrganizationNetworkRequestRateLimit(t *testing.T) {
	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.RuntimeBackend = "linux-secure"
	cfg.MaxNetworkRequestsPerMinutePerOrganization = 1
	cfg.ProxyTimeout = 1
	recorder := observer.NewRecorder(100)
	policyService := policy.NewService(cfg)
	manager, err := lifecycle.NewManagerWithConfig(recorder, policyService, cfg, nil, nil)
	if err != nil {
		t.Fatalf("expected lifecycle manager, got %v", err)
	}
	server := &Server{
		config:    cfg,
		lifecycle: manager,
		observer:  recorder,
		policy:    policyService,
		mux:       http.NewServeMux(),
	}
	server.registerRoutes()

	box, err := manager.Create(lifecycle.CreateRequest{
		RuntimeProfile:    string(sandbox.RuntimeSession),
		OrganizationID:    "organization-network-rate-http",
		NetworkEnabled:    true,
		NetworkPolicy:     "workflow-safe",
		DependencyProfile: "stdlib",
	})
	if err != nil {
		t.Fatalf("expected sandbox create, got %v", err)
	}
	recorder.Record("network.egress.proxy", box.ID, "network egress proxied", map[string]any{
		"organization_id": box.OrganizationID,
		"status":          "success",
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/network/egress/proxy", strings.NewReader(fmt.Sprintf(`{
		"sandbox_id":%q,
		"organization_id":%q,
		"destination":"https://93.184.216.34/resource"
	}`, box.ID, box.OrganizationID)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req_network_rate_http")
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("expected egress proxy to return 429, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), `"code":"organization_network_request_rate_limit_exceeded"`) {
		t.Fatalf("expected network request rate limit response, got %s", res.Body.String())
	}
	events := recorder.Query(observer.Query{SandboxID: box.ID, Type: "network.egress.proxy.failed", RequestID: "req_network_rate_http", Limit: 1})
	if len(events) != 1 {
		t.Fatalf("expected one egress proxy failure event, got %d", len(events))
	}
	if events[0].Metadata["code"] != "organization_network_request_rate_limit_exceeded" || events[0].Metadata["organization_id"] != box.OrganizationID {
		t.Fatalf("expected structured network rate metadata, got %#v", events[0].Metadata)
	}
}

func TestFileManifestEndpointEnforcesArtifactLimits(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox create to return 200, got %d body=%s", createRes.Code, createRes.Body.String())
	}
	var createPayload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("expected create payload, got %v", err)
	}

	uploadReq := httptest.NewRequest(http.MethodPost, "/v1/files/upload", strings.NewReader(fmt.Sprintf(`{"sandbox_id":%q,"path":"artifacts/report.txt","content":"hello manifest"}`, createPayload.Data.ID)))
	uploadReq.Header.Set("Content-Type", "application/json")
	uploadRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(uploadRes, uploadReq)
	if uploadRes.Code != http.StatusOK {
		t.Fatalf("expected upload to return 200, got %d body=%s", uploadRes.Code, uploadRes.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/files/manifest?sandbox_id="+url.QueryEscape(createPayload.Data.ID)+"&path=artifacts&max_total_bytes=4", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected artifact byte limit to return 429, got %d body=%s", rr.Code, rr.Body.String())
	}
	for _, expected := range []string{
		`"error_type":"limit_exceeded"`,
		`"code":"artifact_manifest_total_bytes_exceeded"`,
		`"limit":"max_artifact_manifest_total_bytes"`,
	} {
		if !strings.Contains(rr.Body.String(), expected) {
			t.Fatalf("expected response to include %s, got %s", expected, rr.Body.String())
		}
	}
}

func TestSandboxCreateReturnsStructuredLimitError(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxActive = 1
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(firstRes, firstReq)
	if firstRes.Code != http.StatusOK {
		t.Fatalf("expected first sandbox create to return 200, got %d body=%s", firstRes.Code, firstRes.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(secondRes, secondReq)
	if secondRes.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second sandbox create to return 429, got %d body=%s", secondRes.Code, secondRes.Body.String())
	}
	if !strings.Contains(secondRes.Body.String(), `"error_type":"limit_exceeded"`) {
		t.Fatalf("expected structured limit error, got %s", secondRes.Body.String())
	}
	if !strings.Contains(secondRes.Body.String(), `"limit":"max_active_sandboxes"`) {
		t.Fatalf("expected max active limit details, got %s", secondRes.Body.String())
	}
}

func TestSandboxCreateReturnsStructuredOrganizationLimitError(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxActive = 10
	cfg.MaxActivePerOrganization = 1
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	firstReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60,"organization_id":"organization-api"}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(firstRes, firstReq)
	if firstRes.Code != http.StatusOK {
		t.Fatalf("expected first organization sandbox create to return 200, got %d body=%s", firstRes.Code, firstRes.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60,"organization_id":"organization-api"}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(secondRes, secondReq)
	if secondRes.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second organization sandbox create to return 429, got %d body=%s", secondRes.Code, secondRes.Body.String())
	}
	for _, expected := range []string{
		`"error_type":"limit_exceeded"`,
		`"code":"organization_active_sandbox_limit_exceeded"`,
		`"limit":"max_active_sandboxes_per_organization"`,
		`"organization_id":"organization-api"`,
	} {
		if !strings.Contains(secondRes.Body.String(), expected) {
			t.Fatalf("expected organization limit response to include %s, got %s", expected, secondRes.Body.String())
		}
	}

	thirdReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60,"organization_id":"organization-other"}`))
	thirdReq.Header.Set("Content-Type", "application/json")
	thirdRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(thirdRes, thirdReq)
	if thirdRes.Code != http.StatusOK {
		t.Fatalf("expected other organization sandbox create to return 200, got %d body=%s", thirdRes.Code, thirdRes.Body.String())
	}
}

func TestWriteKnownErrorMapsQueueTimeout(t *testing.T) {
	rr := httptest.NewRecorder()

	writeKnownError(rr, &runner.QueueTimeoutError{TimeoutMS: 50})

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"code":"execution_queue_timeout"`) {
		t.Fatalf("expected queue timeout details, got %s", rr.Body.String())
	}
}

func TestWriteKnownErrorIncludesPolicyDetails(t *testing.T) {
	rr := httptest.NewRecorder()

	writeKnownError(rr, &executor.DependencyInstallError{PackageManager: "pip", Action: "install"})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	for _, expected := range []string{`"error_type":"policy_denied"`, `"code":"dependency_install_disabled"`, `"package_manager":"pip"`, `"action":"install"`} {
		if !strings.Contains(rr.Body.String(), expected) {
			t.Fatalf("expected %s in response, got %s", expected, rr.Body.String())
		}
	}
}

func TestWriteKnownErrorMapsCancellation(t *testing.T) {
	rr := httptest.NewRecorder()

	writeKnownError(rr, &runner.CancellationError{Phase: "execution"})

	if rr.Code != 499 {
		t.Fatalf("expected 499, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"code":-499`) {
		t.Fatalf("expected cancellation code, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"error_type":"execution_canceled"`) {
		t.Fatalf("expected cancellation details, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"phase":"execution"`) {
		t.Fatalf("expected cancellation phase, got %s", rr.Body.String())
	}
}

func TestUploadFileRejectsOversizedRequestBody(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxFileSizeKB = 1
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	body := `{"sandbox_id":"sbx_missing","path":"too-large.txt","content":"` + strings.Repeat("x", 80*1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/files/upload", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDependencyEndpoint(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sandbox/dependencies?language=python3", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected dependency list to return 200, got %d", rr.Code)
	}

	if !strings.Contains(rr.Body.String(), "workflow-safe") {
		t.Fatalf("expected managed dependency profile in response, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"version":"2026.05.01"`) {
		t.Fatalf("expected versioned dependency profiles in response, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"ready"`) {
		t.Fatalf("expected dependency profile status in response, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"enabled":true`) {
		t.Fatalf("expected dependency profile enabled flag in response, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"package_policy"`) || !strings.Contains(rr.Body.String(), `"default_action":"deny-unlisted"`) {
		t.Fatalf("expected dependency package policy in response, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"build_policy"`) || !strings.Contains(rr.Body.String(), `"build_timeout_seconds":600`) {
		t.Fatalf("expected dependency build policy in response, got %s", rr.Body.String())
	}
}

func TestDependencyPrepareScansArchive(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "prepare-test-key"
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	payload, err := json.Marshal(map[string]any{
		"archive_base64": testZipBase64(t, map[string]string{
			"SKILL.md": "Skill package\n",
			"skill.manifest.json": `{
				"entrypoint": "scripts/run.py",
				"language": "python3",
				"dependencies": {
					"python": ["pydantic==2.7.4"]
				}
			}`,
			"requirements.txt": "pandas==2.2.3\n",
			"scripts/run.py":   "import json\nfrom PIL import Image\n",
		}),
		"format":       "zip",
		"base_runtime": "linux-secure",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/prepare", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "prepare-test-key")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected dependency prepare to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Code int                              `json:"code"`
		Data executor.DependencyPrepareResult `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 || body.Data.Status != "build_required" {
		t.Fatalf("expected build_required response, got %#v", body)
	}
	if body.Data.Fingerprint == "" || !strings.HasPrefix(body.Data.Fingerprint, "sha256:") {
		t.Fatalf("expected dependency fingerprint, got %#v", body.Data)
	}
	if body.Data.Request.BaseRuntime != "linux-secure" || body.Data.Request.Language != "python3" {
		t.Fatalf("unexpected normalized dependency request: %#v", body.Data.Request)
	}
	if !strings.Contains(rr.Body.String(), `"name":"pandas"`) ||
		!strings.Contains(rr.Body.String(), `"name":"pydantic"`) ||
		!strings.Contains(rr.Body.String(), `"name":"pillow"`) ||
		!strings.Contains(rr.Body.String(), `"dependency_request"`) {
		t.Fatalf("expected scanned dependencies in response, got %s", rr.Body.String())
	}
}

func TestDependencyPrepareRequiresAPIKey(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "prepare-test-key"
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/prepare", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected prepare endpoint to require api key, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDependencyBuildQueuesPreparedArchive(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "build-test-key"
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	payload, err := json.Marshal(map[string]any{
		"organization_id": "organization-build-test",
		"archive_base64": testZipBase64(t, map[string]string{
			"SKILL.md":         "Skill package\n",
			"requirements.txt": "pandas==2.2.3\n",
			"scripts/run.py":   "import pandas as pd\nprint('ok')\n",
		}),
		"format":       "zip",
		"base_runtime": "linux-secure",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/builds", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "build-test-key")
	req.Header.Set("X-Request-ID", "req_dependency_build_queue_test")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected dependency build queue to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			BuildID        string `json:"build_id"`
			Fingerprint    string `json:"fingerprint"`
			Status         string `json:"status"`
			ProfileName    string `json:"profile_name"`
			OrganizationID string `json:"organization_id"`
			NextAction     string `json:"next_action"`
			PackageCount   int    `json:"package_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 || body.Data.Status != "queued" || body.Data.NextAction != "wait_for_dependency_build" {
		t.Fatalf("expected queued dependency build, got %#v", body)
	}
	if body.Data.BuildID == "" || body.Data.Fingerprint == "" || !strings.HasPrefix(body.Data.ProfileName, "auto-") {
		t.Fatalf("expected build identifiers, got %#v", body.Data)
	}
	if body.Data.OrganizationID != "organization-build-test" || body.Data.PackageCount != 1 {
		t.Fatalf("unexpected queued build response: %#v", body.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/sandbox/dependencies/builds?fingerprint="+url.QueryEscape(body.Data.Fingerprint), nil)
	getReq.Header.Set("X-API-Key", "build-test-key")
	getRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("expected dependency build lookup to return 200, got %d body=%s", getRes.Code, getRes.Body.String())
	}
	if !strings.Contains(getRes.Body.String(), body.Data.BuildID) || !strings.Contains(getRes.Body.String(), `"status":"queued"`) {
		t.Fatalf("expected queued build lookup response, got %s", getRes.Body.String())
	}

	events := server.observer.Query(observer.Query{Type: "dependency_build.queued", RequestID: "req_dependency_build_queue_test", Limit: 1})
	if len(events) != 1 || events[0].Metadata["fingerprint"] != body.Data.Fingerprint {
		t.Fatalf("expected dependency build queued event, got %#v", events)
	}
}

func TestDependencyBuildRunMaterializesReusableProfile(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "build-test-key"
	cfg.DependencyRootFSDir = t.TempDir()
	cfg.DependencyBuildCommand = "python3 " + writeDependencyBuildStubCommand(t)
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	payload, err := json.Marshal(map[string]any{
		"archive_base64": testZipBase64(t, map[string]string{
			"SKILL.md":         "Skill package\n",
			"requirements.txt": "pandas==2.2.3\n",
			"scripts/run.py":   "import pandas as pd\nprint('ok')\n",
		}),
		"format":       "zip",
		"base_runtime": "linux-secure",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	queueReq := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/builds", bytes.NewReader(payload))
	queueReq.Header.Set("Content-Type", "application/json")
	queueReq.Header.Set("X-API-Key", "build-test-key")
	queueRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(queueRes, queueReq)
	if queueRes.Code != http.StatusOK {
		t.Fatalf("expected dependency build queue to return 200, got %d body=%s", queueRes.Code, queueRes.Body.String())
	}
	var queued struct {
		Data struct {
			Fingerprint string `json:"fingerprint"`
			ProfileName string `json:"profile_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(queueRes.Body.Bytes(), &queued); err != nil {
		t.Fatalf("decode queued build: %v", err)
	}

	runReq := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/builds/"+queued.Data.Fingerprint+"/run", nil)
	runReq.Header.Set("X-API-Key", "build-test-key")
	runReq.Header.Set("X-Request-ID", "req_dependency_build_run_test")
	runRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(runRes, runReq)
	if runRes.Code != http.StatusOK {
		t.Fatalf("expected dependency build run to return 200, got %d body=%s", runRes.Code, runRes.Body.String())
	}
	var ready struct {
		Data struct {
			Status           string `json:"status"`
			ProfileName      string `json:"profile_name"`
			ArtifactChecksum string `json:"artifact_checksum"`
			SizeBytes        int64  `json:"size_bytes"`
			NextAction       string `json:"next_action"`
		} `json:"data"`
	}
	if err := json.Unmarshal(runRes.Body.Bytes(), &ready); err != nil {
		t.Fatalf("decode ready build: %v", err)
	}
	if ready.Data.Status != "ready" || ready.Data.NextAction != "use_dependency_profile" || ready.Data.ArtifactChecksum == "" || ready.Data.SizeBytes <= 0 {
		t.Fatalf("expected ready dependency build, got %#v", ready.Data)
	}
	if ready.Data.ProfileName != queued.Data.ProfileName {
		t.Fatalf("expected same profile name, got queued=%s ready=%s", queued.Data.ProfileName, ready.Data.ProfileName)
	}

	catalogReq := httptest.NewRequest(http.MethodGet, "/v1/sandbox/dependencies?language=python3", nil)
	catalogRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(catalogRes, catalogReq)
	if catalogRes.Code != http.StatusOK || !strings.Contains(catalogRes.Body.String(), `"name":"`+queued.Data.ProfileName+`"`) {
		t.Fatalf("expected ready dependency profile in catalog, got %d body=%s", catalogRes.Code, catalogRes.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","dependency_profile":"`+queued.Data.ProfileName+`"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-API-Key", "build-test-key")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK || !strings.Contains(createRes.Body.String(), `"dependency_artifact_checksum":"`+ready.Data.ArtifactChecksum+`"`) {
		t.Fatalf("expected sandbox to use ready dependency artifact, got %d body=%s", createRes.Code, createRes.Body.String())
	}

	events := server.observer.Query(observer.Query{Type: "dependency_build.ready", RequestID: "req_dependency_build_run_test", Limit: 1})
	if len(events) != 1 || events[0].Metadata["artifact_checksum"] != ready.Data.ArtifactChecksum {
		t.Fatalf("expected dependency build ready event, got %#v", events)
	}
}

func TestDependencyBuildCreateRequeuesStaleReadyRecordWhenArtifactIsMissing(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "build-test-key"
	cfg.DependencyRootFSDir = t.TempDir()
	cfg.DependencyBuildCommand = "python3 " + writeDependencyBuildStubCommand(t)
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	payload, err := json.Marshal(map[string]any{
		"archive_base64": testZipBase64(t, map[string]string{
			"SKILL.md":         "Skill package\n",
			"requirements.txt": "pandas==2.2.3\n",
			"scripts/run.py":   "import pandas as pd\nprint('ok')\n",
		}),
		"format":       "zip",
		"base_runtime": "linux-secure",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	queueReq := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/builds", bytes.NewReader(payload))
	queueReq.Header.Set("Content-Type", "application/json")
	queueReq.Header.Set("X-API-Key", "build-test-key")
	queueRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(queueRes, queueReq)
	if queueRes.Code != http.StatusOK {
		t.Fatalf("expected dependency build queue to return 200, got %d body=%s", queueRes.Code, queueRes.Body.String())
	}
	var queued struct {
		Data struct {
			Fingerprint string `json:"fingerprint"`
			ProfileName string `json:"profile_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(queueRes.Body.Bytes(), &queued); err != nil {
		t.Fatalf("decode queued build: %v", err)
	}

	runReq := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/builds/"+queued.Data.Fingerprint+"/run", nil)
	runReq.Header.Set("X-API-Key", "build-test-key")
	runRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(runRes, runReq)
	if runRes.Code != http.StatusOK || !strings.Contains(runRes.Body.String(), `"status":"ready"`) {
		t.Fatalf("expected dependency build run to return ready, got %d body=%s", runRes.Code, runRes.Body.String())
	}
	if err := os.RemoveAll(filepath.Join(cfg.DependencyRootFSDir, queued.Data.ProfileName)); err != nil {
		t.Fatalf("remove dependency artifact: %v", err)
	}

	requeueReq := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/builds", bytes.NewReader(payload))
	requeueReq.Header.Set("Content-Type", "application/json")
	requeueReq.Header.Set("X-API-Key", "build-test-key")
	requeueRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(requeueRes, requeueReq)
	if requeueRes.Code != http.StatusOK {
		t.Fatalf("expected stale dependency build create to return 200, got %d body=%s", requeueRes.Code, requeueRes.Body.String())
	}
	var requeued struct {
		Data struct {
			Status           string `json:"status"`
			ArtifactChecksum string `json:"artifact_checksum"`
			SizeBytes        int64  `json:"size_bytes"`
			NextAction       string `json:"next_action"`
		} `json:"data"`
	}
	if err := json.Unmarshal(requeueRes.Body.Bytes(), &requeued); err != nil {
		t.Fatalf("decode requeued build: %v", err)
	}
	if requeued.Data.Status != "queued" || requeued.Data.NextAction != "wait_for_dependency_build" || requeued.Data.ArtifactChecksum != "" || requeued.Data.SizeBytes != 0 {
		t.Fatalf("expected stale ready build to be requeued, got %#v", requeued.Data)
	}

	if !server.runOneQueuedDependencyBuild(context.Background()) {
		t.Fatal("expected dependency build worker to rebuild stale artifact")
	}
	rebuilt, err := server.store.GetDependencyBuildRequest(queued.Data.Fingerprint)
	if err != nil {
		t.Fatalf("get rebuilt dependency build: %v", err)
	}
	if rebuilt.Status != "ready" || rebuilt.ArtifactChecksum == "" || rebuilt.SizeBytes <= 0 {
		t.Fatalf("expected rebuilt ready dependency build, got %+v", rebuilt)
	}
}

func TestFilterCachedDependencyProfilesKeepsOnlyProfilesWithLocalArtifacts(t *testing.T) {
	cached := []policy.DependencyProfile{
		{Name: "stdlib", Status: "ready", Enabled: true},
		{Name: "auto-present", Status: "ready", Enabled: true, ArtifactChecksum: "sha256:present"},
		{Name: "auto-missing", Status: "ready", Enabled: true, ArtifactChecksum: "sha256:missing"},
	}
	artifacts := []policy.DependencyProfile{
		{Name: "auto-present", Status: "ready", Enabled: true, ArtifactChecksum: "sha256:present"},
	}

	filtered := filterCachedDependencyProfilesWithLocalArtifacts(cached, artifacts, t.TempDir())
	names := make([]string, 0, len(filtered))
	for _, profile := range filtered {
		names = append(names, profile.Name)
	}
	if strings.Join(names, ",") != "stdlib,auto-present" {
		t.Fatalf("expected stdlib and available artifact profile, got %v", names)
	}
}

func TestFilterCachedDependencyProfilesKeepsCachedProfilesWithoutRootFSDir(t *testing.T) {
	cached := []policy.DependencyProfile{
		{Name: "stdlib", Status: "ready", Enabled: true},
		{Name: "office-safe", Status: "ready", Enabled: true, ArtifactChecksum: "sha256:office-safe"},
	}

	filtered := filterCachedDependencyProfilesWithLocalArtifacts(cached, nil, "")
	names := make([]string, 0, len(filtered))
	for _, profile := range filtered {
		names = append(names, profile.Name)
	}
	if strings.Join(names, ",") != "stdlib,office-safe" {
		t.Fatalf("expected cached profiles to remain without dependency rootfs dir, got %v", names)
	}
}

func TestDependencyBuildWorkerMaterializesQueuedProfile(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "build-test-key"
	cfg.DependencyRootFSDir = t.TempDir()
	cfg.DependencyBuildCommand = "python3 " + writeDependencyBuildStubCommand(t)
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	payload, err := json.Marshal(map[string]any{
		"archive_base64": testZipBase64(t, map[string]string{
			"SKILL.md":         "Skill package\n",
			"requirements.txt": "pandas==2.2.3\n",
			"scripts/run.py":   "import pandas as pd\nprint('ok')\n",
		}),
		"format":       "zip",
		"base_runtime": "linux-secure",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	queueReq := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/builds", bytes.NewReader(payload))
	queueReq.Header.Set("Content-Type", "application/json")
	queueReq.Header.Set("X-API-Key", "build-test-key")
	queueRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(queueRes, queueReq)
	if queueRes.Code != http.StatusOK {
		t.Fatalf("expected dependency build queue to return 200, got %d body=%s", queueRes.Code, queueRes.Body.String())
	}
	var queued struct {
		Data struct {
			Fingerprint string `json:"fingerprint"`
			ProfileName string `json:"profile_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(queueRes.Body.Bytes(), &queued); err != nil {
		t.Fatalf("decode queued build: %v", err)
	}

	if !server.runOneQueuedDependencyBuild(context.Background()) {
		t.Fatal("expected dependency build worker to process one queued build")
	}
	ready, err := server.store.GetDependencyBuildRequest(queued.Data.Fingerprint)
	if err != nil {
		t.Fatalf("get ready dependency build: %v", err)
	}
	if ready.Status != "ready" || ready.ArtifactChecksum == "" || ready.SizeBytes <= 0 {
		t.Fatalf("expected worker to materialize ready build, got %+v", ready)
	}
	if server.runOneQueuedDependencyBuild(context.Background()) {
		t.Fatal("expected no queued dependency build after worker drained queue")
	}

	catalogReq := httptest.NewRequest(http.MethodGet, "/v1/sandbox/dependencies?language=python3", nil)
	catalogRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(catalogRes, catalogReq)
	if catalogRes.Code != http.StatusOK || !strings.Contains(catalogRes.Body.String(), `"name":"`+queued.Data.ProfileName+`"`) {
		t.Fatalf("expected ready dependency profile in catalog, got %d body=%s", catalogRes.Code, catalogRes.Body.String())
	}
}

func TestDependencyBuildRunCleansPartialArtifactOnFailure(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "build-test-key"
	cfg.DependencyRootFSDir = t.TempDir()
	cfg.DependencyBuildCommand = "python3 " + writeFailingDependencyBuildStubCommand(t)
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	payload, err := json.Marshal(map[string]any{
		"archive_base64": testZipBase64(t, map[string]string{
			"SKILL.md":         "Skill package\n",
			"requirements.txt": "pandas==2.2.3\n",
			"scripts/run.py":   "import pandas as pd\nprint('ok')\n",
		}),
		"format":       "zip",
		"base_runtime": "linux-secure",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	queueReq := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/builds", bytes.NewReader(payload))
	queueReq.Header.Set("Content-Type", "application/json")
	queueReq.Header.Set("X-API-Key", "build-test-key")
	queueRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(queueRes, queueReq)
	if queueRes.Code != http.StatusOK {
		t.Fatalf("expected dependency build queue to return 200, got %d body=%s", queueRes.Code, queueRes.Body.String())
	}
	var queued struct {
		Data struct {
			Fingerprint string `json:"fingerprint"`
			ProfileName string `json:"profile_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(queueRes.Body.Bytes(), &queued); err != nil {
		t.Fatalf("decode queued build: %v", err)
	}

	runReq := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/builds/"+queued.Data.Fingerprint+"/run", nil)
	runReq.Header.Set("X-API-Key", "build-test-key")
	runRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(runRes, runReq)
	if runRes.Code != http.StatusBadRequest || !strings.Contains(runRes.Body.String(), `"status":"failed"`) {
		t.Fatalf("expected failed dependency build, got %d body=%s", runRes.Code, runRes.Body.String())
	}
	if _, err := os.Stat(filepath.Join(cfg.DependencyRootFSDir, queued.Data.ProfileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected partial dependency artifact cleanup, got err=%v", err)
	}
}

func TestDependencyUpdateRequiresAdminAPIKey(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "admin-test-key"
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/update", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected admin endpoint to require api key, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestDependencyUpdateBuildsSelectableProfile(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "admin-test-key"
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	body := `{
		"name": "office-safe",
		"version": "2026.05.31",
		"languages": ["python3"],
		"packages": [{"name": "data-tools", "version": "managed"}],
		"base_runtime": "preview-process",
		"checksum": "sha256:office-safe",
		"size_bytes": 1024,
		"description": "Managed document automation profile."
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "admin-test-key")
	req.Header.Set("X-Request-ID", "req_profile_build")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected dependency profile build to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"ready"`) || !strings.Contains(rr.Body.String(), `"name":"office-safe"`) {
		t.Fatalf("expected ready dependency profile build, got %s", rr.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","dependency_profile":"office-safe"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-API-Key", "admin-test-key")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected built profile to be selectable, got %d body=%s", createRes.Code, createRes.Body.String())
	}
	if !strings.Contains(createRes.Body.String(), `"dependency_profile":"office-safe"`) {
		t.Fatalf("expected created sandbox to use built profile, got %s", createRes.Body.String())
	}

	events := server.observer.Query(observer.Query{Type: "dependency_profile.build", RequestID: "req_profile_build", Limit: 1})
	if len(events) != 1 || events[0].Metadata["dependency_profile"] != "office-safe" {
		t.Fatalf("expected dependency profile build event, got %#v", events)
	}
}

func TestDependencyUpdatePromotesReservedProfile(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "admin-test-key"
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	body := `{
		"name": "skill-office",
		"version": "2026.05.31",
		"languages": ["python3", "nodejs"],
		"packages": [
			{"ecosystem": "python3", "name": "office-tools", "version": "managed"},
			{"ecosystem": "nodejs", "name": "office-tools", "version": "managed"}
		],
		"base_runtime": "linux-secure",
		"checksum": "sha256:skill-office",
		"size_bytes": 1024,
		"description": "Managed document automation profile."
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "admin-test-key")
	req.Header.Set("X-Request-ID", "req_skill_office_release")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected reserved profile promotion to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"name":"skill-office"`) || !strings.Contains(rr.Body.String(), `"status":"ready"`) {
		t.Fatalf("expected promoted skill-office profile, got %s", rr.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","dependency_profile":"skill-office"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-API-Key", "admin-test-key")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected promoted skill-office profile to be selectable, got %d body=%s", createRes.Code, createRes.Body.String())
	}
	if !strings.Contains(createRes.Body.String(), `"dependency_profile":"skill-office"`) {
		t.Fatalf("expected created sandbox to use skill-office, got %s", createRes.Body.String())
	}

	events := server.observer.Query(observer.Query{Type: "dependency_profile.build", RequestID: "req_skill_office_release", Limit: 1})
	if len(events) != 1 || events[0].Metadata["dependency_profile"] != "skill-office" {
		t.Fatalf("expected dependency profile release event, got %#v", events)
	}

	if err := server.store.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}
	restarted, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected restarted server, got %v", err)
	}
	restartReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","dependency_profile":"skill-office"}`))
	restartReq.Header.Set("Content-Type", "application/json")
	restartReq.Header.Set("X-API-Key", "admin-test-key")
	restartRes := httptest.NewRecorder()
	restarted.Handler().ServeHTTP(restartRes, restartReq)
	if restartRes.Code != http.StatusOK {
		t.Fatalf("expected promoted skill-office profile to remain selectable after restart, got %d body=%s", restartRes.Code, restartRes.Body.String())
	}
}

func TestDependencyUpdateCreatesOrganizationProfileReference(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "admin-test-key"
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	body := `{
		"name": "team-data",
		"version": "2026.06.01",
		"scope": "organization",
		"organization_id": "organization-a",
		"languages": ["python3"],
		"packages": [{"ecosystem": "python3", "name": "data-tools", "version": "managed"}],
		"base_runtime": "preview-process",
		"checksum": "sha256:team-data",
		"artifact_checksum": "sha256:shared-data-artifact",
		"size_bytes": 1024,
		"description": "Organization managed data runtime."
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "admin-test-key")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected organization profile update to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"scope":"organization"`) || !strings.Contains(rr.Body.String(), `"organization_id":"organization-a"`) {
		t.Fatalf("expected organization profile response, got %s", rr.Body.String())
	}

	globalCatalogReq := httptest.NewRequest(http.MethodGet, "/v1/sandbox/dependencies?language=python3", nil)
	globalCatalogRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(globalCatalogRes, globalCatalogReq)
	if strings.Contains(globalCatalogRes.Body.String(), `"name":"team-data"`) {
		t.Fatalf("expected global catalog to hide organization profile, got %s", globalCatalogRes.Body.String())
	}

	orgCatalogReq := httptest.NewRequest(http.MethodGet, "/v1/sandbox/dependencies?language=python3&organization_id=organization-a", nil)
	orgCatalogRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(orgCatalogRes, orgCatalogReq)
	if !strings.Contains(orgCatalogRes.Body.String(), `"name":"team-data"`) || !strings.Contains(orgCatalogRes.Body.String(), `"artifact_checksum":"sha256:shared-data-artifact"`) {
		t.Fatalf("expected organization catalog to include profile, got %s", orgCatalogRes.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","organization_id":"organization-a","dependency_profile":"team-data"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-API-Key", "admin-test-key")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected organization profile to be selectable, got %d body=%s", createRes.Code, createRes.Body.String())
	}
	if !strings.Contains(createRes.Body.String(), `"dependency_artifact_checksum":"sha256:shared-data-artifact"`) {
		t.Fatalf("expected sandbox to include dependency artifact checksum, got %s", createRes.Body.String())
	}

	otherReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","organization_id":"organization-b","dependency_profile":"team-data"}`))
	otherReq.Header.Set("Content-Type", "application/json")
	otherReq.Header.Set("X-API-Key", "admin-test-key")
	otherRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(otherRes, otherReq)
	if otherRes.Code != http.StatusBadRequest || !strings.Contains(otherRes.Body.String(), "unsupported dependency profile") {
		t.Fatalf("expected other organization to be rejected, got %d body=%s", otherRes.Code, otherRes.Body.String())
	}
}

func TestServerLoadsVerifiedDependencyProfileArtifacts(t *testing.T) {
	cfg := testConfig(t)
	cfg.DependencyRootFSDir = t.TempDir()
	writeServerDependencyProfileArtifact(t, cfg.DependencyRootFSDir, "skill-office")

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	catalogReq := httptest.NewRequest(http.MethodGet, "/v1/sandbox/dependencies?language=python3", nil)
	catalogRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(catalogRes, catalogReq)
	if catalogRes.Code != http.StatusOK {
		t.Fatalf("expected dependency catalog, got %d body=%s", catalogRes.Code, catalogRes.Body.String())
	}
	if !strings.Contains(catalogRes.Body.String(), `"name":"skill-office"`) ||
		!strings.Contains(catalogRes.Body.String(), `"status":"ready"`) ||
		!strings.Contains(catalogRes.Body.String(), `"enabled":true`) {
		t.Fatalf("expected verified artifact to promote skill-office, got %s", catalogRes.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","dependency_profile":"skill-office"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected artifact-backed skill-office profile to be selectable, got %d body=%s", createRes.Code, createRes.Body.String())
	}
	if !strings.Contains(createRes.Body.String(), `"dependency_profile_version":"2026.05.31"`) {
		t.Fatalf("expected artifact-backed version in create response, got %s", createRes.Body.String())
	}
}

func TestDependencyUpdatePersistsBuiltProfileAcrossRestart(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "admin-test-key"
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	body := `{
		"name": "office-safe",
		"version": "2026.05.31",
		"languages": ["python3"],
		"packages": [{"name": "data-tools", "version": "managed"}],
		"base_runtime": "preview-process",
		"checksum": "sha256:office-safe",
		"size_bytes": 1024,
		"description": "Managed document automation profile."
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "admin-test-key")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected dependency profile build to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if err := server.store.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	restarted, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected restarted server, got %v", err)
	}

	catalogReq := httptest.NewRequest(http.MethodGet, "/v1/sandbox/dependencies?language=python3", nil)
	catalogRes := httptest.NewRecorder()
	restarted.Handler().ServeHTTP(catalogRes, catalogReq)
	if catalogRes.Code != http.StatusOK {
		t.Fatalf("expected dependency catalog to return 200, got %d body=%s", catalogRes.Code, catalogRes.Body.String())
	}
	if !strings.Contains(catalogRes.Body.String(), `"name":"office-safe"`) || !strings.Contains(catalogRes.Body.String(), `"version":"2026.05.31"`) {
		t.Fatalf("expected restarted server to load cached profile, got %s", catalogRes.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","dependency_profile":"office-safe"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-API-Key", "admin-test-key")
	createRes := httptest.NewRecorder()
	restarted.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected cached profile to be selectable after restart, got %d body=%s", createRes.Code, createRes.Body.String())
	}
	if !strings.Contains(createRes.Body.String(), `"dependency_profile_version":"2026.05.31"`) {
		t.Fatalf("expected created sandbox to use cached profile version, got %s", createRes.Body.String())
	}
}

func TestDependencyUpdateReportsBuildFailure(t *testing.T) {
	cfg := testConfig(t)
	cfg.APIKey = "admin-test-key"
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	body := `{"name":"bad-profile","version":"latest","languages":["python3"],"checksum":"sha256:bad","size_bytes":1024}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sandbox/dependencies/update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "admin-test-key")
	req.Header.Set("X-Request-ID", "req_profile_build_failed")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected dependency profile build failure to return 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"status":"failed"`) || !strings.Contains(rr.Body.String(), "version must be pinned") {
		t.Fatalf("expected failed build details, got %s", rr.Body.String())
	}
	events := server.observer.Query(observer.Query{Type: "dependency_profile.build.failed", RequestID: "req_profile_build_failed", Limit: 1})
	if len(events) != 1 || !strings.Contains(fmt.Sprint(events[0].Metadata["error"]), "version must be pinned") {
		t.Fatalf("expected dependency profile build failure event, got %#v", events)
	}
}

func TestCreateSandboxRejectsUnavailableDependencyProfile(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","dependency_profile":"python-data-preview"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected disabled dependency profile to return 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "dependency profile is not enabled") {
		t.Fatalf("expected dependency profile rejection message, got %s", rr.Body.String())
	}
}

func TestMetricsEndpointReportsSandboxRunnerAndObserverCounters(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxWorkers = 2
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected sandbox create to return 200, got %d body=%s", createRes.Code, createRes.Body.String())
	}

	var createPayload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("expected create payload, got %v", err)
	}

	execReq := httptest.NewRequest(http.MethodPost, "/v1/exec/code", strings.NewReader(fmt.Sprintf(`{"sandbox_id":%q,"language":"python3","code":"print('metrics-ok')"}`, createPayload.Data.ID)))
	execReq.Header.Set("Content-Type", "application/json")
	execRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(execRes, execReq)
	if execRes.Code != http.StatusOK {
		t.Fatalf("expected code execution to return 200, got %d body=%s", execRes.Code, execRes.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/metrics", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected metrics to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Data struct {
			WorkerID        string `json:"worker_id"`
			ActiveSandboxes int    `json:"active_sandboxes"`
			Runner          struct {
				MaxWorkers       int   `json:"max_workers"`
				ActiveWorkers    int   `json:"active_workers"`
				QueuedExecutions int64 `json:"queued_executions"`
			} `json:"runner"`
			Observer struct {
				ExecutionSuccessCount      int     `json:"execution_success_count"`
				ExecutionFailureCount      int     `json:"execution_failure_count"`
				ExecutionDurationCount     int     `json:"execution_duration_count"`
				ExecutionDurationAverageMS float64 `json:"execution_duration_average_ms"`
			} `json:"observer"`
			ObserverRetention struct {
				RetentionDays int `json:"retention_days"`
				MaxEvents     int `json:"max_events"`
			} `json:"observer_retention"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected metrics payload, got %v", err)
	}
	if payload.Data.WorkerID != "test-worker" {
		t.Fatalf("expected worker id test-worker, got %q", payload.Data.WorkerID)
	}
	if payload.Data.ActiveSandboxes != 1 {
		t.Fatalf("expected one active sandbox, got %d", payload.Data.ActiveSandboxes)
	}
	if payload.Data.Runner.MaxWorkers != 2 {
		t.Fatalf("expected two max workers, got %d", payload.Data.Runner.MaxWorkers)
	}
	if payload.Data.Runner.ActiveWorkers != 0 {
		t.Fatalf("expected no active workers after request, got %d", payload.Data.Runner.ActiveWorkers)
	}
	if payload.Data.Runner.QueuedExecutions != 0 {
		t.Fatalf("expected no queued executions after request, got %d", payload.Data.Runner.QueuedExecutions)
	}
	if payload.Data.Observer.ExecutionSuccessCount != 1 {
		t.Fatalf("expected one execution success, got %d", payload.Data.Observer.ExecutionSuccessCount)
	}
	if payload.Data.Observer.ExecutionFailureCount != 0 {
		t.Fatalf("expected no execution failures, got %d", payload.Data.Observer.ExecutionFailureCount)
	}
	if payload.Data.Observer.ExecutionDurationCount != 1 {
		t.Fatalf("expected one duration sample, got %d", payload.Data.Observer.ExecutionDurationCount)
	}
	if payload.Data.ObserverRetention.RetentionDays != 7 {
		t.Fatalf("expected default observer retention 7 days, got %d", payload.Data.ObserverRetention.RetentionDays)
	}
	if payload.Data.ObserverRetention.MaxEvents != 10000 {
		t.Fatalf("expected default observer max events 10000, got %d", payload.Data.ObserverRetention.MaxEvents)
	}
}

func TestObserverEventsEndpointPaginatesWithCursor(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	server.observer.Record("sandbox.test", "sbx_page", "first", nil)
	time.Sleep(2 * time.Millisecond)
	server.observer.Record("sandbox.test", "sbx_page", "second", nil)
	time.Sleep(2 * time.Millisecond)
	server.observer.Record("sandbox.test", "sbx_page", "third", nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/observer/events?sandbox_id=sbx_page&limit=2", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected first page to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var firstPage struct {
		Data struct {
			Events []struct {
				Message string `json:"message"`
			} `json:"events"`
			Limit      int    `json:"limit"`
			HasMore    bool   `json:"has_more"`
			NextCursor string `json:"next_cursor"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &firstPage); err != nil {
		t.Fatalf("expected first page payload, got %v", err)
	}
	if firstPage.Data.Limit != 2 {
		t.Fatalf("expected limit 2, got %d", firstPage.Data.Limit)
	}
	if len(firstPage.Data.Events) != 2 {
		t.Fatalf("expected two first page events, got %d", len(firstPage.Data.Events))
	}
	if !firstPage.Data.HasMore {
		t.Fatal("expected first page to have more events")
	}
	if firstPage.Data.NextCursor == "" {
		t.Fatal("expected next cursor")
	}
	if firstPage.Data.Events[0].Message != "third" {
		t.Fatalf("expected newest event first, got %q", firstPage.Data.Events[0].Message)
	}

	nextReq := httptest.NewRequest(http.MethodGet, "/v1/observer/events?sandbox_id=sbx_page&limit=2&before="+url.QueryEscape(firstPage.Data.NextCursor), nil)
	nextRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(nextRes, nextReq)
	if nextRes.Code != http.StatusOK {
		t.Fatalf("expected second page to return 200, got %d body=%s", nextRes.Code, nextRes.Body.String())
	}

	var secondPage struct {
		Data struct {
			Events []struct {
				Message string `json:"message"`
			} `json:"events"`
			HasMore bool `json:"has_more"`
		} `json:"data"`
	}
	if err := json.Unmarshal(nextRes.Body.Bytes(), &secondPage); err != nil {
		t.Fatalf("expected second page payload, got %v", err)
	}
	if len(secondPage.Data.Events) != 1 {
		t.Fatalf("expected one second page event, got %d", len(secondPage.Data.Events))
	}
	if secondPage.Data.HasMore {
		t.Fatal("expected second page to be terminal")
	}
	if secondPage.Data.Events[0].Message != "first" {
		t.Fatalf("expected oldest remaining event, got %q", secondPage.Data.Events[0].Message)
	}
}

func TestObserverEventsEndpointFiltersByOwnershipScope(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	server.observer.Record("exec.command", "sbx_scope_one", "match", map[string]any{
		"organization_id": "organization-one",
		"workspace_id":    "workspace-one",
		"app_id":          "app-one",
		"workflow_run_id": "run-one",
		"user_id":         "user-one",
	})
	server.observer.Record("exec.command", "sbx_scope_two", "miss", map[string]any{
		"organization_id": "organization-two",
		"workspace_id":    "workspace-two",
		"app_id":          "app-two",
		"workflow_run_id": "run-two",
		"user_id":         "user-two",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/observer/events?organization_id=organization-one&workspace_id=workspace-one&app_id=app-one&workflow_run_id=run-one&user_id=user-one&limit=10", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected scope filter to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Data struct {
			Events []struct {
				Message string `json:"message"`
			} `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected observer payload, got %v", err)
	}
	if len(payload.Data.Events) != 1 {
		t.Fatalf("expected one scoped event, got %d", len(payload.Data.Events))
	}
	if payload.Data.Events[0].Message != "match" {
		t.Fatalf("expected matching event, got %q", payload.Data.Events[0].Message)
	}
}

func TestObserverEventsEndpointFiltersByRequestID(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	server.observer.Record("exec.command", "sbx_request", "match", map[string]any{
		"request_id": "req_filter_match",
	})
	server.observer.Record("exec.command", "sbx_request", "miss", map[string]any{
		"request_id": "req_filter_miss",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/observer/events?sandbox_id=sbx_request&request_id=req_filter_match&limit=10", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected request filter to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Data struct {
			Events []struct {
				Message  string         `json:"message"`
				Metadata map[string]any `json:"metadata"`
			} `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected observer payload, got %v", err)
	}
	if len(payload.Data.Events) != 1 {
		t.Fatalf("expected one request-filtered event, got %d", len(payload.Data.Events))
	}
	if payload.Data.Events[0].Message != "match" || payload.Data.Events[0].Metadata["request_id"] != "req_filter_match" {
		t.Fatalf("expected matching request event, got %+v", payload.Data.Events[0])
	}
}

func TestSandboxExecutionHistoryEndpointReturnsExecutionEventsOnly(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected create to return 200, got %d body=%s", createRes.Code, createRes.Body.String())
	}

	var createPayload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("expected create payload, got %v", err)
	}

	server.observer.Record("files.upload", createPayload.Data.ID, "file upload", nil)
	server.observer.Record("exec.code", createPayload.Data.ID, "code executed", map[string]any{
		"request_id":   "req_history_match",
		"execution_id": "exec_history_code",
	})
	server.observer.Record("exec.command.failed", createPayload.Data.ID, "command failed", map[string]any{
		"request_id":   "req_history_miss",
		"execution_id": "exec_history_command",
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/sandboxes/"+createPayload.Data.ID+"/executions?request_id=req_history_match&limit=10", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected execution history to return 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Data struct {
			Events []struct {
				Type     string         `json:"type"`
				Message  string         `json:"message"`
				Metadata map[string]any `json:"metadata"`
			} `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected execution history payload, got %v", err)
	}
	if len(payload.Data.Events) != 1 {
		t.Fatalf("expected one execution event, got %d", len(payload.Data.Events))
	}
	if payload.Data.Events[0].Type != "exec.code" || payload.Data.Events[0].Message != "code executed" {
		t.Fatalf("expected matching exec.code event, got %+v", payload.Data.Events[0])
	}
	if payload.Data.Events[0].Metadata["execution_id"] != "exec_history_code" {
		t.Fatalf("expected execution metadata, got %+v", payload.Data.Events[0].Metadata)
	}
}

func TestObserverEventsEndpointRejectsNonGet(t *testing.T) {
	server, err := NewServer(testConfig(t))
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/observer/events", nil)
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSandboxPersistenceAcrossServerInstances(t *testing.T) {
	cfg := testConfig(t)

	serverA, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected first server, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"session","ttl_seconds":60}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	serverA.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected create to return 200, got %d", createRes.Code)
	}

	serverB, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected second server, got %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/sandboxes", nil)
	listRes := httptest.NewRecorder()
	serverB.Handler().ServeHTTP(listRes, listReq)
	if !strings.Contains(listRes.Body.String(), `"runtime_profile":"session"`) {
		t.Fatalf("expected persisted sandbox, got %s", listRes.Body.String())
	}
}

func TestInteractiveProxyRoutesToRegisteredEndpoint(t *testing.T) {
	cfg := testConfig(t)
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("expected server, got %v", err)
	}

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "proxy:%s", r.URL.Path)
	}))
	defer targetServer.Close()

	targetURL, err := url.Parse(targetServer.URL)
	if err != nil {
		t.Fatalf("expected target url, got %v", err)
	}
	targetPort, err := strconv.Atoi(targetURL.Port())
	if err != nil {
		t.Fatalf("expected numeric port, got %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes", strings.NewReader(`{"runtime_profile":"interactive","ttl_seconds":60}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusOK {
		t.Fatalf("expected interactive sandbox create to return 200, got %d", createRes.Code)
	}

	var createPayload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("expected create payload, got %v", err)
	}

	registerReq := httptest.NewRequest(http.MethodPost, "/v1/sandboxes/"+createPayload.Data.ID+"/endpoints/3010", strings.NewReader(fmt.Sprintf(`{"target_host":"127.0.0.1","target_port":%d,"scheme":"http"}`, targetPort)))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(registerRes, registerReq)
	if registerRes.Code != http.StatusOK {
		t.Fatalf("expected endpoint register to return 200, got %d", registerRes.Code)
	}

	proxyReq := httptest.NewRequest(http.MethodGet, "/_zgi/ports/"+createPayload.Data.ID+"/3010/hello", nil)
	proxyRes := httptest.NewRecorder()
	server.Handler().ServeHTTP(proxyRes, proxyReq)
	if proxyRes.Code != http.StatusOK {
		t.Fatalf("expected interactive proxy to return 200, got %d", proxyRes.Code)
	}
	if !strings.Contains(proxyRes.Body.String(), "proxy:/hello") {
		t.Fatalf("expected proxied response body, got %s", proxyRes.Body.String())
	}
}

func testConfig(t *testing.T) config.Config {
	t.Helper()

	cfg := config.FromEnv()
	cfg.DataDir = t.TempDir()
	cfg.DatabaseURL = testutil.CreateTestPostgresDSN(t)
	cfg.WorkerID = "test-worker"
	cfg.AdvertiseURL = "http://127.0.0.1:2660"
	cfg.PublicBaseURL = cfg.AdvertiseURL
	cfg.Environment = "local"
	cfg.RedisAddr = ""
	cfg.RuntimeBackend = "preview"
	return cfg
}

func writeServerDependencyProfileArtifact(t *testing.T, dependencyRoot string, profile string) {
	t.Helper()
	profileDir := filepath.Join(dependencyRoot, profile, "opt", "zgi", "profiles", profile)
	files := map[string]string{
		"venv/bin/python":       "python",
		"node_modules/pkg.json": "{}",
	}
	for name, content := range files {
		path := filepath.Join(profileDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create artifact parent: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write artifact file: %v", err)
		}
	}
	checksum, size := checksumServerDependencyProfileArtifact(t, profileDir)
	manifest := map[string]any{
		"name":         profile,
		"version":      "2026.05.31",
		"status":       "disabled",
		"enabled":      false,
		"owner_scope":  "global",
		"languages":    []string{"python3", "nodejs"},
		"base_runtime": "linux-secure",
		"description":  "Managed document automation profile.",
		"packages": []map[string]string{
			{"ecosystem": "python3", "name": "office-tools", "version": "managed"},
			{"ecosystem": "nodejs", "name": "office-tools", "version": "managed"},
		},
		"build": map[string]any{
			"checksum":            checksum,
			"size_bytes":          size,
			"verification_passed": true,
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode artifact manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "manifest.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write artifact manifest: %v", err)
	}
}

func checksumServerDependencyProfileArtifact(t *testing.T, root string) (string, int64) {
	t.Helper()
	files := make([]string, 0)
	var size int64
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == "manifest.json" {
			return nil
		}
		files = append(files, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	}); err != nil {
		t.Fatalf("walk artifact: %v", err)
	}
	slices.Sort(files)
	hash := sha256.New()
	for _, rel := range files {
		hash.Write([]byte(filepath.ToSlash(rel)))
		hash.Write([]byte{0})
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read artifact file: %v", err)
		}
		hash.Write(raw)
		hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), size
}

func testZipBase64(t *testing.T, files map[string]string) string {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for path, content := range files {
		fileWriter, err := writer.Create(path)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := fileWriter.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func writeDependencyBuildStubCommand(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dependency_build_stub.py")
	script := `import hashlib
import json
import os
import sys
from pathlib import Path

input_path = Path(sys.argv[1])
output_dir = Path(sys.argv[2])
payload = json.loads(input_path.read_text(encoding="utf-8"))
profile = payload["profile_name"]
output_dir.mkdir(parents=True, exist_ok=True)

files = {
    "venv/bin/python": b"#!/usr/bin/env python3\n",
    "venv/pyvenv.cfg": b"home = /usr/bin\ninclude-system-site-packages = false\n",
    "node_modules/.profile-ready": b"ready\n",
    "bin/profile-ready": b"ready\n",
    "dependency-request.json": json.dumps(payload["dependency_request"], sort_keys=True).encode("utf-8") + b"\n",
    "packages.json": json.dumps(payload["packages"], sort_keys=True).encode("utf-8") + b"\n",
}
for rel, raw in files.items():
    target = output_dir / rel
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(raw)

digest = hashlib.sha256()
size = 0
for rel in sorted(files):
    raw = files[rel]
    digest.update(rel.encode("utf-8"))
    digest.update(b"\0")
    digest.update(raw)
    digest.update(b"\0")
    size += len(raw)
checksum = "sha256:" + digest.hexdigest()
packages = []
languages = set()
for item in payload["packages"]:
    ecosystem = item.get("ecosystem", "")
    if ecosystem in ("python3", "nodejs"):
        languages.add(ecosystem)
    version = item.get("version") or "managed"
    if version.startswith("=="):
        version = version[2:]
    packages.append({
        "ecosystem": ecosystem,
        "name": item.get("name", ""),
        "version": version,
    })
if not languages:
    languages.add(payload["dependency_request"].get("language") or "python3")
manifest = {
    "name": profile,
    "version": "sha256-" + payload["fingerprint"].split(":", 1)[-1][:16],
    "status": "disabled",
    "enabled": False,
    "owner_scope": "global",
    "languages": sorted(languages),
    "base_runtime": payload["dependency_request"].get("base_runtime") or "linux-secure",
    "checksum": payload["fingerprint"],
    "estimated_size_bytes": size,
    "description": "Automatically built dependency profile.",
    "packages": packages,
    "build": {
        "checksum": checksum,
        "size_bytes": size,
        "verification_passed": True,
    },
}
(output_dir / "manifest.json").write_text(json.dumps(manifest, sort_keys=True), encoding="utf-8")
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write dependency build stub: %v", err)
	}
	return path
}

func writeFailingDependencyBuildStubCommand(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dependency_build_fail_stub.py")
	script := `import sys
from pathlib import Path

output_dir = Path(sys.argv[2])
output_dir.mkdir(parents=True, exist_ok=True)
(output_dir / "partial.txt").write_text("partial", encoding="utf-8")
raise SystemExit(2)
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write failing dependency build stub: %v", err)
	}
	return path
}
