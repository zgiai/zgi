package ocr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/binutil"
)

const (
	EngineTesseract = "tesseract"
	EnginePaddleOCR = "paddleocr"
)

type Config struct {
	Engine        string
	Lang          string
	TesseractPSM  int
	Timeout       time.Duration
	TesseractPath string
	PaddleCommand string
	PaddleArgs    []string
}

type Result struct {
	Engine string
	Text   string
	Lines  []Line
	Raw    string
}

type Line struct {
	Order         int
	Text          string
	Left, Top     int
	Right, Bottom int
}

func LoadConfig(defaultTimeout time.Duration) Config {
	engine := normalizeEngine(firstEnv("CONTENT_PARSE_OCR_ENGINE", "CONTENT_PARSE_LOCAL_OCR_ENGINE", "DOCSTILL_OCR_ENGINE", "DOCSTILL_LOCAL_OCR_ENGINE"))
	if engine == "" {
		engine = EngineTesseract
	}
	lang := firstEnv("CONTENT_PARSE_OCR_LANG", "CONTENT_PARSE_LOCAL_OCR_LANG", "DOCSTILL_OCR_LANG", "DOCSTILL_LOCAL_OCR_LANG")
	if lang == "" {
		lang = "eng"
	}
	psm := firstEnvInt("CONTENT_PARSE_TESSERACT_PSM", "CONTENT_PARSE_OCR_TESSERACT_PSM", "DOCSTILL_TESSERACT_PSM", "DOCSTILL_OCR_TESSERACT_PSM")
	if psm <= 0 {
		psm = 6
	}
	timeout := defaultTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if n := firstEnvInt("CONTENT_PARSE_OCR_TIMEOUT_SECONDS", "DOCSTILL_OCR_TIMEOUT_SECONDS"); n > 0 {
		if n > 120 {
			n = 120
		}
		timeout = time.Duration(n) * time.Second
	}
	args := splitArgs(firstEnv("CONTENT_PARSE_PADDLEOCR_ARGS", "DOCSTILL_PADDLEOCR_ARGS"))
	if len(args) == 0 {
		args = []string{"--image_dir", "{image}", "--lang", "{lang}"}
	}
	cmd := firstEnv("CONTENT_PARSE_PADDLEOCR_CMD", "DOCSTILL_PADDLEOCR_CMD")
	if cmd == "" {
		cmd = "paddleocr"
	}
	return Config{
		Engine:        engine,
		Lang:          lang,
		TesseractPSM:  psm,
		Timeout:       timeout,
		TesseractPath: firstEnv("CONTENT_PARSE_TESSERACT_PATH", "DOCSTILL_TESSERACT_PATH"),
		PaddleCommand: cmd,
		PaddleArgs:    args,
	}
}

func (c Config) EngineName() string {
	engine := normalizeEngine(c.Engine)
	if engine == "" {
		return EngineTesseract
	}
	return engine
}

func (c Config) RunText(ctx context.Context, pngBytes []byte) (Result, error) {
	if len(pngBytes) == 0 {
		return Result{Engine: c.EngineName()}, fmt.Errorf("empty crop")
	}
	tmpDir, err := os.MkdirTemp("", "hyperparse-ocr-*")
	if err != nil {
		return Result{Engine: c.EngineName()}, err
	}
	defer os.RemoveAll(tmpDir)
	imgPath := filepath.Join(tmpDir, "input.png")
	if err := os.WriteFile(imgPath, pngBytes, 0600); err != nil {
		return Result{Engine: c.EngineName()}, err
	}
	return c.RunTextFile(ctx, imgPath)
}

func (c Config) RunTextFile(ctx context.Context, imagePath string) (Result, error) {
	switch c.EngineName() {
	case EnginePaddleOCR:
		return c.runPaddle(ctx, imagePath, 0, 0)
	case EngineTesseract:
		return c.runTesseract(ctx, imagePath, false, 0, 0)
	default:
		return Result{Engine: c.EngineName()}, fmt.Errorf("unsupported OCR engine %q", c.EngineName())
	}
}

