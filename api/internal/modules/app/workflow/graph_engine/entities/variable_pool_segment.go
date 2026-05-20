package entities

import (
	"fmt"

	workflowfile "github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/pkg/logger"
)

func (vp *VariablePool) buildSegment(value interface{}) Segment {
	switch v := value.(type) {
	case string:
		return &StringSegment{Value: v}
	case int:
		return &NumberSegment{Value: float64(v)}
	case int64:
		return &NumberSegment{Value: float64(v)}
	case float64:
		return &NumberSegment{Value: v}
	case float32:
		return &NumberSegment{Value: float64(v)}
	case map[string]interface{}:
		// Check if this map represents a file structure
		if vp.isFileStructure(v) {
			logger.Debug("Detected file structure in map", map[string]interface{}{
				"has_type":           v["type"] != nil,
				"has_upload_file_id": v["upload_file_id"] != nil,
				"has_id":             v["id"] != nil,
				"has_related_id":     v["related_id"] != nil,
			})
			file := vp.mapToFile(v)
			logger.Info("Converted map to File entity", map[string]interface{}{
				"file_id":  file.ID,
				"type":     file.Type,
				"filename": file.Filename,
			})
			return &FileSegment{Value: file}
		}
		return &ObjectSegment{Value: v}
	case *workflowfile.File:
		return &FileSegment{Value: vp.workflowFileToFile(v)}
	case []*workflowfile.File:
		return &ArrayFileSegment{Value: vp.workflowFilesToFiles(v)}
	case *File:
		return &FileSegment{Value: v}
	case []*File:
		return &ArrayFileSegment{Value: v}
	case []map[string]interface{}:
		return &ArrayObjectSegment{Value: v}
	case []interface{}:
		// Check if this array contains file structures
		if vp.isFileArray(v) {
			logger.Debug("Detected file array structure", map[string]interface{}{
				"array_length": len(v),
			})
			files := vp.mapArrayToFiles(v)
			logger.Info("Converted array to File entities", map[string]interface{}{
				"file_count": len(files),
			})
			return &ArrayFileSegment{Value: files}
		}
		return &ArrayAnySegment{Value: v}
	case nil:
		return &NoneSegment{}
	default:
		return &StringSegment{Value: fmt.Sprintf("%v", v)}
	}
}

func (vp *VariablePool) segmentToVariable(segment Segment, selector []string) Variable {
	return &variableWrapper{
		segment:  segment,
		name:     selector[1],
		selector: selector,
	}
}

func (vp *VariablePool) extractValue(obj interface{}) interface{} {
	if objectSegment, ok := obj.(*ObjectSegment); ok {
		return objectSegment.Value
	}
	return obj
}

func (vp *VariablePool) getNestedAttribute(obj interface{}, attr string) interface{} {
	if objMap, ok := obj.(map[string]interface{}); ok {
		return objMap[attr]
	}
	return nil
}

func (vp *VariablePool) getNestedAttributeSegment(segment Segment, attr string) Segment {
	// Unwrap variableWrapper if present
	if wrapper, ok := segment.(*variableWrapper); ok {
		segment = wrapper.segment
	}

	// Handle ObjectSegment
	if objectSegment, ok := segment.(*ObjectSegment); ok {
		if value, exists := objectSegment.Value[attr]; exists {
			return vp.buildSegment(value)
		}
	} else if fileSegment, ok := segment.(*FileSegment); ok {
		// Handle FileSegment attributes
		if vp.isValidFileAttribute(attr) {
			val := vp.getFileAttribute(fileSegment.Value, FileAttribute(attr))
			return vp.buildSegment(val)
		}
	}

	return nil
}

func (vp *VariablePool) isValidFileAttribute(attr string) bool {
	validAttrs := []FileAttribute{
		FileAttributeURL,
		FileAttributeName,
		FileAttributeSize,
		FileAttributeType,
		FileAttributeExtension,
		FileAttributeMimeType,
		FileAttributeTransferMethod,
	}

	for _, validAttr := range validAttrs {
		if attr == string(validAttr) {
			return true
		}
	}
	return false
}

func (vp *VariablePool) getFileAttribute(file *File, attr FileAttribute) interface{} {
	switch attr {
	case FileAttributeURL:
		return file.RemoteURL
	case FileAttributeName:
		return file.Filename
	case FileAttributeSize:
		return file.Size
	case FileAttributeType:
		return file.Type
	case FileAttributeExtension:
		return file.Extension
	case FileAttributeMimeType:
		return file.MimeType
	case FileAttributeTransferMethod:
		return file.TransferMethod
	default:
		return nil
	}
}

type variableWrapper struct {
	segment  Segment
	name     string
	selector []string
}
