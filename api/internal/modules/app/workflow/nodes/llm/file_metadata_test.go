package llm

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

func TestFileTemplateProjectsMetadataInsteadOfPointers(t *testing.T) {
	file := &entities.File{ID: "image-id", Filename: "photo.png", Type: "image", Size: 123, StorageKey: "private-storage-key", RemoteURL: "https://private.invalid/signed"}
	for _, segment := range []entities.Segment{
		&entities.FileSegment{Value: file},
		&entities.ArrayFileSegment{Value: []*entities.File{file, nil}},
	} {
		got := (&Node{}).segmentGroupToText(&entities.SegmentGroup{Value: []entities.Segment{segment}})
		if !strings.Contains(got, `"name":"photo.png"`) || !strings.Contains(got, `"size":123`) || !strings.Contains(got, "does not mean image content has been read") {
			t.Fatalf("missing file metadata: %s", got)
		}
		if strings.Contains(got, "private") || strings.Contains(got, "0x") {
			t.Fatalf("private locator or pointer leaked: %s", got)
		}
	}
}

func TestResolvedVisionCapabilityOverridesProviderDefaults(t *testing.T) {
	node := &Node{}
	config := &ModelConfigWithCredentialsEntity{Provider: "openai"}
	config.ModelSchema.Features = []string{}
	image := PromptMessageContent{Type: PromptMessageContentTypeImage}
	if node.isContentSupportedByModel(image, config) {
		t.Fatal("non-vision model inherited its provider's image capability")
	}
	config.ModelSchema.Features = []string{"vision"}
	if !node.isContentSupportedByModel(image, config) {
		t.Fatal("catalog vision capability was ignored")
	}
	if workflowVisionExtensionSupported("svg") || !workflowVisionExtensionSupported(".PNG") {
		t.Fatal("vision format policy differs from chat attachments")
	}
}
