package agentexecution

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type refusingJobRepository struct{ JobRepository }

func (r refusingJobRepository) CreateJob(context.Context, Job) (Job, bool, error) {
	return Job{}, false, ErrCapacityExceeded
}

func TestCapacityAndReleaseGatesPreserveExistingFileDownload(t *testing.T) {
	api := newTestHTTPAPI(t)
	response := performAgentRuntimeRequest(t, api, http.MethodPost, agentRuntimePathPrefix+"bindings", strings.NewReader(`{"agent_id":"agent-1","conversation_id":"conversation-1","profile_id":"profile-1","backend":"fake","idempotency_key":"retained-file"}`), "org-1", "user-1")
	if response.Code != 201 {
		t.Fatal(response.Body.String())
	}
	id := responseDataString(t, response.Body.Bytes(), "id")
	filePath := agentRuntimePathPrefix + "bindings/" + id + "/files?path=result.txt"
	response = performAgentRuntimeRequest(t, api, http.MethodPut, filePath, strings.NewReader("retained-result"), "org-1", "user-1")
	if response.Code != 201 {
		t.Fatal(response.Body.String())
	}
	data := api.data.(*DataPlaneService)
	data.jobs = refusingJobRepository{data.jobs}
	response = performAgentRuntimeRequest(t, api, http.MethodPost, agentRuntimePathPrefix+"bindings/"+id+"/jobs", strings.NewReader(`{"idempotency_key":"quota","command":"true"}`), "org-1", "user-1")
	if response.Code != 429 {
		t.Fatalf("capacity status=%d body=%s", response.Code, response.Body.String())
	}
	api.releaseID = "release-1"
	response = performAgentRuntimeRequest(t, api, http.MethodGet, filePath, nil, "org-1", "user-1")
	if response.Code != 200 || response.Body.String() != "retained-result" {
		t.Fatalf("file blocked: %d %s", response.Code, response.Body.String())
	}
	response = performAgentRuntimeRequest(t, api, http.MethodGet, filePath, nil, "other-org", "user-1")
	if response.Code != 404 {
		t.Fatalf("cross-org download=%d", response.Code)
	}
}

func TestVerifyReleasePins(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	base := ReleaseManifest{Version: 1, ID: "release-1", APIImage: "registry/api@" + digest, WebImage: "registry/web@" + digest,
		WorkerDigest: digest, ConfigurationDigest: digest, PlatformProtocolVersion: PlatformProtocolVersion}
	for _, name := range []string{"valid", "mutable-image", "worker", "config", "protocol", "missing-id", "unknown-field", "trailing", "oversized"} {
		t.Run(name, func(t *testing.T) {
			manifest := base
			worker, configuration := digest, digest
			switch name {
			case "mutable-image":
				manifest.APIImage = "registry/api:latest"
			case "worker":
				worker = "sha256:" + strings.Repeat("b", 64)
			case "config":
				configuration = "sha256:" + strings.Repeat("b", 64)
			case "protocol":
				manifest.PlatformProtocolVersion++
			case "missing-id":
				manifest.ID = ""
			}
			data, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "unknown-field":
				data = append(data[:len(data)-1], []byte(`,"typo":true}`)...)
			case "trailing":
				data = append(data, []byte(` {}`)...)
			case "oversized":
				data = bytes.Repeat([]byte(" "), 65537)
			}
			_, err = VerifyRelease(bytes.NewReader(data), worker, configuration)
			if (err == nil) != (name == "valid") {
				t.Fatalf("verification: %v", err)
			}
		})
	}
}

func TestReleaseAdmissionLeavesRetrievalAndCancellationAvailable(t *testing.T) {
	api := newTestHTTPAPI(t)
	api.releaseID = "release-1"
	for _, test := range []struct {
		method, path, header string
		want                 int
	}{
		{http.MethodPost, "bindings", "", 503},
		{http.MethodPost, "bindings/id/resume", "wrong", 503},
		{http.MethodPost, "bindings/id/jobs", "wrong", 503},
		{http.MethodPost, "bindings", "release-1", 400}, // Normal validation, not release rejection.
		{http.MethodGet, "bindings/id", "wrong", 404},
		{http.MethodGet, "bindings/id/files?path=result.txt", "wrong", 404},
		{http.MethodPost, "jobs/id/cancel", "wrong", 404},
		{http.MethodDelete, "bindings/id?generation=1", "wrong", 404},
		{http.MethodGet, "release", "", 200},
	} {
		t.Run(test.method+test.path+test.header, func(t *testing.T) {
			request := httptest.NewRequest(test.method, agentRuntimePathPrefix+test.path, strings.NewReader(`{}`))
			request.Header.Set("Authorization", "Bearer runtime-secret")
			request.Header.Set("X-ZGI-Organization-ID", "org-1")
			request.Header.Set("X-ZGI-Subject-ID", "user-1")
			request.Header.Set(ReleaseHeader, test.header)
			response := httptest.NewRecorder()
			api.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestMaximumJobTimeout(t *testing.T) {
	bindings := newMemoryRepository()
	bindings.bindings["binding-1"] = activeTestBinding()
	jobs := newMemoryJobRepository()
	service, err := NewDataPlaneService(bindings, jobs, map[string]ExecutionBackend{"fake": newFakeExecutionBackend()}, DataPlaneOptions{MaxJobTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	request := testSubmitJobRequest()
	request.Timeout = 0
	job, err := service.SubmitJob(t.Context(), request)
	if err != nil || job.Timeout != time.Minute {
		t.Fatalf("default timeout: %v %v", job.Timeout, err)
	}
	request.IdempotencyKey = "too-long"
	request.Timeout = time.Minute + time.Second
	if _, err := service.SubmitJob(t.Context(), request); err == nil {
		t.Fatal("unbounded timeout accepted")
	}
	for _, test := range []struct {
		err    error
		status int
	}{{ErrCapacityExceeded, 429}, {ErrAdmissionPolicy, 503}} {
		status, _, _ := agentRuntimeErrorResponse(test.err)
		if status != test.status {
			t.Fatalf("status=%d", status)
		}
	}
}
