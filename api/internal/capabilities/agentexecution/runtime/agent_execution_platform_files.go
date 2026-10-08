package executionruntime

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path"
	"strings"
	"sync"

	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeplatform"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// File transfer reuses the current turn's admitted attachment references and
// the existing owner-scoped Worker/file store. Script arguments cannot select
// an organization, user, conversation, Binding or storage URL.
type agentPlatformFiles struct {
	gateway       agentExecutionGateway
	transfers     agentExecutionPlatformFileGateway
	persister     agentExecutionArtifactPersister
	reader        agentRuntimeAttachmentReader
	attachments   []runtimeservice.RuntimeAttachmentRef
	skillResolver agentSkillPackageResolver
	skillAllowed  func(context.Context, string) (bool, error)
	mu            sync.Mutex
	artifacts     []map[string]interface{}
}

func (f *agentPlatformFiles) Handle(ctx context.Context, scope runtimeplatform.Scope, request runtimeplatform.Request) (runtimeplatform.Response, error) {
	if f.gateway == nil {
		return runtimeplatform.Response{}, platformFileError(catalog.CodeInternal, "runtime_platform.file_gateway", nil)
	}
	var args struct {
		Path string `json:"path"`
		Ref  string `json:"ref,omitempty"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(request.Arguments)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return runtimeplatform.Response{}, platformFileError(catalog.CodeRequestInvalid, "runtime_platform.file_arguments", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return runtimeplatform.Response{}, platformFileError(catalog.CodeRequestInvalid, "runtime_platform.file_arguments", nil)
	}
	if !validPlatformFilePath(args.Path) {
		return runtimeplatform.Response{}, platformFileError(catalog.CodeRequestInvalid, "runtime_platform.file_path", nil)
	}
	executionScope := agentExecutionScope{OrganizationID: scope.OrganizationID, SubjectID: scope.SubjectID}
	if scope.ExecutionIdentity.Version != 0 {
		executionScope.Identity = &scope.ExecutionIdentity
	}
	capabilities, err := f.gateway.Capabilities(ctx, executionScope, scope.BindingID)
	if err != nil || !capabilities.FileTransfer {
		return runtimeplatform.Response{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.file_capability", err)
	}
	switch request.Action {
	case runtimeplatform.ReadFile:
		if strings.HasPrefix(args.Ref, "skill:") {
			return f.readSkill(ctx, executionScope, scope.BindingID, scope.JobID, capabilities, args.Ref, args.Path)
		}
		if f.reader == nil || capabilities.MaxUploadBytes <= 0 {
			return runtimeplatform.Response{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.file_read", nil)
		}
		var reference *runtimeservice.RuntimeAttachmentRef
		for i := range f.attachments {
			if f.attachments[i].ID == args.Ref {
				reference = &f.attachments[i]
				break
			}
		}
		if reference == nil {
			return runtimeplatform.Response{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.input_reference", nil)
		}
		if reference.Size < 0 || reference.Size > agentExecutionFileLimit || reference.Size > capabilities.MaxUploadBytes {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeRequestInvalid, "runtime_platform.file_size", nil)
		}
		data, err := f.reader.DownloadFile(ctx, reference.ID)
		if err != nil {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeResourceNotFound, "runtime_platform.input_read", err)
		}
		if int64(len(data)) != reference.Size || len(data) > agentExecutionFileLimit || int64(len(data)) > capabilities.MaxUploadBytes {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeRequestInvalid, "runtime_platform.file_size", nil)
		}
		if err := f.upload(ctx, executionScope, scope.BindingID, scope.JobID, args.Path, data); err != nil {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeInternal, "runtime_platform.input_upload", err)
		}
		result, _ := json.Marshal(map[string]any{"workspace_path": args.Path, "size": len(data)})
		return runtimeplatform.Response{Status: runtimeplatform.Completed, Data: result}, nil
	case runtimeplatform.ExportFile:
		if args.Ref != "" || f.persister == nil || capabilities.MaxDownloadBytes <= 0 {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeRequestInvalid, "runtime_platform.file_export", nil)
		}
		var data []byte
		if f.transfers != nil {
			data, err = f.transfers.DownloadPlatformFile(ctx, executionScope, scope.JobID, args.Path)
		} else {
			data, err = f.gateway.DownloadFile(ctx, executionScope, scope.BindingID, args.Path)
		}
		if err != nil {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeResourceNotFound, "runtime_platform.result_read", err)
		}
		if len(data) > agentExecutionFileLimit || int64(len(data)) > capabilities.MaxDownloadBytes {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeRequestInvalid, "runtime_platform.file_size", nil)
		}
		artifact, err := f.persister.Persist(ctx, executionScope, scope.ConversationID, args.Path, data)
		if err != nil {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeInternal, "runtime_platform.result_persist", err)
		}
		if len(artifact) == 0 {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeInternal, "runtime_platform.result_metadata", nil)
		}
		artifact["conversation_id"], artifact["message_id"] = scope.ConversationID, scope.RunID
		artifact["skill_id"], artifact["tool_name"], artifact["invocation_id"] = "agent_execution", "workspace_export_file", "platform:"+request.ID
		result, err := json.Marshal(map[string]any{"file": artifact})
		if err != nil || len(result) > runtimeplatform.MaxResponseBytes {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeInternal, "runtime_platform.result_metadata", err)
		}
		f.mu.Lock()
		f.artifacts = append(f.artifacts, artifact)
		f.mu.Unlock()
		return runtimeplatform.Response{Status: runtimeplatform.Completed, Data: result}, nil
	default:
		return runtimeplatform.Response{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.file_action", nil)
	}
}

// The reference binds a currently loaded Skill and immutable version, never a
// host path: skill:<skill-id>:<package-sha256>:<package-relative-file>.
func (f *agentPlatformFiles) readSkill(ctx context.Context, scope agentExecutionScope, bindingID, jobID string, capabilities agentExecutionCapabilities, ref, destination string) (runtimeplatform.Response, error) {
	parts := strings.SplitN(ref, ":", 4)
	if len(parts) != 4 || len(parts[2]) != 64 || !validPlatformFilePath(parts[3]) || f.skillAllowed == nil || f.skillResolver == nil || capabilities.MaxUploadBytes <= 0 {
		return runtimeplatform.Response{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.skill_reference", nil)
	}
	allowed, err := f.skillAllowed(ctx, parts[1])
	if err != nil || !allowed {
		return runtimeplatform.Response{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.skill_scope", err)
	}
	pack, err := f.skillResolver(ctx, parts[1])
	if err != nil || pack.ExpectedDigest != parts[2] {
		return runtimeplatform.Response{}, platformFileError(catalog.CodeResourceConflict, "runtime_platform.skill_version", err)
	}
	archive, digest, err := skills.BuildPortableSkillArchive(pack.Root)
	if err != nil || digest != parts[2] {
		return runtimeplatform.Response{}, platformFileError(catalog.CodeResourceConflict, "runtime_platform.skill_integrity", err)
	}
	bundle, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return runtimeplatform.Response{}, platformFileError(catalog.CodeInternal, "runtime_platform.skill_archive", err)
	}
	for _, entry := range bundle.File {
		if entry.Name != parts[3] || entry.FileInfo().IsDir() {
			continue
		}
		limit := min(int64(agentExecutionFileLimit), capabilities.MaxUploadBytes)
		if entry.UncompressedSize64 > uint64(limit) {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeRequestInvalid, "runtime_platform.file_size", nil)
		}
		source, err := entry.Open()
		if err != nil {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeInternal, "runtime_platform.skill_file", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(source, limit+1))
		closeErr := source.Close()
		if readErr != nil || closeErr != nil || int64(len(data)) > limit {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeInternal, "runtime_platform.skill_file", errors.Join(readErr, closeErr))
		}
		allowed, err = f.skillAllowed(ctx, parts[1])
		if err != nil || !allowed {
			return runtimeplatform.Response{}, platformFileError(catalog.CodePermissionDenied, "runtime_platform.skill_scope", err)
		}
		if err := f.upload(ctx, scope, bindingID, jobID, destination, data); err != nil {
			return runtimeplatform.Response{}, platformFileError(catalog.CodeInternal, "runtime_platform.skill_upload", err)
		}
		result, _ := json.Marshal(map[string]any{"workspace_path": destination, "size": len(data)})
		return runtimeplatform.Response{Status: runtimeplatform.Completed, Data: result}, nil
	}
	return runtimeplatform.Response{}, platformFileError(catalog.CodeResourceNotFound, "runtime_platform.skill_file", nil)
}

func (f *agentPlatformFiles) upload(ctx context.Context, scope agentExecutionScope, bindingID, jobID, path string, data []byte) error {
	if f.transfers != nil {
		return f.transfers.UploadPlatformFile(ctx, scope, jobID, path, data)
	}
	return f.gateway.UploadFile(ctx, scope, bindingID, path, data)
}

func (f *agentPlatformFiles) Artifacts() []map[string]interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	artifacts := make([]map[string]interface{}, len(f.artifacts))
	for i, artifact := range f.artifacts {
		copy := make(map[string]interface{}, len(artifact))
		for key, value := range artifact {
			copy[key] = value
		}
		artifacts[i] = copy
	}
	return artifacts
}

func validPlatformFilePath(value string) bool {
	return value != "" && len(value) <= 256 && !strings.ContainsAny(value, "\\\x00\r\n") && !path.IsAbs(value) && path.Clean(value) == value && value != "." && value != ".." && !strings.HasPrefix(value, "../")
}

func platformFileError(code apperror.Code, operation string, cause error) error {
	if cause == nil {
		return apperror.New(code, apperror.WithOperation(operation))
	}
	return apperror.Wrap(cause, code, apperror.WithOperation(operation))
}
