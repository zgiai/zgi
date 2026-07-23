package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

type preparedSkillInputFile struct {
	Name     string
	Path     string
	FileID   string
	Filename string
	MimeType string
	Size     int64
	Data     []byte
	Multiple bool
}

func (r *SandboxScriptRunner) resolveInputFiles(ctx context.Context, arguments map[string]interface{}, execCtx ExecutionContext, manifest skillScriptManifest) ([]preparedSkillInputFile, error) {
	if len(manifest.InputFiles) == 0 {
		return nil, nil
	}
	if r.inputFileProvider == nil {
		return nil, fmt.Errorf("skill script input file provider is not configured")
	}
	prepared := make([]preparedSkillInputFile, 0, len(manifest.InputFiles))
	for _, spec := range manifest.InputFiles {
		fileIDs, ok := skillScriptInputFileIDs(arguments, spec.Argument)
		if !ok || len(fileIDs) == 0 {
			if spec.Required {
				return nil, fmt.Errorf("skill input file %s requires argument %s", spec.Name, spec.Argument)
			}
			continue
		}
		if !spec.Multiple && len(fileIDs) > 1 {
			return nil, fmt.Errorf("skill input file %s accepts one file, got %d", spec.Name, len(fileIDs))
		}
		if spec.Multiple && spec.MaxCount > 0 && len(fileIDs) > spec.MaxCount {
			return nil, fmt.Errorf("skill input file %s accepts at most %d files", spec.Name, spec.MaxCount)
		}
		for _, fileID := range fileIDs {
			inputFile, err := r.inputFileProvider.GetSkillScriptInputFile(ctx, fileID, spec.MaxBytes, execCtx)
			if err != nil {
				return nil, fmt.Errorf("failed to load skill input file %s: %w", spec.Name, err)
			}
			normalized, err := prepareSkillInputFile(spec, inputFile)
			if err != nil {
				return nil, err
			}
			prepared = append(prepared, normalized)
			if len(prepared) > maxSkillScriptInputFileCount {
				return nil, fmt.Errorf("skill script accepts at most %d input files", maxSkillScriptInputFileCount)
			}
		}
	}
	return prepared, nil
}

func (r *SandboxScriptRunner) uploadInputFiles(ctx context.Context, sandboxID string, files []preparedSkillInputFile, execCtx ExecutionContext) error {
	if len(files) == 0 {
		return nil
	}
	archiveBase64, err := zipSkillInputFilesBase64(files)
	if err != nil {
		return err
	}
	if err := r.uploadArchive(ctx, sandboxID, archiveBase64, execCtx, false); err != nil {
		return fmt.Errorf("failed to upload skill input files: %w", err)
	}
	return nil
}

func skillScriptInputFileIDs(arguments map[string]interface{}, argument string) ([]string, bool) {
	if arguments == nil {
		return nil, false
	}
	value, ok := arguments[argument]
	if !ok || value == nil {
		return nil, false
	}
	ids := []string{}
	appendID := func(raw interface{}) {
		fileID := ""
		switch typed := raw.(type) {
		case string:
			fileID = strings.TrimSpace(typed)
		default:
			fileID = strings.TrimSpace(fmt.Sprint(typed))
		}
		if fileID != "" {
			ids = append(ids, fileID)
		}
	}
	switch typed := value.(type) {
	case string:
		appendID(typed)
	case []string:
		for _, item := range typed {
			appendID(item)
		}
	case []interface{}:
		for _, item := range typed {
			appendID(item)
		}
	default:
		appendID(typed)
	}
	if len(ids) == 0 {
		return nil, false
	}
	return ids, true
}

