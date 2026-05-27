package image

import (
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/model"
	localocr "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/ocr"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/envconfig"
)

type Adapter struct{}

type ParseOptions struct {
	AggressiveRetry bool
}

func (a Adapter) Format() string {
	return "image"
}

func (a Adapter) Parse(path string) (*model.Document, error) {
	return ParsePathWithOptions(path, ParseOptions{})
}

func ParsePathWithOptions(path string, opts ParseOptions) (*model.Document, error) {
	ctx, cancel := context.WithTimeout(context.Background(), imageOCRTimeout())
	defer cancel()

	width, height, _ := imageBounds(path)

	primaryCfg := imageOCRConfig(path)
	primary, primaryErr := primaryCfg.RunLinesFile(ctx, path, width, height)

	best := primary
	bestScore := scoreOCRResult(primary)
	bestLabel := candidateLabel(primaryCfg)
	bestPreprocess := "original"
	var bestErr error
	if primaryErr != nil {
		bestErr = fmt.Errorf("%s: %w", bestLabel, primaryErr)
	}

	if candidates := retryImageOCRConfigs(path, primaryCfg, primary, width, height, opts); len(candidates) > 0 {
		defer cleanupOCRCandidates(candidates)
		for _, candidate := range candidates {
			retryPath := path
			if candidate.ImagePath != "" {
				retryPath = candidate.ImagePath
			}
			retry, retryErr := candidate.Config.RunLinesFile(ctx, retryPath, width, height)
			if retryErr != nil {
				continue
			}
			retryScore := scoreOCRResult(retry)
			if retryScore > bestScore+10 {
				best = retry
				bestScore = retryScore
				bestLabel = candidate.Label
				bestPreprocess = candidate.Preprocess
				bestErr = nil
			}
		}
	}

	if strings.TrimSpace(best.Text) == "" && bestErr != nil {
		return nil, bestErr
	}

	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	text := normalizeOCRText(best.Text)
	if text == "" {
		text = strings.TrimSpace(filepath.Base(path))
	}

	blocks := buildBlocksFromOCR(best, width, height)
	if len(blocks) == 0 {
		blocks = []model.Block{{
			Type:  "text",
			Text:  text,
			Page:  0,
			Order: 1,
		}}
	}

	metadata := map[string]string{
		"ocr_engine":     primaryCfg.EngineName(),
		"ocr_strategy":   bestLabel,
		"ocr_language":   primaryCfg.Lang,
		"ocr_preprocess": bestPreprocess,
	}
	if best.Engine != "" {
		metadata["ocr_engine"] = best.Engine
	}
	if bestLabel != candidateLabel(primaryCfg) {
		metadata["ocr_retry_used"] = "true"
	}

	return &model.Document{
		Format:    "image",
		Title:     title,
		PageCount: 1,
		Metadata:  metadata,
		Sections: []model.Section{
			{
				Path:    "root",
				Heading: "",
				Blocks:  blocks,
			},
		},
	}, nil
}

func buildBlocksFromOCR(result localocr.Result, width, height int) []model.Block {
	if len(result.Lines) == 0 {
		return nil
	}
	blocks := make([]model.Block, 0, len(result.Lines))
	for idx, line := range result.Lines {
		text := normalizeOCRText(line.Text)
		if text == "" {
			continue
		}
		block := model.Block{
			Type:    "text",
			Text:    text,
			Page:    0,
			Order:   idx + 1,
			TraceID: result.Engine,
		}
		if bbox := normalizeOCRLineBBox(line, width, height); bbox != nil {
			block.BBox = bbox
			block.Precision = "reliable"
			block.Payload = map[string]any{
				"bbox_source":       "ocr_line",
				"extraction_method": "ocr",
				"ocr_engine":        result.Engine,
			}
		}
		blocks = append(blocks, block)
	}
	return blocks
}

func normalizeOCRLineBBox(line localocr.Line, width, height int) *model.BBox {
	if width <= 0 || height <= 0 || line.Right <= line.Left || line.Bottom <= line.Top {
		return nil
	}
	left := clamp01(float64(line.Left) / float64(width))
	top := clamp01(float64(line.Top) / float64(height))
	right := clamp01(float64(line.Right) / float64(width))
	bottom := clamp01(float64(line.Bottom) / float64(height))
	if right <= left || bottom <= top {
		return nil
	}
	return &model.BBox{Left: left, Top: top, Right: right, Bottom: bottom}
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

func imageOCRConfig(path string) localocr.Config {
	cfg := localocr.LoadConfig(imageOCRTimeout())
	if !hasExplicitOCRLang() {
		cfg.Lang = defaultImageOCRLang(path)
	}
	return cfg
}

type ocrCandidate struct {
	Label      string
	Preprocess string
	ImagePath  string
	Cleanup    func()
	Config     localocr.Config
}

func retryImageOCRConfigs(path string, primary localocr.Config, primaryResult localocr.Result, width, height int, opts ParseOptions) []ocrCandidate {
	if !shouldRetryImageOCR(primaryResult, width, height, opts) {
		return nil
	}

	candidates := make([]ocrCandidate, 0, 3)

	if primary.EngineName() == localocr.EngineTesseract && !hasExplicitTesseractPSM() {
		psms := []int{11, 4}
		for _, psm := range psms {
			if psm == primary.TesseractPSM {
				continue
			}
			retry := primary
			retry.TesseractPSM = psm
			candidates = append(candidates, ocrCandidate{
				Label:      candidateLabel(retry),
				Preprocess: "original",
				Config:     retry,
			})
		}
	}

	if processed := buildPreprocessedTesseractCandidates(path, primary); len(processed) > 0 {
		candidates = append(candidates, processed...)
	}

	if primary.EngineName() != localocr.EnginePaddleOCR && paddleConfigured() {
		retry := primary
		retry.Engine = localocr.EnginePaddleOCR
		if !hasExplicitOCRLang() {
			retry.Lang = defaultImageOCRLang(path)
		}
		candidates = append(candidates, ocrCandidate{
			Label:      candidateLabel(retry),
			Preprocess: "original",
			Config:     retry,
		})
	}

	// Stable order helps keep benchmark variance low.
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Label < candidates[j].Label
	})
	return candidates
}