func (c Config) RunLinesFile(ctx context.Context, imagePath string, cropW, cropH int) (Result, error) {
	switch c.EngineName() {
	case EnginePaddleOCR:
		return c.runPaddle(ctx, imagePath, cropW, cropH)
	case EngineTesseract:
		return c.runTesseract(ctx, imagePath, true, cropW, cropH)
	default:
		return Result{Engine: c.EngineName()}, fmt.Errorf("unsupported OCR engine %q", c.EngineName())
	}
}

func (c Config) runTesseract(ctx context.Context, imagePath string, tsv bool, cropW, cropH int) (Result, error) {
	tesseract, err := c.resolveTesseract()
	if err != nil {
		return Result{Engine: EngineTesseract}, err
	}
	ocrCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	psm := c.TesseractPSM
	if psm <= 0 {
		psm = 6
	}
	args := []string{imagePath, "stdout", "-l", c.Lang, "--psm", strconv.Itoa(psm)}
	if tsv {
		args = append(args, "tsv")
	}
	cmd := exec.CommandContext(ocrCtx, tesseract, args...)
	raw, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(raw))
	if err != nil {
		return Result{Engine: EngineTesseract, Raw: out}, fmt.Errorf("tesseract: %v (%s)", err, out)
	}
	res := Result{Engine: EngineTesseract, Raw: out}
	if tsv {
		res.Lines = ParseTesseractTSV(out, cropW, cropH)
		res.Text = linesText(res.Lines)
	} else {
		res.Text = out
	}
	return res, nil
}

func (c Config) runPaddle(ctx context.Context, imagePath string, cropW, cropH int) (Result, error) {
	paddle, err := c.resolvePaddle()
	if err != nil {
		return Result{Engine: EnginePaddleOCR}, err
	}
	tmpDir, err := os.MkdirTemp("", "hyperparse-paddleocr-*")
	if err != nil {
		return Result{Engine: EnginePaddleOCR}, err
	}
	defer os.RemoveAll(tmpDir)
	args := expandArgs(c.PaddleArgs, map[string]string{
		"image":      imagePath,
		"lang":       paddleOCRLang(c.Lang),
		"output_dir": tmpDir,
	})
	ocrCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ocrCtx, paddle, args...)
	raw, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(raw))
	if dirText := readOutputDirText(tmpDir); dirText != "" {
		if out != "" {
			out += "\n"
		}
		out += dirText
	}
	if err != nil {
		return Result{Engine: EnginePaddleOCR, Raw: out}, fmt.Errorf("paddleocr: %v (%s)", err, truncate(out, 500))
	}
	lines := parsePaddleOutput(out, cropW, cropH)
	return Result{Engine: EnginePaddleOCR, Text: linesText(lines), Lines: lines, Raw: out}, nil
}

func (c Config) resolveTesseract() (string, error) {
	return binutil.Resolve("tesseract", c.TesseractPath)
}

func (c Config) resolvePaddle() (string, error) {
	if c.PaddleCommand == "" {
		return "", fmt.Errorf("empty paddleocr command")
	}
	return ResolveCommand(c.PaddleCommand)
}

func ResolveCommand(command string) (string, error) {
	return binutil.ResolveCommand(command)
}

type tsvWord struct {
	key           string
	text          string
	left, top     int
	right, bottom int
}

type tsvLineGroup struct {
	key           string
	words         []tsvWord
	left, top     int
	right, bottom int
}

