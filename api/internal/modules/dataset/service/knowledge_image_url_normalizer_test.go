package service

import (
	"net/url"
	"strings"
	"testing"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestNormalizeKnowledgeImageURLsNormalizesImageSources(t *testing.T) {
	restoreKnowledgeImageURLTestConfig(t)

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

	lines := strings.Split(normalized, "\n")
	if len(lines) != 9 {
		t.Fatalf("expected 9 normalized lines, got %d:\n%s", len(lines), normalized)
	}
	assertKnowledgeImageLine(t, lines[0], "![relative-root](https://api.lingyoungai.com/uploads/a.png?size=small#preview)")
	assertKnowledgeImageLine(t, lines[1], "![relative-file](https://api.lingyoungai.com/images/b.png)")
	assertKnowledgeImageLine(t, lines[2], `<img src="https://api.lingyoungai.com/console/api/files/mineru-images?key=document-images%2Fdoc%2Fc.png">`)
	assertKnowledgeParserImageURL(t, extractKnowledgeImgSrc(t, lines[3]), "path", "/tmp/hyperparse/mineru/images/doc/d.png")
	assertKnowledgeParserImageURL(t, extractKnowledgeImgSrc(t, lines[4]), "path", `C:\storage\mineru\images\doc\e.png`)
	assertKnowledgeImageLine(t, lines[5], "![external](https://cdn.example.com/image.png)")
	assertKnowledgeImageLine(t, lines[6], "![protocol-relative](//cdn.example.com/protocol-relative.png)")
	assertKnowledgeImageLine(t, lines[7], "![data](data:image/png;base64,abc)")
	assertKnowledgeImageLine(t, lines[8], "https://example.com/files/mineru-images?key=plain-text-should-not-change")
}

func assertKnowledgeImageLine(t *testing.T, got, want string) {
	t.Helper()

	if got != want {
		t.Fatalf("normalized line mismatch\nwant: %s\n got: %s", want, got)
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
	restoreKnowledgeImageURLTestConfig(t)

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
	assertKnowledgeParserImageURL(t, extractKnowledgeImgSrc(t, response.Records[0].Segment.SignContent), "path", "/storage/mineru/images/sign.png")
	if !strings.Contains(response.Records[0].ChildChunks[0].Content, "https://api.lingyoungai.com/images/child.png") {
		t.Fatalf("child chunk content was not normalized: %q", response.Records[0].ChildChunks[0].Content)
	}
}

func extractKnowledgeImgSrc(t *testing.T, html string) string {
	t.Helper()

	const prefix = `<img src="`
	start := strings.Index(html, prefix)
	if start < 0 {
		t.Fatalf("missing image src in %q", html)
	}
	rest := html[start+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatalf("unterminated image src in %q", html)
	}
	return rest[:end]
}

func assertKnowledgeParserImageURL(t *testing.T, rawURL, wantParam, wantValue string) {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse parser image url %q: %v", rawURL, err)
	}
	if parsed.Scheme != "https" || parsed.Host != "api.lingyoungai.com" || parsed.Path != knowledgeImageEndpoint {
		t.Fatalf("parser image url = %q, want https://api.lingyoungai.com%s", rawURL, knowledgeImageEndpoint)
	}

	query := parsed.Query()
	if got := query.Get(wantParam); got != wantValue {
		t.Fatalf("%s = %q, want %q in %q", wantParam, got, wantValue, rawURL)
	}
	if query.Get("timestamp") == "" || query.Get("nonce") == "" || query.Get("sign") == "" {
		t.Fatalf("parser image url missing signature params: %q", rawURL)
	}
	if !util.VerifyParserImageSignature(wantParam, wantValue, query.Get("timestamp"), query.Get("nonce"), query.Get("sign")) {
		t.Fatalf("parser image url signature did not verify: %q", rawURL)
	}
}

func restoreKnowledgeImageURLTestConfig(t *testing.T) {
	t.Helper()

	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		App: appconfig.AppConfig{
			SecretKey:          "test-secret",
			FilesAccessTimeout: 3600,
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})
}
