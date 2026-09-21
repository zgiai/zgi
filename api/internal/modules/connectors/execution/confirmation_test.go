package execution

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestRuntimeConfirmationCarriesDetachedLocalizedDisplayMetadata(t *testing.T) {
	definition := validDefinition()
	definition.NameI18n = catalog.LocalizedText{"en-US": "GitHub", "zh-Hans": "GitHub"}
	definition.Operations[0].NameI18n = catalog.LocalizedText{"en-US": "Create issue", "zh-Hans": "创建议题"}
	definition.Operations[0].Description = "Create a GitHub issue."
	definition.Operations[0].DescriptionI18n = catalog.LocalizedText{"en-US": "Create a GitHub issue.", "zh-Hans": "创建 GitHub 议题。"}
	accepted := mustAcceptedOperation(t, definition, definition.Operations[0])
	confirmation := runtimeConfirmation("confirmation-display-receipt", accepted, core.TargetRef{Type: core.TargetTypeChat, ID: "chat-1"}, RiskHigh)
	if confirmation.ConnectorName != definition.Name || confirmation.ActionName != definition.Operations[0].Name ||
		confirmation.ConnectorNameI18n["zh-Hans"] != "GitHub" || confirmation.ActionNameI18n["zh-Hans"] != "创建议题" ||
		confirmation.Consequence.DescriptionI18n["zh-Hans"] != "创建 GitHub 议题。" {
		t.Fatalf("localized confirmation = %#v", confirmation)
	}
	encoded, err := json.Marshal(confirmation)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"connector_name_i18n"`) || !strings.Contains(string(encoded), `"action_name_i18n"`) ||
		strings.Contains(string(encoded), `"accepted_operation"`) || !strings.Contains(string(encoded), `"description_i18n"`) {
		t.Fatalf("public localized confirmation = %s", encoded)
	}
	confirmation.ConnectorNameI18n["zh-Hans"] = "changed"
	confirmation.ActionNameI18n["zh-Hans"] = "changed"
	confirmation.Consequence.DescriptionI18n["zh-Hans"] = "changed"
	if accepted.Definition.NameI18n["zh-Hans"] != "GitHub" || accepted.Operation.NameI18n["zh-Hans"] != "创建议题" ||
		accepted.Operation.DescriptionI18n["zh-Hans"] != "创建 GitHub 议题。" {
		t.Fatal("confirmation display mutation changed the pinned operation snapshot")
	}
}

func TestRuntimeConfirmationScalarOnlySnapshotKeepsFallbackAndOmitsMaps(t *testing.T) {
	definition := validDefinition()
	accepted := mustAcceptedOperation(t, definition, definition.Operations[0])
	confirmation := runtimeConfirmation("confirmation-display-receipt", accepted, core.TargetRef{Type: core.TargetTypeChat, ID: "chat-1"}, RiskHigh)
	encoded, err := json.Marshal(confirmation)
	if err != nil {
		t.Fatal(err)
	}
	if confirmation.ConnectorName != definition.Name || confirmation.ActionName != definition.Operations[0].Name ||
		strings.Contains(string(encoded), `"connector_name_i18n"`) || strings.Contains(string(encoded), `"action_name_i18n"`) {
		t.Fatalf("scalar-only confirmation = %s", encoded)
	}
}
