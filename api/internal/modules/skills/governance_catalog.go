package skills

import (
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
	locations, errs, err := runtime.systemSkillLocationsBestEffort()
	if err != nil {
		systemToolGovernanceErr = err
		return
	}

	manifests := map[string]toolgovernance.Manifest{}
	for _, location := range locations {
		doc, err := runtime.loadSkillDocumentFromLocation(location)
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
	if len(manifests) == 0 && len(errs) > 0 {
		systemToolGovernanceErr = errs[0]
		return
	}
	systemToolGovernance = manifests
}

func systemToolGovernanceKey(skillID, toolName string) string {
	return normalizeSkillID(skillID) + "/" + strings.ToLower(strings.TrimSpace(toolName))
}
