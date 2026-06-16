package assetresolver

import "strings"

// Resolve grounds selectors and returns per-selector plus flattened assets.
func (r Resolver) Resolve(req Request) Result {
	catalog := buildCatalog(req)
	selectors := req.Selectors
	if len(selectors) == 0 {
		selectors = []Selector{{Type: AssetTypeFile}}
	}
	result := Result{Resolutions: make([]Resolution, 0, len(selectors))}
	seen := newUniqueStringCollector()
	for _, selector := range selectors {
		resolution := r.resolveOne(catalog, selector, candidateLimit(req.CandidateLimit))
		result.Resolutions = append(result.Resolutions, resolution)
		if resolution.Status != StatusResolved {
			continue
		}
		for _, asset := range resolution.Assets {
			if asset.ID == "" || seen.has(asset.ID) {
				continue
			}
			seen.add(asset.ID)
			result.Assets = append(result.Assets, asset)
		}
	}
	return result
}

func (r Resolver) resolveOne(catalog catalog, selector Selector, limit int) Resolution {
	criteria := criteriaFromSelector(selector)
	if criteria.assetType != "" && criteria.assetType != AssetTypeFile {
		return resolution(selector, StatusUnsupported, "asset type is not supported by file resolver", nil, nil, limit)
	}
	allFiles := catalog.files
	if len(allFiles) == 0 {
		return resolution(selector, StatusNotFound, "no file candidates are available in context", nil, nil, limit)
	}
	if criteria.directID != "" {
		candidate, ok := findCandidateByID(allFiles, criteria.directID)
		if !ok {
			return resolution(selector, StatusNotFound, "file_id was not found in context", nil, visibleCandidates(allFiles), limit)
		}
		if !candidateMatches(candidate, criteria) {
			return resolution(selector, StatusNotFound, "file_id did not match requested filters", nil, []Candidate{candidate}, limit)
		}
		return resolution(selector, StatusResolved, "file_id matched context", []Candidate{candidate}, nil, limit)
	}

	selected := filterCandidates(selectedCandidates(allFiles), criteria)
	if criteria.selectedOnly {
		return resolveMatched(selector, selected, allFiles, "selected file matched", "multiple selected files matched", "selected file was not found", limit)
	}
	recent := filterCandidates(recentCandidates(allFiles), criteria)
	if criteria.recentOnly {
		return resolveMatched(selector, recent, allFiles, "recent file matched", "multiple recent files matched", "recent file was not found", limit)
	}
	if len(selected) > 0 && !criteria.hasOrdinal && !criteria.hasNameMatcher {
		if len(selected) == 1 {
			return resolution(selector, StatusResolved, "selected file matched", []Candidate{selected[0]}, nil, limit)
		}
		return resolution(selector, StatusAmbiguous, "multiple selected files matched", nil, selected, limit)
	}

	visible := visibleCandidates(allFiles)
	matches := filterCandidates(visible, criteria)
	if criteria.hasOrdinal {
		if len(matches) == 0 {
			return resolution(selector, StatusNotFound, "no visible file matched requested filters", nil, visible, limit)
		}
		if criteria.ordinalLast {
			return resolution(selector, StatusResolved, "visible ordinal matched", []Candidate{matches[len(matches)-1]}, nil, limit)
		}
		if criteria.ordinal <= 0 || criteria.ordinal > len(matches) {
			return resolution(selector, StatusNotFound, "visible ordinal is out of range", nil, matches, limit)
		}
		return resolution(selector, StatusResolved, "visible ordinal matched", []Candidate{matches[criteria.ordinal-1]}, nil, limit)
	}

	return resolveMatched(selector, matches, visible, "exactly one visible file matched", "multiple visible files matched", "no visible file matched", limit)
}

func resolveMatched(selector Selector, matches []Candidate, fallback []Candidate, resolvedReason, ambiguousReason, notFoundReason string, limit int) Resolution {
	switch len(matches) {
	case 0:
		return resolution(selector, StatusNotFound, notFoundReason, nil, fallback, limit)
	case 1:
		return resolution(selector, StatusResolved, resolvedReason, []Candidate{matches[0]}, nil, limit)
	default:
		return resolution(selector, StatusAmbiguous, ambiguousReason, nil, matches, limit)
	}
}

func resolution(selector Selector, status Status, reason string, assets []Candidate, candidates []Candidate, limit int) Resolution {
	out := Resolution{
		Selector: selector,
		Status:   status,
		Reason:   strings.TrimSpace(reason),
	}
	if len(assets) > 0 {
		out.Assets = make([]Asset, 0, len(assets))
		for _, candidate := range assets {
			out.Assets = append(out.Assets, assetFromCandidate(candidate))
		}
	}
	if len(candidates) > 0 {
		out.Candidates = limitCandidates(candidates, limit)
	}
	return out
}

func assetFromCandidate(candidate Candidate) Asset {
	metadata := map[string]interface{}{}
	if candidate.Title != "" {
		metadata["title"] = candidate.Title
	}
	if candidate.Extension != "" {
		metadata["extension"] = normalizedExtension(candidate.Extension)
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
	return Asset{
		Type:        AssetTypeFile,
		ID:          candidate.ID,
		Name:        firstNonEmptyString(candidate.Title, candidate.Name),
		WorkspaceID: candidate.WorkspaceID,
		Source:      firstNonEmptyString(candidate.Source, "assetresolver"),
		Metadata:    metadata,
	}
}

func candidateLimit(limit int) int {
	if limit <= 0 {
		return defaultCandidateLimit
	}
	return limit
}
