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

func SystemSkillToolGovernanceManifestByToolName(toolName string) (toolgovernance.Manifest, bool) {
	systemToolGovernanceOnce.Do(loadSystemToolGovernanceCatalog)
	if systemToolGovernanceErr != nil {
		return toolgovernance.Manifest{}, false
	}
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if toolName == "" {
		return toolgovernance.Manifest{}, false
	}
	suffix := "/" + toolName
	var found toolgovernance.Manifest
	matched := false
	for key, manifest := range systemToolGovernance {
		if !strings.HasSuffix(key, suffix) {
			continue
		}
		if matched {
			return toolgovernance.Manifest{}, false
		}
		found = manifest
		matched = true
	}
	return found, matched
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
