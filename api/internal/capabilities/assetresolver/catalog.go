package assetresolver

import "strings"

type catalog struct {
	files []Candidate
}

type catalogBuilder struct {
	files       []Candidate
	byID        map[string]int
	visibleIDs  []string
	visibleSeen map[string]struct{}
}

func buildCatalog(req Request) catalog {
	builder := &catalogBuilder{
		byID:        map[string]int{},
		visibleSeen: map[string]struct{}{},
	}
	for _, candidate := range req.Candidates {
		if strings.TrimSpace(candidate.Source) == "" {
			candidate.Source = "request.candidates"
		}
		builder.add(candidate)
	}
	builder.addContext(req.OperationContext, "operation_context")
	builder.addContext(req.NormalizedOperationContext, "operation_context.normalized")
	builder.addSelectedIDs(req.OperationContext, "operation_context.selected")
	builder.addSelectedIDs(req.NormalizedOperationContext, "operation_context.normalized.selected")
	builder.assignVisibleOrdinals()
	return catalog{files: builder.files}
}

func (b *catalogBuilder) addContext(context map[string]interface{}, source string) {
	if len(context) == 0 {
		return
	}
	for _, candidate := range candidatesFromContext(context, source) {
		b.add(candidate)
	}
}

func (b *catalogBuilder) addSelectedIDs(value interface{}, source string) {
	collector := newUniqueStringCollector()
	collectSelectedFileIDs(value, 0, collector.add)
	for _, id := range collector.values() {
		b.add(Candidate{
			Type:     AssetTypeFile,
			ID:       id,
			Source:   source,
			Selected: true,
		})
	}
}

func (b *catalogBuilder) add(candidate Candidate) {
	id := strings.TrimSpace(candidate.ID)
	if id == "" {
		return
	}
	candidate.ID = id
	candidate.Type = normalizeToken(firstNonEmptyString(candidate.Type, AssetTypeFile))
	if candidate.Type != AssetTypeFile {
		return
	}
	candidate.Name = strings.TrimSpace(candidate.Name)
	candidate.Title = strings.TrimSpace(candidate.Title)
	candidate.Source = strings.TrimSpace(candidate.Source)
	candidate.Extension = normalizedExtension(candidate.Extension)
	candidate.MimeType = strings.TrimSpace(candidate.MimeType)
	candidate.FileType = normalizeToken(candidate.FileType)
	candidate.WorkspaceID = strings.TrimSpace(candidate.WorkspaceID)
	candidate.Metadata = copyStringAnyMap(candidate.Metadata)
	if candidate.Extension == "" {
		candidate.Extension = extensionFromCandidate(candidate)
	}
	if candidate.FileType == "" {
		candidate.FileType = inferFileType(candidate)
	}
	if candidate.Visible || candidate.VisibleOrdinal > 0 {
		if _, seen := b.visibleSeen[id]; !seen {
			b.visibleSeen[id] = struct{}{}
			b.visibleIDs = append(b.visibleIDs, id)
		}
	}
	if idx, exists := b.byID[id]; exists {
		b.files[idx] = mergeCandidates(b.files[idx], candidate)
		return
	}
	b.byID[id] = len(b.files)
	b.files = append(b.files, candidate)
}

func (b *catalogBuilder) assignVisibleOrdinals() {
	for index, id := range b.visibleIDs {
		fileIndex, ok := b.byID[id]
		if !ok {
			continue
		}
		b.files[fileIndex].Visible = true
		if b.files[fileIndex].VisibleOrdinal == 0 {
			b.files[fileIndex].VisibleOrdinal = index + 1
		}
	}
}

func mergeCandidates(existing Candidate, next Candidate) Candidate {
	if existing.Name == "" {
		existing.Name = next.Name
	}
	if existing.Title == "" {
		existing.Title = next.Title
	}
	if existing.Source == "" {
		existing.Source = next.Source
	}
	if existing.Extension == "" {
		existing.Extension = next.Extension
	}
	if existing.MimeType == "" {
		existing.MimeType = next.MimeType
	}
	if existing.FileType == "" {
		existing.FileType = next.FileType
	}
	if existing.WorkspaceID == "" {
		existing.WorkspaceID = next.WorkspaceID
	}
	existing.Selected = existing.Selected || next.Selected
	existing.Visible = existing.Visible || next.Visible
	if existing.VisibleOrdinal == 0 {
		existing.VisibleOrdinal = next.VisibleOrdinal
	}
	if existing.Metadata == nil {
		existing.Metadata = next.Metadata
	}
	return existing
}