func ParseTesseractTSV(tsv string, cropW, cropH int) []Line {
	rows := strings.Split(tsv, "\n")
	if len(rows) < 2 || cropW <= 0 || cropH <= 0 {
		return nil
	}
	header := strings.Split(rows[0], "\t")
	col := map[string]int{}
	for i, name := range header {
		col[strings.TrimSpace(name)] = i
	}
	required := []string{"level", "block_num", "par_num", "line_num", "left", "top", "width", "height", "text"}
	for _, name := range required {
		if _, ok := col[name]; !ok {
			return nil
		}
	}
	groups := make(map[string]*tsvLineGroup)
	order := make([]string, 0)
	for _, row := range rows[1:] {
		fields := strings.Split(row, "\t")
		if len(fields) < len(header) || fields[col["level"]] != "5" {
			continue
		}
		text := strings.TrimSpace(fields[col["text"]])
		if text == "" {
			continue
		}
		left := atoi(fields[col["left"]])
		top := atoi(fields[col["top"]])
		width := atoi(fields[col["width"]])
		height := atoi(fields[col["height"]])
		if width <= 0 || height <= 0 {
			continue
		}
		key := fields[col["block_num"]] + ":" + fields[col["par_num"]] + ":" + fields[col["line_num"]]
		word := tsvWord{key: key, text: text, left: left, top: top, right: left + width, bottom: top + height}
		group := groups[key]
		if group == nil {
			group = &tsvLineGroup{key: key, left: word.left, top: word.top, right: word.right, bottom: word.bottom}
			groups[key] = group
			order = append(order, key)
		}
		group.words = append(group.words, word)
		group.left = min(group.left, word.left)
		group.top = min(group.top, word.top)
		group.right = max(group.right, word.right)
		group.bottom = max(group.bottom, word.bottom)
	}
	out := make([]Line, 0, len(order))
	for i, key := range order {
		group := groups[key]
		if group == nil || len(group.words) == 0 {
			continue
		}
		texts := make([]string, 0, len(group.words))
		for _, word := range group.words {
			texts = append(texts, word.text)
		}
		out = append(out, Line{
			Order:  i,
			Text:   strings.Join(texts, " "),
			Left:   group.left,
			Top:    group.top,
			Right:  group.right,
			Bottom: group.bottom,
		})
	}
	return out
}

var paddlePythonTupleText = regexp.MustCompile(`\(['"]([^'"]{1,500})['"]\s*,\s*[0-9.]+\)`)

func parsePaddleOutput(raw string, cropW, cropH int) []Line {
	var out []Line
	for _, candidate := range jsonCandidates(raw) {
		var v any
		if json.Unmarshal([]byte(candidate), &v) == nil {
			collectPaddleLines(v, &out)
		}
	}
	if len(out) == 0 {
		for _, m := range paddlePythonTupleText.FindAllStringSubmatch(raw, -1) {
			if len(m) > 1 {
				out = append(out, Line{Text: cleanOCRText(m[1])})
			}
		}
	}
	if len(out) == 0 {
		for _, line := range strings.Split(raw, "\n") {
			text := cleanOCRText(line)
			if text == "" || looksLikeLogLine(text) {
				continue
			}
			out = append(out, Line{Text: text})
		}
	}
	return normalizeLines(out, cropW, cropH)
}

func collectPaddleLines(v any, out *[]Line) {
	if line, ok := paddleLineFromValue(v); ok {
		*out = append(*out, line)
		return
	}
	switch x := v.(type) {
	case map[string]any:
		for _, child := range x {
			collectPaddleLines(child, out)
		}
	case []any:
		for _, child := range x {
			collectPaddleLines(child, out)
		}
	}
}

func paddleLineFromValue(v any) (Line, bool) {
	switch x := v.(type) {
	case map[string]any:
		text := firstMapString(x, "text", "rec_text", "transcription", "label")
		box, hasBox := firstMapBox(x, "bbox", "box", "points", "dt_polys", "text_region")
		if text != "" {
			line := Line{Text: text}
			if hasBox {
				line.Left, line.Top, line.Right, line.Bottom = box[0], box[1], box[2], box[3]
			}
			return line, true
		}
	case []any:
		text := ""
		for _, child := range x {
			if text == "" {
				text = textFromValue(child)
			}
		}
		if text == "" {
			return Line{}, false
		}
		if box, ok := boxFromValue(x); ok {
			return Line{Text: text, Left: box[0], Top: box[1], Right: box[2], Bottom: box[3]}, true
		}
		return Line{Text: text}, true
	}
	return Line{}, false
}

func jsonCandidates(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") {
		add(raw)
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[") {
			add(line)
		}
	}
	return out
}

func firstMapString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s := textFromValue(m[key]); s != "" {
			return s
		}
	}
	return ""
}

func firstMapBox(m map[string]any, keys ...string) ([4]int, bool) {
	for _, key := range keys {
		if box, ok := boxFromValue(m[key]); ok {
			return box, true
		}
	}
	return [4]int{}, false
}

func textFromValue(v any) string {
	switch x := v.(type) {
	case string:
		return cleanOCRText(x)
	case []any:
		for _, child := range x {
			if s := textFromValue(child); s != "" {
				return s
			}
		}
	}
	return ""
}

