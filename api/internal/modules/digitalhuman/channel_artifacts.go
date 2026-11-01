package digitalhuman

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	toolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"gorm.io/datatypes"
)

const (
	maxChannelDeliveryArtifacts = 8
	maxChannelDeliveryMentions  = 8
	maxChannelArtifactNameRunes = 256
	maxChannelMentionIDRunes    = 255
	maxChannelMentionNameRunes  = 128
	maxChannelDeliveryActions   = 4
)

type channelDeliveryArtifact struct {
	ToolFileID string `json:"tool_file_id"`
	Filename   string `json:"filename,omitempty"`
	Extension  string `json:"extension,omitempty"`
}

type channelDeliveryPayloadData struct {
	Kind      channelruntime.DeliveryKind      `json:"kind,omitempty"`
	Text      string                           `json:"text,omitempty"`
	Mentions  []channelruntime.DeliveryMention `json:"mentions,omitempty"`
	Artifacts []channelDeliveryArtifact        `json:"artifacts,omitempty"`
	Actions   []channelruntime.DeliveryAction  `json:"actions,omitempty"`
	// Card is the structured prompt a provider with cards renders.
	Card *channelruntime.DeliveryCard `json:"card,omitempty"`
	// EditOf names the delivered prompt this delivery edits in place: the
	// card in its new state replaces the one the prompt sent.
	EditOf *uuid.UUID `json:"edit_of,omitempty"`
}

type channelArtifactLink struct {
	Filename string
	URL      string
}

type channelArtifactLinker interface {
	Resolve(context.Context, *channelruntime.Delivery, []channelDeliveryArtifact) ([]channelArtifactLink, error)
}

type channelExecutionArtifactReader interface {
	DescribeChannelExecutionArtifact(context.Context, *channelruntime.Delivery, string) (*toolfile.ToolFile, error)
	ReadChannelExecutionArtifact(context.Context, *channelruntime.Delivery, string) ([]byte, error)
}

type toolFileChannelArtifactLinker struct {
	runtime channelExecutionArtifactReader
}

func (linker toolFileChannelArtifactLinker) Resolve(
	ctx context.Context,
	delivery *channelruntime.Delivery,
	artifacts []channelDeliveryArtifact,
) ([]channelArtifactLink, error) {
	links, _, err := linker.ResolveDelivery(ctx, delivery, artifacts, channelruntime.ProviderCapabilities{})
	return links, err
}

func (linker toolFileChannelArtifactLinker) ResolveDelivery(ctx context.Context, delivery *channelruntime.Delivery, artifacts []channelDeliveryArtifact, capabilities channelruntime.ProviderCapabilities) ([]channelArtifactLink, []channelruntime.DeliveryAttachment, error) {
	if delivery == nil || delivery.OrganizationID == uuid.Nil || len(artifacts) == 0 {
		return nil, nil, nil
	}
	ids := make([]string, 0, len(artifacts))
	seen := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		id := strings.TrimSpace(artifact.ToolFileID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, nil, nil
	}
	files, err := toolfile.GetToolFilesByIDsGlobal(ctx, ids)
	if err != nil {
		return nil, nil, fmt.Errorf("load channel generated files: %w", err)
	}
	now := time.Now()
	links := make([]channelArtifactLink, 0, len(artifacts))
	attachments := make([]channelruntime.DeliveryAttachment, 0, len(artifacts))
	seen = make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		file := files[strings.TrimSpace(artifact.ToolFileID)]
		if file == nil || file.TenantID != delivery.OrganizationID.String() || file.IsExpired(now) {
			continue
		}
		if _, duplicate := seen[file.ID]; duplicate {
			continue
		}
		seen[file.ID] = struct{}{}
		protected := file.ExecutionIdentity != nil || file.ExecutionMessageID != nil
		if protected {
			if linker.runtime == nil {
				return nil, nil, toolfile.ErrExecutionFileAccess
			}
			file, err = linker.runtime.DescribeChannelExecutionArtifact(ctx, delivery, file.ID)
			if err != nil {
				return nil, nil, err
			}
			if file == nil {
				return nil, nil, toolfile.ErrExecutionFileAccess
			}
		}
		filename := safeChannelArtifactFilename(file.Name)
		if filename == "" {
			filename = "generated-file" + file.GetFileExtension()
		}
		if capabilities.NativeAttachments && file.Size > 0 && file.Size <= capabilities.MaxAttachmentBytes {
			fileID, expectedSize := file.ID, file.Size
			attachments = append(attachments, channelruntime.DeliveryAttachment{ID: fileID, Filename: filename, MimeType: file.MimeType, Size: expectedSize,
				Open: func(ctx context.Context) (io.ReadCloser, error) {
					var data []byte
					var readErr error
					if protected {
						data, readErr = linker.runtime.ReadChannelExecutionArtifact(ctx, delivery, fileID)
					} else {
						// Historical unprotected files retain their tenant-scoped contract.
						manager := toolfile.GlobalToolFileManager
						if manager == nil {
							return nil, toolfile.ErrExecutionFileAccess
						}
						var current *toolfile.ToolFile
						current, readErr = manager.GetToolFileByID(ctx, fileID)
						if readErr != nil || current == nil || current.TenantID != delivery.OrganizationID.String() || current.ExecutionIdentity != nil || current.ExecutionMessageID != nil || current.IsExpired(time.Now()) {
							return nil, toolfile.ErrExecutionFileAccess
						}
						data, _, readErr = manager.GetFileBinary(ctx, fileID)
					}
					if readErr != nil {
						return nil, readErr
					}
					if int64(len(data)) != expectedSize {
						return nil, toolfile.ErrExecutionFileAccess
					}
					return io.NopCloser(bytes.NewReader(data)), nil
				}})
			continue
		}
		extension := normalizeChannelArtifactExtension(artifact.Extension)
		if extension == "" {
			extension = normalizeChannelArtifactExtension(filepath.Ext(file.Name))
		}
		if extension == "" {
			extension = normalizeChannelArtifactExtension(file.GetFileExtension())
		}
		downloadURL, signErr := toolfile.SignChannelDeliveryFileGlobal(file, delivery.ID.String(), extension)
		if signErr != nil {
			return nil, nil, fmt.Errorf("sign channel delivery download: %w", signErr)
		}
		filename = safeChannelArtifactFilename(artifact.Filename)
		if filename == "" {
			filename = safeChannelArtifactFilename(file.Name)
		}
		if filename == "" {
			filename = "generated-file" + extension
		}
		links = append(links, channelArtifactLink{Filename: filename, URL: downloadURL})
	}
	return links, attachments, nil
}

