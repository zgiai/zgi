package extractor

import (
	"context"
	"encoding/json"
)

// Extractor is the unified entity extraction interface.
// All extraction strategies (LLM, OpenIE, and future implementations) should implement it.
type Extractor interface {
	// GenerateGlobalEntities extracts global core entities from text
	GenerateGlobalEntities(ctx context.Context, tenantID string, text string) ([]string, error)

	// Extract extracts entities and relationships from text, optionally using global entities as context
	Extract(ctx context.Context, tenantID string, text string, documentTitle string, globalEntities []string) (*ExtractionResult, error)
}

// ExtractionResult is the unified extraction result.
type ExtractionResult struct {
	Entities      []ExtractedEntity       `json:"entities"`
	Relationships []ExtractedRelationship `json:"relations"`
}

// EntityType represents a bilingual entity type with a key and labels.
type EntityType struct {
	Key     string `json:"key"`      // Original English type key (e.g., "Person")
	LabelZh string `json:"label_zh"` // Chinese label value
	LabelEn string `json:"label_en"` // English label (e.g., "Person")
}

// ExtractedEntity is an extracted entity.
type ExtractedEntity struct {
	Name        string     `json:"name"`
	Type        string     `json:"-"` // Populated from TypeInfo.Key for backward compatibility
	TypeInfo    EntityType `json:"-"` // Full bilingual type information
	Description string     `json:"description,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling to handle both:
// 1. Legacy format: {"type": "Person"}
// 2. New format: {"type": {"key": "Person", "label_zh": "Chinese label text", "label_en": "Person"}}
func (e *ExtractedEntity) UnmarshalJSON(data []byte) error {
	// Temporary struct used for parsing
	type entityAlias struct {
		Name        string          `json:"name"`
		Type        json.RawMessage `json:"type"`
		Description string          `json:"description,omitempty"`
	}

	var alias entityAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}

	e.Name = alias.Name
	e.Description = alias.Description

	// Try parsing the type as an object first (new format)
	var typeObj EntityType
	if err := json.Unmarshal(alias.Type, &typeObj); err == nil && typeObj.Key != "" {
		e.TypeInfo = typeObj
		e.Type = typeObj.Key
		return nil
	}

	// Fallback: parse as a string (legacy format)
	var typeStr string
	if err := json.Unmarshal(alias.Type, &typeStr); err == nil {
		e.Type = typeStr
		e.TypeInfo = EntityType{
			Key:     typeStr,
			LabelZh: typeStr, // Default to the key if no translation exists
			LabelEn: typeStr,
		}
		return nil
	}

	// If both parsing attempts fail, keep the type empty
	e.Type = ""
	e.TypeInfo = EntityType{}
	return nil
}

// ExtractedRelationship is an extracted relationship.
type ExtractedRelationship struct {
	Source      string `json:"source"`
	Target      string `json:"target"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}
