package docx

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/model"
)

type Adapter struct{}

func (a Adapter) Format() string {
	return "docx"
}

func (a Adapter) Parse(path string) (*model.Document, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var documentXML []byte
	for _, f := range reader.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		documentXML, err = io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		break
	}
	if len(documentXML) == 0 {
		return nil, fmt.Errorf("docx: missing word/document.xml")
	}

	paras, err := parseDOCXParagraphs(documentXML)
	if err != nil {
		return nil, err
	}
	blocks := make([]model.Block, 0, len(paras))
	order := 0
	title := ""
	for _, p := range paras {
		txt := strings.TrimSpace(p.Text)
		if txt == "" {
			continue
		}
		typ := "text"
		if strings.HasPrefix(strings.ToLower(p.Style), "heading") {
			typ = "heading"
			if title == "" {
				title = txt
			}
		}
		order++
		blocks = append(blocks, model.Block{
			Type:  typ,
			Text:  txt,
			Page:  0,
			Order: order,
		})
	}

	doc := &model.Document{
		Format:    "docx",
		Title:     title,
		PageCount: 1,
		Sections: []model.Section{
			{
				Path:    "root",
				Heading: "",
				Blocks:  blocks,
			},
		},
		Metadata: map[string]string{
			"source_ext": strings.ToLower(filepath.Ext(path)),
		},
	}
	return doc, nil
}

type docxParagraph struct {
	Text  string
	Style string
}

func parseDOCXParagraphs(xmlData []byte) ([]docxParagraph, error) {
	dec := xml.NewDecoder(strings.NewReader(string(xmlData)))
	out := make([]docxParagraph, 0, 64)

	inP := false
	inT := false
	curStyle := ""
	textParts := make([]string, 0, 8)

	flushParagraph := func() {
		if !inP {
			return
		}
		txt := strings.TrimSpace(strings.Join(textParts, ""))
		if txt != "" {
			out = append(out, docxParagraph{Text: txt, Style: curStyle})
		}
		textParts = textParts[:0]
		curStyle = ""
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "p":
				inP = true
				textParts = textParts[:0]
				curStyle = ""
			case "t":
				if inP {
					inT = true
				}
			case "pStyle":
				if inP {
					for _, at := range el.Attr {
						if at.Name.Local == "val" {
							curStyle = strings.TrimSpace(at.Value)
							break
						}
					}
				}
			case "tab":
				if inP {
					textParts = append(textParts, "\t")
				}
			case "br":
				if inP {
					textParts = append(textParts, "\n")
				}
			}
		case xml.EndElement:
			switch el.Name.Local {
			case "t":
				inT = false
			case "p":
				flushParagraph()
				inP = false
			}
		case xml.CharData:
			if inP && inT {
				textParts = append(textParts, string(el))
			}
		}
	}
	return out, nil
}