func prepareSkillInputFile(spec skillScriptInputFileSpec, input SkillScriptInputFile) (preparedSkillInputFile, error) {
	dataSize := int64(len(input.Data))
	size := input.Size
	if size <= 0 {
		size = dataSize
	}
	if size > spec.MaxBytes || dataSize > spec.MaxBytes {
		return preparedSkillInputFile{}, fmt.Errorf("skill input file %s exceeds max_bytes %d", spec.Name, spec.MaxBytes)
	}
	filename := safeSkillInputFilename(input.Filename, input.FileID, input.Extension)
	extension := strings.ToLower(path.Ext(filename))
	if len(spec.Extensions) > 0 && !stringInList(extension, spec.Extensions) {
		return preparedSkillInputFile{}, fmt.Errorf("skill input file %s extension %s is not allowed", spec.Name, extension)
	}
	mimeType := strings.ToLower(strings.TrimSpace(strings.Split(input.MimeType, ";")[0]))
	if mimeType == "" {
		mimeType = skillArtifactMimeType(filename, "", input.Data)
	}
	if len(spec.MimeTypes) > 0 && !stringInList(mimeType, spec.MimeTypes) {
		return preparedSkillInputFile{}, fmt.Errorf("skill input file %s mime type %s is not allowed", spec.Name, mimeType)
	}
	fileID := strings.TrimSpace(input.FileID)
	inputPath := "inputs/" + spec.Name + "/" + filename
	if spec.Multiple {
		inputPath = "inputs/" + spec.Name + "/" + safeSkillInputPathSegment(fileID) + "/" + filename
	}
	return preparedSkillInputFile{
		Name:     spec.Name,
		Path:     inputPath,
		FileID:   fileID,
		Filename: filename,
		MimeType: mimeType,
		Size:     dataSize,
		Data:     input.Data,
		Multiple: spec.Multiple,
	}, nil
}

func safeSkillInputPathSegment(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	out := builder.String()
	if out == "" || out == "." || out == ".." {
		return "input"
	}
	return out
}

func safeSkillInputFilename(filename string, fileID string, extension string) string {
	name := filepath.Base(filepath.ToSlash(strings.TrimSpace(filename)))
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.Trim(name, " ./")
	if name == "" || name == "." {
		ext := strings.TrimSpace(extension)
		if ext != "" && !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		name = strings.TrimSpace(fileID)
		if name == "" {
			name = "input"
		}
		name += ext
	}
	if name == "" || name == "." || name == ".." {
		return "input.bin"
	}
	return name
}

func zipSkillInputFilesBase64(files []preparedSkillInputFile) (string, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range files {
		if unsafeSkillManifestPath(file.Path) || !strings.HasPrefix(file.Path, "inputs/") {
			_ = writer.Close()
			return "", fmt.Errorf("skill input file path is invalid: %s", file.Path)
		}
		header := &zip.FileHeader{
			Name:   filepath.ToSlash(file.Path),
			Method: zip.Deflate,
		}
		header.SetMode(0o644)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			return "", err
		}
		if _, err := entry.Write(file.Data); err != nil {
			_ = writer.Close()
			return "", err
		}
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

func skillScriptStdinPayload(arguments map[string]interface{}, inputFiles []preparedSkillInputFile) (map[string]interface{}, error) {
	if len(inputFiles) == 0 {
		return arguments, nil
	}
	if _, exists := arguments["input_files"]; exists {
		return nil, fmt.Errorf("skill script argument input_files is reserved")
	}
	payload := make(map[string]interface{}, len(arguments)+1)
	for key, value := range arguments {
		payload[key] = value
	}
	files := make(map[string]interface{}, len(inputFiles))
	for _, inputFile := range inputFiles {
		item := map[string]interface{}{
			"path":      inputFile.Path,
			"file_id":   inputFile.FileID,
			"filename":  inputFile.Filename,
			"mime_type": inputFile.MimeType,
			"size":      inputFile.Size,
		}
		if inputFile.Multiple {
			current, _ := files[inputFile.Name].([]map[string]interface{})
			files[inputFile.Name] = append(current, item)
			continue
		}
		files[inputFile.Name] = item
	}
	payload["input_files"] = files
	return payload, nil
}

func stringInList(value string, allowed []string) bool {
	for _, item := range allowed {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}
