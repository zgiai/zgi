package skills

import (
	"os"
	"strings"
	"sync"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

var (
	systemToolGovernanceOnce sync.Once
	systemToolGovernance     map[string]toolgovernance.Manifest
	systemToolGovernanceErr  error
)

func SystemSkillToolGovernanceManifest(skillID, toolName string) (toolgovernance.Manifest, bool) {
	systemToolGovernanceOnce.Do(loadSystemToolGovernanceCatalog)
	if systemToolGovernanceErr != nil {
		return toolgovernance.Manifest{}, false
	}
	manifest, ok := systemToolGovernance[systemToolGovernanceKey(skillID, toolName)]
	return manifest, ok
}

func loadSystemToolGovernanceCatalog() {
	runtime := NewRuntime(nil, nil)
	entries, err := os.ReadDir(runtime.catalogDir)
	if err != nil {
		systemToolGovernanceErr = err
		return
	}

	manifests := map[string]toolgovernance.Manifest{}
	for _, entry := range entries {
		if entry == nil || !entry.IsDir() {
			continue
		}
		doc, err := runtime.loadSkillDocument(entry.Name())
		if err != nil {
			systemToolGovernanceErr = err
			return
		}
		for _, tool := range doc.Tools {
			if tool.Governance == nil {
				continue
			}
			manifests[systemToolGovernanceKey(doc.Metadata.ID, tool.Name)] = *tool.Governance
		}
	}
	systemToolGovernance = manifests
}

func systemToolGovernanceKey(skillID, toolName string) string {
	return normalizeSkillID(skillID) + "/" + strings.ToLower(strings.TrimSpace(toolName))
}
