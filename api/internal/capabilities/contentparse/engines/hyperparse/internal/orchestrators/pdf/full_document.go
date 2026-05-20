package pdf

import (
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	pdfadapter "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/pdf"
	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/core/chunking"
	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/core/layoutdoc"
)

// full_document progress logging is enabled by CONTENT_PARSE_FULLDOC_LOG=1
// or compatible truthy values.

func fullDocProgressEnabled() bool {
	return envTruthy(contentParseEnv("CONTENT_PARSE_FULLDOC_LOG"))
}

type fullDocTrace struct {
	source string
	t0     time.Time
	last   time.Time
}

func newFullDocTrace(source string) *fullDocTrace {
	return &fullDocTrace{source: source, t0: time.Now(), last: time.Now()}
}

func (tr *fullDocTrace) step(name string, detail string) {
	if tr == nil || !fullDocProgressEnabled() {
		return
	}
	now := time.Now()
	elapsed := now.Sub(tr.t0).Milliseconds()
	sinceLast := now.Sub(tr.last).Milliseconds()
	tr.last = now
	src := tr.source
	if len(src) > 120 {
		src = src[:117] + "..."
	}
	if detail != "" {
		log.Printf("[full_document] step=%s source=%q elapsed_ms=%d since_last_ms=%d %s", name, src, elapsed, sinceLast, detail)
		return
	}
	log.Printf("[full_document] step=%s source=%q elapsed_ms=%d since_last_ms=%d", name, src, elapsed, sinceLast)
}

func envTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on", "y":
		return true
	default:
		return false
	}
}

func contentParseEnv(key string) string {
	if strings.HasPrefix(key, "CONTENT_PARSE_") {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
		return strings.TrimSpace(os.Getenv("DOCSTILL_" + strings.TrimPrefix(key, "CONTENT_PARSE_")))
	}
	return strings.TrimSpace(os.Getenv(key))
}

// maxFullDocumentCombinedTextBytes caps text_summary.combined_text so large
// documents do not allocate many megabytes during strings.Join.
const maxFullDocumentCombinedTextBytes = 1 << 20 // 1 MiB

func joinSegmentLinesTruncated(lines []string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		return "", false
	}
	const mark = "\n... [combined_text truncated]"
	var b strings.Builder
	n := 0
	truncated := false
	for i, ln := range lines {
		sep := 0
		if i > 0 {
			sep = 1
		}
		need := sep + len(ln)
		if n+need > maxBytes {
			truncated = true
			room := maxBytes - n
			if room <= 0 {
				break
			}
			if i > 0 && room > 0 {
				b.WriteByte('\n')
				room--
			}
			if room > 0 {
				if len(ln) > room {
					b.WriteString(ln[:room])
				} else {
					b.WriteString(ln)
				}
			}
			break
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(ln)
		n += need
	}
	if truncated {
		b.WriteString(mark)
	}
	return b.String(), truncated
}

type pageBoxRow struct {
	PageIndex    int              `json:"page_index"`
	ObjectNumber int              `json:"object_number"`
	MediaBox     string           `json:"media_box"`
	CropBox      string           `json:"crop_box,omitempty"`
	BleedBox     string           `json:"bleed_box,omitempty"`
	TrimBox      string           `json:"trim_box,omitempty"`
	ArtBox       string           `json:"art_box,omitempty"`
	LineElements []map[string]any `json:"line_elements,omitempty"`
}

type FullDocumentTimingBreakdown struct {
	PageInfosMs          int64          `json:"page_infos_ms,omitempty"`
	RenderSpecsMs        int64          `json:"render_specs_ms,omitempty"`
	TextExtractMs        int64          `json:"text_extract_ms,omitempty"`
	TextExtractDetail    map[string]any `json:"text_extract_detail,omitempty"`
	ImageExtractMs       int64          `json:"image_extract_ms,omitempty"`
	OutlineExtractMs     int64          `json:"outline_extract_ms,omitempty"`
	AnnotationsExtractMs int64          `json:"annotations_extract_ms,omitempty"`
	FormsExtractMs       int64          `json:"forms_extract_ms,omitempty"`
	AttachmentsExtractMs int64          `json:"attachments_extract_ms,omitempty"`
	ParallelExtractMs    int64          `json:"parallel_extract_ms,omitempty"`
	ChunkingMs           int64          `json:"chunking_ms,omitempty"`
	TotalMs              int64          `json:"total_ms,omitempty"`
}

