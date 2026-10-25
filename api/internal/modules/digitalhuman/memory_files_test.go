package digitalhuman

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
)

func fileMemory(class memoryruntime.MemoryClass, scope memoryruntime.MemoryScopeType, memoryType memoryruntime.MemoryType, key, content string) *DigitalHumanMemory {
	return &DigitalHumanMemory{ID: uuid.New(), MemoryClass: class, ScopeType: scope, MemoryType: memoryType, MemoryKey: key, Content: content, Revision: 2}
}

func TestMemoryFilePathsSayWhoTheMemoryIsAbout(t *testing.T) {
	for want, memory := range map[string]*DigitalHumanMemory{
		"/me/return_policy.md":         fileMemory(memoryruntime.MemoryClassSelf, "", memoryruntime.MemoryTypeFact, "return_policy", "x"),
		"/me/team/pricing_floor.md":    fileMemory(memoryruntime.MemoryClassBusiness, memoryruntime.MemoryScopeWorkspace, memoryruntime.MemoryTypeFact, "pricing_floor", "x"),
		"/conversation/launch_plan.md": fileMemory(memoryruntime.MemoryClassBusiness, memoryruntime.MemoryScopeChannel, memoryruntime.MemoryTypeDecision, "launch_plan", "x"),
		"/user/notes/quote_due.md":     fileMemory(memoryruntime.MemoryClassBusiness, memoryruntime.MemoryScopeInstance, memoryruntime.MemoryTypeOpenLoop, "quote_due", "x"),
		"/me/a_b.md":                   fileMemory(memoryruntime.MemoryClassSelf, "", memoryruntime.MemoryTypeFact, "a/b", "x"),
		"":                             fileMemory(memoryruntime.MemoryClassBusiness, memoryruntime.MemoryScopeType("project"), memoryruntime.MemoryTypeFact, "never_resolved", "x"),
	} {
		if got := memoryFilePath(memory); got != want {
			t.Fatalf("path of %s/%s/%s = %q, want %q", memory.MemoryClass, memory.ScopeType, memory.MemoryKey, got, want)
		}
	}
	identity := fileMemory(memoryruntime.MemoryClassSelf, "", memoryruntime.MemoryTypeIdentity, "persona", "x")
	if attributes := memoryFileAttributes(identity); !strings.Contains(attributes, `read_only="true"`) || !strings.Contains(attributes, `audience="everyone"`) || !strings.Contains(attributes, `revision="2"`) {
		t.Fatalf("identity attributes = %s", attributes)
	}
}

func TestMemoryFilesRenderRecalledFilesThenAnIndexWithinBudget(t *testing.T) {
	recalled := fileMemory(memoryruntime.MemoryClassSelf, "", memoryruntime.MemoryTypeFact, "return_policy", "退货政策：45 天内可退。")
	listed := []*DigitalHumanMemory{recalled}
	for i := 0; i < 80; i++ {
		listed = append(listed, fileMemory(memoryruntime.MemoryClassSelf, "", memoryruntime.MemoryTypeFact, "fact_"+string(rune('a'+i%26))+strings.Repeat("x", i), "第一行内容"+strings.Repeat("长", 120)+"\n第二行"))
	}
	rendered := renderMemoryFiles([]*DigitalHumanMemory{recalled}, listed, 3000, false)
	if utf8.RuneCountInString(rendered) > 3000 {
		t.Fatalf("rendered %d chars over the 3000 budget", utf8.RuneCountInString(rendered))
	}
	if !strings.Contains(rendered, "<recalled>") || !strings.Contains(rendered, "45 天内可退") {
		t.Fatalf("recalled file missing:\n%s", rendered)
	}
	if strings.Count(rendered, "/me/return_policy.md") != 1 {
		t.Fatal("a recalled file is listed again in the index")
	}
	if !strings.Contains(rendered, "<index>") || !strings.Contains(rendered, "more files are not listed") || strings.Contains(rendered, "第二行") {
		t.Fatalf("index is not one first line per file with an omitted count:\n%s", rendered)
	}

	// A recalled file that does not fit is cut with a marker, never dropped silently.
	long := fileMemory(memoryruntime.MemoryClassSelf, "", memoryruntime.MemoryTypeFact, "handbook", strings.Repeat("规", 5000))
	rendered = renderMemoryFiles([]*DigitalHumanMemory{long}, nil, 2000, true)
	if utf8.RuneCountInString(rendered) > 2000 || !strings.Contains(rendered, "[truncated; read the whole file with memory_view]") || !strings.Contains(rendered, "most recently updated") {
		t.Fatalf("long recalled file:\n%s", rendered)
	}
}

