package container

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/onboarding"
)

func TestConnectorOnboardingDefinitionPinsCompiledTemplate(t *testing.T) {
	d, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := catalog.DefinitionContractDigest(d)
	f := onboarding.Flow{ConnectorID: d.ID, TemplateID: d.Setup.TemplateID, TemplateDigest: d.Setup.TemplateDigest, AcceptedDefinitionDigest: digest}
	if err := checkOnboardingDefinition(f, d); err != nil {
		t.Fatal(err)
	}
	old := f
	old.TemplateID = "feishu-app-template-v1"
	old.TemplateDigest = "c651f5dad7b40168c2b4a1fb6f09ec42f2254b758dfbae4242b10f16faac0afc"
	if err := checkOnboardingDefinition(old, d); !core.IsErrorCode(err, core.ErrorCodeConnectorUnavailable) {
		t.Fatal("an old flow was allowed to use the new template")
	}
	oldDefinition := d
	setup := *d.Setup
	setup.TemplateID, setup.TemplateDigest = old.TemplateID, old.TemplateDigest
	oldDefinition.Setup = &setup
	oldDefinition.PluginVersion = "1.0.0"
	old.AcceptedDefinitionDigest, _ = catalog.DefinitionContractDigest(oldDefinition)
	if err := checkOnboardingDefinition(old, oldDefinition); !core.IsErrorCode(err, core.ErrorCodeConnectorUnavailable) {
		t.Fatal("a stale accepted definition does not match the compiled registration client")
	}
	changed := f
	changed.TemplateDigest = strings.Repeat("f", 64)
	if err := checkOnboardingDefinition(changed, d); !core.IsErrorCode(err, core.ErrorCodeConnectorUnavailable) {
		t.Fatal("same template ID with different permissions accepted")
	}
	changed = f
	changed.AcceptedDefinitionDigest = strings.Repeat("f", 64)
	if err := checkOnboardingDefinition(changed, d); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatal("changed accepted operation contract accepted")
	}
}