func ParseFullDocument(sourcePath, validateMode string) (map[string]any, error) {
	tRead := time.Now()
	if fullDocProgressEnabled() {
		log.Printf("[full_document] step=read_file_start source=%q mode=%s", sourcePath, validateMode)
	}
	data, err := os.ReadFile(sourcePath)
	if fullDocProgressEnabled() {
		if err != nil {
			log.Printf("[full_document] step=read_file_error elapsed_ms=%d err=%v", time.Since(tRead).Milliseconds(), err)
		} else {
			log.Printf("[full_document] step=read_file_done bytes=%d elapsed_ms=%d", len(data), time.Since(tRead).Milliseconds())
		}
	}
	if err != nil {
		return nil, err
	}
	return ParseFullDocumentBytes(data, sourcePath, validateMode)
}

func ParseFullDocumentBytes(data []byte, sourceName, validateMode string) (map[string]any, error) {
	return ParseFullDocumentBytesWithBasic(data, sourceName, validateMode, nil)
}

func ParseFullDocumentBytesWithBasicProfiled(data []byte, sourceName, validateMode string, basic *pdfadapter.BasicInfo) (map[string]any, FullDocumentTimingBreakdown, error) {
	return parseFullDocumentBytesWithBasic(data, sourceName, validateMode, basic)
}

// ParseFullDocumentBytesWithBasic is like ParseFullDocumentBytes, but reuses a
// non-nil BasicInfo to avoid repeated InspectBasicBytes work.
func ParseFullDocumentBytesWithBasic(data []byte, sourceName, validateMode string, basic *pdfadapter.BasicInfo) (map[string]any, error) {
	fullDoc, _, err := parseFullDocumentBytesWithBasic(data, sourceName, validateMode, basic)
	return fullDoc, err
}

