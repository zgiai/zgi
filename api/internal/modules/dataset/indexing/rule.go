// Package indexing provides document indexing functionality for the dataset module.
package indexing

import (
	"encoding/json"
)

// Rule struct represents processing rules
type Rule struct {
	PreProcessingRules   []PreProcessingRule       `json:"pre_processing_rules"`
	Segmentation         *SegmentationRule         `json:"segmentation"`
	ParentMode           *string                   `json:"parent_mode,omitempty"`
	SubchunkSegmentation *SubchunkSegmentationRule `json:"subchunk_segmentation,omitempty"`
}

// PreProcessingRule struct represents pre-processing rules
type PreProcessingRule struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// SegmentationRule struct represents segmentation rules
type SegmentationRule struct {
	MaxTokens    int    `json:"max_tokens"`
	ChunkOverlap int    `json:"chunk_overlap"`
	Separator    string `json:"separator"`
}

// SubchunkSegmentationRule struct represents subchunk segmentation rules
type SubchunkSegmentationRule struct {
	MaxTokens    int    `json:"max_tokens"`
	ChunkOverlap int    `json:"chunk_overlap"`
	Separator    string `json:"separator"`
}

// ParseRule parses Rule object from map
func ParseRule(rulesMap map[string]interface{}) (*Rule, error) {
	rule := &Rule{}

	// Parse pre_processing_rules
	if preProcessingRules, ok := rulesMap["pre_processing_rules"].([]interface{}); ok {
		for _, item := range preProcessingRules {
			if ruleMap, ok := item.(map[string]interface{}); ok {
				preRule := PreProcessingRule{}
				if id, ok := ruleMap["id"].(string); ok {
					preRule.ID = id
				}
				if enabled, ok := ruleMap["enabled"].(bool); ok {
					preRule.Enabled = enabled
				}
				rule.PreProcessingRules = append(rule.PreProcessingRules, preRule)
			}
		}
	}

	// Parse segmentation with default values
	segmentation := SegmentationRule{
		MaxTokens:    500,  // Default value
		ChunkOverlap: 0,    // Default value
		Separator:    "\n", // Default value
	}
	if segmentationMap, ok := rulesMap["segmentation"].(map[string]interface{}); ok {
		if maxTokens, ok := segmentationMap["max_tokens"].(int); ok {
			segmentation.MaxTokens = maxTokens
		} else if maxTokens, ok := segmentationMap["max_tokens"].(float64); ok {
			segmentation.MaxTokens = int(maxTokens)
		}

		if chunkOverlap, ok := segmentationMap["chunk_overlap"].(int); ok {
			segmentation.ChunkOverlap = chunkOverlap
		} else if chunkOverlap, ok := segmentationMap["chunk_overlap"].(float64); ok {
			segmentation.ChunkOverlap = int(chunkOverlap)
		}

		if separator, ok := segmentationMap["separator"].(string); ok {
			segmentation.Separator = separator
		}
	}
	rule.Segmentation = &segmentation

	// Parse parent_mode
	if parentMode, ok := rulesMap["parent_mode"].(string); ok {
		rule.ParentMode = &parentMode
	}

	// Parse subchunk_segmentation with default values
	subchunkSegmentation := SubchunkSegmentationRule{
		MaxTokens:    100,  // Default value for subchunk
		ChunkOverlap: 20,   // Default value for subchunk
		Separator:    "\n", // Default value for subchunk
	}
	if subchunkSegmentationMap, ok := rulesMap["subchunk_segmentation"].(map[string]interface{}); ok {
		if maxTokens, ok := subchunkSegmentationMap["max_tokens"].(int); ok {
			subchunkSegmentation.MaxTokens = maxTokens
		} else if maxTokens, ok := subchunkSegmentationMap["max_tokens"].(float64); ok {
			subchunkSegmentation.MaxTokens = int(maxTokens)
		}

		if chunkOverlap, ok := subchunkSegmentationMap["chunk_overlap"].(int); ok {
			subchunkSegmentation.ChunkOverlap = chunkOverlap
		} else if chunkOverlap, ok := subchunkSegmentationMap["chunk_overlap"].(float64); ok {
			subchunkSegmentation.ChunkOverlap = int(chunkOverlap)
		}

		if separator, ok := subchunkSegmentationMap["separator"].(string); ok {
			subchunkSegmentation.Separator = separator
		}
	}
	rule.SubchunkSegmentation = &subchunkSegmentation

	return rule, nil
}

// RuleToJSONMap converts a Rule struct to map[string]interface{}
func RuleToJSONMap(rule *Rule) (map[string]interface{}, error) {
	// Convert the rule to JSON and then back to map[string]interface{}
	// This is a simple way to convert struct to map
	jsonData, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}

	var jsonMap map[string]interface{}
	err = json.Unmarshal(jsonData, &jsonMap)
	if err != nil {
		return nil, err
	}

	return jsonMap, nil
}
