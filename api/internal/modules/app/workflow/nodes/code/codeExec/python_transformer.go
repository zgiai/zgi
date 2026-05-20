package codeexec

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	pythonResultTag      = "<<RESULT>>"
	pythonRunnerTemplate = `# declare main function
%s

import json
from base64 import b64decode

# decode and prepare input dict
inputs_obj = json.loads(b64decode('%s').decode('utf-8'))

# execute main function
output_obj = main(**inputs_obj)

# convert output to json and print
output_json = json.dumps(output_obj, indent=4)
result = f'<<RESULT>>{output_json}<<RESULT>>'
print(result)
`
)

// NewPythonTransformer returns a Python template transformer.
func NewPythonTransformer() TemplateTransformer {
	return &pythonTransformer{}
}

type pythonTransformer struct{}

func (t *pythonTransformer) Language() Language {
	return LanguagePython3
}

func (t *pythonTransformer) TransformCaller(code string, inputs map[string]any) (string, string, error) {
	if strings.TrimSpace(code) == "" {
		return "", "", errors.New("code is empty")
	}

	inputBytes, err := json.Marshal(inputs)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal inputs: %w", err)
	}

	encodedInputs := base64.StdEncoding.EncodeToString(inputBytes)

	runner := fmt.Sprintf(pythonRunnerTemplate, code, encodedInputs)
	return runner, "", nil
}

func (t *pythonTransformer) TransformResponse(raw string) (map[string]any, error) {
	start := strings.Index(raw, pythonResultTag)
	if start == -1 {
		return nil, errors.New("result tag not found in response")
	}

	start += len(pythonResultTag)
	end := strings.Index(raw[start:], pythonResultTag)
	if end == -1 {
		return nil, errors.New("result end tag not found in response")
	}
	end += start

	resultStr := raw[start:end]

	var result map[string]any
	if err := json.Unmarshal([]byte(resultStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse result json: %w", err)
	}
	return result, nil
}
