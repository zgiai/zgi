package extractor

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
)

const (
	docxDefaultDocumentPart = "word/document.xml"
	docxPackageRelsPart     = "_rels/.rels"
	docxOfficeDocumentRel   = "/officeDocument"

	wordprocessingMLNamespace       = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	wordprocessingMLStrictNamespace = "http://purl.oclc.org/ooxml/wordprocessingml/main"
	markupCompatibilityNamespace    = "http://schemas.openxmlformats.org/markup-compatibility/2006"
)

type WordExtractor struct {
	filePath string
	tenantID string
	userID   string
}

func NewWordExtractor(filePath, tenantID, userID string) *WordExtractor {
	return &WordExtractor{
		filePath: filePath,
		tenantID: tenantID,
		userID:   userID,
	}
}

func (e *WordExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	content, err := e.parseDocx(ctx, e.filePath)
	if err != nil {
		return nil, fmt.Errorf("error parsing docx file: %w", err)
	}

	metadata := map[string]interface{}{"source": e.filePath}
	documents := []dto.Document{
		{
			PageContent: content,
			Metadata:    metadata,
		},
	}
	return dto.NewExtractOutputFromDocuments("zgi:word", documents), nil
}

func (e *WordExtractor) parseDocx(ctx context.Context, docxPath string) (string, error) {
	readFile, err := os.Open(docxPath)
	if err != nil {
		return "", fmt.Errorf("failed to open docx file: %w", err)
	}
	defer readFile.Close()

	fileInfo, err := readFile.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}

	// Check file format by reading magic bytes
	header := make([]byte, 4)
	if _, err := io.ReadFull(readFile, header); err != nil {
		return "", fmt.Errorf("failed to read file header: %w", err)
	}

	// Check if it's an old .doc format (CFB: D0 CF 11 E0)
	if header[0] == 0xD0 && header[1] == 0xCF && header[2] == 0x11 && header[3] == 0xE0 {
		return "", fmt.Errorf("file is in old Word 97-2003 (.doc) format, please convert to modern .docx format")
	}

	archive, err := zip.NewReader(readFile, fileInfo.Size())
	if err != nil {
		return "", fmt.Errorf("failed to open docx package: %w", err)
	}

	documentPart, err := openDocxDocumentPart(archive)
	if err != nil {
		return "", err
	}
	defer documentPart.Close()

	paragraphs, tableRows, err := parseDocxBody(ctx, xml.NewDecoder(documentPart))
	if err != nil {
		return "", fmt.Errorf("failed to parse docx document part: %w", err)
	}

	// Body paragraphs come first, followed by table rows with tab-separated cells.
	content := make([]string, 0, len(paragraphs)+len(tableRows))
	content = append(content, paragraphs...)
	content = append(content, tableRows...)
	return strings.Join(content, "\n"), nil
}

// openDocxDocumentPart resolves the main document part through the package
// relationships, falling back to the conventional word/document.xml location.
func openDocxDocumentPart(archive *zip.Reader) (io.ReadCloser, error) {
	files := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		files[strings.TrimPrefix(file.Name, "/")] = file
	}

	partName := docxDefaultDocumentPart
	if relsFile, ok := files[docxPackageRelsPart]; ok {
		target, err := findDocxOfficeDocumentTarget(relsFile)
		if err != nil {
			return nil, err
		}
		if target != "" {
			partName = target
		}
	}

	file, ok := files[partName]
	if !ok {
		return nil, fmt.Errorf("docx package is missing main document part %s", partName)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open docx document part %s: %w", partName, err)
	}
	return reader, nil
}

