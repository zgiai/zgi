package text

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/model"
)

type Adapter struct{}

func (a Adapter) Format() string {
	return "text"
}

func (a Adapter) Parse(path string) (*model.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	paragraphs := splitParagraphs(content)
	blocks := make([]model.Block, 0, len(paragraphs))
	for i, p := range paragraphs {
		blocks = append(blocks, model.Block{
			Type:  "text",
			Text:  p,
			Page:  0,
			Order: i + 1,
		})
	}
	doc := &model.Document{
		Format:    "text",
		Title:     strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		PageCount: 1,
		Sections: []model.Section{{
			Path:    "root",
			Heading: "",
			Blocks:  blocks,
		}},
	}
	return doc, nil
}

func splitParagraphs(content string) []string {
	scanner := bufio.NewScanner(bytes.NewBufferString(content))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	out := make([]string, 0, 8)
	cur := make([]string, 0, 8)

	flush := func() {
		if len(cur) == 0 {
			return
		}
		joined := strings.TrimSpace(strings.Join(cur, "\n"))
		if joined != "" {
			out = append(out, joined)
		}
		cur = cur[:0]
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()

	if len(out) == 0 {
		trimmed := strings.TrimSpace(content)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
