package connectorfinalize

import (
	"context"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
)

func TestRuntimeSkillTransitionPreservesAllOtherContractFields(t *testing.T) {
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		for i := range definition.Operations {
			definition.Operations[i].SkillID = "external-apps"
		}
		beforeDigest, err := catalog.DefinitionContractDigest(definition)
		if err != nil {
			t.Fatal(err)
		}
		after, err := PlanCatalogTransition(definition)
		if err != nil {
			t.Fatal(err)
		}
		if after.PluginVersion == definition.PluginVersion {
			t.Fatal("old accepted version reused")
		}
		for _, op := range after.Operations {
			if op.SkillID != "connector-runtime" {
				t.Fatal("runtime skill not transitioned")
			}
		}
		unchangedDigest, _ := catalog.DefinitionContractDigest(definition)
		if beforeDigest != unchangedDigest {
			t.Fatal("old snapshot mutated")
		}
		if catalog.CheckCompatibleUpdate(definition, after) == nil {
			t.Fatal("ordinary admission unexpectedly allowed maintenance transition")
		}
		manifest, err := catalog.BuildAgentPluginManifest([]catalog.Definition{definition})
		if err != nil {
			t.Fatal(err)
		}
		loader, err := catalog.NewPluginLoader()
		if err != nil {
			t.Fatal(err)
		}
		packageVersion, err := loader.LoadManifestPackage(context.Background(), manifest)
		if err != nil || len(packageVersion.Definitions) != 1 {
			t.Fatal("embedded manifest cannot be admitted", err)
		}
		admitted, err := PlanCatalogTransition(packageVersion.Definitions[0])
		if err != nil || admitted.PluginDigest == packageVersion.PluginDigest {
			t.Fatal("content manifest did not produce a new accepted version", err)
		}
		unrecognized := definition
		unrecognized.PluginDigest = strings.Repeat("0", 64)
		if _, err := PlanCatalogTransition(unrecognized); err == nil {
			t.Fatal("unknown package reconstructed from partial metadata")
		}
		after.Operations[0].ToolName = "changed-business-tool"
		if validateSkillIdentityTransition(definition, after) == nil {
			t.Fatal("maintenance accepted unrelated operation changes")
		}
	}
}