func TestMemoryViewListsDirectoriesAndReadsOneFile(t *testing.T) {
	policy := fileMemory(memoryruntime.MemoryClassSelf, "", memoryruntime.MemoryTypeFact, "return_policy", "第一行\n第二行")
	note := fileMemory(memoryruntime.MemoryClassBusiness, memoryruntime.MemoryScopeInstance, memoryruntime.MemoryTypeOpenLoop, "quote_due", "周五前发报价单")
	listing := viewMemoryFiles([]*DigitalHumanMemory{policy, note}, "")
	directories := listing["directories"].([]map[string]any)
	if len(directories) != 2 || directories[0]["directory"] != "/me/" || directories[1]["directory"] != "/user/notes/" {
		t.Fatalf("listing = %+v", directories)
	}
	if only := viewMemoryFiles([]*DigitalHumanMemory{policy, note}, "/user/notes/")["directories"].([]map[string]any); len(only) != 1 {
		t.Fatalf("directory listing = %+v", only)
	}
	file := viewMemoryFiles([]*DigitalHumanMemory{policy, note}, "/me/return_policy.md")
	if file["content"] != "1\t第一行\n2\t第二行" || file["key"] != "return_policy" || file["memory"] != policy {
		t.Fatalf("file = %+v", file)
	}
	if missing := viewMemoryFiles([]*DigitalHumanMemory{policy}, "/me/other.md"); missing["status"] != "not_found" {
		t.Fatalf("missing file = %+v", missing)
	}
}

func TestMemorySearchKeepsMatchesInsideTheRequestedDirectory(t *testing.T) {
	items := []*DigitalHumanMemory{
		fileMemory(memoryruntime.MemoryClassSelf, "", memoryruntime.MemoryTypeFact, "a", "x"),
		fileMemory(memoryruntime.MemoryClassBusiness, memoryruntime.MemoryScopeInstance, memoryruntime.MemoryTypeFact, "b", "x"),
		fileMemory(memoryruntime.MemoryClassSelf, "", memoryruntime.MemoryTypeFact, "c", "x"),
	}
	if got := searchMemoryFiles(items, "/me/", 5); len(got) != 2 || got[0].MemoryKey != "a" || got[1].MemoryKey != "c" {
		t.Fatalf("search in /me/ = %d files", len(got))
	}
	if got := searchMemoryFiles(items, "", 1); len(got) != 1 {
		t.Fatalf("limit not applied: %d", len(got))
	}
	if summaries := memoryFileSummaries(items[:1]); summaries[0]["path"] != "/me/a.md" {
		t.Fatalf("summary = %+v", summaries[0])
	}
}

func TestMemoryFilesGuidanceReplacesRetrievalOnlyWhenEnabled(t *testing.T) {
	legacy := renderDigitalHumanMemoryGuidance(true, false, false)
	files := renderDigitalHumanMemoryGuidance(true, false, true)
	if !strings.Contains(legacy, "retrieve_digital_human_memory") || strings.Contains(files, "retrieve_digital_human_memory") || !strings.Contains(files, "memory_search") {
		t.Fatalf("guidance did not switch:\nlegacy: %s\nfiles: %s", legacy, files)
	}
	if renderDigitalHumanMemoryContext(nil, true, false) != legacy {
		t.Fatal("legacy guidance changed")
	}
}

func TestMemorySearchFindsArchivedEntriesUnderArchive(t *testing.T) {
	expired := fileMemory(memoryruntime.MemoryClassBusiness, memoryruntime.MemoryScopeInstance, memoryruntime.MemoryTypeOpenLoop, "quote_due", "答应周五前把报价单发过去。")
	other := fileMemory(memoryruntime.MemoryClassBusiness, memoryruntime.MemoryScopeInstance, memoryruntime.MemoryTypeFact, "office", "办公室在三楼。")
	found := searchArchivedMemoryFiles([]*DigitalHumanMemory{other, expired}, "报价单什么时候发", "", 5)
	if len(found) != 1 || found[0]["path"] != "/user/notes/archive/quote_due.md" || found[0]["tags"] != "open_loop" {
		t.Fatalf("archive matches = %+v", found)
	}
	if found := searchArchivedMemoryFiles([]*DigitalHumanMemory{expired}, "报价单", "/me/", 5); len(found) != 0 {
		t.Fatalf("a /me/ search returned the person's archived notes: %+v", found)
	}
}
