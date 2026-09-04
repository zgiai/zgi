package container

import (
	"slices"
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
	// Adding runtime actions does not rewrite a v3 flow's frozen accepted
	// contract even though the QR permissions themselves are unchanged.
	priorDefinition := d
	priorDefinition.PluginVersion = "1.1.0"
	priorDefinition.Operations = slices.DeleteFunc(slices.Clone(d.Operations), func(op catalog.Operation) bool {
		return op.ID == "feishu.tenant.get" || op.ID == "feishu.chat.members.list"
	})
	priorFlow := f
	priorFlow.AcceptedDefinitionDigest, _ = catalog.DefinitionContractDigest(priorDefinition)
	if priorFlow.AcceptedDefinitionDigest != "66d7eea4a48346671b7756a11be144fbb5704c60e82eb9557cb26f731a19ff43" {
		t.Fatal("prior accepted v3 definition drifted")
	}
	if err := checkOnboardingDefinition(priorFlow, d); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatal("same-template flow silently inherited new operations")
	}
	if err := checkOnboardingDefinition(priorFlow, priorDefinition); err != nil {
		t.Fatal("unchanged v3 template no longer matches its own frozen definition")
	}
	previous := f
	previous.TemplateID = "feishu-app-template-v2"
	previous.TemplateDigest = "4930f0a57d31ed17ed4d86384fd05219cfce7c5b4f20bbfcadec2329307634cc"
	if err := checkOnboardingDefinition(previous, d); !core.IsErrorCode(err, core.ErrorCodeConnectorUnavailable) {
		t.Fatal("v2 flow silently inherited additional v3 permissions")
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
