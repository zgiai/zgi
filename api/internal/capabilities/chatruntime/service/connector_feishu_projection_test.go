package service

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"strings"
	"testing"
)

func TestFeishuChatAndAgentProjectionUsesNativeToolSchemasNotCredentials(t *testing.T) {
	d, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	for _, surface := range []catalog.Surface{catalog.SurfaceChat, catalog.SurfaceAgent} {
		t.Run(string(surface), func(t *testing.T) {
			snapshot := connectorChatTestSnapshot(uuid.New(), uuid.New(), uuid.New())
			snapshot.Capabilities[0].ConnectorID = "feishu"
			snapshot.Capabilities[0].Definition = d
			snapshot.Capabilities[0].Operations = d.Operations
			resolved, err := projectConnectorOperations(nil, snapshot, surface)
			if err != nil {
				t.Fatal(err)
			}
			doc, ok := resolved.Get(skills.SkillConnectorRuntime)
			if !ok || len(doc.Tools) != len(d.Operations) {
				t.Fatal("Feishu native operations lost")
			}
			for _, tool := range doc.Tools {
				if tool.ProviderID != "feishu" || !strings.HasPrefix(tool.ConnectorOperationID, "feishu.") {
					t.Fatal("wrong native identity")
				}
			}
			raw, _ := json.Marshal(doc.Tools)
			if strings.Contains(string(raw), "app_secret") || strings.Contains(string(raw), "device_code") || strings.Contains(string(raw), "execute_action") {
				t.Fatal("setup credential or legacy envelope leaked into model tools")
			}
		})
	}
}