func buildPreprocessedTesseractCandidates(path string, primary localocr.Config) []ocrCandidate {
	if primary.EngineName() != localocr.EngineTesseract {
		return nil
	}
	modes := []struct {
		name string
		psm  int
	}{
		{name: "gray", psm: 11},
		{name: "binary", psm: 11},
		{name: "gray", psm: 4},
	}
	candidates := make([]ocrCandidate, 0, len(modes))
	for _, mode := range modes {
		imagePath, cleanup, err := preprocessImageForOCR(path, mode.name)
		if err != nil {
			continue
		}
		retry := primary
		retry.TesseractPSM = mode.psm
		label := fmt.Sprintf("%s+%s", candidateLabel(retry), mode.name)
		candidates = append(candidates, ocrCandidate{
			Label:      label,
			Preprocess: mode.name,
			ImagePath:  imagePath,
			Cleanup:    cleanup,
			Config:     retry,
		})
	}
	return candidates
}

func cleanupOCRCandidates(candidates []ocrCandidate) {
	for _, candidate := range candidates {
		if candidate.Cleanup != nil {
			candidate.Cleanup()
		}
	}
}

func hasExplicitOCRLang() bool {
	for _, key := range []string{"CONTENT_PARSE_OCR_LANG", "CONTENT_PARSE_LOCAL_OCR_LANG", "DOCSTILL_OCR_LANG", "DOCSTILL_LOCAL_OCR_LANG", "LOCAL_OCR_LANG"} {
		if envconfig.String(key) != "" {
			return true
		}
	}
	return false
}

func hasExplicitTesseractPSM() bool {
	for _, key := range []string{"CONTENT_PARSE_TESSERACT_PSM", "CONTENT_PARSE_OCR_TESSERACT_PSM", "DOCSTILL_TESSERACT_PSM", "DOCSTILL_OCR_TESSERACT_PSM"} {
		if envconfig.String(key) != "" {
			return true
		}
	}
	return false
}

func paddleConfigured() bool {
	if strings.TrimSpace(firstNonEmptyEnv("CONTENT_PARSE_PADDLEOCR_CMD", "DOCSTILL_PADDLEOCR_CMD")) == "" {
		return false
	}
	return true
}

func defaultImageOCRLang(path string) string {
	base := strings.TrimSpace(filepath.Base(path))
	for _, r := range base {
		if r > unicode.MaxASCII && unicode.IsLetter(r) {
			return "chi_sim+eng"
		}
	}
	return "eng"
}

