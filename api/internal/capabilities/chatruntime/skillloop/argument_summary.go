package skillloop

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func summarizeSkillToolArguments(skillID string, toolName string, args map[string]interface{}) map[string]interface{} {
	switch strings.ToLower(strings.TrimSpace(skillID)) {
	case skills.SkillFileGenerator:
		return summarizeFileGeneratorArguments(args)
	case skills.SkillFileManager:
		return summarizeFileManagerArguments(toolName, args)
	case skills.SkillTime:
		return summarizeAllowedArguments(args, []string{"timezone", "format", "operation", "base_date", "target_date", "date", "unit", "amount"})
	case skills.SkillCalculator:
		return summarizeCalculatorArguments(toolName, args)
	case skills.SkillAgentDatabase, skills.SkillInternalDatabase:
		return summarizeDatabaseArguments(args)
	case skills.SkillFileReader:
		return summarizeFileReaderArguments(args)
	case skills.SkillConsoleNavigator:
		return summarizeConsoleNavigatorArguments(args)
	default:
		return summarizeGenericArguments(args)
	}
}

func summarizeFileGeneratorArguments(args map[string]interface{}) map[string]interface{} {
	summary := summarizeAllowedArguments(args, []string{"format", "filename", "title", "lifecycle", "target", "workspace_id", "folder_id"})
	if filename, ok := summary["filename"].(string); ok {
		if format, ok := summary["format"].(string); ok {
			summary["filename"] = fileGeneratorDisplayFilename(filename, format)
		}
	}
	if content, ok := args["content"].(string); ok {
		summary["content_length"] = len(content)
	}
	return summary
}

func fileGeneratorDisplayFilename(filename string, format string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return filename
	}
	extension := fileGeneratorFormatExtension(format)
	if extension == "" {
		return filename
	}
	if dot := strings.LastIndex(filename, "."); dot > 0 {
		filename = filename[:dot]
	}
	return filename + extension
}

func fileGeneratorFormatExtension(format string) string {
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), ".")) {
	case "txt", "text":
		return ".txt"
	case "md", "markdown":
		return ".md"
	case "html", "htm":
		return ".html"
	case "json":
		return ".json"
	case "csv":
		return ".csv"
	case "svg":
		return ".svg"
	case "docx", "word":
		return ".docx"
	case "xlsx", "excel":
		return ".xlsx"
	case "pdf":
		return ".pdf"
	default:
		return ""
	}
}

func summarizeFileManagerArguments(toolName string, args map[string]interface{}) map[string]interface{} {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "save_file_to_management":
		return summarizeAllowedArguments(args, []string{"source_type", "filename", "file_name", "target", "workspace_id", "folder_id"})
	case "delete_file":
		return summarizeAllowedArguments(args, []string{"filename", "file_name", "target"})
	default:
		return summarizeAllowedArguments(args, []string{"filename", "file_name", "target"})
	}
}

func summarizeCalculatorArguments(toolName string, args map[string]interface{}) map[string]interface{} {
	allowed := []string{"operation", "left", "right", "value", "percent", "from", "to", "precision"}
	summary := summarizeAllowedArguments(args, allowed)
	if strings.EqualFold(strings.TrimSpace(toolName), "evaluate_expression") {
		if expression, ok := args["expression"].(string); ok {
			summary["expression_length"] = len(expression)
		}
	}
	return summary
}

func summarizeDatabaseArguments(args map[string]interface{}) map[string]interface{} {
	return summarizeAllowedArguments(args, []string{"query", "limit", "offset", "order"})
}

func summarizeFileReaderArguments(args map[string]interface{}) map[string]interface{} {
	summary := summarizeAllowedArguments(args, []string{"file_id", "include_content", "max_chars"})
	if fileIDs := sanitizedStringListArgumentValue(args["file_ids"]); len(fileIDs) > 0 {
		summary["file_ids"] = fileIDs
	}
	return summary
}

func summarizeConsoleNavigatorArguments(args map[string]interface{}) map[string]interface{} {
	return summarizeAllowedArguments(args, []string{"href", "reason"})
}

func summarizeAllowedArguments(args map[string]interface{}, keys []string) map[string]interface{} {
	summary := map[string]interface{}{}
	for _, key := range keys {
		if value, ok := sanitizedArgumentValue(args[key]); ok {
			summary[key] = value
		}
	}
	return summary
}

func summarizeGenericArguments(args map[string]interface{}) map[string]interface{} {
	summary := map[string]interface{}{}
	for key, value := range args {
		if summarized, ok := summarizedGenericArgumentValue(value); ok {
			summary[key] = summarized
		}
	}
	return summary
}

func sanitizedArgumentValue(value interface{}) (interface{}, bool) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, false
		}
		return text, true
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		return typed, true
	default:
		return nil, false
	}
}

func sanitizedStringListArgumentValue(value interface{}) []string {
	out := []string{}
	add := func(item string) {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			add(item)
		}
	case []interface{}:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				add(text)
			}
		}
	}
	return out
}

func summarizedGenericArgumentValue(value interface{}) (interface{}, bool) {
	switch typed := value.(type) {
	case string:
		return map[string]interface{}{"type": "string", "length": len(typed)}, true
	case []interface{}:
		return map[string]interface{}{"type": "array", "length": len(typed)}, true
	case map[string]interface{}:
		return map[string]interface{}{"type": "object", "keys": len(typed)}, true
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		return typed, true
	default:
		return nil, false
	}
}
