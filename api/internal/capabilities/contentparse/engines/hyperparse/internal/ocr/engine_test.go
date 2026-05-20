package ocr

import (
	"reflect"
	"testing"
	"time"
)

func TestLoadConfigPaddleOCR(t *testing.T) {
	t.Setenv("DOCSTILL_OCR_ENGINE", "paddle")
	t.Setenv("DOCSTILL_OCR_LANG", "ch")
	t.Setenv("DOCSTILL_OCR_TIMEOUT_SECONDS", "7")
	t.Setenv("DOCSTILL_PADDLEOCR_CMD", "/tmp/paddle ocr")
	t.Setenv("DOCSTILL_PADDLEOCR_ARGS", `ocr --image_dir "{image}" --lang {lang} --output "{output_dir}"`)

	cfg := LoadConfig(2 * time.Second)
	if cfg.EngineName() != EnginePaddleOCR {
		t.Fatalf("engine = %q", cfg.EngineName())
	}
	if cfg.Lang != "ch" {
		t.Fatalf("lang = %q", cfg.Lang)
	}
	if cfg.Timeout != 7*time.Second {
		t.Fatalf("timeout = %v", cfg.Timeout)
	}
	wantArgs := []string{"ocr", "--image_dir", "{image}", "--lang", "{lang}", "--output", "{output_dir}"}
	if !reflect.DeepEqual(cfg.PaddleArgs, wantArgs) {
		t.Fatalf("args = %#v", cfg.PaddleArgs)
	}
}

func TestLoadConfigTesseractPSM(t *testing.T) {
	t.Setenv("DOCSTILL_OCR_ENGINE", "tesseract")
	t.Setenv("DOCSTILL_TESSERACT_PSM", "11")

	cfg := LoadConfig(2 * time.Second)
	if cfg.TesseractPSM != 11 {
		t.Fatalf("psm = %d", cfg.TesseractPSM)
	}
}

func TestParseTesseractTSV(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t10\t20\t30\t10\t95\tHello\n" +
		"5\t1\t1\t1\t1\t2\t45\t20\t20\t10\t90\tworld\n" +
		"5\t1\t1\t1\t2\t1\t10\t45\t18\t10\t92\tNext\n"
	lines := ParseTesseractTSV(tsv, 100, 100)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %#v", lines)
	}
	if lines[0].Text != "Hello world" || lines[0].Left != 10 || lines[0].Right != 65 {
		t.Fatalf("unexpected first line: %#v", lines[0])
	}
	if lines[1].Text != "Next" {
		t.Fatalf("unexpected second line: %#v", lines[1])
	}
}

func TestParsePaddleJSONOutput(t *testing.T) {
	raw := `[{"rec_text":"Customer service","dt_polys":[[10,20],[90,20],[90,36],[10,36]]}]`
	lines := parsePaddleOutput(raw, 100, 100)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %#v", lines)
	}
	if lines[0].Text != "Customer service" {
		t.Fatalf("text = %q", lines[0].Text)
	}
	if lines[0].Left != 10 || lines[0].Top != 20 || lines[0].Right != 90 || lines[0].Bottom != 36 {
		t.Fatalf("bbox = %#v", lines[0])
	}
}

func TestParsePaddlePlainOutputAssignsBoxes(t *testing.T) {
	raw := "Namespace(use_gpu=false)\n[[[[0,0],[1,0],[1,1],[0,1]], ('First line', 0.98)]]\n('Second line', 0.91)"
	lines := parsePaddleOutput(raw, 200, 100)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %#v", lines)
	}
	if lines[0].Text != "First line" || lines[1].Text != "Second line" {
		t.Fatalf("unexpected text: %#v", lines)
	}
	if lines[0].Right <= lines[0].Left || lines[1].Bottom <= lines[1].Top {
		t.Fatalf("expected synthetic boxes, got %#v", lines)
	}
}

func TestPaddleOCRLangNormalizesSharedOCRLangs(t *testing.T) {
	cases := map[string]string{
		"":            "en",
		"eng":         "en",
		"en_us":       "en",
		"chi_sim":     "ch",
		"chi_sim+eng": "ch",
		"fr":          "fr",
	}
	for input, want := range cases {
		if got := paddleOCRLang(input); got != want {
			t.Fatalf("paddleOCRLang(%q)=%q want %q", input, got, want)
		}
	}
}