func candidatesFromContext(context map[string]interface{}, source string) []Candidate {
	var candidates []Candidate
	var visit func(interface{}, int)
	visit = func(value interface{}, depth int) {
		if depth > 6 || value == nil {
			return
		}
		switch typed := value.(type) {
		case map[string]interface{}:
			if candidate, ok := candidateFromMap(typed, source, false); ok {
				candidates = append(candidates, candidate)
			}
			for key, item := range typed {
				normalizedKey := normalizeKey(key)
				switch {
				case isVisibleFileListKey(normalizedKey):
					candidates = append(candidates, candidatesFromList(item, source+"."+key, true)...)
				case isResourceListKey(normalizedKey):
					candidates = append(candidates, candidatesFromList(item, source+"."+key, false)...)
				case isCapabilityListKey(normalizedKey):
					continue
				default:
					visit(item, depth+1)
				}
			}
		case []interface{}:
			for _, item := range typed {
				visit(item, depth+1)
			}
		case []map[string]interface{}:
			for _, item := range typed {
				visit(item, depth+1)
			}
		}
	}
	visit(context, 0)
	return candidates
}

func candidatesFromList(value interface{}, source string, allowImplicitFile bool) []Candidate {
	items := itemsFromValue(value)
	if len(items) == 0 {
		return nil
	}
	candidates := make([]Candidate, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case map[string]interface{}:
			if candidate, ok := candidateFromMap(typed, source, allowImplicitFile); ok {
				candidates = append(candidates, candidate)
			}
		case string:
			if allowImplicitFile {
				candidates = append(candidates, Candidate{
					Type:    AssetTypeFile,
					ID:      strings.TrimSpace(typed),
					Source:  source,
					Visible: true,
				})
			}
		}
	}
	return candidates
}

func candidateFromMap(value map[string]interface{}, source string, allowImplicitFile bool) (Candidate, bool) {
	if len(value) == 0 {
		return Candidate{}, false
	}
	if !allowImplicitFile && !isFileMap(value) {
		return Candidate{}, false
	}
	metadata := mapFromValue(value["metadata"])
	id := firstNonEmptyString(
		stringValue(firstMapValue(value, "resource_id", "id", "file_id", "upload_file_id", "related_id")),
		stringValue(firstMapValue(metadata, "resource_id", "id", "file_id", "upload_file_id", "related_id")),
	)
	if id == "" {
		return Candidate{}, false
	}
	title := firstNonEmptyString(
		stringValue(firstMapValue(value, "title", "name", "filename", "file_name", "label")),
		stringValue(firstMapValue(metadata, "title", "name", "filename", "file_name", "label")),
	)
	name := firstNonEmptyString(
		stringValue(firstMapValue(value, "name", "filename", "file_name", "title", "label")),
		stringValue(firstMapValue(metadata, "name", "filename", "file_name", "title", "label")),
	)
	extension := firstNonEmptyString(
		stringValue(firstMapValue(value, "extension", "ext", "suffix")),
		stringValue(firstMapValue(metadata, "extension", "ext", "suffix")),
	)
	mimeType := firstNonEmptyString(
		stringValue(firstMapValue(value, "mime_type", "mime", "content_type")),
		stringValue(firstMapValue(metadata, "mime_type", "mime", "content_type")),
	)
	fileType := firstNonEmptyString(
		stringValue(firstMapValue(value, "file_type", "format", "category")),
		stringValue(firstMapValue(metadata, "file_type", "format", "category")),
	)
	return Candidate{
		Type:      AssetTypeFile,
		ID:        id,
		Name:      name,
		Title:     title,
		Source:    firstNonEmptyString(stringValue(value["source"]), source),
		Extension: extension,
		MimeType:  mimeType,
		FileType:  fileType,
		WorkspaceID: firstNonEmptyString(
			stringValue(firstMapValue(value, "workspace_id", "workspaceId")),
			stringValue(firstMapValue(metadata, "workspace_id", "workspaceId")),
		),
		Selected: boolValue(firstMapValue(value, "selected", "is_selected")) ||
			boolValue(firstMapValue(metadata, "selected", "is_selected")),
		Visible:  true,
		Metadata: copyStringAnyMap(metadata),
	}, true
}

func isVisibleFileListKey(key string) bool {
	switch key {
	case "visiblefiles", "visiblefile", "visiblefileresources", "visiblefileids":
		return true
	default:
		return false
	}
}

func isResourceListKey(key string) bool {
	return key == "resources" || key == "resource"
}

func isCapabilityListKey(key string) bool {
	return key == "capabilities" || key == "capability"
}
