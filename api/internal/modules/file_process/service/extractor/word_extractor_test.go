package extractor

import (
	"archive/zip"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

const testDocxDocumentOpen = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
	`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" ` +
	`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
	`xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" ` +
	`xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape" ` +
	`xmlns:v="urn:schemas-microsoft-com:vml"><w:body>`

const testDocxDocumentClose = `<w:sectPr><w:pgSz w:w="11906" w:h="16838"/></w:sectPr></w:body></w:document>`

const testDocxPackageRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="%s"/>` +
	`</Relationships>`

func writeTestDocx(t *testing.T, parts map[string]string) string {
	t.Helper()

	filePath := filepath.Join(t.TempDir(), "document.docx")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	writer := zip.NewWriter(file)
	for name, content := range parts {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("zip Create(%s) error = %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("zip Write(%s) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file Close() error = %v", err)
	}
	return filePath
}

func writeTestDocxBody(t *testing.T, body string) string {
	t.Helper()
	return writeTestDocx(t, map[string]string{
		"_rels/.rels":       strings.Replace(testDocxPackageRels, "%s", "word/document.xml", 1),
		"word/document.xml": testDocxDocumentOpen + body + testDocxDocumentClose,
	})
}

func extractTestDocx(t *testing.T, filePath string) string {
	t.Helper()

	output, err := NewWordExtractor(filePath, "tenant-1", "user-1").Extract(context.Background())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(output.Elements) != 1 {
		t.Fatalf("Extract() returned %d elements, want 1", len(output.Elements))
	}
	return dto.ExtractOutputText(output)
}

func TestWordExtractorExtractsParagraphRuns(t *testing.T) {
	filePath := writeTestDocxBody(t,
		`<w:p><w:r><w:t>Hello </w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>World</w:t></w:r></w:p>`+
			`<w:p></w:p>`+
			`<w:p><w:r><w:t>Name</w:t><w:tab/><w:t>Café &amp; Co</w:t></w:r></w:p>`+
			`<w:p><w:r><w:t>Line one</w:t><w:br/><w:t>Line two</w:t></w:r></w:p>`+
			`<w:p><w:hyperlink r:id="rId9"><w:r><w:t>linked text</w:t></w:r></w:hyperlink></w:p>`+
			`<w:p><w:r><w:t>kept</w:t></w:r><w:del w:id="1" w:author="a"><w:r><w:delText>removed</w:delText></w:r></w:del>`+
			`<w:ins w:id="2" w:author="a"><w:r><w:t> inserted</w:t></w:r></w:ins></w:p>`+
			`<w:p><w:r><w:br w:type="page"/></w:r></w:p>`+
			`<w:p><w:r><w:t>After break</w:t></w:r></w:p>`,
	)

	got := extractTestDocx(t, filePath)
	want := strings.Join([]string{
		"Hello World",
		"Name\tCafé & Co",
		"Line one\nLine two",
		"linked text",
		"kept inserted",
		"After break",
	}, "\n")
	if got != want {
		t.Fatalf("Extract() text = %q, want %q", got, want)
	}
}

func TestWordExtractorExtractsHeadings(t *testing.T) {
	filePath := writeTestDocxBody(t,
		`<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Chapter 1</w:t></w:r></w:p>`+
			`<w:p><w:r><w:t>Intro text.</w:t></w:r></w:p>`+
			`<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Section 1.1</w:t></w:r></w:p>`,
	)

	got := extractTestDocx(t, filePath)
	if want := "Chapter 1\nIntro text.\nSection 1.1"; got != want {
		t.Fatalf("Extract() text = %q, want %q", got, want)
	}
}

func TestWordExtractorAppendsTableRowsAfterParagraphs(t *testing.T) {
	filePath := writeTestDocxBody(t,
		`<w:p><w:r><w:t>Before table</w:t></w:r></w:p>`+
			`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/></w:tblPr><w:tblGrid><w:gridCol/><w:gridCol/></w:tblGrid>`+
			`<w:tr><w:tc><w:p><w:r><w:t>Item</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Amount</w:t></w:r></w:p></w:tc></w:tr>`+
			`<w:tr><w:tc><w:p><w:r><w:t>Electricity</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>113.47</w:t></w:r></w:p></w:tc></w:tr>`+
			`<w:tr><w:tc><w:p/></w:tc><w:tc><w:p/></w:tc></w:tr>`+
			`<w:tr><w:tc><w:p/></w:tc><w:tc><w:p><w:r><w:t>only second</w:t></w:r></w:p></w:tc></w:tr>`+
			`</w:tbl>`+
			`<w:p><w:r><w:t>After table</w:t></w:r></w:p>`,
	)

	got := extractTestDocx(t, filePath)
	want := strings.Join([]string{
		"Before table",
		"After table",
		"Item\tAmount",
		"Electricity\t113.47",
		"only second",
	}, "\n")
	if got != want {
		t.Fatalf("Extract() text = %q, want %q", got, want)
	}
}

func TestWordExtractorKeepsTextAroundImagesWithoutDuplicates(t *testing.T) {
	inlineImage := `<w:r><w:drawing><wp:inline><wp:extent cx="100" cy="100"/><wp:docPr id="1" name="Picture 1" descr="alt text"/>` +
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"></a:graphicData></a:graphic>` +
		`</wp:inline></w:drawing></w:r>`
	textBox := `<w:r><mc:AlternateContent><mc:Choice Requires="wps"><w:drawing><wp:anchor>` +
		`<a:graphic><a:graphicData uri="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"><wps:wsp><wps:txbx><w:txbxContent>` +
		`<w:p><w:r><w:t>Box text</w:t></w:r></w:p>` +
		`</w:txbxContent></wps:txbx></wps:wsp></a:graphicData></a:graphic></wp:anchor></w:drawing></mc:Choice>` +
		`<mc:Fallback><w:pict><v:shape><v:textbox><w:txbxContent><w:p><w:r><w:t>Box text</w:t></w:r></w:p></w:txbxContent></v:textbox></v:shape></w:pict></mc:Fallback>` +
		`</mc:AlternateContent></w:r>`
	filePath := writeTestDocxBody(t,
		`<w:p><w:r><w:t>Before image </w:t></w:r>`+inlineImage+`<w:r><w:t>after image</w:t></w:r></w:p>`+
			`<w:p>`+inlineImage+`</w:p>`+
			`<w:p>`+textBox+`</w:p>`,
	)

	got := extractTestDocx(t, filePath)
	if want := "Before image after image\nBox text"; got != want {
		t.Fatalf("Extract() text = %q, want %q", got, want)
	}
}

