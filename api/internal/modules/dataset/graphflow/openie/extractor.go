package openie

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/ginext/internal/prompt"
)

// LLMClient generates extraction output for OpenIE.
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// Schema defines the ontology used for OpenIE extraction.
type Schema struct {
	Entities  []EntityDef   `json:"entities"`
	Relations []RelationDef `json:"relations"`
}

type EntityDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Attributes  []string `json:"attributes"`
}

type RelationDef struct {
	Subject     string `json:"subject"`
	Predicate   string `json:"predicate"`
	Object      string `json:"object"`
	Description string `json:"description"`
}

// Extractor performs OpenIE extraction.
type Extractor struct {
	llmClient    LLMClient
	schema       *Schema
	strictSchema bool
}

// NewExtractor creates an OpenIE extractor.
func NewExtractor(llmClient LLMClient) *Extractor {
	return &Extractor{
		llmClient:    llmClient,
		strictSchema: true,
	}
}

// SetSchema sets the schema used during extraction.
func (e *Extractor) SetSchema(schema *Schema, strict bool) {
	e.schema = schema
	e.strictSchema = strict
}

// Triple represents a relationship triple.
type Triple struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
}

// ExtractionResult holds OpenIE entities and triples.
type ExtractionResult struct {
	Entities []interface{} `json:"entities"`
	Triples  []Triple      `json:"triples"`
}

type extractionPromptData struct {
	SchemaJSON  string
	SchemaRules string
	SegmentText string
}

// Extract extracts entities and relationships from text.
func (e *Extractor) Extract(ctx context.Context, text string) (*ExtractionResult, error) {
	promptText, err := e.buildExtractionPrompt(text, e.schema)
	if err != nil {
		return nil, fmt.Errorf("build prompt: %w", err)
	}

	response, err := e.llmClient.Complete(ctx, promptText)
	if err != nil {
		return nil, fmt.Errorf("llm complete: %w", err)
	}

	result, err := parseExtractionResponse(response)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return result, nil
}

func (e *Extractor) buildExtractionPrompt(text string, schema *Schema) (string, error) {
	schemaJSON := "No schema provided."
	schemaRules := "No schema-specific extraction constraints provided."
	if schema != nil {
		payload, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshal schema: %w", err)
		}
		schemaJSON = string(payload)
		schemaRules = e.buildSchemaRules()
	}

	tmpl, err := prompt.GetTemplate(prompt.GraphFlowOpenIEExtraction)
	if err != nil {
		return "", err
	}
	return tmpl.Render(extractionPromptData{
		SchemaJSON:  schemaJSON,
		SchemaRules: schemaRules,
		SegmentText: text,
	})
}

func (e *Extractor) buildSchemaRules() string {
	if e.strictSchema {
		return strings.TrimSpace(`
Rules:
1. STRICTLY ONLY extract entities that match the defined 'entities' types in the schema. Ignore everything else.
2. ONLY extract relationships that match the defined 'relations' in the schema.
3. For entities, use the 'name' field for the entity string.
`)
	}

	return strings.TrimSpace(`
Rules:
1. PRIORITIZE extracting entities that match the defined 'entities' types.
2. ALSO EXTRACT other significant entities and relations found in the text, even if not in the schema.
3. For schema-matched entities, strictly follow the attribute definitions.
`)
}

func parseExtractionResponse(response string) (*ExtractionResult, error) {
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var result ExtractionResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}

	return &result, nil
}

// ExtractBatch performs extraction one text at a time.
func (e *Extractor) ExtractBatch(ctx context.Context, texts []string) ([]*ExtractionResult, error) {
	results := make([]*ExtractionResult, len(texts))

	for i, text := range texts {
		result, err := e.Extract(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("extract text %d: %w", i, err)
		}
		results[i] = result
	}

	return results, nil
}
