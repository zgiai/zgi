package agentexecution

import (
	"context"
	"io"
	"net/http"
	"strconv"
)

func (s *DataPlaneService) platformJob(ctx context.Context, scope Scope, jobID string) (Job, PlatformBackend, error) {
	job, err := s.GetJob(ctx, scope, jobID)
	if err != nil {
		return Job{}, nil, err
	}
	if job.Status != JobStatusRunning || job.CancelRequested || job.Environment["ZGI_JOB_PLATFORM_ENABLED"] != "true" {
		return Job{}, nil, ErrInvalidTransition
	}
	binding, err := s.activeBinding(ctx, scope, job.BindingID, false)
	if err != nil {
		return Job{}, nil, err
	}
	if binding.Generation != job.BindingGeneration || binding.BackendRef != job.BackendRef {
		return Job{}, nil, ErrGenerationConflict
	}
	backend, ok := s.backends[job.Backend].(PlatformBackend)
	if !ok {
		return Job{}, nil, ErrBackendUnavailable
	}
	return job, backend, nil
}

func (s *DataPlaneService) PlatformRequests(ctx context.Context, scope Scope, jobID string) ([]PlatformRequest, error) {
	job, backend, err := s.platformJob(ctx, scope, jobID)
	if err != nil {
		return nil, err
	}
	return backend.PlatformRequests(ctx, allocationForJob(job), job.ID)
}

func (s *DataPlaneService) ResolvePlatformRequest(ctx context.Context, scope Scope, jobID string, response PlatformResponse) error {
	job, backend, err := s.platformJob(ctx, scope, jobID)
	if err != nil {
		return err
	}
	return backend.ResolvePlatformRequest(ctx, allocationForJob(job), job.ID, response)
}

// Platform transfers belong to a running Job and do not renew the Binding's
// CAS generation. Ordinary external file operations retain their existing
// activity update and therefore fence an in-flight platform grant.
func (s *DataPlaneService) UploadPlatformFile(ctx context.Context, scope Scope, jobID, filePath string, reader io.Reader, size int64) (FileInfo, error) {
	clean, err := cleanWorkspacePath(filePath, false)
	if err != nil {
		return FileInfo{}, err
	}
	if reader == nil || size < 0 || size > s.maxFileTransferBytes {
		return FileInfo{}, ErrInvalidRequest
	}
	job, _, err := s.platformJob(ctx, scope, jobID)
	if err != nil {
		return FileInfo{}, err
	}
	info, err := s.backends[job.Backend].UploadFile(ctx, allocationForJob(job), clean, io.LimitReader(reader, size), size)
	if err != nil {
		return FileInfo{}, err
	}
	if _, _, err := s.platformJob(ctx, scope, jobID); err != nil {
		return FileInfo{}, err
	}
	return info, nil
}

func (s *DataPlaneService) DownloadPlatformFile(ctx context.Context, scope Scope, jobID, filePath string) (io.ReadCloser, FileInfo, error) {
	clean, err := cleanWorkspacePath(filePath, false)
	if err != nil {
		return nil, FileInfo{}, err
	}
	job, _, err := s.platformJob(ctx, scope, jobID)
	if err != nil {
		return nil, FileInfo{}, err
	}
	reader, info, err := s.backends[job.Backend].DownloadFile(ctx, allocationForJob(job), clean)
	if err != nil {
		return nil, FileInfo{}, err
	}
	if info.Size < 0 || info.Size > s.maxFileTransferBytes {
		_ = reader.Close()
		return nil, FileInfo{}, ErrOutputLimit
	}
	if _, _, err := s.platformJob(ctx, scope, jobID); err != nil {
		_ = reader.Close()
		return nil, FileInfo{}, err
	}
	return reader, info, nil
}

func (a *HTTPAPI) handlePlatformFiles(w http.ResponseWriter, request *http.Request, scope Scope, jobID string) {
	files, ok := a.data.(interface {
		UploadPlatformFile(context.Context, Scope, string, string, io.Reader, int64) (FileInfo, error)
		DownloadPlatformFile(context.Context, Scope, string, string) (io.ReadCloser, FileInfo, error)
	})
	if !ok {
		a.writeResult(w, http.StatusOK, nil, ErrBackendUnavailable)
		return
	}
	filePath := request.URL.Query().Get("path")
	switch request.Method {
	case http.MethodPut:
		if request.ContentLength < 0 {
			writeAgentRuntimeError(w, http.StatusLengthRequired, "content_length_required", "Content-Length is required")
			return
		}
		if request.ContentLength > defaultFileTransferMax {
			writeAgentRuntimeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "file exceeds the transfer limit")
			return
		}
		info, err := files.UploadPlatformFile(request.Context(), scope, jobID, filePath, request.Body, request.ContentLength)
		a.writeResult(w, http.StatusCreated, info, err)
	case http.MethodGet:
		reader, info, err := files.DownloadPlatformFile(request.Context(), scope, jobID, filePath)
		if err != nil {
			a.writeResult(w, http.StatusOK, nil, err)
			return
		}
		defer reader.Close()
		w.Header().Set("Content-Type", defaultContentType(info.ContentType))
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, io.LimitReader(reader, info.Size))
	default:
		writeMethodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}
