package code

import (
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

type CodeLanguage string

const (
	CodeLanguagePython3    CodeLanguage = "python3"
	CodeLanguageJavaScript CodeLanguage = "javascript"
)

// AllowedOutputTypes defines the allowed output types for code nodes
var AllowedOutputTypes = map[shared.SegmentType]bool{
	shared.SegmentTypeString:       true,
	shared.SegmentTypeNumber:       true,
	shared.SegmentTypeObject:       true,
	shared.SegmentTypeBoolean:      true,
	shared.SegmentTypeArrayString:  true,
	shared.SegmentTypeArrayNumber:  true,
	shared.SegmentTypeArrayObject:  true,
	shared.SegmentTypeArrayBoolean: true,
}

// ValidateOutputType validates if the segment type is allowed for code output
func ValidateOutputType(segmentType shared.SegmentType) error {
	if !AllowedOutputTypes[segmentType] {
		return fmt.Errorf("invalid type for code output, expected one of %v, actual %s", getAllowedTypes(), segmentType)
	}
	return nil
}

// getAllowedTypes returns a slice of allowed types for error messages
func getAllowedTypes() []shared.SegmentType {
	types := make([]shared.SegmentType, 0, len(AllowedOutputTypes))
	for t := range AllowedOutputTypes {
		types = append(types, t)
	}
	return types
}

type VariableSelector struct {
	Variable      string   `json:"variable"`
	ValueSelector []string `json:"value_selector"`
}

type Output struct {
	Type     shared.SegmentType `json:"type"`
	Children map[string]*Output `json:"children,omitempty"`
}

// Validate validates the output type and recursively validates children
func (o *Output) Validate() error {
	if err := ValidateOutputType(o.Type); err != nil {
		return err
	}

	// Recursively validate children if they exist
	for key, child := range o.Children {
		if child != nil {
			if err := child.Validate(); err != nil {
				return fmt.Errorf("validation failed for child '%s': %w", key, err)
			}
		}
	}

	return nil
}

type Dependency struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type NodeData struct {
	base.NodeData
	Variables    []VariableSelector `json:"variables"`
	CodeLanguage CodeLanguage       `json:"code_language"`
	Code         string             `json:"code"`
	Outputs      map[string]Output  `json:"outputs"`
	Dependencies []Dependency       `json:"dependencies,omitempty"`
}

// ValidateOutputs validates all output configurations
func (nd *NodeData) ValidateOutputs() error {
	for outputName, output := range nd.Outputs {
		if err := output.Validate(); err != nil {
			return fmt.Errorf("validation failed for output '%s': %w", outputName, err)
		}
	}
	return nil
}

type Node struct {
	base.NodeStruct
	NodeData
}
