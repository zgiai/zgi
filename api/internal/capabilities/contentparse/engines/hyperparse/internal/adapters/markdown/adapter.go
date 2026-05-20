package markdown

import (
	"bufio"
	"os"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/model"
)

type Adapter struct{}

func (a Adapter) Format() string {
	return "markdown"
}

func (a Adapter) Parse(path string) (*model.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	doc := &model.Document{
		Format:    "markdown",
		PageCount: 1,
		Sections: []model.Section{
			{Path: "root", Heading: "", Blocks: make([]model.Block, 0, 16)},
		},
	}
	blocks := doc.Sections[0].Blocks
	order := 0

	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	paragraph := make([]string, 0, 8)

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		text := strings.TrimSpace(strings.Join(paragraph, "\n"))
		paragraph = paragraph[:0]
		if text == "" {
			return
		}
		order++
		blocks = append(blocks, model.Block{
			Type:  "text",
			Text:  text,
			Page:  0,
			Order: order,
		})
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			flushParagraph()
			continue
		}
		if strings.HasPrefix(line, "#") {
			flushParagraph()
			title := strings.TrimSpace(strings.TrimLeft(line, "#"))
			if title != "" {
				if doc.Title == "" {
					doc.Title = title
				}
				order++
				blocks = append(blocks, model.Block{
					Type:  "heading",
					Text:  title,
					Page:  0,
					Order: order,
				})
			}
			continue
		}
		paragraph = append(paragraph, line)
	}
	flushParagraph()
	doc.Sections[0].Blocks = blocks
	return doc, nil
}
