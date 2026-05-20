package codeexec

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	jsResultTag      = "<<RESULT>>"
	jsRunnerTemplate = `// declare main function
%s

// decode and prepare input object
const inputsBase64 = '%s';
const inputsJson = Buffer.from(inputsBase64, 'base64').toString('utf-8');
const inputsObj = JSON.parse(inputsJson);

// execute main function
async function __run__() {
    const outputObj = await main(inputsObj);
    const outputJson = JSON.stringify(outputObj, null, 4);
    const result = '<<RESULT>>' + outputJson + '<<RESULT>>';
    console.log(result);
}

__run__().catch(err => {
    console.error(err.message || err);
    process.exit(1);
});
`
)

// NewJavaScriptTransformer returns a JavaScript template transformer.
func NewJavaScriptTransformer() TemplateTransformer {
	return &jsTransformer{}
}

type jsTransformer struct{}

func (t *jsTransformer) Language() Language {
	return LanguageJavascript
}

func (t *jsTransformer) TransformCaller(code string, inputs map[string]any) (string, string, error) {
	if strings.TrimSpace(code) == "" {
		return "", "", errors.New("code is empty")
	}

	inputBytes, err := json.Marshal(inputs)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal inputs: %w", err)
	}

	encodedInputs := base64.StdEncoding.EncodeToString(inputBytes)

	runner := fmt.Sprintf(jsRunnerTemplate, code, encodedInputs)
	return runner, "", nil
}

func (t *jsTransformer) TransformResponse(raw string) (map[string]any, error) {
	start := strings.Index(raw, jsResultTag)
	if start == -1 {
		return nil, errors.New("result tag not found in response")
	}

	start += len(jsResultTag)
	end := strings.Index(raw[start:], jsResultTag)
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
