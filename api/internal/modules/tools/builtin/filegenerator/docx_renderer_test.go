package filegenerator

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service/extractor"
)

func renderTestRichDocx(t *testing.T, raw string) []byte {
	t.Helper()

	spec, err := parseDocxDocumentSpec(raw)
	require.NoError(t, err)
	data, err := renderRichDocx(spec)
	require.NoError(t, err)
	requireDocxPartsWellFormed(t, data)
	return data
}

func requireDocxPartsWellFormed(t *testing.T, data []byte) {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	for _, file := range reader.File {
		handle, err := file.Open()
		require.NoError(t, err)
		decoder := xml.NewDecoder(handle)
		for {
			_, err := decoder.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoErrorf(t, err, "part %s is not well-formed XML", file.Name)
		}
		require.NoError(t, handle.Close())
	}
}

func extractDocxText(t *testing.T, data []byte) string {
	t.Helper()

	filePath := filepath.Join(t.TempDir(), "rendered.docx")
	require.NoError(t, os.WriteFile(filePath, data, 0o600))
	output, err := extractor.NewWordExtractor(filePath, "tenant-1", "user-1").Extract(context.Background())
	require.NoError(t, err)
	return dto.ExtractOutputText(output)
}

func TestRenderDocxPlainTextPreservesLinesAndWhitespace(t *testing.T) {
	data, err := renderDocx("  Indented line\nTabbed\tvalue\n\n<tag> & \"quotes\"")
	require.NoError(t, err)
	requireDocxPartsWellFormed(t, data)

	requireZipEntryContains(t, data, "word/document.xml", `<w:t xml:space="preserve">  Indented line</w:t>`)
	requireZipEntryContains(t, data, "word/document.xml", `<w:t xml:space="preserve">Tabbed</w:t><w:tab/><w:t xml:space="preserve">value</w:t>`)
	requireZipEntryContains(t, data, "word/document.xml", `&lt;tag&gt; &amp; &#34;quotes&#34;`)
	requireZipEntryContains(t, data, "word/document.xml", `<w:pgSz w:w="11906" w:h="16838"/>`)

	// ExtractOutputText trims the outer whitespace of the whole document.
	require.Equal(t, "Indented line\nTabbed\tvalue\n<tag> & \"quotes\"", extractDocxText(t, data))
}

func TestRenderRichDocxRendersHeadingLevels(t *testing.T) {
	data := renderTestRichDocx(t, `{"blocks":[
  {"type":"heading","level":1,"text":"Title"},
  {"type":"heading","level":2,"text":"Section"},
  {"type":"heading","level":3,"text":"Subsection","style":{"bold":false,"alignment":"right"}},
  {"type":"paragraph","text":"Body"}
]}`)

	requireZipEntryContains(t, data, "word/document.xml",
		`<w:p><w:pPr><w:pStyle w:val="Heading1"/><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:rFonts w:ascii="Arial" w:eastAsia="Arial" w:hAnsi="Arial" w:hint="eastAsia"/><w:b/><w:bCs/><w:sz w:val="36"/><w:szCs w:val="36"/></w:rPr><w:t xml:space="preserve">Title</w:t></w:r></w:p>`)
	requireZipEntryContains(t, data, "word/document.xml",
		`<w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:rPr><w:rFonts w:ascii="Arial" w:eastAsia="Arial" w:hAnsi="Arial" w:hint="eastAsia"/><w:b/><w:bCs/><w:sz w:val="32"/>`)
	requireZipEntryContains(t, data, "word/document.xml",
		`<w:pPr><w:pStyle w:val="Heading3"/><w:jc w:val="end"/></w:pPr><w:r><w:rPr><w:rFonts w:ascii="Arial" w:eastAsia="Arial" w:hAnsi="Arial" w:hint="eastAsia"/><w:sz w:val="28"/>`)
	for _, styleID := range []string{"Heading1", "Heading2", "Heading3"} {
		requireZipEntryContains(t, data, "word/styles.xml", `w:styleId="`+styleID+`"`)
	}

	require.Equal(t, "Title\nSection\nSubsection\nBody", extractDocxText(t, data))
}

func TestRenderRichDocxRendersParagraphFormatting(t *testing.T) {
	data := renderTestRichDocx(t, `{
  "default_style": {"font_family":"Times New Roman","east_asia_font":"SimSun"},
  "blocks": [
    {"type":"paragraph","style":{"space_before":6,"space_after":12,"line_spacing":1.5,"indent_left":18,"first_line_indent":24,"alignment":"justify"},"runs":[
      {"text":"Plain "},
      {"text":"styled","italic":true,"underline":"double","highlight":"darkBlue","color":"#1f4e79","font_size":10.5},
      {"text":" A&B <\"Co\">","font_family":"Font \"Quoted\""}
    ]}
  ]
}`)

	requireZipEntryContains(t, data, "word/document.xml",
		`<w:pPr><w:spacing w:before="120" w:after="240" w:line="360" w:lineRule="auto"/><w:ind w:left="360" w:firstLine="480"/><w:jc w:val="both"/></w:pPr>`)
	requireZipEntryContains(t, data, "word/document.xml",
		`<w:rFonts w:ascii="Times New Roman" w:eastAsia="SimSun" w:hAnsi="Times New Roman" w:hint="eastAsia"/><w:i/><w:iCs/><w:color w:val="1F4E79"/><w:sz w:val="21"/><w:szCs w:val="21"/><w:highlight w:val="darkBlue"/><w:u w:val="double"/>`)
	requireZipEntryContains(t, data, "word/document.xml", `w:ascii="Font &#34;Quoted&#34;"`)
	requireZipEntryContains(t, data, "word/document.xml", `<w:t xml:space="preserve"> A&amp;B &lt;&#34;Co&#34;&gt;</w:t>`)

	require.Equal(t, `Plain styled A&B <"Co">`, extractDocxText(t, data))
}

