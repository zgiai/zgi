package docx

import "testing"

func TestParseDOCXParagraphs(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Main Title</w:t></w:r></w:p>
    <w:p><w:r><w:t>First paragraph.</w:t></w:r></w:p>
  </w:body>
</w:document>`)
	paras, err := parseDOCXParagraphs(xmlData)
	if err != nil {
		t.Fatalf("parse xml: %v", err)
	}
	if len(paras) != 2 {
		t.Fatalf("paras=%d", len(paras))
	}
	if paras[0].Style != "Heading1" {
		t.Fatalf("style=%q", paras[0].Style)
	}
	if paras[1].Text != "First paragraph." {
		t.Fatalf("text=%q", paras[1].Text)
	}
}