func parseFullDocumentBytesWithBasic(data []byte, sourceName, validateMode string, basic *pdfadapter.BasicInfo) (map[string]any, FullDocumentTimingBreakdown, error) {
	startedAt := time.Now()
	var timing FullDocumentTimingBreakdown
	tr := newFullDocTrace(sourceName)
	tr.step("begin", fmt.Sprintf("bytes=%d mode=%s", len(data), validateMode))

	unregisterObjIndex := pdfadapter.RegisterObjectIndexForParse(data)
	defer unregisterObjIndex()
	tr.step("object_index_ready", "")

	var info *pdfadapter.BasicInfo
	var err error
	if basic != nil {
		info = basic
		tr.step("inspect_basic", "reused=true")
	} else {
		info, err = pdfadapter.InspectBasicBytes(data, validateMode)
		if err != nil {
			tr.step("inspect_basic_error", err.Error())
			return nil, timing, err
		}
		tr.step("inspect_basic", "reused=false")
	}

	pageInfosStartedAt := time.Now()
	pageInfos, err := pdfadapter.DetectPageInfosBytes(data, validateMode)
	timing.PageInfosMs = time.Since(pageInfosStartedAt).Milliseconds()
	if err != nil {
		tr.step("page_infos_error", err.Error())
		return nil, timing, err
	}
	tr.step("page_infos", fmt.Sprintf("pages=%d", len(pageInfos)))

	renderSpecsStartedAt := time.Now()
	renderSpecs, err := pdfadapter.DetectPageRenderSpecsFromPageInfos(data, pageInfos)
	timing.RenderSpecsMs = time.Since(renderSpecsStartedAt).Milliseconds()
	if err != nil {
		tr.step("render_specs_error", err.Error())
		return nil, timing, err
	}
	tr.step("render_specs", fmt.Sprintf("specs=%d", len(renderSpecs)))

	var (
		segments       []pdfadapter.TextSegment
		geometryLines  []pdfadapter.GeometryLine
		geometryTokens []pdfadapter.GeometryToken
		images         []pdfadapter.ExtractedImageBytes
		bookmarks      []pdfadapter.OutlineEntry
		annotations    []pdfadapter.AnnotationEntry
		forms          []pdfadapter.FormFieldEntry
		attachments    []pdfadapter.AttachmentEntry
	)

	var errMu sync.Mutex
	var firstErr error
	setErr := func(e error) {
		if e == nil {
			return
		}
		errMu.Lock()
		defer errMu.Unlock()
		if firstErr == nil {
			firstErr = e
		}
	}

	tr.step("parallel_extract_start", "workers=6(text,images,outline,annotations,forms,attachments)")
	parallelExtractStartedAt := time.Now()

	var wg sync.WaitGroup
	wg.Add(6)
	var textExtractMs int64
	var imageExtractMs int64
	var outlineExtractMs int64
	var annotationsExtractMs int64
	var formsExtractMs int64
	var attachmentsExtractMs int64
	go func() {
		defer wg.Done()
		textStartedAt := time.Now()
		var textProfile pdfadapter.TextExtractTimingBreakdown
		segments, geometryLines, geometryTokens, textProfile = pdfadapter.ExtractAllTextFromPageSpecsProfiled(data, renderSpecs)
		textExtractMs = time.Since(textStartedAt).Milliseconds()
		if detail := textProfile.ToMap(); len(detail) > 0 {
			timing.TextExtractDetail = detail
		}
	}()
	go func() {
		defer wg.Done()
		// Full-document parsing only needs image metadata chunks, so avoid full
		// DecodeStreamFilters per image, which dominates scanned PDF cost.
		imageStartedAt := time.Now()
		im, e := pdfadapter.ExtractEmbeddedImagesFromBytesWithSpecsLight(data, validateMode, renderSpecs)
		imageExtractMs = time.Since(imageStartedAt).Milliseconds()
		if e != nil {
			setErr(e)
			return
		}
		images = im
	}()
	go func() {
		defer wg.Done()
		outlineStartedAt := time.Now()
		bm, e := pdfadapter.ExtractOutlineEntriesFromBytes(data, validateMode)
		outlineExtractMs = time.Since(outlineStartedAt).Milliseconds()
		if e != nil {
			setErr(e)
			return
		}
		bookmarks = bm
	}()
	go func() {
		defer wg.Done()
		annotationsStartedAt := time.Now()
		an, e := pdfadapter.ExtractAnnotationEntriesFromBytesWithPageInfos(data, validateMode, pageInfos)
		annotationsExtractMs = time.Since(annotationsStartedAt).Milliseconds()
		if e != nil {
			setErr(e)
			return
		}
		annotations = an
	}()
	go func() {
		defer wg.Done()
		formsStartedAt := time.Now()
		fm, e := pdfadapter.ExtractFormFieldEntriesFromBytes(data, validateMode)
		formsExtractMs = time.Since(formsStartedAt).Milliseconds()
		if e != nil {
			setErr(e)
			return
		}
		forms = fm
	}()
	go func() {
		defer wg.Done()
		attachmentsStartedAt := time.Now()
		at, e := pdfadapter.ExtractAttachmentEntriesFromBytes(data, validateMode)
		attachmentsExtractMs = time.Since(attachmentsStartedAt).Milliseconds()
		if e != nil {
			setErr(e)
			return
		}
		attachments = at
	}()
	wg.Wait()
	timing.TextExtractMs = textExtractMs
	timing.ImageExtractMs = imageExtractMs
	timing.OutlineExtractMs = outlineExtractMs
	timing.AnnotationsExtractMs = annotationsExtractMs
	timing.FormsExtractMs = formsExtractMs
	timing.AttachmentsExtractMs = attachmentsExtractMs
	timing.ParallelExtractMs = time.Since(parallelExtractStartedAt).Milliseconds()
	tr.step("parallel_extract_done", fmt.Sprintf(
		"segments=%d geom_lines=%d geom_tokens=%d images=%d bookmarks=%d annotations=%d forms=%d attachments=%d",
		len(segments), len(geometryLines), len(geometryTokens), len(images), len(bookmarks), len(annotations), len(forms), len(attachments),
	))
	if firstErr != nil {
		tr.step("error_parallel", firstErr.Error())
		return nil, timing, firstErr
	}

	pages := make([]pageBoxRow, 0, len(pageInfos))
	for i := range pageInfos {
		pi := pageInfos[i]
		pages = append(pages, pageBoxRow{
			PageIndex:    i + 1,
			ObjectNumber: pi.ObjectNumber,
			MediaBox:     pi.MediaBox,
			CropBox:      pi.CropBox,
			BleedBox:     pi.BleedBox,
			TrimBox:      pi.TrimBox,
			ArtBox:       pi.ArtBox,
		})
	}

	lines := make([]string, 0, len(segments))
	typeCount := map[string]int{}
	pageGeomByIndex := make(map[int]chunking.PageGeom, len(pageInfos))
	for i := range pageInfos {
		if g, ok := parsePageGeom(pageInfos[i].CropBox); ok {
			pageGeomByIndex[i+1] = g
			continue
		}
		if g, ok := parsePageGeom(pageInfos[i].MediaBox); ok {
			pageGeomByIndex[i+1] = g
		}
	}
	attachLayoutLineElements(pages, geometryLines, pageGeomByIndex)
	for _, s := range segments {
		lines = append(lines, s.Text)
		if s.ChunkType != "" {
			typeCount[s.ChunkType]++
		}
	}
	pageRefs := make([]chunking.PageRef, 0, len(pages))
	for _, p := range pages {
		pageRefs = append(pageRefs, chunking.PageRef{PageIndex: p.PageIndex, ObjectNumber: p.ObjectNumber})
	}
	texts := make([]chunking.TextLike, 0, len(segments))
	for _, s := range segments {
		texts = append(texts, chunking.TextLike{
			Order:       s.Order,
			SourceTrace: s.SourceTrace,
			Text:        s.Text,
			ChunkType:   s.ChunkType,
			GeomX:       s.GeomX,
			GeomY:       s.GeomY,
			BBox:        normalizeAdapterBBox(s.BBox, pageGeomByIndex[pageIndexFromTrace(s.SourceTrace)]),
		})
	}
	geometryLineLikes := make([]chunking.GeometryLineLike, 0, len(geometryLines))
	for _, gl := range geometryLines {
		geometryLineLikes = append(geometryLineLikes, chunking.GeometryLineLike{
			Order:       gl.Order,
			SourceTrace: gl.SourceTrace,
			Text:        gl.Text,
			PageIndex:   gl.PageIndex,
			GeomX:       gl.GeomX,
			GeomY:       gl.GeomY,
			BBox:        normalizeAdapterBBox(gl.BBox, pageGeomByIndex[gl.PageIndex]),
		})
	}
	geometryTokenLikes := make([]chunking.GeometryTokenLike, 0, len(geometryTokens))
	for _, gt := range geometryTokens {
		geometryTokenLikes = append(geometryTokenLikes, chunking.GeometryTokenLike{
			Order:       gt.Order,
			SourceTrace: gt.SourceTrace,
			Text:        gt.Text,
			PageIndex:   gt.PageIndex,
			GeomX:       gt.GeomX,
			GeomY:       gt.GeomY,
			BBox:        normalizeAdapterBBox(gt.BBox, pageGeomByIndex[gt.PageIndex]),
			FontKey:     gt.FontKey,
			BaseFont:    gt.BaseFont,
			FontSizePt:  gt.FontSizePt,
		})
	}
	imageLikes := make([]chunking.ImageLike, 0, len(images))
	for _, im := range images {
		bs := len(im.Bytes)
		if bs == 0 && im.StreamByteSize > 0 {
			bs = im.StreamByteSize
		}
		imageLikes = append(imageLikes, chunking.ImageLike{
			PageIndex:     im.PageIndex,
			PageObject:    im.PageObject,
			XObjectName:   im.XObjectName,
			ObjectNumber:  im.ObjectNumber,
			Format:        im.Format,
			Width:         im.Width,
			Height:        im.Height,
			ByteSize:      bs,
			DecodeWarning: im.DecodeWarning,
		})
	}
	bookmarkLikes := make([]chunking.BookmarkLike, 0, len(bookmarks))
	for _, bm := range bookmarks {
		bookmarkLikes = append(bookmarkLikes, chunking.BookmarkLike{
			Title:      bm.Title,
			PageObject: bm.PageObject,
			Object:     bm.Object,
			Level:      bm.Level,
			Dest:       bm.Dest,
			TargetRaw:  bm.TargetRaw,
			TargetKind: bm.TargetKind,
		})
	}
	annotationLikes := make([]chunking.AnnotationLike, 0, len(annotations))
	for _, an := range annotations {
		annotationLikes = append(annotationLikes, chunking.AnnotationLike{
			PageIndex:    an.PageIndex,
			ObjectNumber: an.ObjectNumber,
			Subtype:      an.Subtype,
			Rect:         an.Rect,
			Contents:     an.Contents,
		})
	}
	formLikes := make([]chunking.FormLike, 0, len(forms))
	for _, f := range forms {
		formLikes = append(formLikes, chunking.FormLike{
			ObjectNumber: f.ObjectNumber,
			Name:         f.Name,
			AltName:      f.AltName,
			FieldType:    f.FieldType,
			Value:        f.Value,
			Flags:        f.Flags,
			PageObject:   f.PageObject,
			Rect:         f.Rect,
		})
	}
	attachmentLikes := make([]chunking.AttachmentLike, 0, len(attachments))
	for _, at := range attachments {
		attachmentLikes = append(attachmentLikes, chunking.AttachmentLike{
			FileSpecObject:    at.FileSpecObject,
			FileName:          at.FileName,
			UnicodeFileName:   at.UnicodeFileName,
			EmbeddedFileObj:   at.EmbeddedFileObj,
			EmbeddedSizeBytes: at.EmbeddedSizeBytes,
			EmbeddedSubtype:   at.EmbeddedSubtype,
		})
	}
	tr.step("build_chunk_inputs", fmt.Sprintf(
		"text_likes=%d geom_lines=%d geom_tokens=%d images=%d bookmarks=%d annotations=%d forms=%d attachments=%d",
		len(texts), len(geometryLineLikes), len(geometryTokenLikes), len(imageLikes), len(bookmarkLikes), len(annotationLikes), len(formLikes), len(attachmentLikes),
	))

	skipTableChunking := envTruthy(contentParseEnv("CONTENT_PARSE_SKIP_TABLE"))
	tr.step("chunking_start", fmt.Sprintf("skip_table_rule=%v", skipTableChunking))
	chunkingStartedAt := time.Now()
	allChunks := chunking.Build(chunking.BuildInput{
		Source:         sourceName,
		PageGeoms:      pageGeomByIndex,
		Pages:          pageRefs,
		Texts:          texts,
		GeometryLines:  geometryLineLikes,
		GeometryTokens: geometryTokenLikes,
		Images:         imageLikes,
		Bookmarks:      bookmarkLikes,
		Annotations:    annotationLikes,
		Forms:          formLikes,
		Attachments:    attachmentLikes,
		SkipTableRule:  skipTableChunking,
	})
	timing.ChunkingMs = time.Since(chunkingStartedAt).Milliseconds()
	tr.step("chunking_done", fmt.Sprintf("chunks=%d", len(allChunks)))
	layoutQuality := analyzeAndRepairLocalLayout(sourceName, data, allChunks, geometryLines, pageGeomByIndex)
	allChunks = layoutQuality.Chunks
	if layoutQuality.RepairAdded > 0 || layoutQuality.SuggestHybrid {
		tr.step("layout_quality", fmt.Sprintf("repair_chunks=%d suggest_hybrid=%v", layoutQuality.RepairAdded, layoutQuality.SuggestHybrid))
	}
	allChunks, layoutPipeline, err := layoutdoc.NormalizeAndOrderChunkMaps(sourceName, len(pages), allChunks)
	if err != nil {
		tr.step("layout_pipeline_error", err.Error())
		return nil, timing, err
	}
	tr.step("layout_pipeline", fmt.Sprintf("chunks=%d stages=%d", len(allChunks), len(layoutPipeline.Stages)))

	imgHints := pdfadapter.BuildImageLikePDFHintsWithLayout(data, validateMode, renderSpecs, segments, geometryLines)
	if imgHints.Likely {
		tr.step("suggest_vlm", strings.Join(imgHints.Reasons, ";"))
	}

	combinedText, combinedTrunc := joinSegmentLinesTruncated(lines, maxFullDocumentCombinedTextBytes)
	if layoutQuality.RepairAdded > 0 {
		combinedText, combinedTrunc = joinSegmentLinesTruncated(chunkTextLinesForSummary(allChunks), maxFullDocumentCombinedTextBytes)
	}
	bizVLM := pdfadapter.BuildBusinessDocVLMRouteHint(data, combinedText, info)
	pageRouteCandidates := buildPageRouteCandidates(len(pageInfos), geometryLines, geometryTokens, images, bizVLM, forceVLMFromEnv())
	routeDecision := buildRouteDecision(imgHints, bizVLM, pageRouteCandidates)
	suggestVLM, _ := routeDecision["legacy_suggest_vlm"].(bool)
	if bizVLM.Suggest {
		tr.step("suggest_vlm_business_doc", strings.Join(bizVLM.Reasons, ";"))
	}
	if layoutQuality.SuggestHybrid {
		suggestVLM = true
		routeDecision["legacy_suggest_vlm"] = true
		routeDecision["local_quality_suggest_hybrid"] = true
		tr.step("suggest_vlm_layout_quality", "")
	}

	docPayload := map[string]any{
		"source":           sourceName,
		"validation_mode":  validateMode,
		"generated_at_utc": time.Now().UTC().Format(time.RFC3339),
		"image_like_pdf": map[string]any{
			"likely":                      imgHints.Likely,
			"reasons":                     imgHints.Reasons,
			"total_decoded_content_bytes": imgHints.TotalDecodedContentBytes,
			"content_stream_object_count": imgHints.ContentStreamObjectCount,
			"basic_text_rune_count":       imgHints.BasicTextRuneCount,
			"basic_segment_count":         imgHints.BasicSegmentCount,
			"file_size_bytes":             imgHints.FileSizeBytes,
			"page_count":                  imgHints.PageCount,
			"bytes_per_page":              imgHints.BytesPerPage,
			"geometry_line_count":         imgHints.GeometryLineCount,
			"short_geometry_line_count":   imgHints.ShortGeometryLineCount,
			"form_like_line_count":        imgHints.FormLikeGeometryLineCount,
		},
		"suggest_vlm":           suggestVLM,
		"route_decision":        routeDecision,
		"page_route_candidates": pageRouteCandidates,
		"business_doc_vlm_hint": map[string]any{
			"suggest": bizVLM.Suggest,
			"kinds":   bizVLM.Kinds,
			"reasons": bizVLM.Reasons,
		},
		"local_quality":             layoutQuality.Payload,
		"layout_processor_pipeline": layoutdoc.ReportMap(layoutPipeline),
		"basic": map[string]any{
			"pdf_version":    info.Version,
			"file_size":      info.FileSize,
			"page_count":     info.PageCount,
			"count_source":   info.CountSource,
			"has_eof_marker": info.HasEOFMarker,
			"has_trailer":    info.HasTrailer,
			"start_xref":     info.StartXRef,
			"xref_type":      info.XRefType,
			"title":          info.Title,
			"author":         info.Author,
			"subject":        info.Subject,
			"keywords":       info.Keywords,
			"creator":        info.Creator,
			"producer":       info.Producer,
		},
		"layout": map[string]any{
			"page_count": len(pages),
			"pages":      pages,
		},
		"text_summary": func() map[string]any {
			m := map[string]any{
				"segment_count":        len(segments),
				"geometry_line_count":  len(geometryLines),
				"geometry_token_count": len(geometryTokens),
				"chunk_type_count":     typeCount,
				"combined_text":        combinedText,
			}
			if combinedTrunc {
				m["combined_text_truncated"] = true
			}
			return m
		}(),
	}
	if skipTableChunking {
		docPayload["skip_table_chunking"] = true
	}
	if forceVLMFromEnv() {
		docPayload["force_vlm"] = true
	}

	tr.step("complete", fmt.Sprintf("chunks=%d pages=%d", len(allChunks), len(pages)))
	timing.TotalMs = time.Since(startedAt).Milliseconds()

	return map[string]any{
		"schema_version": "full_document_v7",
		"document":       docPayload,
		"chunks": map[string]any{
			"count":                 len(allChunks),
			"items":                 allChunks,
			"bbox_coordinate_space": "page_relative_0_1",
		},
	}, timing, nil
}

