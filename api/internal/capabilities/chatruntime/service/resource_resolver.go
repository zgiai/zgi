package service

import (
	"strconv"
	"strings"
	"unicode"

	actiondto "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/dto"
	"github.com/zgiai/zgi/api/internal/capabilities/assetresolver"
)

const (
	resourceResolverCandidateLimit = 8
	resourceTypeFile               = "file"
)

// ResourceResolutionStatus describes whether a planner resource reference could
// be grounded to concrete resources from the current chat/page context.
type ResourceResolutionStatus string

const (
	// ResourceResolutionStatusResolved means the resolver found a concrete file.
	ResourceResolutionStatusResolved ResourceResolutionStatus = "resolved"
	// ResourceResolutionStatusAmbiguous means more than one visible candidate matched.
	ResourceResolutionStatusAmbiguous ResourceResolutionStatus = "ambiguous"
	// ResourceResolutionStatusNotFound means no visible/context candidate matched.
	ResourceResolutionStatusNotFound ResourceResolutionStatus = "not_found"
)

// PlannerResourceRef is the structured reference emitted by the semantic planner.
//
// The shape is intentionally tolerant: newer planner workers can use typed fields,
// while earlier experiments can pass equivalent values through Metadata.
type PlannerResourceRef struct {
	ResourceType  string                 `json:"resource_type,omitempty"`
	Type          string                 `json:"type,omitempty"`
	Kind          string                 `json:"kind,omitempty"`
	ID            string                 `json:"id,omitempty"`
	FileID        string                 `json:"file_id,omitempty"`
	Source        string                 `json:"source,omitempty"`
	Selector      string                 `json:"selector,omitempty"`
	Scope         string                 `json:"scope,omitempty"`
	Selected      bool                   `json:"selected,omitempty"`
	Ordinal       int                    `json:"ordinal,omitempty"`
	VisibleIndex  int                    `json:"visible_index,omitempty"`
	OrdinalText   string                 `json:"ordinal_text,omitempty"`
	Title         string                 `json:"title,omitempty"`
	Name          string                 `json:"name,omitempty"`
	TitleContains string                 `json:"title_contains,omitempty"`
	NameContains  string                 `json:"name_contains,omitempty"`
	FuzzyName     string                 `json:"fuzzy_name,omitempty"`
	Extension     string                 `json:"extension,omitempty"`
	Extensions    []string               `json:"extensions,omitempty"`
	MimeType      string                 `json:"mime_type,omitempty"`
	MimeTypes     []string               `json:"mime_types,omitempty"`
	FileType      string                 `json:"file_type,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// ResourceResolverInput contains the bounded context used to ground planner
// references. OperationContext should be the raw page context when available;
// NormalizedOperationContext can carry the ledger summary as a fallback.
type ResourceResolverInput struct {
	OperationContext           map[string]interface{} `json:"operation_context,omitempty"`
	NormalizedOperationContext map[string]interface{} `json:"normalized_operation_context,omitempty"`
	AttachmentFiles            []ResourceCandidate    `json:"attachment_files,omitempty"`
}

// ResourceCandidate is a file-like resource visible to the current AIChat turn.
type ResourceCandidate struct {
	Type           string                 `json:"type,omitempty"`
	ID             string                 `json:"id,omitempty"`
	Name           string                 `json:"name,omitempty"`
	Title          string                 `json:"title,omitempty"`
	Source         string                 `json:"source,omitempty"`
	Extension      string                 `json:"extension,omitempty"`
	MimeType       string                 `json:"mime_type,omitempty"`
	FileType       string                 `json:"file_type,omitempty"`
	Selected       bool                   `json:"selected,omitempty"`
	Recent         bool                   `json:"recent,omitempty"`
	Visible        bool                   `json:"visible,omitempty"`
	VisibleOrdinal int                    `json:"visible_ordinal,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// ResourceResolution is the per-reference grounding result returned to the
// chat runtime coordinator.
type ResourceResolution struct {
	Ref        PlannerResourceRef       `json:"ref,omitempty"`
	Status     ResourceResolutionStatus `json:"status"`
	Reason     string                   `json:"reason,omitempty"`
	Resources  []actiondto.ResourceRef  `json:"resources,omitempty"`
	FileIDs    []string                 `json:"file_ids,omitempty"`
	Candidates []ResourceCandidate      `json:"candidates,omitempty"`
}

// ResourceResolverResult contains all per-reference results and the flattened
// resolved action resources/file IDs.
type ResourceResolverResult struct {
	Results   []ResourceResolution    `json:"results"`
	Resources []actiondto.ResourceRef `json:"resources,omitempty"`
	FileIDs   []string                `json:"file_ids,omitempty"`
}

// ResourceResolver grounds planner resource references against chat/page context.
type ResourceResolver struct{}

// NewResourceResolver creates a resource resolver.
func NewResourceResolver() ResourceResolver {
	return ResourceResolver{}
}

// Resolve grounds planner refs and returns both per-ref status and flattened
// action resources for resolved refs.
func (r ResourceResolver) Resolve(input ResourceResolverInput, refs []PlannerResourceRef) ResourceResolverResult {
	if len(refs) == 0 {
		return ResourceResolverResult{}
	}
	resolved := assetresolver.Resolve(assetResolverRequestFromInput(input, refs))
	return resourceResolverResultFromAssets(resolved)
}

func assetResolverRequestFromInput(input ResourceResolverInput, refs []PlannerResourceRef) assetresolver.Request {
	selectors := make([]assetresolver.Selector, 0, len(refs))
	for _, ref := range refs {
		selectors = append(selectors, assetResolverSelectorFromPlannerRef(ref))
	}
	candidates := make([]assetresolver.Candidate, 0, len(input.AttachmentFiles))
	for _, candidate := range input.AttachmentFiles {
		candidates = append(candidates, assetResolverCandidateFromResourceCandidate(candidate))
	}
	return assetresolver.Request{
		OperationContext:           input.OperationContext,
		NormalizedOperationContext: input.NormalizedOperationContext,
		Candidates:                 candidates,
		Selectors:                  selectors,
	}
}

func resourceResolverResultFromAssets(resolved assetresolver.Result) ResourceResolverResult {
	result := ResourceResolverResult{Results: make([]ResourceResolution, 0, len(resolved.Resolutions))}
	fileIDs := newUniqueStringCollector()
	resources := make([]actiondto.ResourceRef, 0, len(resolved.Assets))
	for _, resolution := range resolved.Resolutions {
		converted := resourceResolutionFromAssetResolution(resolution)
		result.Results = append(result.Results, converted)
		if converted.Status != ResourceResolutionStatusResolved {
			continue
		}
		for _, resource := range converted.Resources {
			if resource.ID == "" || containsString(fileIDs.values(), resource.ID) {
				continue
			}
			fileIDs.add(resource.ID)
			resources = append(resources, resource)
		}
	}
	result.FileIDs = fileIDs.values()
	result.Resources = resources
	return result
}

func assetResolverSelectorFromPlannerRef(ref PlannerResourceRef) assetresolver.Selector {
	return assetresolver.Selector{
		ResourceType:  ref.ResourceType,
		Type:          ref.Type,
		Kind:          ref.Kind,
		ID:            ref.ID,
		FileID:        ref.FileID,
		Source:        ref.Source,
		Selector:      ref.Selector,
		Scope:         ref.Scope,
		Selected:      ref.Selected,
		Ordinal:       ref.Ordinal,
		VisibleIndex:  ref.VisibleIndex,
		OrdinalText:   ref.OrdinalText,
		Title:         ref.Title,
		Name:          ref.Name,
		TitleContains: ref.TitleContains,
		NameContains:  ref.NameContains,
		FuzzyName:     ref.FuzzyName,
		Extension:     ref.Extension,
		Extensions:    append([]string(nil), ref.Extensions...),
		MimeType:      ref.MimeType,
		MimeTypes:     append([]string(nil), ref.MimeTypes...),
		FileType:      ref.FileType,
		Metadata:      copyStringAnyMap(ref.Metadata),
	}
}

func plannerRefFromAssetResolverSelector(selector assetresolver.Selector) PlannerResourceRef {
	return PlannerResourceRef{
		ResourceType:  selector.ResourceType,
		Type:          selector.Type,
		Kind:          selector.Kind,
		ID:            selector.ID,
		FileID:        selector.FileID,
		Source:        selector.Source,
		Selector:      selector.Selector,
		Scope:         selector.Scope,
		Selected:      selector.Selected,
		Ordinal:       selector.Ordinal,
		VisibleIndex:  selector.VisibleIndex,
		OrdinalText:   selector.OrdinalText,
		Title:         selector.Title,
		Name:          selector.Name,
		TitleContains: selector.TitleContains,
		NameContains:  selector.NameContains,
		FuzzyName:     selector.FuzzyName,
		Extension:     selector.Extension,
		Extensions:    append([]string(nil), selector.Extensions...),
		MimeType:      selector.MimeType,
		MimeTypes:     append([]string(nil), selector.MimeTypes...),
		FileType:      selector.FileType,
		Metadata:      copyStringAnyMap(selector.Metadata),
	}
}

func assetResolverCandidateFromResourceCandidate(candidate ResourceCandidate) assetresolver.Candidate {
	return assetresolver.Candidate{
		Type:           candidate.Type,
		ID:             candidate.ID,
		Name:           candidate.Name,
		Title:          candidate.Title,
		Source:         candidate.Source,
		Extension:      candidate.Extension,
		MimeType:       candidate.MimeType,
		FileType:       candidate.FileType,
		Selected:       candidate.Selected,
		Recent:         candidate.Recent,
		Visible:        candidate.Visible,
		VisibleOrdinal: candidate.VisibleOrdinal,
		Metadata:       copyStringAnyMap(candidate.Metadata),
	}
}

func resourceCandidateFromAssetResolverCandidate(candidate assetresolver.Candidate) ResourceCandidate {
	return ResourceCandidate{
		Type:           candidate.Type,
		ID:             candidate.ID,
		Name:           candidate.Name,
		Title:          candidate.Title,
		Source:         candidate.Source,
		Extension:      candidate.Extension,
		MimeType:       candidate.MimeType,
		FileType:       candidate.FileType,
		Selected:       candidate.Selected,
		Recent:         candidate.Recent,
		Visible:        candidate.Visible,
		VisibleOrdinal: candidate.VisibleOrdinal,
		Metadata:       copyStringAnyMap(candidate.Metadata),
	}
}

func resourceResolutionFromAssetResolution(resolution assetresolver.Resolution) ResourceResolution {
	out := ResourceResolution{
		Ref:        plannerRefFromAssetResolverSelector(resolution.Selector),
		Status:     resourceStatusFromAssetStatus(resolution.Status),
		Reason:     resolution.Reason,
		Candidates: make([]ResourceCandidate, 0, len(resolution.Candidates)),
	}
	for _, candidate := range resolution.Candidates {
		out.Candidates = append(out.Candidates, resourceCandidateFromAssetResolverCandidate(candidate))
	}
	for _, asset := range resolution.Assets {
		resource := actionResourceRefFromAsset(asset)
		out.Resources = append(out.Resources, resource)
		if resource.ID != "" {
			out.FileIDs = append(out.FileIDs, resource.ID)
		}
	}
	if len(out.Candidates) == 0 {
		out.Candidates = nil
	}
	return out
}

func resourceStatusFromAssetStatus(status assetresolver.Status) ResourceResolutionStatus {
	switch status {
	case assetresolver.StatusResolved:
		return ResourceResolutionStatusResolved
	case assetresolver.StatusAmbiguous:
		return ResourceResolutionStatusAmbiguous
	default:
		return ResourceResolutionStatusNotFound
	}
}

func actionResourceRefFromAsset(asset assetresolver.Asset) actiondto.ResourceRef {
	metadata := copyStringAnyMap(asset.Metadata)
	if asset.WorkspaceID != "" {
		if metadata == nil {
			metadata = map[string]interface{}{}
		}
		metadata["workspace_id"] = asset.WorkspaceID
	}
	return actiondto.ResourceRef{
		Type:     firstNonEmptyString(asset.Type, resourceTypeFile),
		ID:       asset.ID,
		Name:     asset.Name,
		Source:   firstNonEmptyString(asset.Source, "assetresolver"),
		Metadata: metadata,
	}
}

func resolveChatResourceRefs(parts *chatRequestParts, refs []PlannerResourceRef) ResourceResolverResult {
	return NewResourceResolver().Resolve(resourceResolverInputFromChatParts(parts), refs)
}

func resourceResolverInputFromChatParts(parts *chatRequestParts) ResourceResolverInput {
	if parts == nil {
		return ResourceResolverInput{}
	}
	input := ResourceResolverInput{
		OperationContext:           parts.RawOperationContext,
		NormalizedOperationContext: parts.OperationContext,
	}
	if len(parts.RecentAssetCandidates) > 0 {
		input.AttachmentFiles = append(input.AttachmentFiles, parts.RecentAssetCandidates...)
	}
	if parts.Attachments == nil || len(parts.Attachments.Files) == 0 {
		return input
	}
	for _, file := range parts.Attachments.Files {
		input.AttachmentFiles = append(input.AttachmentFiles, ResourceCandidate{
			Type:      resourceTypeFile,
			ID:        file.ID,
			Name:      file.Name,
			Title:     attachmentDisplayName(file),
			Source:    "chat.attachments",
			Extension: file.Extension,
			MimeType:  file.MimeType,
			FileType:  file.kind(),
			Selected:  true,
		})
	}
	return input
}

func (r ResourceResolver) resolveOne(catalog resourceResolverCatalog, ref PlannerResourceRef) ResourceResolution {
	criteria := resourceRefCriteriaFromPlannerRef(ref)
	if criteria.resourceType != "" && criteria.resourceType != resourceTypeFile {
		return resourceResolutionNotFound(ref, "resource type is not supported by file resolver", nil)
	}

	allFiles := catalog.files
	if len(allFiles) == 0 {
		return resourceResolutionNotFound(ref, "no file candidates are available in context", nil)
	}

	if criteria.directID != "" {
		candidate, ok := findResourceCandidateByID(allFiles, criteria.directID)
		if !ok {
			return resourceResolutionNotFound(ref, "file_id was not found in context", visibleResourceCandidates(allFiles))
		}
		if !resourceCandidateMatches(candidate, criteria) {
			return resourceResolutionNotFound(ref, "file_id did not match requested filters", []ResourceCandidate{candidate})
		}
		return resourceResolutionResolved(ref, candidate, "file_id matched context")
	}

	selected := filterResourceCandidates(selectedResourceCandidates(allFiles), criteria)
	if criteria.selectedOnly {
		return resolveFromMatchedCandidates(ref, selected, allFiles, "selected file matched", "multiple selected files matched", "selected file was not found")
	}
	if len(selected) > 0 && !criteria.hasOrdinal && !criteria.hasNameMatcher {
		if len(selected) == 1 {
			return resourceResolutionResolved(ref, selected[0], "selected file matched")
		}
		return resourceResolutionAmbiguous(ref, "multiple selected files matched", selected)
	}

	visible := visibleResourceCandidates(allFiles)
	matches := filterResourceCandidates(visible, criteria)
	if criteria.hasOrdinal {
		if len(matches) == 0 {
			return resourceResolutionNotFound(ref, "no visible file matched requested filters", visible)
		}
		if criteria.ordinalLast {
			return resourceResolutionResolved(ref, matches[len(matches)-1], "visible ordinal matched")
		}
		if criteria.ordinal <= 0 || criteria.ordinal > len(matches) {
			return resourceResolutionNotFound(ref, "visible ordinal is out of range", matches)
		}
		return resourceResolutionResolved(ref, matches[criteria.ordinal-1], "visible ordinal matched")
	}

	return resolveFromMatchedCandidates(ref, matches, visible, "exactly one visible file matched", "multiple visible files matched", "no visible file matched")
}

func resolveFromMatchedCandidates(ref PlannerResourceRef, matches []ResourceCandidate, fallback []ResourceCandidate, resolvedReason, ambiguousReason, notFoundReason string) ResourceResolution {
	switch len(matches) {
	case 0:
		return resourceResolutionNotFound(ref, notFoundReason, fallback)
	case 1:
		return resourceResolutionResolved(ref, matches[0], resolvedReason)
	default:
		return resourceResolutionAmbiguous(ref, ambiguousReason, matches)
	}
}

func resourceResolutionResolved(ref PlannerResourceRef, candidate ResourceCandidate, reason string) ResourceResolution {
	resource := actionResourceRefFromCandidate(candidate)
	return ResourceResolution{
		Ref:       ref,
		Status:    ResourceResolutionStatusResolved,
		Reason:    reason,
		Resources: []actiondto.ResourceRef{resource},
		FileIDs:   []string{resource.ID},
	}
}

func resourceResolutionAmbiguous(ref PlannerResourceRef, reason string, candidates []ResourceCandidate) ResourceResolution {
	return ResourceResolution{
		Ref:        ref,
		Status:     ResourceResolutionStatusAmbiguous,
		Reason:     reason,
		Candidates: limitResourceCandidates(candidates, resourceResolverCandidateLimit),
	}
}

func resourceResolutionNotFound(ref PlannerResourceRef, reason string, candidates []ResourceCandidate) ResourceResolution {
	return ResourceResolution{
		Ref:        ref,
		Status:     ResourceResolutionStatusNotFound,
		Reason:     reason,
		Candidates: limitResourceCandidates(candidates, resourceResolverCandidateLimit),
	}
}

func actionResourceRefFromCandidate(candidate ResourceCandidate) actiondto.ResourceRef {
	metadata := map[string]interface{}{}
	if candidate.Title != "" {
		metadata["title"] = candidate.Title
	}
	if candidate.Extension != "" {
		metadata["extension"] = normalizedResolverFileExtension(candidate.Extension)
	}
	if candidate.MimeType != "" {
		metadata["mime_type"] = strings.TrimSpace(candidate.MimeType)
	}
	if candidate.FileType != "" {
		metadata["file_type"] = strings.TrimSpace(candidate.FileType)
	}
	if candidate.Selected {
		metadata["selected"] = true
	}
	if candidate.Recent {
		metadata["recent"] = true
	}
	if candidate.VisibleOrdinal > 0 {
		metadata["visible_ordinal"] = candidate.VisibleOrdinal
	}
	if len(metadata) == 0 {
		metadata = nil
	}
	return actiondto.ResourceRef{
		Type:     resourceTypeFile,
		ID:       candidate.ID,
		Name:     firstNonEmptyString(candidate.Title, candidate.Name),
		Source:   firstNonEmptyString(candidate.Source, "aichat.resource_resolver"),
		Metadata: metadata,
	}
}

type resourceResolverCatalog struct {
	files []ResourceCandidate
}

type resourceResolverCatalogBuilder struct {
	files       []ResourceCandidate
	byID        map[string]int
	visibleIDs  []string
	visibleSeen map[string]struct{}
}

func buildResourceResolverCatalog(input ResourceResolverInput) resourceResolverCatalog {
	builder := &resourceResolverCatalogBuilder{
		byID:        map[string]int{},
		visibleSeen: map[string]struct{}{},
	}
	for _, file := range input.AttachmentFiles {
		file.Type = resourceTypeFile
		if strings.TrimSpace(file.Source) == "" {
			file.Source = "chat.attachments"
		}
		builder.add(file)
	}
	builder.addContext(input.OperationContext, "operation_context")
	builder.addContext(input.NormalizedOperationContext, "operation_context.normalized")
	builder.addSelectedIDs(input.OperationContext, "operation_context.selected")
	builder.addSelectedIDs(input.NormalizedOperationContext, "operation_context.normalized.selected")
	builder.assignVisibleOrdinals()
	return resourceResolverCatalog{files: builder.files}
}

func (b *resourceResolverCatalogBuilder) addContext(context map[string]interface{}, source string) {
	if len(context) == 0 {
		return
	}
	for _, candidate := range visibleFileCandidatesFromContext(context, source) {
		b.add(candidate)
	}
}

func (b *resourceResolverCatalogBuilder) addSelectedIDs(value interface{}, source string) {
	collector := newUniqueStringCollector()
	collectSelectedFileIDs(value, 0, collector.add)
	for _, id := range collector.values() {
		b.add(ResourceCandidate{
			Type:     resourceTypeFile,
			ID:       id,
			Source:   source,
			Selected: true,
		})
	}
}

func (b *resourceResolverCatalogBuilder) add(candidate ResourceCandidate) {
	id := strings.TrimSpace(candidate.ID)
	if id == "" {
		return
	}
	candidate.ID = id
	candidate.Type = resourceTypeFile
	candidate.Extension = normalizedResolverFileExtension(candidate.Extension)
	candidate.MimeType = strings.TrimSpace(candidate.MimeType)
	candidate.FileType = normalizeResourceToken(candidate.FileType)
	if candidate.Extension == "" {
		candidate.Extension = extensionFromResourceCandidate(candidate)
	}
	if candidate.FileType == "" {
		candidate.FileType = inferFileType(candidate)
	}
	if candidate.Visible {
		if _, seen := b.visibleSeen[id]; !seen {
			b.visibleSeen[id] = struct{}{}
			b.visibleIDs = append(b.visibleIDs, id)
		}
	}
	if idx, exists := b.byID[id]; exists {
		b.files[idx] = mergeResourceCandidates(b.files[idx], candidate)
		return
	}
	b.byID[id] = len(b.files)
	b.files = append(b.files, candidate)
}

func (b *resourceResolverCatalogBuilder) assignVisibleOrdinals() {
	for index, id := range b.visibleIDs {
		fileIndex, ok := b.byID[id]
		if !ok {
			continue
		}
		b.files[fileIndex].Visible = true
		b.files[fileIndex].VisibleOrdinal = index + 1
	}
}

func mergeResourceCandidates(existing ResourceCandidate, next ResourceCandidate) ResourceCandidate {
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
	existing.Selected = existing.Selected || next.Selected
	existing.Recent = existing.Recent || next.Recent
	existing.Visible = existing.Visible || next.Visible
	if existing.VisibleOrdinal == 0 {
		existing.VisibleOrdinal = next.VisibleOrdinal
	}
	if existing.Metadata == nil {
		existing.Metadata = next.Metadata
	}
	return existing
}

func visibleFileCandidatesFromContext(context map[string]interface{}, source string) []ResourceCandidate {
	var candidates []ResourceCandidate
	var visit func(interface{}, int)
	visit = func(value interface{}, depth int) {
		if depth > 6 || value == nil {
			return
		}
		switch typed := value.(type) {
		case map[string]interface{}:
			if candidate, ok := resourceCandidateFromMap(typed, source, false); ok {
				candidates = append(candidates, candidate)
			}
			for key, item := range typed {
				normalizedKey := normalizeResourceKey(key)
				switch {
				case isVisibleFileListKey(normalizedKey):
					candidates = append(candidates, resourceCandidatesFromList(item, source+"."+key, true)...)
				case isResourceListKey(normalizedKey):
					candidates = append(candidates, resourceCandidatesFromList(item, source+"."+key, false)...)
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

func resourceCandidatesFromList(value interface{}, source string, allowImplicitFile bool) []ResourceCandidate {
	items := operationItemsFromValue(value)
	if len(items) == 0 {
		return nil
	}
	candidates := make([]ResourceCandidate, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case map[string]interface{}:
			if candidate, ok := resourceCandidateFromMap(typed, source, allowImplicitFile); ok {
				candidates = append(candidates, candidate)
			}
		case string:
			if allowImplicitFile {
				candidates = append(candidates, ResourceCandidate{
					Type:    resourceTypeFile,
					ID:      strings.TrimSpace(typed),
					Source:  source,
					Visible: true,
				})
			}
		}
	}
	return candidates
}

func resourceCandidateFromMap(value map[string]interface{}, source string, allowImplicitFile bool) (ResourceCandidate, bool) {
	if len(value) == 0 {
		return ResourceCandidate{}, false
	}
	if !allowImplicitFile && !isFileResourceMap(value) {
		return ResourceCandidate{}, false
	}
	metadata := mapFromOperationContext(value["metadata"])
	id := firstNonEmptyString(
		firstMapValue(value, "resource_id", "id", "file_id", "upload_file_id", "related_id"),
		firstMapValue(metadata, "resource_id", "id", "file_id", "upload_file_id", "related_id"),
	)
	if id == "" {
		return ResourceCandidate{}, false
	}
	title := firstNonEmptyString(
		firstMapValue(value, "title", "name", "filename", "file_name", "label"),
		firstMapValue(metadata, "title", "name", "filename", "file_name", "label"),
	)
	name := firstNonEmptyString(
		firstMapValue(value, "name", "filename", "file_name", "title", "label"),
		firstMapValue(metadata, "name", "filename", "file_name", "title", "label"),
	)
	extension := firstNonEmptyString(
		firstMapValue(value, "extension", "ext", "suffix"),
		firstMapValue(metadata, "extension", "ext", "suffix"),
	)
	mimeType := firstNonEmptyString(
		firstMapValue(value, "mime_type", "mime", "content_type"),
		firstMapValue(metadata, "mime_type", "mime", "content_type"),
	)
	fileType := firstNonEmptyString(
		firstMapValue(value, "file_type", "format", "category"),
		firstMapValue(metadata, "file_type", "format", "category"),
	)
	return ResourceCandidate{
		Type:      resourceTypeFile,
		ID:        id,
		Name:      name,
		Title:     title,
		Source:    firstNonEmptyString(value["source"], source),
		Extension: extension,
		MimeType:  mimeType,
		FileType:  fileType,
		Selected: boolMetadataValue(firstMapValue(value, "selected", "is_selected")) ||
			boolMetadataValue(firstMapValue(metadata, "selected", "is_selected")),
		Visible:  true,
		Metadata: nil,
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

type resourceRefCriteria struct {
	resourceType   string
	directID       string
	selectedOnly   bool
	hasOrdinal     bool
	ordinal        int
	ordinalLast    bool
	hasNameMatcher bool
	titleContains  string
	nameContains   string
	fuzzyName      string
	extensions     []string
	mimeTypes      []string
	fileTypes      []string
}

func resourceRefCriteriaFromPlannerRef(ref PlannerResourceRef) resourceRefCriteria {
	metadataType := stringMetadataValue(firstMapValue(ref.Metadata, "type", "resource_type", "kind"))
	criteria := resourceRefCriteria{
		resourceType: normalizeResourceKind(firstNonEmptyString(ref.ResourceType, ref.Kind, ref.Metadata["resource_type"], ref.Metadata["kind"])),
	}
	if criteria.resourceType == "" {
		if kind := normalizeResourceKind(ref.Type); kind == resourceTypeFile || kind == "agent" || kind == "workflow" {
			criteria.resourceType = kind
		}
	}
	if criteria.resourceType == "" {
		if kind := normalizeResourceKind(metadataType); kind == resourceTypeFile || kind == "agent" || kind == "workflow" {
			criteria.resourceType = kind
		}
	}
	if criteria.resourceType == "" {
		criteria.resourceType = resourceTypeFile
	}
	if ref.Type != "" && normalizeResourceKind(ref.Type) != resourceTypeFile && criteria.resourceType == resourceTypeFile {
		criteria.fileTypes = append(criteria.fileTypes, normalizeResourceToken(ref.Type))
	}
	if metadataType != "" && normalizeResourceKind(metadataType) != resourceTypeFile && criteria.resourceType == resourceTypeFile {
		criteria.fileTypes = append(criteria.fileTypes, normalizeResourceToken(metadataType))
	}

	criteria.directID = plannerRefDirectID(ref)
	criteria.selectedOnly = ref.Selected ||
		boolMetadataValue(ref.Metadata["selected"]) ||
		isSelectedScope(ref.Scope) ||
		isSelectedScope(stringMetadataValue(ref.Metadata["scope"]))
	criteria.titleContains = firstNonEmptyString(ref.TitleContains, ref.Metadata["title_contains"], ref.Metadata["contains"])
	criteria.nameContains = firstNonEmptyString(ref.NameContains, ref.Metadata["name_contains"], ref.Metadata["filename_contains"])
	criteria.fuzzyName = firstNonEmptyString(ref.FuzzyName, ref.Title, ref.Name, ref.Metadata["fuzzy_name"], ref.Metadata["title"], ref.Metadata["name"], ref.Metadata["filename"])
	criteria.hasNameMatcher = strings.TrimSpace(criteria.titleContains) != "" ||
		strings.TrimSpace(criteria.nameContains) != "" ||
		strings.TrimSpace(criteria.fuzzyName) != ""

	criteria.extensions = append(criteria.extensions, normalizedResolverFileExtension(ref.Extension))
	criteria.extensions = append(criteria.extensions, normalizedResolverFileExtensions(ref.Extensions)...)
	criteria.extensions = append(criteria.extensions, normalizedResolverFileExtensions(stringMetadataSlice(firstMapValue(ref.Metadata, "extensions", "extension", "ext")))...)
	criteria.mimeTypes = append(criteria.mimeTypes, normalizedMimeType(ref.MimeType))
	criteria.mimeTypes = append(criteria.mimeTypes, normalizedMimeTypes(ref.MimeTypes)...)
	criteria.mimeTypes = append(criteria.mimeTypes, normalizedMimeTypes(stringMetadataSlice(firstMapValue(ref.Metadata, "mime_types", "mime_type", "mime", "content_type")))...)
	criteria.fileTypes = append(criteria.fileTypes, normalizeResourceToken(ref.FileType))
	criteria.fileTypes = append(criteria.fileTypes, normalizeResourceToken(stringMetadataValue(firstMapValue(ref.Metadata, "file_type", "format", "category"))))
	criteria.extensions = compactStrings(criteria.extensions)
	criteria.mimeTypes = compactStrings(criteria.mimeTypes)
	criteria.fileTypes = compactStrings(criteria.fileTypes)

	criteria.applyOrdinal(ref)
	return criteria
}

func (c *resourceRefCriteria) applyOrdinal(ref PlannerResourceRef) {
	for _, value := range []interface{}{
		ref.Ordinal,
		ref.VisibleIndex,
		ref.OrdinalText,
		ref.Selector,
		ref.Source,
		firstMapValue(ref.Metadata, "ordinal", "visible_ordinal", "visible_index", "index", "position", "rank", "selector", "reference"),
	} {
		ordinal, last, ok := parsePlannerOrdinal(value)
		if !ok {
			continue
		}
		c.hasOrdinal = true
		c.ordinal = ordinal
		c.ordinalLast = last
		return
	}
	if ordinal, last, ok := parsePlannerOrdinal(ref.ID); ok {
		c.hasOrdinal = true
		c.ordinal = ordinal
		c.ordinalLast = last
	}
}

func plannerRefDirectID(ref PlannerResourceRef) string {
	for _, value := range []string{
		ref.FileID,
		stringMetadataValue(firstMapValue(ref.Metadata, "file_id", "upload_file_id", "resource_id")),
		ref.ID,
	} {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		if _, _, ok := parsePlannerOrdinal(id); ok {
			continue
		}
		return id
	}
	return ""
}

func parsePlannerOrdinal(value interface{}) (int, bool, bool) {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed, false, true
		}
	case int64:
		if typed > 0 {
			return int(typed), false, true
		}
	case float64:
		asInt := int(typed)
		if typed == float64(asInt) && asInt > 0 {
			return asInt, false, true
		}
	case string:
		return parsePlannerOrdinalString(typed)
	}
	return 0, false, false
}

func parsePlannerOrdinalString(value string) (int, bool, bool) {
	text := strings.ToLower(strings.TrimSpace(value))
	if text == "" {
		return 0, false, false
	}
	switch text {
	case "last", "latest", "final":
		return 0, true, true
	case "first":
		return 1, false, true
	}
	if selectorValue, ok := visibleFilesSelectorValue(text); ok {
		return parsePlannerOrdinalString(selectorValue)
	}
	ordinal, err := strconv.Atoi(text)
	if err != nil || ordinal <= 0 {
		return 0, false, false
	}
	return ordinal, false, true
}

func visibleFilesSelectorValue(text string) (string, bool) {
	open := strings.Index(text, "[")
	close := strings.Index(text, "]")
	if open <= 0 || close <= open {
		return "", false
	}
	prefix := normalizeResourceKey(text[:open])
	if prefix != "visiblefiles" {
		return "", false
	}
	return strings.TrimSpace(text[open+1 : close]), true
}

func findResourceCandidateByID(candidates []ResourceCandidate, id string) (ResourceCandidate, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ResourceCandidate{}, false
	}
	for _, candidate := range candidates {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return ResourceCandidate{}, false
}

func selectedResourceCandidates(candidates []ResourceCandidate) []ResourceCandidate {
	out := make([]ResourceCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Selected {
			out = append(out, candidate)
		}
	}
	return out
}

func visibleResourceCandidates(candidates []ResourceCandidate) []ResourceCandidate {
	out := make([]ResourceCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Visible {
			out = append(out, candidate)
		}
	}
	return out
}

func filterResourceCandidates(candidates []ResourceCandidate, criteria resourceRefCriteria) []ResourceCandidate {
	out := make([]ResourceCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if resourceCandidateMatches(candidate, criteria) {
			out = append(out, candidate)
		}
	}
	return out
}

func resourceCandidateMatches(candidate ResourceCandidate, criteria resourceRefCriteria) bool {
	if len(criteria.extensions) > 0 && !candidateMatchesExtension(candidate, criteria.extensions) {
		return false
	}
	if len(criteria.mimeTypes) > 0 && !candidateMatchesMimeType(candidate, criteria.mimeTypes) {
		return false
	}
	if len(criteria.fileTypes) > 0 && !candidateMatchesAnyFileType(candidate, criteria.fileTypes) {
		return false
	}
	if criteria.titleContains != "" && !candidateTextContains(candidate, criteria.titleContains) {
		return false
	}
	if criteria.nameContains != "" && !candidateTextContains(candidate, criteria.nameContains) {
		return false
	}
	if criteria.fuzzyName != "" && !candidateFuzzyNameMatches(candidate, criteria.fuzzyName) {
		return false
	}
	return true
}

func candidateMatchesExtension(candidate ResourceCandidate, extensions []string) bool {
	candidateExtension := extensionFromResourceCandidate(candidate)
	for _, extension := range extensions {
		if extension != "" && candidateExtension == extension {
			return true
		}
	}
	return false
}

func candidateMatchesMimeType(candidate ResourceCandidate, mimeTypes []string) bool {
	candidateMime := normalizedMimeType(candidate.MimeType)
	if candidateMime == "" {
		return false
	}
	for _, mimeType := range mimeTypes {
		if mimeType == "" {
			continue
		}
		if candidateMime == mimeType || strings.HasPrefix(candidateMime, strings.TrimSuffix(mimeType, "/*")+"/") {
			return true
		}
	}
	return false
}

func candidateMatchesAnyFileType(candidate ResourceCandidate, fileTypes []string) bool {
	for _, fileType := range fileTypes {
		if fileType != "" && candidateMatchesFileType(candidate, fileType) {
			return true
		}
	}
	return false
}

func candidateMatchesFileType(candidate ResourceCandidate, fileType string) bool {
	fileType = normalizeResourceToken(fileType)
	if fileType == "" {
		return true
	}
	candidateFileType := normalizeResourceToken(candidate.FileType)
	if candidateFileType == fileType {
		return true
	}
	if candidateMatchesExtension(candidate, fileTypeExtensions(fileType)) {
		return true
	}
	candidateMime := normalizedMimeType(candidate.MimeType)
	switch fileType {
	case "image":
		return strings.HasPrefix(candidateMime, "image/")
	case "document":
		return candidateMime != "" && !strings.HasPrefix(candidateMime, "image/")
	default:
		return false
	}
}

func fileTypeExtensions(fileType string) []string {
	switch normalizeResourceToken(fileType) {
	case "pdf":
		return []string{"pdf"}
	case "excel", "spreadsheet":
		return []string{"xls", "xlsx", "xlsm", "xlsb"}
	case "csv":
		return []string{"csv"}
	case "image":
		return []string{"png", "jpg", "jpeg", "gif", "webp", "bmp", "svg"}
	case "document":
		return []string{"pdf", "doc", "docx", "txt", "md", "rtf"}
	default:
		extension := normalizedResolverFileExtension(fileType)
		if extension == "" {
			return nil
		}
		return []string{extension}
	}
}

func candidateTextContains(candidate ResourceCandidate, query string) bool {
	needle := normalizeResourceSearchText(query)
	if needle == "" {
		return true
	}
	for _, value := range candidateSearchTexts(candidate) {
		if strings.Contains(normalizeResourceSearchText(value), needle) {
			return true
		}
	}
	return false
}

func candidateFuzzyNameMatches(candidate ResourceCandidate, query string) bool {
	needle := normalizeResourceSearchText(query)
	if needle == "" {
		return true
	}
	needleBase := normalizeResourceSearchText(stripCandidateExtension(query))
	for _, value := range candidateSearchTexts(candidate) {
		haystack := normalizeResourceSearchText(value)
		haystackBase := normalizeResourceSearchText(stripCandidateExtension(value))
		if haystack == needle || haystackBase == needleBase {
			return true
		}
		if strings.Contains(haystack, needle) || strings.Contains(haystackBase, needleBase) {
			return true
		}
		if allSearchTokensContained(haystack, needle) {
			return true
		}
	}
	return false
}

func candidateSearchTexts(candidate ResourceCandidate) []string {
	values := []string{candidate.Title, candidate.Name}
	if candidate.Title != "" && candidate.Extension != "" && !strings.HasSuffix(strings.ToLower(candidate.Title), "."+candidate.Extension) {
		values = append(values, candidate.Title+"."+candidate.Extension)
	}
	if candidate.Name != "" && candidate.Extension != "" && !strings.HasSuffix(strings.ToLower(candidate.Name), "."+candidate.Extension) {
		values = append(values, candidate.Name+"."+candidate.Extension)
	}
	return values
}

func allSearchTokensContained(haystack string, needle string) bool {
	tokens := strings.Fields(needle)
	if len(tokens) == 0 {
		return false
	}
	for _, token := range tokens {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}

func normalizeResourceSearchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastSpace := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			builder.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func stripCandidateExtension(value string) string {
	value = strings.TrimSpace(value)
	extension := fileNameExtension(value)
	if extension == "" {
		return value
	}
	return strings.TrimSuffix(value, "."+extension)
}

func extensionFromResourceCandidate(candidate ResourceCandidate) string {
	if extension := normalizedResolverFileExtension(candidate.Extension); extension != "" {
		return extension
	}
	for _, value := range []string{candidate.Title, candidate.Name} {
		if extension := fileNameExtension(value); extension != "" {
			return extension
		}
	}
	return ""
}

func fileNameExtension(value string) string {
	value = strings.TrimSpace(value)
	dot := strings.LastIndex(value, ".")
	if dot < 0 || dot == len(value)-1 {
		return ""
	}
	return normalizedResolverFileExtension(value[dot+1:])
}

func inferFileType(candidate ResourceCandidate) string {
	extension := extensionFromResourceCandidate(candidate)
	switch {
	case extension == "pdf":
		return "pdf"
	case containsString(fileTypeExtensions("excel"), extension):
		return "excel"
	case containsString(fileTypeExtensions("image"), extension):
		return "image"
	case strings.HasPrefix(normalizedMimeType(candidate.MimeType), "image/"):
		return "image"
	default:
		return ""
	}
}

func normalizedResolverFileExtension(value string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), ".")
}

func normalizedResolverFileExtensions(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, normalizedResolverFileExtension(value))
	}
	return out
}

func normalizedMimeType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizedMimeTypes(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, normalizedMimeType(value))
	}
	return out
}

func normalizeResourceKind(value string) string {
	return normalizeResourceToken(value)
}

func normalizeResourceToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeResourceKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func isSelectedScope(value string) bool {
	scope := normalizeResourceKey(value)
	return scope == "selected" || scope == "selectedfile" || scope == "selectedfiles"
}

func compactStrings(values []string) []string {
	collector := newUniqueStringCollector()
	for _, value := range values {
		collector.add(value)
	}
	return collector.values()
}

func limitResourceCandidates(candidates []ResourceCandidate, limit int) []ResourceCandidate {
	if len(candidates) == 0 || limit <= 0 {
		return nil
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	out := make([]ResourceCandidate, len(candidates))
	copy(out, candidates)
	for i := range out {
		out[i].Metadata = nil
	}
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
