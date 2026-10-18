package llm

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

func (n *Node) resolveModelVision(ctx context.Context, config *ModelConfigWithCredentialsEntity) error {
	if n.db == nil {
		return nil // Isolated legacy callers have no catalog; keep their existing contract.
	}
	config.ModelSchema.Features = []string{}
	organizationID, err := uuid.Parse(n.OrganizationID)
	if err != nil {
		return err
	}
	spec, found, err := chatruntime.NewDatabaseModelSpecResolver(n.db).Resolve(ctx, organizationID, config.Provider, config.Model)
	if err == nil && found && spec.SupportsVision() {
		config.ModelSchema.Features = []string{"vision"}
	}
	return err
}

func workflowVisionExtensionSupported(extension string) bool {
	switch strings.TrimPrefix(strings.ToLower(extension), ".") {
	case "png", "jpg", "jpeg", "gif", "webp":
		return true
	default:
		return false
	}
}

// fileMetadataText projects file variables without exposing storage keys, signed
// URLs or Go pointer addresses. Vision payloads are handled separately.
func fileMetadataText(value any) string {
	type metadata struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Type     string `json:"type"`
		Size     int64  `json:"size"`
		MIMEType string `json:"mime_type"`
	}
	files := []*entities.File{}
	switch typed := value.(type) {
	case *entities.File:
		files = append(files, typed)
	case []*entities.File:
		files = typed
	}
	items := make([]metadata, 0, len(files))
	for _, file := range files {
		if file != nil {
			items = append(items, metadata{ID: file.ID, Name: file.Filename, Type: file.Type, Size: file.Size, MIMEType: file.MimeType})
		}
	}
	data, _ := json.Marshal(items) // Only strings and integers are projected.
	return string(data) + "\nFile metadata does not mean image content has been read."
}
