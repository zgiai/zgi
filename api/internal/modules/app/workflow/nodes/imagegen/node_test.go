package imagegen

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	llmnode "github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/llm"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	doubaoSeedream50LiteTestModel = doubaoSeedream50ModelPrefix + "-lite-260128"
	doubaoSeedream45TestModel     = doubaoSeedream45ModelPrefix + "-250828"
	doubaoSeedream40TestModel     = doubaoSeedream40ModelPrefix + "-250428"
)

type fakeImageInvoker struct {
	lastAccountID string
	lastAppID     string
	lastAppType   string
	lastRequest   *InvokeRequest
	result        *InvokeResult
	err           error
}

func (f *fakeImageInvoker) Invoke(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (*InvokeResult, error) {
	f.lastAccountID = accountID
	f.lastAppID = appID
	f.lastAppType = appType
	f.lastRequest = req
	return f.result, f.err
}

type fakeFileSaver struct {
	remoteURLs []string
	binaryData [][]byte
}

func (f *fakeFileSaver) SaveRemoteURL(url string, fileType file.FileType) (*file.File, error) {
	f.remoteURLs = append(f.remoteURLs, url)
	signedURL := "https://internal.example/remote.png"
	filename := "remote.png"
	extension := ".png"
	mimeType := "image/png"
	id := "tool-file-remote"
	return &file.File{
		ZgiModelIdentity: file.FILE_MODEL_IDENTITY,
		ID:               &id,
		Type:             fileType,
		TransferMethod:   file.FileTransferMethodToolFile,
		Filename:         &filename,
		Extension:        &extension,
		MimeType:         &mimeType,
		RelatedID:        &id,
		URL:              &signedURL,
	}, nil
}

func (f *fakeFileSaver) SaveBinaryString(data []byte, mimeType string, fileType file.FileType, extensionOverride *string) (*file.File, error) {
	buf := make([]byte, len(data))
	copy(buf, data)
	f.binaryData = append(f.binaryData, buf)
	signedURL := "https://internal.example/b64.png"
	filename := "b64.png"
	extension := ".png"
	id := "tool-file-b64"
	return &file.File{
		ZgiModelIdentity: file.FILE_MODEL_IDENTITY,
		ID:               &id,
		Type:             fileType,
		TransferMethod:   file.FileTransferMethodToolFile,
		Filename:         &filename,
		Extension:        &extension,
		MimeType:         &mimeType,
		RelatedID:        &id,
		URL:              &signedURL,
	}, nil
}

var _ llmnode.FileSaver = (*fakeFileSaver)(nil)

func TestParseNodeDataFromConfig_AppliesDefaults(t *testing.T) {
	config := map[string]any{
		"id": "image-node-1",
		"data": map[string]any{
			"type":   "image-gen",
			"title":  "Image Generation",
			"model":  map[string]any{"provider": "openai", "name": "gpt-image-1"},
			"prompt": "draw a cat",
		},
	}

	nodeData, nodeID, err := parseNodeDataFromConfig(config)
	if err != nil {
		t.Fatalf("parseNodeDataFromConfig returned error: %v", err)
	}
	if nodeID != "image-node-1" {
		t.Fatalf("nodeID = %q, want %q", nodeID, "image-node-1")
	}
	if nodeData.Generation.N != 1 {
		t.Fatalf("Generation.N = %d, want 1", nodeData.Generation.N)
	}
	if nodeData.Generation.Size != "" {
		t.Fatalf("Generation.Size = %q, want empty string", nodeData.Generation.Size)
	}
	if nodeData.Output.Lifecycle != "persistent" {
		t.Fatalf("Output.Lifecycle = %q, want %q", nodeData.Output.Lifecycle, "persistent")
	}
}

func TestNodeExecuteRun_DoubaoSeedream5UsesHighResolutionPresetForSquare(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs = map[string]any{
		"image_gen_config": map[string]any{
			"aspect_ratio": "1:1",
		},
	}

	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:      ImageModelConfig{Provider: doubaoProviderName, Name: doubaoSeedream50LiteTestModel},
			Prompt:     "draw a cat",
			Generation: GenerationConfig{N: 1},
			Output:     OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if invoker.lastRequest == nil {
		t.Fatalf("lastRequest is nil")
	}
	if invoker.lastRequest.Size != "2048x2048" {
		t.Fatalf("Size = %q, want %q", invoker.lastRequest.Size, "2048x2048")
	}
}

func TestNodeExecuteRun_DoubaoSeedream5UsesHighResolutionPresetForWideAspectRatio(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs = map[string]any{
		"image_gen_config": map[string]any{
			"aspect_ratio": "16:9",
		},
	}

	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:      ImageModelConfig{Provider: doubaoProviderName, Name: doubaoSeedream50LiteTestModel},
			Prompt:     "draw a cat",
			Generation: GenerationConfig{N: 1},
			Output:     OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if invoker.lastRequest == nil {
		t.Fatalf("lastRequest is nil")
	}
	if invoker.lastRequest.Size != "2560x1440" {
		t.Fatalf("Size = %q, want %q", invoker.lastRequest.Size, "2560x1440")
	}
}

func TestNodeExecuteRun_DoubaoSeedream45UsesHighResolutionDefaultSize(t *testing.T) {
	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(entities.NewVariablePool()),
		nodeData: NodeData{
			Model:      ImageModelConfig{Provider: doubaoProviderName, Name: doubaoSeedream45TestModel},
			Prompt:     "draw a cat",
			Generation: GenerationConfig{N: 1},
			Output:     OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if invoker.lastRequest == nil {
		t.Fatalf("lastRequest is nil")
	}
	if invoker.lastRequest.Size != "2048x2048" {
		t.Fatalf("Size = %q, want %q", invoker.lastRequest.Size, "2048x2048")
	}
}

func TestNodeExecuteRun_DoubaoSeedream40UsesOneKPresetForWideAspectRatio(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs = map[string]any{
		"image_gen_config": map[string]any{
			"aspect_ratio": "16:9",
		},
	}

	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:      ImageModelConfig{Provider: doubaoProviderName, Name: doubaoSeedream40TestModel},
			Prompt:     "draw a cat",
			Generation: GenerationConfig{N: 1},
			Output:     OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if invoker.lastRequest == nil {
		t.Fatalf("lastRequest is nil")
	}
	if invoker.lastRequest.Size != "1312x736" {
		t.Fatalf("Size = %q, want %q", invoker.lastRequest.Size, "1312x736")
	}
}

func TestNodeExecuteRun_DoubaoSeedream40KeepsDefaultOneKSquareSize(t *testing.T) {
	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(entities.NewVariablePool()),
		nodeData: NodeData{
			Model:      ImageModelConfig{Provider: doubaoProviderName, Name: doubaoSeedream40TestModel},
			Prompt:     "draw a cat",
			Generation: GenerationConfig{N: 1},
			Output:     OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if invoker.lastRequest == nil {
		t.Fatalf("lastRequest is nil")
	}
	if invoker.lastRequest.Size != "1024x1024" {
		t.Fatalf("Size = %q, want %q", invoker.lastRequest.Size, "1024x1024")
	}
}

func TestNodeExecuteRun_DoubaoSeedream40AllowsExplicitSupportedFourKSize(t *testing.T) {
	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(entities.NewVariablePool()),
		nodeData: NodeData{
			Model:  ImageModelConfig{Provider: doubaoProviderName, Name: doubaoSeedream40TestModel},
			Prompt: "draw a cat",
			Generation: GenerationConfig{
				N:    1,
				Size: "4096x4096",
			},
			Output: OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if invoker.lastRequest == nil {
		t.Fatalf("lastRequest is nil")
	}
	if invoker.lastRequest.Size != "4096x4096" {
		t.Fatalf("Size = %q, want %q", invoker.lastRequest.Size, "4096x4096")
	}
}

func TestNodeExecuteRun_DoubaoSeedream40RejectsUnsupportedExplicitSize(t *testing.T) {
	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(entities.NewVariablePool()),
		nodeData: NodeData{
			Model:  ImageModelConfig{Provider: doubaoProviderName, Name: doubaoSeedream40TestModel},
			Prompt: "draw a cat",
			Generation: GenerationConfig{
				N:    1,
				Size: "1920x1080",
			},
			Output: OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err == nil {
		t.Fatalf("executeRun error = nil, want unsupported size validation error")
	}
	if !strings.Contains(err.Error(), "1920x1080") {
		t.Fatalf("executeRun error = %q, want invalid size echoed back", err.Error())
	}
	if !strings.Contains(err.Error(), "supported aspect_ratio preset") {
		t.Fatalf("executeRun error = %q, want supported size guidance", err.Error())
	}
	if invoker.lastRequest != nil {
		t.Fatalf("lastRequest = %#v, want nil because upstream should not be called", invoker.lastRequest)
	}
}

func TestNodeExecuteRun_NonDoubaoModelKeepsExistingSquarePreset(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs = map[string]any{
		"image_gen_config": map[string]any{
			"aspect_ratio": "1:1",
		},
	}

	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:      ImageModelConfig{Provider: "openai", Name: "gpt-image-1"},
			Prompt:     "draw a cat",
			Generation: GenerationConfig{N: 1},
			Output:     OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if invoker.lastRequest == nil {
		t.Fatalf("lastRequest is nil")
	}
	if invoker.lastRequest.Size != "1024x1024" {
		t.Fatalf("Size = %q, want %q", invoker.lastRequest.Size, "1024x1024")
	}
}

func TestNodeExecuteRun_DoubaoRejectsExplicitSizeBelowMinimumPixels(t *testing.T) {
	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(entities.NewVariablePool()),
		nodeData: NodeData{
			Model:  ImageModelConfig{Provider: doubaoProviderName, Name: doubaoSeedream50LiteTestModel},
			Prompt: "draw a cat",
			Generation: GenerationConfig{
				N:    1,
				Size: "1024x1024",
			},
			Output: OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err == nil {
		t.Fatalf("executeRun error = nil, want explicit size validation error")
	}
	if !strings.Contains(err.Error(), "3686400") {
		t.Fatalf("executeRun error = %q, want minimum pixel requirement", err.Error())
	}
	if !strings.Contains(err.Error(), "1024x1024") {
		t.Fatalf("executeRun error = %q, want invalid size echoed back", err.Error())
	}
	if invoker.lastRequest != nil {
		t.Fatalf("lastRequest = %#v, want nil because upstream should not be called", invoker.lastRequest)
	}
}

func TestNodeExecuteRun_RendersPromptAndPersistsURLAndBase64(t *testing.T) {
	subject := "橘猫"
	style := "日系漫画"
	promptTemplate := "{{#start.subject#}}，风格：{{ style }}"
	renderedPrompt := subject + "，风格：" + style

	vp := entities.NewVariablePool()
	vp.Add([]string{"start", "subject"}, subject)
	vp.Add([]string{"sys", "style"}, style)
	vp.UserInputs = map[string]any{
		"model_config": map[string]any{
			"provider": "qwen",
			"model":    "qwen-image-2.0",
		},
	}

	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
				{B64JSON: base64.StdEncoding.EncodeToString([]byte("png-binary"))},
			},
		},
	}
	fileSaver := &fakeFileSaver{}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:  ImageModelConfig{Provider: "openai", Name: "gpt-image-1"},
			Prompt: promptTemplate,
			PromptConfig: llmnode.PromptConfig{
				TemplateVariables: []llmnode.VariableSelector{
					{
						Variable:      "style",
						ValueSelector: []string{"sys", "style"},
					},
				},
			},
			Generation: GenerationConfig{
				N:    2,
				Size: "1024x1024",
			},
			Output: OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    fileSaver,
	}

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}

	if invoker.lastAppType != workflowAppType {
		t.Fatalf("lastAppType = %q, want %q", invoker.lastAppType, workflowAppType)
	}
	if invoker.lastAppID != "workflow-1" {
		t.Fatalf("lastAppID = %q, want %q", invoker.lastAppID, "workflow-1")
	}
	if invoker.lastRequest == nil {
		t.Fatalf("lastRequest is nil")
	}
	if invoker.lastRequest.ModelSlug != "qwen-image-2.0" {
		t.Fatalf("ModelSlug = %q, want %q", invoker.lastRequest.ModelSlug, "qwen-image-2.0")
	}
	if invoker.lastRequest.Prompt != renderedPrompt {
		t.Fatalf("Prompt = %q, want %q", invoker.lastRequest.Prompt, renderedPrompt)
	}

	if len(fileSaver.remoteURLs) != 1 || fileSaver.remoteURLs[0] != "https://provider.example/generated.png" {
		t.Fatalf("remoteURLs = %#v, want one provider url", fileSaver.remoteURLs)
	}
	if len(fileSaver.binaryData) != 1 || string(fileSaver.binaryData[0]) != "png-binary" {
		t.Fatalf("binaryData = %#v, want decoded base64 payload", fileSaver.binaryData)
	}

	files, ok := result.Outputs["files"].([]*file.File)
	if !ok {
		t.Fatalf("files output type = %T, want []*file.File", result.Outputs["files"])
	}
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}

	urls, ok := result.Outputs["urls"].([]string)
	if !ok {
		t.Fatalf("urls output type = %T, want []string", result.Outputs["urls"])
	}
	if len(urls) != 2 {
		t.Fatalf("len(urls) = %d, want 2", len(urls))
	}
	if urls[0] != "https://internal.example/remote.png" || urls[1] != "https://internal.example/b64.png" {
		t.Fatalf("urls = %#v, want internal signed urls", urls)
	}

	promptVariables, ok := result.Inputs["prompt_variables"].(map[string]any)
	if !ok {
		t.Fatalf("prompt_variables type = %T, want map[string]any", result.Inputs["prompt_variables"])
	}
	if got := promptVariables["start.subject"]; got != subject {
		t.Fatalf("prompt variable start.subject = %#v, want selected variable value", got)
	}
	if got := promptVariables["style"]; got != style {
		t.Fatalf("prompt variable style = %#v, want selected variable value", got)
	}
	if got := result.Inputs["prompt"]; got != renderedPrompt {
		t.Fatalf("prompt = %#v, want rendered prompt", got)
	}
	for _, key := range []string{"model", "generation"} {
		if _, exists := result.Inputs[key]; exists {
			t.Fatalf("input %s should be omitted from frontend input snapshot: %#v", key, result.Inputs)
		}
	}
}

