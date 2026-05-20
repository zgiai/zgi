package template

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type TemplateTransformer interface {
	TransformCaller(code string, inputs map[string]any) (runner, preload string, err error)
	TransformResponse(response string) (map[string]any, error)
	GetRunnerScript() string
	GetPreloadScript() string
}

type BaseTemplateTransformer struct {
	CodePlaceholder   string
	InputsPlaceholder string
	ResultTag         string
}

func NewBaseTemplateTransformer() *BaseTemplateTransformer {
	return &BaseTemplateTransformer{
		CodePlaceholder:   "{{code}}",
		InputsPlaceholder: "{{inputs}}",
		ResultTag:         "<<RESULT>>",
	}
}

func (bt *BaseTemplateTransformer) SerializeInputs(inputs map[string]any) (string, error) {
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("failed to marshal inputs: %v", err)
	}

	return base64.StdEncoding.EncodeToString(inputsJSON), nil
}

func (bt *BaseTemplateTransformer) AssembleRunnerScript(code string, inputs map[string]any) (string, error) {
	script := bt.GetRunnerScript()
	if script == "" {
		return "", fmt.Errorf("runner script template is empty")
	}

	script = strings.ReplaceAll(script, bt.CodePlaceholder, code)

	inputsStr, err := bt.SerializeInputs(inputs)
	if err != nil {
		return "", err
	}
	script = strings.ReplaceAll(script, bt.InputsPlaceholder, inputsStr)

	return script, nil
}

func (bt *BaseTemplateTransformer) ExtractResultFromResponse(response string) (string, error) {
	pattern := regexp.MustCompile(regexp.QuoteMeta(bt.ResultTag) + "(.*?)" + regexp.QuoteMeta(bt.ResultTag))
	matches := pattern.FindStringSubmatch(response)

	if len(matches) < 2 {
		return "", fmt.Errorf("failed to parse result: no result tag found in response. Response: %.200s", response)
	}

	return matches[1], nil
}

func (bt *BaseTemplateTransformer) GetRunnerScript() string {
	return ""
}

func (bt *BaseTemplateTransformer) GetPreloadScript() string {
	return ""
}

type Pongo2TemplateTransformer struct {
	*BaseTemplateTransformer
}

func NewPongo2TemplateTransformer() *Pongo2TemplateTransformer {
	return &Pongo2TemplateTransformer{
		BaseTemplateTransformer: NewBaseTemplateTransformer(),
	}
}

func (pt *Pongo2TemplateTransformer) TransformCaller(code string, inputs map[string]any) (string, string, error) {
	script := fmt.Sprintf(`
template := '''%s'''
inputs := '''%s'''
`, code, pt.mustSerializeInputs(inputs))

	preload := pt.GetPreloadScript()
	return script, preload, nil
}

func (pt *Pongo2TemplateTransformer) mustSerializeInputs(inputs map[string]any) string {
	inputsStr, err := pt.SerializeInputs(inputs)
	if err != nil {
		panic(fmt.Sprintf("failed to serialize inputs: %v", err))
	}
	return inputsStr
}

func (pt *Pongo2TemplateTransformer) TransformResponse(response string) (map[string]any, error) {
	result, err := pt.ExtractResultFromResponse(response)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"result": result,
	}, nil
}

func (pt *Pongo2TemplateTransformer) GetRunnerScript() string {
	return "// Pongo2 template will be executed directly in Go"
}

func (pt *Pongo2TemplateTransformer) GetPreloadScript() string {
	return "// No preload needed for pongo2"
}
