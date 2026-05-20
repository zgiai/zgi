package image

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	localocr "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/ocr"
)

func TestNormalizeOCRText(t *testing.T) {
	got := normalizeOCRText("  line1 \r\n\r\n line2  \n")
	if got != "line1\nline2" {
		t.Fatalf("got=%q", got)
	}
}

func TestDefaultImageOCRLang(t *testing.T) {
	if got := defaultImageOCRLang("invoice-scan.jpg"); got != "eng" {
		t.Fatalf("ascii filename lang=%q", got)
	}
	if got := defaultImageOCRLang("合同扫描件.jpg"); got != "chi_sim+eng" {
		t.Fatalf("cjk filename lang=%q", got)
	}
}

func TestLooksLikePoorOCR(t *testing.T) {
	if !looksLikePoorOCR("AL Sn. « Win emeeae me tiie fhe ER oes a SE") {
		t.Fatal("expected garbled OCR text to be detected")
	}
	if looksLikePoorOCR("Quarterly revenue increased by 18 percent in 2025.") {
		t.Fatal("expected clean OCR text not to be detected as poor")
	}
}

func TestScoreOCRTextPrefersReadableOutput(t *testing.T) {
	garbled := "AL Sn. « Win emeeae me tiie fhe ER oes a SE"
	clean := "Quarterly revenue increased by 18 percent in 2025."
	if scoreOCRText(clean) <= scoreOCRText(garbled) {
		t.Fatalf("expected clean OCR score to be higher: clean=%d garbled=%d", scoreOCRText(clean), scoreOCRText(garbled))
	}
}

func TestShouldRetryImageOCRForLargeSparseImage(t *testing.T) {
	result := localocr.Result{
		Text:  "Form",
		Lines: []localocr.Line{{Text: "Form"}},
	}
	if !shouldRetryImageOCR(result, 1800, 1600, ParseOptions{}) {
		t.Fatal("expected retry for large sparse image OCR result")
	}
}

func TestShouldRetryImageOCRForShortNoisyScanText(t *testing.T) {
	result := localocr.Result{
		Text: "A\nB\nC\nDate:\nRon Milstein\n@\n#\n$\nFax\n1\n2\n3",
		Lines: []localocr.Line{
			{Text: "A"}, {Text: "B"}, {Text: "C"}, {Text: "Date:"},
			{Text: "Ron Milstein"}, {Text: "@"}, {Text: "#"}, {Text: "$"},
			{Text: "Fax"}, {Text: "1"}, {Text: "2"}, {Text: "3"},
		},
	}
	if !shouldRetryImageOCR(result, 1500, 1200, ParseOptions{}) {
		t.Fatal("expected retry for short noisy scan-like OCR text")
	}
}

func TestLooksLikeStructuredForm(t *testing.T) {
	text := "CONFIDENTIAL FACSIMILE\nTRANSMISSION COVER SHEET\nFAX NUMBER: (336) 335-7392\nPHONE NUMBER: (336) 335-7363\nDATE: 12/10/98\nCOMPANY: Lorillard"
	if !looksLikeStructuredForm(text) {
		t.Fatal("expected fax/form-like text to be detected")
	}
}

func TestCandidateLabel(t *testing.T) {
	if got := candidateLabel(localocr.Config{Engine: localocr.EnginePaddleOCR}); got != "paddleocr" {
		t.Fatalf("got=%q", got)
	}
	if got := candidateLabel(localocr.Config{Engine: localocr.EngineTesseract, TesseractPSM: 11}); got != "tesseract:psm11" {
		t.Fatalf("got=%q", got)
	}
}

func TestScoreOCRResultRewardsLineStructure(t *testing.T) {
	flat := localocr.Result{Text: "RevenueGrowth2025", Lines: nil}
	structured := localocr.Result{
		Text: "Revenue\nGrowth\n2025",
		Lines: []localocr.Line{
			{Text: "Revenue"},
			{Text: "Growth"},
			{Text: "2025"},
		},
	}
	if scoreOCRResult(structured) <= scoreOCRResult(flat) {
		t.Fatalf("expected structured OCR result to score higher: structured=%d flat=%d", scoreOCRResult(structured), scoreOCRResult(flat))
	}
}

func TestBuildBlocksFromOCRPreservesLineBBox(t *testing.T) {
	blocks := buildBlocksFromOCR(localocr.Result{
		Engine: "tesseract",
		Lines: []localocr.Line{
			{Text: "Customer service", Left: 20, Top: 10, Right: 120, Bottom: 30},
		},
	}, 200, 100)
	if len(blocks) != 1 {
		t.Fatalf("blocks=%d want 1", len(blocks))
	}
	if blocks[0].BBox == nil {
		t.Fatal("expected bbox")
	}
	if blocks[0].BBox.Left != 0.1 || blocks[0].BBox.Top != 0.1 || blocks[0].BBox.Right != 0.6 || blocks[0].BBox.Bottom != 0.3 {
		t.Fatalf("bbox=%+v", blocks[0].BBox)
	}
	if blocks[0].Precision != "reliable" {
		t.Fatalf("precision=%q want reliable", blocks[0].Precision)
	}
	if blocks[0].Payload["bbox_source"] != "ocr_line" || blocks[0].Payload["ocr_engine"] != "tesseract" {
		t.Fatalf("payload=%v", blocks[0].Payload)
	}
}

func TestBuildBlocksFromOCRSkipsInvalidBBox(t *testing.T) {
	blocks := buildBlocksFromOCR(localocr.Result{
		Engine: "tesseract",
		Lines:  []localocr.Line{{Text: "No geometry"}},
	}, 200, 100)
	if len(blocks) != 1 {
		t.Fatalf("blocks=%d want 1", len(blocks))
	}
	if blocks[0].BBox != nil || blocks[0].Precision != "" || len(blocks[0].Payload) > 0 {
		t.Fatalf("unexpected geometry metadata: %+v", blocks[0])
	}
}

func TestBuildPreprocessedTesseractCandidates(t *testing.T) {
	path := buildTinyImageFixture(t)
	primary := localocr.Config{Engine: localocr.EngineTesseract, TesseractPSM: 6}
	candidates := buildPreprocessedTesseractCandidates(path, primary)
	defer cleanupOCRCandidates(candidates)
	if len(candidates) == 0 {
		t.Fatal("expected preprocessed candidates")
	}
	foundBinary := false
	for _, candidate := range candidates {
		if candidate.Preprocess == "binary" {
			foundBinary = true
		}
		if candidate.ImagePath == "" {
			t.Fatalf("candidate missing image path: %+v", candidate)
		}
		if _, err := os.Stat(candidate.ImagePath); err != nil {
			t.Fatalf("preprocessed image not found: %v", err)
		}
	}
	if !foundBinary {
		t.Fatal("expected binary preprocessed candidate")
	}
}

func buildTinyImageFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.png")
	img := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			if x < 16 {
				img.Set(x, y, color.White)
			} else {
				img.Set(x, y, color.Black)
			}
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return path
}