func TestNodeExecuteRun_RuntimeModelConfigPreservesFullModelSlug(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs = map[string]any{
		"model_config": map[string]any{
			"provider": "siliconflow",
			"model":    "Qwen/Qwen-Image",
		},
	}

	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:      ImageModelConfig{Provider: "openai", Name: "gpt-image-1"},
			Prompt:     "draw a cat",
			Generation: GenerationConfig{N: 1},
			Output:     OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if invoker.lastRequest == nil {
		t.Fatalf("lastRequest is nil")
	}
	if invoker.lastRequest.ModelSlug != "Qwen/Qwen-Image" {
		t.Fatalf("ModelSlug = %q, want %q", invoker.lastRequest.ModelSlug, "Qwen/Qwen-Image")
	}
}

func TestNodeResolveModelConfig_InvalidRuntimeOverrideFails(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs = map[string]any{
		"model_config": map[string]any{
			"provider": "qwen",
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:  ImageModelConfig{Provider: "openai", Name: "gpt-image-1"},
			Prompt: "draw a cat",
			Generation: GenerationConfig{
				N:    1,
				Size: "1024x1024",
			},
			Output: OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: &fakeImageInvoker{result: &InvokeResult{}},
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err == nil {
		t.Fatalf("executeRun error = nil, want invalid runtime model_config error")
	}
}

func TestNodeExecuteRun_RuntimeImageGenConfigOverridesGenerationSettings(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs = map[string]any{
		"image_gen_config": map[string]any{
			"aspect_ratio": "16:9",
			"n":            float64(3),
			"quality":      "hd",
		},
	}

	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated-1.png"},
				{URL: "https://provider.example/generated-2.png"},
				{URL: "https://provider.example/generated-3.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:  ImageModelConfig{Provider: "openai", Name: "gpt-image-1"},
			Prompt: "draw a cat",
			Generation: GenerationConfig{
				N:       1,
				Size:    "1024x1024",
				Quality: "standard",
				Style:   "natural",
			},
			Output: OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if invoker.lastRequest == nil {
		t.Fatalf("lastRequest is nil")
	}
	if invoker.lastRequest.Size != "1920x1080" {
		t.Fatalf("Size = %q, want %q", invoker.lastRequest.Size, "1920x1080")
	}
	if invoker.lastRequest.Style != "natural" {
		t.Fatalf("Style = %q, want %q", invoker.lastRequest.Style, "natural")
	}
	if invoker.lastRequest.Quality != "hd" {
		t.Fatalf("Quality = %q, want %q", invoker.lastRequest.Quality, "hd")
	}
	if invoker.lastRequest.N != 3 {
		t.Fatalf("N = %d, want %d", invoker.lastRequest.N, 3)
	}
}

func TestNodeExecuteRun_AllowsProviderToReturnFewerImagesThanRequested(t *testing.T) {
	const requestedImageCount = 3

	vp := entities.NewVariablePool()
	vp.UserInputs = map[string]any{
		"image_gen_config": map[string]any{
			"n": float64(requestedImageCount),
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:  ImageModelConfig{Provider: "openai", Name: "gpt-image-1"},
			Prompt: "draw a cat",
			Generation: GenerationConfig{
				N:    1,
				Size: "1024x1024",
			},
			Output: OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: &fakeImageInvoker{
			result: &InvokeResult{
				Images: []llmadapter.ImageItem{
					{URL: "https://provider.example/generated.png"},
				},
			},
		},
		fileSaver: &fakeFileSaver{},
	}

	result, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if result == nil {
		t.Fatal("executeRun returned nil result")
	}
	files, ok := result.Outputs["files"].([]*file.File)
	if !ok {
		t.Fatalf("outputs.files = %#v, want []*file.File", result.Outputs["files"])
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
}

func TestNodeExecuteRun_RuntimeImageGenConfigRejectsInvalidAspectRatio(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs = map[string]any{
		"image_gen_config": map[string]any{
			"aspect_ratio": "2:3",
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:  ImageModelConfig{Provider: "openai", Name: "gpt-image-1"},
			Prompt: "draw a cat",
			Generation: GenerationConfig{
				N:    1,
				Size: "1024x1024",
			},
			Output: OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: &fakeImageInvoker{result: &InvokeResult{}},
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err == nil {
		t.Fatalf("executeRun error = nil, want invalid runtime image_gen_config error")
	}
	if !strings.Contains(err.Error(), "unsupported runtime image_gen_config.aspect_ratio") {
		t.Fatalf("executeRun error = %q, want unsupported aspect_ratio error", err.Error())
	}
}

func TestNodeExecuteRun_RuntimeImageGenConfigRejectsInvalidImageCount(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs = map[string]any{
		"image_gen_config": map[string]any{
			"n": float64(5),
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:  ImageModelConfig{Provider: "openai", Name: "gpt-image-1"},
			Prompt: "draw a cat",
			Generation: GenerationConfig{
				N:    1,
				Size: "1024x1024",
			},
			Output: OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: &fakeImageInvoker{result: &InvokeResult{}},
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err == nil {
		t.Fatalf("executeRun error = nil, want invalid runtime image_gen_config error")
	}
	if !strings.Contains(err.Error(), "runtime image_gen_config.n must be between 1 and 4") {
		t.Fatalf("executeRun error = %q, want invalid image count error", err.Error())
	}
}

func TestNodeExecuteRun_RuntimeImageGenConfigDoesNotReadFlatStyleInput(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs = map[string]any{
		"style": "vivid",
	}

	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:  ImageModelConfig{Provider: "openai", Name: "gpt-image-1"},
			Prompt: "draw a cat",
			Generation: GenerationConfig{
				N:     1,
				Size:  "1024x1024",
				Style: "natural",
			},
			Output: OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if invoker.lastRequest == nil {
		t.Fatalf("lastRequest is nil")
	}
	if invoker.lastRequest.Style != "natural" {
		t.Fatalf("Style = %q, want %q", invoker.lastRequest.Style, "natural")
	}
}

func TestNodeExecuteRun_RuntimeImageGenConfigIgnoresNestedStyleInput(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.UserInputs = map[string]any{
		"image_gen_config": map[string]any{
			"style": "vivid",
		},
	}

	invoker := &fakeImageInvoker{
		result: &InvokeResult{
			Images: []llmadapter.ImageItem{
				{URL: "https://provider.example/generated.png"},
			},
		},
	}

	node := &Node{
		NodeStruct: baseNodeStructForTest(vp),
		nodeData: NodeData{
			Model:  ImageModelConfig{Provider: "openai", Name: "gpt-image-1"},
			Prompt: "draw a cat",
			Generation: GenerationConfig{
				N:     1,
				Size:  "1024x1024",
				Style: "natural",
			},
			Output: OutputConfig{Lifecycle: "persistent"},
		},
		imageInvoker: invoker,
		fileSaver:    &fakeFileSaver{},
	}

	_, err := node.executeRun(context.Background())
	if err != nil {
		t.Fatalf("executeRun returned error: %v", err)
	}
	if invoker.lastRequest == nil {
		t.Fatalf("lastRequest is nil")
	}
	if invoker.lastRequest.Style != "natural" {
		t.Fatalf("Style = %q, want %q", invoker.lastRequest.Style, "natural")
	}
}

func baseNodeStructForTest(vp *entities.VariablePool) base.NodeStruct {
	return base.NodeStruct{
		NodeID:            "image-node-1",
		NodeType:          shared.ImageGen,
		TenantID:          "tenant-1",
		APPID:             "app-1",
		WorkflowID:        "workflow-1",
		UserID:            "user-1",
		GraphRuntimeState: entities.NewGraphRuntimeState(vp),
	}
}
