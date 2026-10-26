package agentmemoryworker

import (
	"context"
	"testing"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// previewClient answers every chat with a fixed reply and records the call.
type previewClient struct {
	llmclient.LLMClient
	reply   string
	appCtx  *llmclient.AppContext
	request *adapter.ChatRequest
}

func (c *previewClient) AppChat(_ context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	c.appCtx, c.request = appCtx, req
	return &adapter.ChatResponse{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: c.reply}}}}, nil
}

func TestPreviewReportsEachEditAndTheFilesItWouldWrite(t *testing.T) {
	client := &previewClient{reply: `{"edits":[
		{"command":"insert","path":"/user/profile.md","text":"名叫林夏，做数据分析。","evidence":"我叫林夏，在一家电商公司做数据分析","reason":"identity"},
		{"command":"insert","path":"/user/preferences.md","text":"回答简短，先给结论。","evidence":"以后回答尽量简短，先给结论","reason":"preference"},
		{"command":"insert","path":"/user/profile.md","text":"今天有点累","evidence":"今天有点累","reason":"mood"},
		{"command":"insert","path":"/user/Persona File.md","text":"x","evidence":"我叫林夏","reason":"persona"}
	]}`}
	result, err := Preview(t.Context(), client, PreviewRequest{
		Instructions: "General assistant.",
		Turns:        []PreviewTurn{{User: "我叫林夏，在一家电商公司做数据分析。以后回答尽量简短，先给结论。", Assistant: "好的。"}},
		Current:      map[string]string{"profile": "住在杭州。"},
		Provider:     "deepseek", Model: "deepseek-v4-flash",
		AppContext: &llmclient.AppContext{AppID: "agent-1", AppType: "agent"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.appCtx.ModelUseCase != consolidationUseCase || !client.appCtx.SuppressInvocationContent || client.request.Model != "deepseek-v4-flash" {
		t.Fatalf("preview call = %+v, model %s", client.appCtx, client.request.Model)
	}
	// The preference creates a new file, the mood is not in any user
	// message, and the persona path is not a valid file name.
	want := []string{"accepted", "accepted", "rejected", "rejected"}
	for index, edit := range result.Edits {
		if edit.Status != want[index] {
			t.Fatalf("edit %d = %+v, want %s", index, edit, want[index])
		}
	}
	if len(result.Files) != 2 || result.Files[0].Key != "preferences" || result.Files[0].Before != "" || result.Files[0].After != "回答简短，先给结论。" ||
		result.Files[1].Before != "住在杭州。" || result.Files[1].After != "住在杭州。\n名叫林夏，做数据分析。" {
		t.Fatalf("files = %+v", result.Files)
	}

	if _, err := Preview(t.Context(), client, PreviewRequest{Current: map[string]string{"profile": "x"}}); err == nil {
		t.Fatal("preview without turns was accepted")
	}
}
