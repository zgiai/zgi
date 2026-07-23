package service

import (
	"encoding/json"
	"sort"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func skillReferencePaths(doc skills.SkillDocument) []string {
	references := make([]string, 0, len(doc.Metadata.References))
	for _, ref := range doc.Metadata.References {
		references = append(references, ref.Path)
	}
	sort.Strings(references)
	return references
}

func previewValidationErrors(preview *SkillImportPreview) []string {
	if preview == nil || len(preview.ValidationErrors) == 0 {
		return []string{"skill package cannot be imported"}
	}
	return preview.ValidationErrors
}

func skillDisplayMap(display skills.SkillDisplayMetadata) map[string]interface{} {
	data, err := json.Marshal(display)
	if err != nil {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{}
	}
	if out == nil {
		return map[string]interface{}{}
	}
	return out
}