func imageBounds(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func looksLikePoorOCR(text string) bool {
	text = normalizeOCRText(text)
	if len([]rune(text)) < 12 {
		return true
	}

	letters := 0
	vowels := 0
	separators := 0
	nonASCII := 0
	weirdSymbols := 0
	for _, r := range text {
		switch {
		case r >= 'A' && r <= 'Z':
			letters++
			if strings.ContainsRune("AEIOU", r) {
				vowels++
			}
		case r >= 'a' && r <= 'z':
			letters++
			if strings.ContainsRune("aeiou", r) {
				vowels++
			}
		case r > unicode.MaxASCII && unicode.IsLetter(r):
			nonASCII++
		case r == ' ' || r == '\n' || r == '\t':
			separators++
		case r == '«' || r == '»' || r == '•':
			weirdSymbols++
		}
	}

	totalLetters := letters + nonASCII
	if totalLetters == 0 {
		return true
	}
	if weirdSymbols > 0 {
		return true
	}
	if nonASCII >= 4 {
		return false
	}
	if letters < 10 {
		return false
	}
	vowelRatio := float64(vowels) / float64(letters)
	separatorRatio := float64(separators) / float64(len([]rune(text)))
	return vowelRatio < 0.18 || separatorRatio < 0.04
}

func shouldRetryImageOCR(result localocr.Result, width, height int, opts ParseOptions) bool {
	text := normalizeOCRText(result.Text)
	if looksLikePoorOCR(text) {
		return true
	}

	lineCount := nonEmptyLineCount(text)
	runes := len([]rune(text))
	shortRatio := shortLineRatio(text)
	symbolRatio := punctuationHeavyRatio(text)
	if width >= 1200 || height >= 1200 {
		if lineCount <= 2 || runes < 220 || shortRatio > 0.22 {
			return true
		}
	}
	if lineCount <= 1 && runes < 120 {
		return true
	}
	if lineCount >= 10 && shortRatio > 0.30 {
		return true
	}
	if looksLikeStructuredForm(text) {
		return true
	}
	if opts.AggressiveRetry && lineCount >= 8 && shortRatio > 0.12 {
		return true
	}
	if opts.AggressiveRetry && runes >= 300 && punctuationHeavyRatio(text) > 0.04 {
		return true
	}
	if runes >= 180 && symbolRatio > 0.20 {
		return true
	}
	return false
}

func scoreOCRResult(result localocr.Result) int {
	text := normalizeOCRText(result.Text)
	if text == "" {
		return 0
	}
	score := len([]rune(text))
	if !looksLikePoorOCR(text) {
		score += 200
	}
	lines := strings.Split(text, "\n")
	nonEmpty := make([]string, 0, len(lines))
	shortLines := 0
	totalLen := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		nonEmpty = append(nonEmpty, line)
		l := len([]rune(line))
		totalLen += l
		if l <= 2 {
			shortLines++
		}
	}
	if len(nonEmpty) > 1 {
		score += 20
	}
	if len(nonEmpty) > 4 {
		avg := float64(totalLen) / float64(len(nonEmpty))
		if avg < 4 {
			score -= 150
		}
		if float64(shortLines)/float64(len(nonEmpty)) > 0.35 {
			score -= 100
		}
	}
	if len(result.Lines) > 0 {
		score += min(60, len(result.Lines)*3)
	}
	return score
}

func scoreOCRText(text string) int {
	return scoreOCRResult(localocr.Result{Text: text})
}

func nonEmptyLineCount(text string) int {
	lines := strings.Split(normalizeOCRText(text), "\n")
	count := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func shortLineRatio(text string) float64 {
	lines := strings.Split(normalizeOCRText(text), "\n")
	total := 0
	short := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		total++
		if len([]rune(line)) <= 2 {
			short++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(short) / float64(total)
}

func punctuationHeavyRatio(text string) float64 {
	runes := []rune(normalizeOCRText(text))
	if len(runes) == 0 {
		return 0
	}
	punct := 0
	for _, r := range runes {
		if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			punct++
		}
	}
	return float64(punct) / float64(len(runes))
}

func looksLikeStructuredForm(text string) bool {
	text = normalizeOCRText(text)
	if text == "" {
		return false
	}
	lines := strings.Split(text, "\n")
	labelLines := 0
	upperHeavyLines := 0
	keywordHits := 0
	keywords := []string{
		"fax", "phone", "date", "company", "message", "cover sheet",
		"pages", "number", "transmission", "confidential", "sender",
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		for _, keyword := range keywords {
			if strings.Contains(lower, keyword) {
				keywordHits++
				break
			}
		}
		if strings.Contains(line, ":") {
			labelLines++
		}
		if isUpperHeavy(line) {
			upperHeavyLines++
		}
	}
	return keywordHits >= 3 || (labelLines >= 4 && upperHeavyLines >= 3)
}

func isUpperHeavy(line string) bool {
	letters := 0
	uppers := 0
	for _, r := range line {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				uppers++
			}
		}
	}
	if letters == 0 {
		return false
	}
	return float64(uppers)/float64(letters) > 0.55
}

func candidateLabel(cfg localocr.Config) string {
	label := cfg.EngineName()
	if cfg.EngineName() == localocr.EngineTesseract {
		label = fmt.Sprintf("%s:psm%d", label, cfg.TesseractPSM)
	}
	return label
}

func preprocessImageForOCR(path, mode string) (string, func(), error) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return "", nil, err
	}

	bounds := img.Bounds()
	out := image.NewGray(bounds)
	total := 0
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			out.SetGray(x, y, gray)
			total += int(gray.Y)
			count++
		}
	}

	if mode == "binary" && count > 0 {
		threshold := uint8(total / count)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				current := out.GrayAt(x, y).Y
				if current > threshold {
					out.SetGray(x, y, color.Gray{Y: 255})
				} else {
					out.SetGray(x, y, color.Gray{Y: 0})
				}
			}
		}
	}

	tmpDir, err := os.MkdirTemp("", "hyperparse-image-ocr-*")
	if err != nil {
		return "", nil, err
	}
	tmpPath := filepath.Join(tmpDir, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+"-"+mode+".png")
	writer, err := os.Create(tmpPath)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, err
	}
	defer writer.Close()

	if err := png.Encode(writer, out); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, err
	}
	return tmpPath, func() { _ = os.RemoveAll(tmpDir) }, nil
}

func imageOCRTimeout() time.Duration {
	cfg := localocr.LoadConfig(20 * time.Second)
	return cfg.Timeout
}

func normalizeOCRText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