func channelDeliveryArtifacts(trace datatypes.JSON) []channelDeliveryArtifact {
	if len(trace) == 0 {
		return nil
	}
	var metadata map[string]any
	if json.Unmarshal(trace, &metadata) != nil {
		return nil
	}
	seen := make(map[string]struct{})
	result := make([]channelDeliveryArtifact, 0, maxChannelDeliveryArtifacts)
	for _, key := range []string{"generated_files", "conversation_artifacts"} {
		for _, item := range channelArtifactRecords(metadata[key]) {
			artifact, ok := channelDeliveryArtifactFromRecord(item)
			if !ok {
				continue
			}
			if _, exists := seen[artifact.ToolFileID]; exists {
				continue
			}
			seen[artifact.ToolFileID] = struct{}{}
			result = append(result, artifact)
			if len(result) == maxChannelDeliveryArtifacts {
				return result
			}
		}
	}
	return result
}

func channelArtifactRecords(value any) []map[string]any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var records []map[string]any
	if json.Unmarshal(encoded, &records) != nil {
		return nil
	}
	return records
}

func channelDeliveryArtifactFromRecord(record map[string]any) (channelDeliveryArtifact, bool) {
	if len(record) == 0 || strings.EqualFold(channelArtifactString(record, "availability"), "expired") ||
		strings.EqualFold(channelArtifactString(record, "target"), "managed_file") ||
		channelArtifactString(record, "upload_file_id") != "" {
		return channelDeliveryArtifact{}, false
	}
	transferMethod := strings.ToLower(channelArtifactString(record, "transfer_method"))
	if transferMethod != "" && transferMethod != "tool_file" {
		return channelDeliveryArtifact{}, false
	}
	toolFileID := channelArtifactString(record, "tool_file_id")
	if toolFileID == "" {
		toolFileID = strings.TrimPrefix(channelArtifactString(record, "artifact_id"), "tool_file:")
	}
	if toolFileID == "" {
		toolFileID = channelArtifactString(record, "file_id")
	}
	if toolFileID == "" {
		return channelDeliveryArtifact{}, false
	}
	return channelDeliveryArtifact{
		ToolFileID: toolFileID,
		Filename:   safeChannelArtifactFilename(channelArtifactString(record, "filename")),
		Extension:  normalizeChannelArtifactExtension(channelArtifactString(record, "extension")),
	}, true
}

func channelArtifactString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return strings.TrimSpace(value)
}

func normalizeChannelArtifactExtension(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || strings.ContainsAny(value, `/\\?&#\r\n\t`) || utf8.RuneCountInString(value) > 16 {
		return ""
	}
	if !strings.HasPrefix(value, ".") {
		value = "." + value
	}
	return value
}

func safeChannelArtifactFilename(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	return boundedRunes(value, maxChannelArtifactNameRunes)
}

func renderChannelDeliveryText(text string, links []channelArtifactLink) string {
	blocks := make([]string, 0, len(links))
	for _, link := range links {
		filename := safeChannelArtifactFilename(link.Filename)
		if filename == "" || strings.TrimSpace(link.URL) == "" {
			continue
		}
		blocks = append(blocks, "📎 "+filename+"\n"+strings.TrimSpace(link.URL))
	}
	attachmentText := strings.Join(blocks, "\n\n")
	if attachmentText == "" {
		return boundedRunes(strings.TrimSpace(text), maxChannelDeliveryRunes)
	}
	if utf8.RuneCountInString(attachmentText) >= maxChannelDeliveryRunes {
		return boundedRunes(attachmentText, maxChannelDeliveryRunes)
	}
	separator := ""
	if strings.TrimSpace(text) != "" {
		separator = "\n\n"
	}
	textLimit := maxChannelDeliveryRunes - utf8.RuneCountInString(separator) - utf8.RuneCountInString(attachmentText)
	return boundedRunes(strings.TrimSpace(text), textLimit) + separator + attachmentText
}
