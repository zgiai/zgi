package service

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestNormalizeKnowledgeImageURLsNormalizesImageSources(t *testing.T) {
	const filesBaseURL = "https://api.lingyoungai.com"
	content := strings.Join([]string{
		"![relative-root](/uploads/a.png?size=small#preview)",
		"![relative-file](images/b.png)",
		`<img src="/console/api/files/mineru-images?key=document-images%2Fdoc%2Fc.png">`,
		`<img src="/tmp/hyperparse/mineru/images/doc/d.png">`,
		`<img src="C:\storage\mineru\images\doc\e.png">`,
		"![external](https://cdn.example.com/image.png)",
		"![protocol-relative](//cdn.example.com/protocol-relative.png)",
		"![data](data:image/png;base64,abc)",
		"https://example.com/files/mineru-images?key=plain-text-should-not-change",
	}, "\n")

	normalized, err := NormalizeKnowledgeImageURLs(content, filesBaseURL)
	if err != nil {
		t.Fatalf("NormalizeKnowledgeImageURLs() error = %v", err)
	}

	expected := strings.Join([]string{
		"![relative-root](https://api.lingyoungai.com/uploads/a.png?size=small#preview)",
		"![relative-file](https://api.lingyoungai.com/images/b.png)",
		`<img src="https://api.lingyoungai.com/console/api/files/mineru-images?key=document-images%2Fdoc%2Fc.png">`,
		`<img src="https://api.lingyoungai.com/console/api/files/mineru-images?path=%2Ftmp%2Fhyperparse%2Fmineru%2Fimages%2Fdoc%2Fd.png">`,
		`<img src="https://api.lingyoungai.com/console/api/files/mineru-images?path=C%3A%5Cstorage%5Cmineru%5Cimages%5Cdoc%5Ce.png">`,
		"![external](https://cdn.example.com/image.png)",
		"![protocol-relative](//cdn.example.com/protocol-relative.png)",
		"![data](data:image/png;base64,abc)",
		"https://example.com/files/mineru-images?key=plain-text-should-not-change",
	}, "\n")
	if normalized != expected {
		t.Fatalf("normalized content mismatch\nwant:\n%s\n\ngot:\n%s", expected, normalized)
	}
}

func TestNormalizeKnowledgeImageURLsRejectsInvalidBaseOnlyWhenNeeded(t *testing.T) {
	unchanged, err := NormalizeKnowledgeImageURLs("plain text", "")
	if err != nil {
		t.Fatalf("NormalizeKnowledgeImageURLs() plain text error = %v", err)
	}
	if unchanged != "plain text" {
		t.Fatalf("plain text changed: %q", unchanged)
	}

	_, err = NormalizeKnowledgeImageURLs("![figure](/uploads/a.png)", "api.example.com")
	if err == nil {
		t.Fatalf("expected invalid files base URL error")
	}

	_, err = NormalizeKnowledgeImageURLs("![figure](/uploads/a.png)", "https://api.example.com?bad=1")
	if err == nil {
		t.Fatalf("expected files base URL with query to be rejected")
	}
}

func TestNormalizeHitTestingResponseKnowledgeImageURLs(t *testing.T) {
	response := &dto.HitTestingResponse{
		Records: []dto.HitTestingRecordResponse{{
			Segment: dto.SegmentResponse{
				Content:     "![content](/document-images/content.png)",
				SignContent: `<img src="/storage/mineru/images/sign.png">`,
			},
			ChildChunks: []dto.ChildChunkResponse{{
				Content: "![child](images/child.png)",
			}},
		}},
	}

	err := normalizeHitTestingResponseKnowledgeImageURLs(response, "https://api.lingyoungai.com")
	if err != nil {
		t.Fatalf("normalizeHitTestingResponseKnowledgeImageURLs() error = %v", err)
	}

	if !strings.Contains(response.Records[0].Segment.Content, "https://api.lingyoungai.com/document-images/content.png") {
		t.Fatalf("segment content was not normalized: %q", response.Records[0].Segment.Content)
	}
	if !strings.Contains(response.Records[0].Segment.SignContent, "https://api.lingyoungai.com/console/api/files/mineru-images?path=%2Fstorage%2Fmineru%2Fimages%2Fsign.png") {
		t.Fatalf("segment sign content was not normalized: %q", response.Records[0].Segment.SignContent)
	}
	if !strings.Contains(response.Records[0].ChildChunks[0].Content, "https://api.lingyoungai.com/images/child.png") {
		t.Fatalf("child chunk content was not normalized: %q", response.Records[0].ChildChunks[0].Content)
	}
}
