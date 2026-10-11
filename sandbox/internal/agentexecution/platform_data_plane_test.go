package agentexecution

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type platformExecutionBackend struct {
	*fakeExecutionBackend
	requests  int
	responses int
}

func TestPlatformFileHTTPKeepsJobGenerationAndRejectsExternalMutation(t *testing.T) {
	bindings := newMemoryRepository()
	bindings.bindings["binding-1"] = activeTestBinding()
	jobs := newMemoryJobRepository()
	backend := &platformExecutionBackend{fakeExecutionBackend: newFakeExecutionBackend()}
	service := newTestDataPlane(t, bindings, jobs, backend)
	request := testSubmitJobRequest()
	request.Environment["ZGI_JOB_PLATFORM_ENABLED"] = "true"
	job, err := service.SubmitJob(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	job.Status = JobStatusRunning
	jobs.jobs[job.ID] = job
	api, err := NewHTTPAPI(newTestService(t, bindings, &fakeBackend{}), service, HTTPAPIOptions{APIKey: "runtime-secret"})
	if err != nil {
		t.Fatal(err)
	}
	for _, other := range []bool{true, false} {
		org := request.Scope.OrganizationID
		if other {
			org = "other"
		}
		response := performAgentRuntimeRequest(t, api, http.MethodPut, agentRuntimePathPrefix+"jobs/"+job.ID+"/platform-files?path=inputs/test.txt", strings.NewReader("input"), org, request.Scope.SubjectID)
		if other {
			if response.Code != http.StatusNotFound || len(backend.files) != 0 {
				t.Fatalf("other tenant wrote a file: status=%d", response.Code)
			}
		} else if response.Code != http.StatusCreated {
			t.Fatalf("upload=%d %s", response.Code, response.Body.String())
		}
	}
	response := performAgentRuntimeRequest(t, api, http.MethodGet, agentRuntimePathPrefix+"jobs/"+job.ID+"/platform-files?path=inputs/test.txt", nil, request.Scope.OrganizationID, request.Scope.SubjectID)
	if response.Code != http.StatusOK || response.Body.String() != "input" || bindings.bindings[job.BindingID].Generation != job.BindingGeneration {
		t.Fatalf("transfer changed generation: status=%d generation=%d", response.Code, bindings.bindings[job.BindingID].Generation)
	}
	// Keep the legacy activity semantics: another external transfer increments
	// generation, so an in-flight platform Job must no longer publish data.
	if _, err := service.UploadFile(t.Context(), request.Scope, job.BindingID, "external.txt", strings.NewReader("external"), 8); err != nil {
		t.Fatal(err)
	}
	reader, _, err := service.DownloadPlatformFile(t.Context(), request.Scope, job.ID, "inputs/test.txt")
	if reader != nil {
		_ = reader.Close()
	}
	if !errors.Is(err, ErrGenerationConflict) {
		t.Fatalf("external mutation did not fence Job: %v", err)
	}
	if _, err := service.UploadPlatformFile(t.Context(), request.Scope, job.ID, "must-not-write.txt", io.NopCloser(strings.NewReader("x")), 1); !errors.Is(err, ErrGenerationConflict) || backend.files["must-not-write.txt"] != nil {
		t.Fatalf("stale Job wrote a file: %v", err)
	}
}

func (b *platformExecutionBackend) PlatformRequests(context.Context, Allocation, string) ([]PlatformRequest, error) {
	b.requests++
	return []PlatformRequest{{ID: "r1", Action: "file.export", Arguments: []byte(`{"path":"out.txt"}`)}}, nil
}

func (b *platformExecutionBackend) ResolvePlatformRequest(context.Context, Allocation, string, PlatformResponse) error {
	b.responses++
	return nil
}

func TestPlatformRelayChecksOwnershipGenerationAndLiveJob(t *testing.T) {
	for _, state := range []string{"active", "other_org", "other_subject", "generation", "allocation", "canceled", "terminal", "legacy"} {
		t.Run(state, func(t *testing.T) {
			bindings := newMemoryRepository()
			bindings.bindings["binding-1"] = activeTestBinding()
			jobs := newMemoryJobRepository()
			backend := &platformExecutionBackend{fakeExecutionBackend: newFakeExecutionBackend()}
			service := newTestDataPlane(t, bindings, jobs, backend)
			request := testSubmitJobRequest()
			request.Environment["ZGI_JOB_PLATFORM_ENABLED"] = "true"
			job, err := service.SubmitJob(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			job.Status = JobStatusRunning
			scope := request.Scope
			binding := bindings.bindings[job.BindingID]
			switch state {
			case "other_org":
				scope.OrganizationID = "other"
			case "other_subject":
				scope.SubjectID = "other"
			case "generation":
				binding.Generation++
			case "allocation":
				binding.BackendRef = "other"
			case "canceled":
				job.CancelRequested = true
			case "terminal":
				job.Status = JobStatusSucceeded
			case "legacy":
				delete(job.Environment, "ZGI_JOB_PLATFORM_ENABLED")
			}
			bindings.bindings[job.BindingID] = binding
			jobs.jobs[job.ID] = job
			_, readErr := service.PlatformRequests(t.Context(), scope, job.ID)
			writeErr := service.ResolvePlatformRequest(t.Context(), scope, job.ID, PlatformResponse{ID: "r1", Status: "completed"})
			if state == "active" {
				if readErr != nil || writeErr != nil || backend.requests != 1 || backend.responses != 1 {
					t.Fatalf("active read=%v write=%v calls=%d/%d", readErr, writeErr, backend.requests, backend.responses)
				}
			} else if readErr == nil || writeErr == nil || backend.requests != 0 || backend.responses != 0 {
				t.Fatalf("denied state=%s read=%v write=%v calls=%d/%d", state, readErr, writeErr, backend.requests, backend.responses)
			}
		})
	}
}

func TestPlatformGrantCannotBeStoredInJobEnvironment(t *testing.T) {
	bindings := newMemoryRepository()
	bindings.bindings["binding-1"] = activeTestBinding()
	jobs := newMemoryJobRepository()
	service := newTestDataPlane(t, bindings, jobs, newFakeExecutionBackend())
	request := testSubmitJobRequest()
	request.Environment["ZGI_PLATFORM_TOKEN"] = "caller-token"
	if _, err := service.SubmitJob(t.Context(), request); !errors.Is(err, ErrInvalidRequest) || len(jobs.jobs) != 0 {
		t.Fatalf("grant persisted: err=%v jobs=%d", err, len(jobs.jobs))
	}
}
