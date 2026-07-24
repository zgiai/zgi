package service

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func customSkillRecordFromDocument(scope Scope, doc skills.SkillDocument, storagePath string, extracted *extractedSkillPackage) *runtimemodel.CustomSkill {
	return &runtimemodel.CustomSkill{
		ID:             uuid.New(),
		OrganizationID: scope.OrganizationID,
		SkillID:        doc.Metadata.ID,
		Name:           doc.Metadata.Name,
		Description:    doc.Metadata.Description,
		WhenToUse:      doc.Metadata.WhenToUse,
		RuntimeType:    skills.SkillRuntimeTypePrompt,
		Display:        skillDisplayMap(doc.Metadata.Display),
		StoragePath:    storagePath,
		Manifest:       customSkillManifest(doc, extracted),
		Status:         runtimemodel.CustomSkillStatusActive,
		CreatedBy:      scope.AccountID,
	}
}

func customSkillManifest(doc skills.SkillDocument, extracted *extractedSkillPackage) map[string]interface{} {
	references := make([]string, 0, len(doc.Metadata.References))
	for _, ref := range doc.Metadata.References {
		references = append(references, ref.Path)
	}
	manifest := map[string]interface{}{
		"file_count":        0,
		"total_size":        int64(0),
		"files":             []string{},
		"references":        references,
		"has_scripts":       doc.Metadata.HasScripts,
		"scripts_supported": doc.Metadata.ScriptsSupported,
		"imported_at":       time.Now().Unix(),
	}
	if extracted != nil {
		manifest["file_count"] = extracted.FileCount
		manifest["total_size"] = extracted.TotalSize
		manifest["files"] = append([]string(nil), extracted.Files...)
	}
	return manifest
}

func skillImportPreviewFromStored(preview *storedSkillPreview) *SkillImportPreview {
	if preview == nil {
		return &SkillImportPreview{Files: []SkillImportPreviewFile{}, References: []string{}, Warnings: []string{}, ValidationErrors: []string{}}
	}
	files := make([]SkillImportPreviewFile, 0, len(preview.Files))
	for _, file := range preview.Files {
		files = append(files, SkillImportPreviewFile{Path: file.Path, Size: file.Size})
	}
	return &SkillImportPreview{
		ImportID:         preview.ImportID,
		ExpiresAt:        preview.ExpiresAt,
		FileCount:        preview.FileCount,
		TotalSize:        preview.TotalSize,
		Files:            files,
		References:       []string{},
		Warnings:         []string{},
		ValidationErrors: []string{},
		CanImport:        false,
	}
}

func existingSkillPreview(skill *runtimemodel.CustomSkill) *ExistingSkill {
	if skill == nil {
		return nil
	}
	return &ExistingSkill{
		SkillID:   strings.ToLower(strings.TrimSpace(skill.SkillID)),
		Name:      strings.TrimSpace(skill.Name),
		UpdatedAt: skill.UpdatedAt,
	}
}

func extractedSkillPackageFromPreview(preview *storedSkillPreview) *extractedSkillPackage {
	if preview == nil {
		return nil
	}
	files := make([]string, 0, len(preview.Files))
	for _, file := range preview.Files {
		files = append(files, file.Path)
	}
	sort.Strings(files)
	return &extractedSkillPackage{
		Root:        preview.Root,
		FileCount:   preview.FileCount,
		TotalSize:   preview.TotalSize,
		Files:       files,
		FileDetails: append([]extractedSkillFile(nil), preview.Files...),
	}
}

func skillDiscoveryMetadataPtr(doc skills.SkillDocument) *skills.SkillDiscoveryMetadata {
	metadata := skills.SkillDiscoveryMetadata{
		ID:               doc.Metadata.ID,
		Source:           doc.Metadata.Source,
		Name:             doc.Metadata.Name,
		Description:      doc.Metadata.Description,
		WhenToUse:        doc.Metadata.WhenToUse,
		Display:          doc.Metadata.Display,
		RuntimeType:      doc.Metadata.RuntimeType,
		HasTools:         len(doc.Tools) > 0,
		HasReferences:    len(doc.Metadata.References) > 0,
		HasScripts:       doc.Metadata.HasScripts,
		ScriptsSupported: doc.Metadata.ScriptsSupported,
		MaxCallsPerTurn:  doc.Metadata.MaxCallsPerTurn,
		TimeoutSeconds:   doc.Metadata.TimeoutSeconds,
		Status:           skills.SkillStatusActive,
		SupportedCallers: append([]string(nil), doc.Metadata.SupportedCallers...),
		RequiredConfig:   append([]string(nil), doc.Metadata.RequiredConfig...),
	}
	return &metadata
}