func TestRenderRichDocxRendersTables(t *testing.T) {
	data := renderTestRichDocx(t, `{"blocks":[
  {"type":"paragraph","text":"Intro"},
  {"type":"table","headers":["Item","Amount"],"column_widths":[100,50],"rows":[
    ["Electricity",{"text":"113.47","background_color":"fff2cc","vertical_align":"bottom"}],
    [{"text":"Total","col_span":2},""]
  ],"table_style":{"width":150,"alignment":"center","border_color":"#336699"}},
  {"type":"table","border":false,"rows":[["a","b","c"]]}
]}`)

	document := docxDocumentXML(t, data)
	require.Equal(t, 2, strings.Count(document, "<w:tbl>"))
	require.Contains(t, document, `<w:tblW w:w="3000" w:type="dxa"/><w:jc w:val="center"/>`)
	require.Contains(t, document, `<w:top w:val="single" w:sz="4" w:space="0" w:color="336699"/>`)
	require.Contains(t, document, `<w:tblGrid><w:gridCol w:w="2000"/><w:gridCol w:w="1000"/></w:tblGrid>`)
	require.Contains(t, document, `<w:tr><w:trPr><w:tblHeader/></w:trPr>`)
	require.Contains(t, document, `<w:tcW w:w="1000" w:type="dxa"/><w:shd w:val="clear" w:color="auto" w:fill="FFF2CC"/><w:vAlign w:val="bottom"/>`)
	require.Contains(t, document, `<w:tcW w:w="3000" w:type="dxa"/><w:gridSpan w:val="2"/>`)

	// Without explicit widths the A4 portrait text area (11906 - 2*1440 twips) is split evenly.
	require.Contains(t, document, `<w:tblW w:w="0" w:type="auto"/><w:tblBorders><w:top w:val="nil"/>`)
	require.Contains(t, document, `<w:gridCol w:w="3008"/><w:gridCol w:w="3008"/><w:gridCol w:w="3008"/>`)
	require.Equal(t, 1, strings.Count(document, "<w:tblHeader/>"))

	require.Equal(t, "Intro\nItem\tAmount\nElectricity\t113.47\nTotal\na\tb\tc", extractDocxText(t, data))
}

func TestRenderRichDocxRendersPageBreaksAndSection(t *testing.T) {
	data := renderTestRichDocx(t, `{
  "page": {"orientation":"landscape","margins":{"top":36,"left":54}},
  "blocks": [
    {"type":"paragraph","text":"Page one"},
    {"type":"page_break"},
    {"type":"paragraph","text":"Line A\nLine B"}
  ]
}`)

	document := docxDocumentXML(t, data)
	require.Contains(t, document, `<w:p><w:r><w:br w:type="page"/></w:r></w:p>`)
	require.Contains(t, document, `<w:t xml:space="preserve">Line A</w:t><w:br/><w:t xml:space="preserve">Line B</w:t>`)
	require.Contains(t, document, `<w:pgSz w:w="16838" w:h="11906" w:orient="landscape"/>`)
	require.Contains(t, document, `<w:pgMar w:top="720" w:right="1440" w:bottom="1440" w:left="1080" w:header="720" w:footer="720" w:gutter="0"/>`)
	require.True(t, strings.HasSuffix(document, `</w:sectPr></w:body></w:document>`))

	require.Equal(t, "Page one\nLine A\nLine B", extractDocxText(t, data))
}

func TestRenderDocxPackageDeclaresEveryPart(t *testing.T) {
	data, err := renderDocx("Hello")
	require.NoError(t, err)

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	require.Equal(t, "[Content_Types].xml", reader.File[0].Name)
	for _, file := range reader.File {
		if file.Name == "[Content_Types].xml" || strings.HasSuffix(file.Name, ".rels") {
			continue
		}
		requireZipEntryContains(t, data, "[Content_Types].xml", `PartName="/`+file.Name+`"`)
	}
	requireZipEntryNotContains(t, data, "docProps/app.xml", "docxlib")
}

func docxDocumentXML(t *testing.T, data []byte) string {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		handle, err := file.Open()
		require.NoError(t, err)
		defer handle.Close()
		content, err := io.ReadAll(handle)
		require.NoError(t, err)
		return string(content)
	}
	require.FailNow(t, "word/document.xml not found")
	return ""
}
