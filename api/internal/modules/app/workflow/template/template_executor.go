package template

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

type TemplateLanguage string

const (
	LanguagePongo2 TemplateLanguage = "pongo2"
)

type TemplateExecutionError struct {
	Message string
	Code    int
}

func (e *TemplateExecutionError) Error() string {
	return fmt.Sprintf("template execution error (code %d): %s", e.Code, e.Message)
}

type TemplateExecutionResponse struct {
	Code int                    `json:"code"`
	Data map[string]interface{} `json:"data"`
}

type TemplateExecutor struct {
	transformers map[TemplateLanguage]TemplateTransformer
}

func NewTemplateExecutor() *TemplateExecutor {
	return &TemplateExecutor{
		transformers: map[TemplateLanguage]TemplateTransformer{
			LanguagePongo2: NewPongo2TemplateTransformer(),
		},
	}
}

func (te *TemplateExecutor) ExecuteWorkflowCodeTemplate(
	language TemplateLanguage,
	code string,
	inputs map[string]interface{},
) (map[string]interface{}, error) {
	transformer, ok := te.transformers[language]
	if !ok {
		return nil, &TemplateExecutionError{
			Message: fmt.Sprintf("Unsupported language %s", language),
			Code:    400,
		}
	}

	runner, preload, err := transformer.TransformCaller(code, inputs)
	if err != nil {
		return nil, err
	}

	response, err := te.executeCode(language, preload, runner)
	if err != nil {
		return nil, err
	}

	return transformer.TransformResponse(response)
}

func (te *TemplateExecutor) executeCode(language TemplateLanguage, preload, code string) (string, error) {
	if language == LanguagePongo2 {
		return te.executePongo2Direct(code)
	}

	return "", &TemplateExecutionError{
		Message: fmt.Sprintf("Unsupported language %s", language),
		Code:    400,
	}
}

func (te *TemplateExecutor) executePongo2Direct(code string) (string, error) {
	codeLines := strings.Split(code, "\n")
	var templateContent string
	var inputsBase64 string

	for _, line := range codeLines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "template := '''") {
			start := strings.Index(line, "'''") + 3
			end := strings.LastIndex(line, "'''")
			if start > 2 && end > start {
				templateContent = line[start:end]
			}
		}
		if strings.Contains(line, "inputs := '''") {
			start := strings.Index(line, "'''") + 3
			end := strings.LastIndex(line, "'''")
			if start > 2 && end > start {
				inputsBase64 = line[start:end]
			}
		}
	}

	if templateContent == "" || inputsBase64 == "" {
		return "", fmt.Errorf("failed to extract template or inputs from code")
	}

	inputsBytes, err := base64.StdEncoding.DecodeString(inputsBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode inputs: %v", err)
	}

	var inputs map[string]interface{}
	if err := json.Unmarshal(inputsBytes, &inputs); err != nil {
		return "", fmt.Errorf("failed to unmarshal inputs: %v", err)
	}

	renderer := NewPongo2Renderer()
	result, err := renderer.Render(templateContent, inputs)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("<<RESULT>>%s<<RESULT>>", result), nil
}

func getInputKeys(inputs map[string]interface{}) []string {
	keys := make([]string, 0, len(inputs))
	for k := range inputs {
		keys = append(keys, k)
	}
	return keys
}

func (te *TemplateExecutor) ExecutePongo2Template(
	templateStr string,
	inputs map[string]interface{},
) (string, error) {
	renderer := NewPongo2Renderer()
	return renderer.Render(templateStr, inputs)
}

var DefaultTemplateExecutor = NewTemplateExecutor()

func ExecuteWorkflowCodeTemplate(
	language TemplateLanguage,
	code string,
	inputs map[string]any,
) (map[string]any, error) {
	return DefaultTemplateExecutor.ExecuteWorkflowCodeTemplate(language, code, inputs)
}

func ExecutePongo2Template(
	templateStr string,
	inputs map[string]interface{},
) (string, error) {
	return DefaultTemplateExecutor.ExecutePongo2Template(templateStr, inputs)
}