func findDocxOfficeDocumentTarget(relsFile *zip.File) (string, error) {
	reader, err := relsFile.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open docx package relationships: %w", err)
	}
	defer reader.Close()

	var rels struct {
		Relationships []struct {
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.NewDecoder(reader).Decode(&rels); err != nil {
		return "", fmt.Errorf("failed to parse docx package relationships: %w", err)
	}
	for _, rel := range rels.Relationships {
		if strings.HasSuffix(rel.Type, docxOfficeDocumentRel) && rel.Target != "" {
			return strings.TrimPrefix(path.Clean("/"+rel.Target), "/"), nil
		}
	}
	return "", nil
}

// parseDocxBody walks the block-level content of a WordprocessingML document.
// It returns the non-empty paragraph texts and the non-empty table rows.
func parseDocxBody(ctx context.Context, decoder *xml.Decoder) ([]string, []string, error) {
	var paragraphs []string
	var tableRows []string
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return paragraphs, tableRows, nil
		}
		if err != nil {
			return nil, nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if !isWordElement(start.Name) {
			if err := decoder.Skip(); err != nil {
				return nil, nil, err
			}
			continue
		}
		switch start.Name.Local {
		case "document", "body", "sdt", "sdtContent", "customXml":
			// Containers: descend into their block-level children.
		case "p":
			text, err := collectDocxText(decoder)
			if err != nil {
				return nil, nil, err
			}
			if text != "" {
				paragraphs = append(paragraphs, text)
			}
		case "tbl":
			rows, err := collectDocxTableRows(decoder)
			if err != nil {
				return nil, nil, err
			}
			tableRows = append(tableRows, rows...)
		default:
			if err := decoder.Skip(); err != nil {
				return nil, nil, err
			}
		}
	}
}

// collectDocxTableRows consumes a w:tbl element and returns one string per
// non-empty row, with non-empty cell texts joined by tabs.
func collectDocxTableRows(decoder *xml.Decoder) ([]string, error) {
	var rows []string
	var cells []string
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch tok := token.(type) {
		case xml.StartElement:
			if !isWordElement(tok.Name) {
				if err := decoder.Skip(); err != nil {
					return nil, err
				}
				continue
			}
			switch tok.Name.Local {
			case "tr":
				cells = cells[:0]
			case "tc":
				text, err := collectDocxText(decoder)
				if err != nil {
					return nil, err
				}
				if text != "" {
					cells = append(cells, text)
				}
			case "sdt", "sdtContent", "customXml":
				// Row and cell level containers.
			default:
				if err := decoder.Skip(); err != nil {
					return nil, err
				}
			}
		case xml.EndElement:
			if !isWordElement(tok.Name) {
				continue
			}
			switch tok.Name.Local {
			case "tr":
				if len(cells) > 0 {
					rows = append(rows, strings.Join(cells, "\t"))
				}
			case "tbl":
				return rows, nil
			}
		}
	}
}

// collectDocxText consumes the element whose start tag was just read and
// returns the visible text of its runs.
func collectDocxText(decoder *xml.Decoder) (string, error) {
	var text strings.Builder
	inText := false
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}
		switch tok := token.(type) {
		case xml.StartElement:
			if tok.Name.Space == markupCompatibilityNamespace && tok.Name.Local == "Fallback" {
				// mc:Fallback repeats the content of mc:Choice for older readers.
				if err := decoder.Skip(); err != nil {
					return "", err
				}
				continue
			}
			depth++
			if !isWordElement(tok.Name) {
				continue
			}
			switch tok.Name.Local {
			case "t":
				inText = true
			case "tab":
				text.WriteByte('\t')
			case "br", "cr":
				if !isDocxLayoutBreak(tok) {
					text.WriteByte('\n')
				}
			case "noBreakHyphen":
				text.WriteByte('-')
			case "del", "moveFrom":
				// Tracked deletions are not part of the visible text.
				if err := decoder.Skip(); err != nil {
					return "", err
				}
				depth--
			}
		case xml.EndElement:
			depth--
			if isWordElement(tok.Name) && tok.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				text.Write(tok)
			}
		}
	}
	if strings.Trim(text.String(), "\n") == "" {
		return "", nil
	}
	return text.String(), nil
}

func isDocxLayoutBreak(start xml.StartElement) bool {
	for _, attr := range start.Attr {
		if attr.Name.Local == "type" && (attr.Value == "page" || attr.Value == "column") {
			return true
		}
	}
	return false
}

func isWordElement(name xml.Name) bool {
	return name.Space == wordprocessingMLNamespace || name.Space == wordprocessingMLStrictNamespace
}