func TestWordExtractorResolvesDocumentPartFromRelationships(t *testing.T) {
	filePath := writeTestDocx(t, map[string]string{
		"_rels/.rels":         strings.Replace(testDocxPackageRels, "%s", "/custom/main.xml", 1),
		"custom/main.xml":     testDocxDocumentOpen + `<w:sdt><w:sdtContent><w:p><w:r><w:t>From custom part</w:t></w:r></w:p></w:sdtContent></w:sdt>` + testDocxDocumentClose,
		"word/document.xml":   testDocxDocumentOpen + `<w:p><w:r><w:t>Wrong part</w:t></w:r></w:p>` + testDocxDocumentClose,
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`,
	})

	if got, want := extractTestDocx(t, filePath), "From custom part"; got != want {
		t.Fatalf("Extract() text = %q, want %q", got, want)
	}
}

func TestWordExtractorRejectsInvalidFiles(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		parts   map[string]string
		want    string
	}{
		{
			name:    "legacy doc",
			content: []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1},
			want:    "old Word 97-2003 (.doc) format",
		},
		{
			name:    "not a zip",
			content: []byte("plain text, not a docx"),
			want:    "failed to open docx package",
		},
		{
			name:  "missing document part",
			parts: map[string]string{"[Content_Types].xml": `<Types/>`},
			want:  "missing main document part word/document.xml",
		},
		{
			name: "malformed document xml",
			parts: map[string]string{
				"word/document.xml": testDocxDocumentOpen + `<w:p><w:r><w:t>unterminated`,
			},
			want: "failed to parse docx document part",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filePath string
			if tt.parts != nil {
				filePath = writeTestDocx(t, tt.parts)
			} else {
				filePath = filepath.Join(t.TempDir(), "input.docx")
				if err := os.WriteFile(filePath, tt.content, 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			}

			_, err := NewWordExtractor(filePath, "tenant-1", "user-1").Extract(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Extract() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestWordExtractorHonorsContextCancellation(t *testing.T) {
	filePath := writeTestDocxBody(t, `<w:p><w:r><w:t>text</w:t></w:r></w:p>`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewWordExtractor(filePath, "tenant-1", "user-1").Extract(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Extract() error = %v, want context.Canceled", err)
	}
}
