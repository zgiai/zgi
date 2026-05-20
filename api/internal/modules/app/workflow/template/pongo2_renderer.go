package template

import (
	"context"
	"fmt"
	"regexp"

	"github.com/flosch/pongo2/v6"
	"github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type Pongo2Renderer struct {
	fileService  interfaces.FileService
	variablePool map[string]interface{}
}

func NewPongo2Renderer() *Pongo2Renderer {
	return &Pongo2Renderer{}
}

func NewPongo2RendererWithFileService(fileService interfaces.FileService) *Pongo2Renderer {
	renderer := &Pongo2Renderer{
		fileService: fileService,
	}
	renderer.registerFileFilter()
	return renderer
}

func NewPongo2RendererWithVariablePool(variablePool map[string]interface{}) *Pongo2Renderer {
	renderer := &Pongo2Renderer{
		variablePool: variablePool,
	}
	renderer.registerFileFilter()
	return renderer
}

func NewPongo2RendererWithFileServiceAndVariablePool(fileService interfaces.FileService, variablePool map[string]interface{}) *Pongo2Renderer {
	renderer := &Pongo2Renderer{
		fileService:  fileService,
		variablePool: variablePool,
	}
	renderer.registerFileFilter()
	return renderer
}

func preprocessFileContentSyntax(templateStr string) string {
	re := regexp.MustCompile(`\{\{#([^#]+)#\}\}`)
	result := re.ReplaceAllStringFunc(templateStr, func(match string) string {
		varPath := re.FindStringSubmatch(match)[1]
		return "{{" + varPath + "_content}}"
	})

	return result
}

func (pr *Pongo2Renderer) registerFileFilter() {
	pongo2.RegisterFilter("file_content", func(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		if !in.IsNil() && in.IsString() {
			varName := in.String()

			if pr.variablePool != nil {
				contentVarName := varName + "_content"
				if contentValue, exists := pr.variablePool[contentVarName]; exists {
					if contentStr, ok := contentValue.(string); ok && contentStr != "" {
						const maxContentSize = 100 * 1024
						if len(contentStr) > maxContentSize {
							contentStr = contentStr[:maxContentSize] + "\n... (content truncated due to size limit)"
						}
						return pongo2.AsValue(contentStr), nil
					}
				}
			}
		}

		if !in.IsNil() && in.CanSlice() {
			fileMap, ok := in.Interface().(map[string]interface{})
			if !ok {
				return in, nil
			}

			uploadFileID, exists := fileMap["upload_file_id"]
			if !exists {
				return pongo2.AsValue("[File: No upload_file_id]"), nil
			}

			fileIDStr, ok := uploadFileID.(string)
			if !ok || fileIDStr == "" {
				return pongo2.AsValue("[File: Invalid upload_file_id]"), nil
			}

			if pr.fileService != nil {
				content, err := pr.fileService.GetFilePreview(context.Background(), fileIDStr)
				if err != nil {
					logger.Error(fmt.Sprintf("Failed to get file content for %s", fileIDStr), err)
					return pongo2.AsValue(fmt.Sprintf("[File: %s (content unavailable)]", fileIDStr)), nil
				}

				const maxContentSize = 100 * 1024
				if len(content) > maxContentSize {
					content = content[:maxContentSize] + "\n... (content truncated due to size limit)"
				}

				return pongo2.AsValue(content), nil
			}

			filename, _ := fileMap["name"].(string)
			if filename == "" {
				filename = fileIDStr
			}
			return pongo2.AsValue(fmt.Sprintf("[File: %s]", filename)), nil
		}

		return in, nil
	})
}

func (pr *Pongo2Renderer) Render(templateStr string, variables map[string]interface{}) (string, error) {
	originalTemplate := templateStr
	templateStr = preprocessFileContentSyntax(templateStr)

	if originalTemplate != templateStr {
		logger.Info(fmt.Sprintf("[Pongo2Renderer] Template preprocessed: '%s' -> '%s'", originalTemplate, templateStr))
	}

	tmpl, err := pongo2.FromString(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse pongo2 template: %v", err)
	}

	ctx := pongo2.Context{}
	for k, v := range variables {
		ctx[k] = v
	}

	logger.Info(fmt.Sprintf("[Pongo2Renderer] Available variables: %v", getVariableKeys(variables)))

	result, err := tmpl.Execute(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to execute pongo2 template: %v", err)
	}

	logger.Info(fmt.Sprintf("[Pongo2Renderer] Template result: '%s'", result))

	return result, nil
}

func getVariableKeys(variables map[string]interface{}) []string {
	keys := make([]string, 0, len(variables))
	for k := range variables {
		keys = append(keys, k)
	}
	return keys
}

func (pr *Pongo2Renderer) RenderWithContext(templateStr string, ctx pongo2.Context) (string, error) {
	tmpl, err := pongo2.FromString(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse pongo2 template: %v", err)
	}

	result, err := tmpl.Execute(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to execute pongo2 template: %v", err)
	}

	return result, nil
}

type Pongo2Formatter struct {
	renderer *Pongo2Renderer
}

func (pf *Pongo2Formatter) Format(template string, inputs map[string]interface{}) (string, error) {
	return pf.renderer.Render(template, inputs)
}

func NewPongo2Formatter() *Pongo2Formatter {
	return &Pongo2Formatter{
		renderer: NewPongo2Renderer(),
	}
}

func RegisterCustomFilter(name string, filterFunc pongo2.FilterFunction) error {
	return pongo2.RegisterFilter(name, filterFunc)
}

func RegisterCustomTag(name string, tagFunc pongo2.TagParser) error {
	return pongo2.RegisterTag(name, tagFunc)
}
