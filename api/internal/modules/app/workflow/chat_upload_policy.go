package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
	"github.com/zgiai/zgi/api/internal/dto"
	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

// WorkflowChatUploadPolicy resolves only the conversational attachment feature.
// Typed file variables retain their existing, independent contract.
func WorkflowChatUploadPolicy(features any, config *interfaces.FileUploadConfigResponse) chatfile.Effective {
	var value struct {
		FileUpload struct {
			Enabled    bool     `json:"enabled"`
			Types      []string `json:"allowed_file_types"`
			Extensions []string `json:"allowed_file_extensions"`
			Count      int      `json:"number_limits"`
		} `json:"file_upload"`
	}
	data, err := json.Marshal(features)
	if err != nil || json.Unmarshal(data, &value) != nil {
		return config.ChatPolicy(&chatfile.Policy{AllowedFileTypes: []string{}})
	}
	policy := config.ChatPolicy(&chatfile.Policy{AllowedFileTypes: value.FileUpload.Types})
	if !value.FileUpload.Enabled {
		policy.AllowedExtensions = nil
		return policy
	}
	if value.FileUpload.Count > 0 {
		policy.NumberLimit = value.FileUpload.Count
	}
	if config != nil && config.WorkflowFileUploadLimit > 0 {
		policy.NumberLimit = min(policy.NumberLimit, config.WorkflowFileUploadLimit)
	}
	// Preserve existing custom/media workflow configurations without making
	// those types available to newly created chat configurations.
	if slices.Contains(value.FileUpload.Types, "custom") {
		policy.AllowedExtensions = nil
		for _, extension := range value.FileUpload.Extensions {
			policy.AllowedExtensions = append(policy.AllowedExtensions, strings.TrimPrefix(strings.ToLower(extension), "."))
		}
	} else {
		for _, kind := range value.FileUpload.Types {
			switch kind {
			case "audio":
				policy.AllowedExtensions = append(policy.AllowedExtensions, "mp3", "m4a", "wav", "amr", "mpga")
			case "video":
				policy.AllowedExtensions = append(policy.AllowedExtensions, "mp4", "mov", "webm", "mpeg")
			}
		}
	}
	return policy
}

// ValidateChatAttachments checks original file metadata before parsing or
// execution. Authorization is supplied by the authenticated entry point.
func ValidateChatAttachments(ctx context.Context, files []dto.FileInfo, service interfaces.FileService, policy chatfile.Effective, authorize func(context.Context, *dto.UploadFile) error) error {
	if len(files) == 0 {
		return nil
	}
	if service == nil || authorize == nil {
		return fmt.Errorf("chat attachment access is unavailable")
	}
	seen := make(map[string]bool, len(files))
	sizes := make([]int64, 0, len(files))
	for _, ref := range files {
		id := strings.TrimSpace(ref.UploadFileID)
		if id == "" {
			return fmt.Errorf("chat attachments require an uploaded file reference")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		file, err := service.GetFileByID(ctx, id)
		if err != nil || file == nil || file.ID != id {
			return fmt.Errorf("chat attachment is unavailable")
		}
		if err := authorize(ctx, file); err != nil {
			return fmt.Errorf("authorize chat attachment: %w", err)
		}
		extension := chatfile.Extension(file.Name)
		if extension == "" {
			extension = strings.TrimPrefix(strings.ToLower(file.Extension), ".")
		}
		// Retain category-specific legacy media limits.
		kind := workflowfile.InferFileType(extension, "")
		if kind == workflowfile.FileTypeAudio || kind == workflowfile.FileTypeVideo {
			if err := policy.ValidateFile(extension, 0); err != nil {
				return err
			}
			if file.Size < 0 || !service.IsFileSizeWithinLimit(extension, file.Size) {
				return chatfile.ErrFileTooLarge
			}
		} else if err := policy.ValidateFile(extension, file.Size); err != nil {
			return err
		}
		sizes = append(sizes, file.Size)
	}
	return policy.ValidateTurn(sizes)
}

// ChatAttachmentReferences also covers the legacy #files# input spelling. It
// deliberately does not collect typed file variables from arbitrary inputs.
func ChatAttachmentReferences(files []dto.FileInfo, inputs map[string]interface{}) ([]dto.FileInfo, error) {
	if len(files) > 0 || inputs["#files#"] == nil {
		return files, nil
	}
	data, err := json.Marshal(inputs["#files#"])
	if err != nil {
		return nil, fmt.Errorf("invalid chat attachments: %w", err)
	}
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("invalid chat attachments: %w", err)
	}
	return files, nil
}

// chatInputsWithFiles keeps blocking and streaming chat requests on the same
// sys.files path without mutating the caller's input map or typed file variables.
func chatInputsWithFiles(inputs map[string]interface{}, files []dto.FileInfo) map[string]interface{} {
	result := make(map[string]interface{}, len(inputs)+1)
	for key, value := range inputs {
		if key == "sys.files" {
			continue // Only admitted chat attachments may populate the system file pool.
		}
		result[key] = value
	}
	if len(files) == 0 {
		return result
	}
	values := make([]interface{}, 0, len(files))
	for _, file := range files {
		values = append(values, map[string]interface{}{
			"type": file.Type, "transfer_method": file.TransferMethod,
			"url": file.URL, "upload_file_id": file.UploadFileID,
		})
	}
	result["#files#"] = values
	return result
}