func parsePageGeom(raw string) (chunking.PageGeom, bool) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) != 4 {
		return chunking.PageGeom{}, false
	}
	v := make([]float64, 4)
	for i := 0; i < 4; i++ {
		n, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return chunking.PageGeom{}, false
		}
		v[i] = n
	}
	l, b, r, t := v[0], v[1], v[2], v[3]
	if r <= l || t <= b {
		return chunking.PageGeom{}, false
	}
	return chunking.PageGeom{Left: l, Bottom: b, Right: r, Top: t}, true
}

func attachLayoutLineElements(pages []pageBoxRow, geometryLines []pdfadapter.GeometryLine, pageGeoms map[int]chunking.PageGeom) {
	if len(pages) == 0 || len(geometryLines) == 0 || len(pageGeoms) == 0 {
		return
	}

	linesByPage := make(map[int][]map[string]any, len(pages))
	for _, gl := range geometryLines {
		pageIndex := gl.PageIndex
		if pageIndex <= 0 {
			continue
		}
		g, ok := pageGeoms[pageIndex]
		if !ok {
			continue
		}
		box := normalizeAdapterBBox(gl.BBox, g)
		if box == nil {
			box = chunking.AnchorToBBox(gl.GeomX, gl.GeomY, g)
		}
		if box == nil {
			continue
		}
		boxMap := chunking.BBoxTopLeftMap(box)
		if boxMap == nil {
			continue
		}
		linesByPage[pageIndex] = append(linesByPage[pageIndex], map[string]any{
			"id":           fmt.Sprintf("line_%d_%d", pageIndex-1, gl.Order),
			"type":         "line",
			"order":        gl.Order,
			"text":         gl.Text,
			"source_trace": gl.SourceTrace,
			"box":          boxMap,
		})
	}

	for i := range pages {
		pageIndex := pages[i].PageIndex
		if pageIndex <= 0 {
			continue
		}
		if elems := linesByPage[pageIndex]; len(elems) > 0 {
			pages[i].LineElements = elems
		}
	}
}

func pageIndexFromTrace(trace string) int {
	const prefix = "page#"
	idx := strings.Index(trace, prefix)
	if idx < 0 {
		return 0
	}
	rest := trace[idx+len(prefix):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil || n < 1 {
		return 0
	}
	return n
}

func normalizeAdapterBBox(bb *pdfadapter.TextBBox, g chunking.PageGeom) *chunking.BBox {
	if bb == nil || g.Right <= g.Left || g.Top <= g.Bottom {
		return nil
	}
	left := clamp01((bb.Left - g.Left) / (g.Right - g.Left))
	right := clamp01((bb.Right - g.Left) / (g.Right - g.Left))
	top := clamp01((bb.Top - g.Bottom) / (g.Top - g.Bottom))
	bottom := clamp01((bb.Bottom - g.Bottom) / (g.Top - g.Bottom))
	if right < left {
		left, right = right, left
	}
	if top < bottom {
		top, bottom = bottom, top
	}
	if right <= left || top <= bottom {
		return nil
	}
	return &chunking.BBox{
		Left:   roundCoord(left),
		Right:  roundCoord(right),
		Top:    roundCoord(top),
		Bottom: roundCoord(bottom),
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func roundCoord(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}