func boxFromValue(v any) ([4]int, bool) {
	switch x := v.(type) {
	case []any:
		if len(x) == 4 {
			if a, ok := numberFromAny(x[0]); ok {
				if b, ok := numberFromAny(x[1]); ok {
					if c, ok := numberFromAny(x[2]); ok {
						if d, ok := numberFromAny(x[3]); ok {
							return [4]int{int(a), int(b), int(c), int(d)}, true
						}
					}
				}
			}
		}
		minX, minY := 1<<30, 1<<30
		maxX, maxY := -1, -1
		points := 0
		for _, p := range x {
			pp, ok := p.([]any)
			if !ok || len(pp) < 2 {
				continue
			}
			px, okX := numberFromAny(pp[0])
			py, okY := numberFromAny(pp[1])
			if !okX || !okY {
				continue
			}
			ix, iy := int(px), int(py)
			minX, minY = min(minX, ix), min(minY, iy)
			maxX, maxY = max(maxX, ix), max(maxY, iy)
			points++
		}
		if points >= 2 && maxX > minX && maxY > minY {
			return [4]int{minX, minY, maxX, maxY}, true
		}
	}
	return [4]int{}, false
}

func numberFromAny(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

func normalizeLines(lines []Line, cropW, cropH int) []Line {
	out := make([]Line, 0, len(lines))
	for _, line := range lines {
		line.Text = cleanOCRText(line.Text)
		if line.Text == "" {
			continue
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return nil
	}
	for i := range out {
		if !validBox(out[i]) && cropW > 0 && cropH > 0 {
			rowH := max(8, cropH/max(4, len(out)*3))
			centerY := int(float64(cropH) * float64(i+1) / float64(len(out)+1))
			out[i].Left = 0
			out[i].Right = cropW
			out[i].Top = max(0, centerY-rowH/2)
			out[i].Bottom = min(cropH, centerY+rowH/2)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Top != out[j].Top {
			return out[i].Top < out[j].Top
		}
		return out[i].Left < out[j].Left
	})
	for i := range out {
		out[i].Order = i
	}
	return out
}

func validBox(line Line) bool {
	return line.Right > line.Left && line.Bottom > line.Top
}

func linesText(lines []Line) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		if text := cleanOCRText(line.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func cleanOCRText(s string) string {
	s = strings.TrimSpace(strings.Trim(s, "\ufeff"))
	s = strings.Trim(s, `"'`)
	return strings.TrimSpace(s)
}

func looksLikeLogLine(s string) bool {
	lower := strings.ToLower(s)
	return strings.HasPrefix(lower, "download ") ||
		strings.Contains(lower, "namespace(") ||
		strings.Contains(lower, "paddleocr") ||
		strings.Contains(lower, "running command") ||
		strings.Contains(lower, "could not find") ||
		strings.Contains(lower, "warning:")
}

func readOutputDirText(dir string) string {
	var parts []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" && ext != ".txt" && ext != ".tsv" {
			return nil
		}
		if b, err := os.ReadFile(path); err == nil {
			if text := strings.TrimSpace(string(b)); text != "" {
				parts = append(parts, text)
			}
		}
		return nil
	})
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

func expandArgs(args []string, values map[string]string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		for key, value := range values {
			arg = strings.ReplaceAll(arg, "{"+key+"}", value)
		}
		out = append(out, arg)
	}
	return out
}

func splitArgs(s string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	escaped := false
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

func normalizeEngine(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto":
		return ""
	case "paddle", "paddleocr", "paddle_ocr", "ppocr":
		return EnginePaddleOCR
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func paddleOCRLang(lang string) string {
	raw := strings.ToLower(strings.TrimSpace(lang))
	switch raw {
	case "", "eng", "en-us", "en_us", "english":
		return "en"
	case "chi_sim", "chi-sim", "chi_sim+eng", "chi_sim+en", "zh", "zh-cn", "zh_cn", "ch_sim", "chs":
		return "ch"
	default:
		return strings.TrimSpace(lang)
	}
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func firstEnvInt(keys ...string) int {
	for _, key := range keys {
		if n := envInt(key); n > 0 {
			return n
		}
	}
	return 0
}

func envInt(key string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	return n
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
