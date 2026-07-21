package imagegenerator

import (
	"context"
	"encoding/base64"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	workflowtoolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	defaultmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	llmsharedtypes "github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type fakeGenerationFileService struct {
	file *dto.UploadFile
	url  string
}

func (f *fakeGenerationFileService) GetUploadConfig() *interfaces.FileUploadConfigResponse {
	return &interfaces.FileUploadConfigResponse{ImageFileSizeLimit: 10}
}

func (f *fakeGenerationFileService) GetFileByID(ctx context.Context, fileID string) (*dto.UploadFile, error) {
	_ = ctx
	if f.file == nil {
		return nil, nil
	}
	file := *f.file
	file.ID = fileID
	return &file, nil
}

func (f *fakeGenerationFileService) GetFileURL(ctx context.Context, fileID string) (string, error) {
	_ = ctx
	_ = fileID
	if f.url != "" {
		return f.url, nil
	}
	return "https://files.example.test/reference.png", nil
}

type fakeImageModels struct{}

func (f *fakeImageModels) ResolveModelType(ctx context.Context, organizationID string, explicitProvider, explicitModel *string, modelType sharedmodel.ModelType) (*defaultmodelservice.ResolvedModel, error) {
	_ = ctx
	_ = organizationID
	_ = modelType
	return resolvedImage(explicitProvider, explicitModel), nil
}

func (f *fakeImageModels) ResolveUseCase(ctx context.Context, organizationID string, useCase llmmodelmodel.UseCase, explicitProvider, explicitModel *string) (*defaultmodelservice.ResolvedModel, error) {
	_ = ctx
	_ = organizationID
	_ = useCase
	return resolvedImage(explicitProvider, explicitModel), nil
}

func resolvedImage(explicitProvider, explicitModel *string) *defaultmodelservice.ResolvedModel {
	provider := "openai"
	model := "gpt-image-1"
	source := defaultmodelservice.SourceAuto
	if explicitProvider != nil && strings.TrimSpace(*explicitProvider) != "" {
		provider = strings.TrimSpace(*explicitProvider)
		source = defaultmodelservice.SourceExplicit
	}
	if explicitModel != nil && strings.TrimSpace(*explicitModel) != "" {
		model = strings.TrimSpace(*explicitModel)
		source = defaultmodelservice.SourceExplicit
	}
	return &defaultmodelservice.ResolvedModel{
		UseCase:  string(llmmodelmodel.UseCaseImageGen),
		Provider: provider,
		Model:    model,
		Params:   llmsharedtypes.JSONObject{},
		Source:   source,
	}
}

type fakeImageLLMClient struct {
	lastAppCtx *llmclient.AppContext
	lastReq    *adapter.ImageRequest
	callCount  int
	response   *adapter.ImageResponse
}

func (f *fakeImageLLMClient) Chat(context.Context, string, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, nil
}

func (f *fakeImageLLMClient) ChatStream(context.Context, string, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, nil
}

func (f *fakeImageLLMClient) CreateResponse(context.Context, string, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, nil
}

func (f *fakeImageLLMClient) Embed(context.Context, string, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, nil
}

func (f *fakeImageLLMClient) CreateImage(context.Context, string, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, nil
}

func (f *fakeImageLLMClient) Rerank(context.Context, string, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, nil
}

func (f *fakeImageLLMClient) AppChat(context.Context, *llmclient.AppContext, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, nil
}

func (f *fakeImageLLMClient) AppChatStream(context.Context, *llmclient.AppContext, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, nil
}

func (f *fakeImageLLMClient) AppCreateResponse(context.Context, *llmclient.AppContext, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, nil
}

func (f *fakeImageLLMClient) AppEmbed(context.Context, *llmclient.AppContext, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, nil
}

func (f *fakeImageLLMClient) AppCreateImage(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	_ = ctx
	f.lastAppCtx = appCtx
	f.lastReq = req
	f.callCount++
	if f.response != nil {
		return f.response, nil
	}
	return &adapter.ImageResponse{
		Data: []adapter.ImageItem{{
			B64JSON:       base64.StdEncoding.EncodeToString(fakePNGBytes),
			RevisedPrompt: "revised prompt",
		}},
	}, nil
}

func (f *fakeImageLLMClient) AppRerank(context.Context, *llmclient.AppContext, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, nil
}

func TestProviderExposesImageGenerationTools(t *testing.T) {
	provider := NewProvider(&fakeGenerationFileService{}, &fakeImageLLMClient{}, &fakeImageModels{})
	entity := provider.GetEntity()
	require.Equal(t, ProviderID, entity.Identity.Name)
	_, err := provider.GetTool("generate_image")
	require.NoError(t, err)
	_, err = provider.GetTool("edit_image")
	require.NoError(t, err)
}

func TestGenerateImageReturnsDownloadableFileMetadata(t *testing.T) {
	db, mock, cleanup := openImageGeneratorMockDB(t)
	defer cleanup()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "tool_files"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	fileStorage := installImageGeneratorToolFileGlobals(t, db)
	llm := &fakeImageLLMClient{}
	tool := NewGenerateImageTool("tenant-1", &fakeGenerationFileService{}, llm, &fakeImageModels{})
	tool = tool.ForkToolRuntime(&tools.ToolRuntime{TenantID: "tenant-1"}).(*GenerateImageTool)

	conversationID := uuid.NewString()
	appID := "agent-1"
	messages, err := tool.Invoke(
		context.Background(),
		"user-1",
		map[string]interface{}{
			"prompt":       "A clean product concept image of a smart desk lamp",
			"style":        "product",
			"aspect_ratio": "16:9",
			"count":        float64(1),
			"filename":     "desk-lamp",
		},
		&conversationID,
		&appID,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, tools.ToolInvokeMessageTypeFile, messages[0].Type)
	require.Equal(t, tools.ToolInvokeMessageTypeJSON, messages[1].Type)

	payload := messages[1].Data
	require.Equal(t, 1, payload["count"])
	require.Equal(t, "16:9", payload["aspect_ratio"])
	require.Equal(t, "product", payload["style"])
	require.Equal(t, "png", payload["format"])
	require.Equal(t, defaultImageMIME, payload["mime_type"])
	require.Equal(t, "gpt-image-1", payload["model"])
	require.Equal(t, "openai", payload["model_provider"])
	require.NotEmpty(t, payload["files"])

	require.NotNil(t, llm.lastAppCtx)
	require.Equal(t, "tenant-1", llm.lastAppCtx.OrganizationID)
	require.Equal(t, llmclient.BillingSubjectTypeOrganization, llm.lastAppCtx.BillingSubjectType)
	require.Equal(t, "agent-1", llm.lastAppCtx.AppID)

	require.NotNil(t, llm.lastReq)
	require.Equal(t, "gpt-image-1", llm.lastReq.Model)
	require.Equal(t, "1792x1024", llm.lastReq.Size)
	require.Equal(t, "url", llm.lastReq.ResponseFormat)
	require.NotNil(t, llm.lastReq.N)
	require.Equal(t, 1, *llm.lastReq.N)
	require.Contains(t, llm.lastReq.Prompt, "smart desk lamp")
	require.Contains(t, llm.lastReq.Prompt, "product-focused")

	require.Equal(t, fakePNGBytes, fileStorage.onlyFileData(t))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateImageRejectsInvalidCount(t *testing.T) {
	llm := &fakeImageLLMClient{}
	tool := NewGenerateImageTool("tenant-1", &fakeGenerationFileService{}, llm, &fakeImageModels{})
	_, err := tool.Invoke(context.Background(), "user-1", map[string]interface{}{
		"prompt": "A small icon",
		"count":  5,
	}, nil, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "count must be between 1 and 4")
	require.Equal(t, 0, llm.callCount)
}

func TestEditImageRequiresImage(t *testing.T) {
	llm := &fakeImageLLMClient{}
	tool := NewEditImageTool("tenant-1", &fakeGenerationFileService{}, llm, &fakeImageModels{})
	_, err := tool.Invoke(context.Background(), "user-1", map[string]interface{}{
		"edit_instruction": "Change the background to a clean studio scene",
	}, nil, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "image file object is required")
	require.Equal(t, 0, llm.callCount)
}

func TestEditImageRejectsSameTenantFileOwnedByAnotherUser(t *testing.T) {
	llm := &fakeImageLLMClient{}
	tool := NewEditImageTool(
		"tenant-1",
		&fakeGenerationFileService{file: &dto.UploadFile{Name: "product.png", Extension: ".png", MimeType: "image/png", Size: 1024, OrganizationID: "tenant-1", CreatedBy: "other-user"}},
		llm,
		&fakeImageModels{},
	)
	_, err := tool.Invoke(context.Background(), "user-1", map[string]interface{}{
		"image": map[string]interface{}{
			"upload_file_id": "file-1",
		},
		"edit_instruction": "Change the background to a bright office scene",
	}, nil, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not accessible")
	require.Equal(t, 0, llm.callCount)
}

func TestEditImageUsesReferenceImageInPrompt(t *testing.T) {
	db, mock, cleanup := openImageGeneratorMockDB(t)
	defer cleanup()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "tool_files"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	installImageGeneratorToolFileGlobals(t, db)
	llm := &fakeImageLLMClient{}
	tool := NewEditImageTool(
		"tenant-1",
		&fakeGenerationFileService{file: &dto.UploadFile{Name: "product.png", Extension: ".png", MimeType: "image/png", Size: 1024, OrganizationID: "tenant-1", CreatedBy: "user-1"}},
		llm,
		&fakeImageModels{},
	)
	messages, err := tool.Invoke(context.Background(), "user-1", map[string]interface{}{
		"image": map[string]interface{}{
			"upload_file_id": "file-1",
		},
		"edit_instruction": "Change the background to a bright office scene",
		"edit_type":        "background",
	}, nil, nil, nil)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.NotNil(t, llm.lastReq)
	require.Contains(t, llm.lastReq.Prompt, "bright office scene")
	require.Contains(t, llm.lastReq.Prompt, "Edit type: background")
	require.Contains(t, llm.lastReq.Prompt, "product.png")
	require.Contains(t, llm.lastReq.Prompt, "https://files.example.test/reference.png")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGenerateImageRejectsNonImageResult(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("<html>not an image</html>"))
	llm := &fakeImageLLMClient{response: &adapter.ImageResponse{Data: []adapter.ImageItem{{B64JSON: encoded}}}}
	tool := NewGenerateImageTool("tenant-1", &fakeGenerationFileService{}, llm, &fakeImageModels{})
	_, err := tool.Invoke(context.Background(), "user-1", map[string]interface{}{
		"prompt": "A small icon",
	}, nil, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a supported image")
	require.Equal(t, 1, llm.callCount)
}

type imageGeneratorMemoryStorage struct {
	files map[string][]byte
}

func newImageGeneratorMemoryStorage() *imageGeneratorMemoryStorage {
	return &imageGeneratorMemoryStorage{files: make(map[string][]byte)}
}

func (s *imageGeneratorMemoryStorage) Save(filename string, data []byte) error {
	s.files[filename] = append([]byte(nil), data...)
	return nil
}

func (s *imageGeneratorMemoryStorage) Load(filename string) ([]byte, error) {
	data, ok := s.files[filename]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (s *imageGeneratorMemoryStorage) LoadStream(filename string) (<-chan []byte, error) {
	data, err := s.Load(filename)
	if err != nil {
		return nil, err
	}
	ch := make(chan []byte, 1)
	ch <- data
	close(ch)
	return ch, nil
}

func (s *imageGeneratorMemoryStorage) Download(filename string, targetPath string) error {
	return nil
}

func (s *imageGeneratorMemoryStorage) Exists(filename string) (bool, error) {
	_, ok := s.files[filename]
	return ok, nil
}

func (s *imageGeneratorMemoryStorage) Delete(filename string) error {
	delete(s.files, filename)
	return nil
}

func (s *imageGeneratorMemoryStorage) List(prefix string) ([]storage.FileInfo, error) {
	return nil, nil
}

func (s *imageGeneratorMemoryStorage) onlyFileData(t *testing.T) []byte {
	t.Helper()
	require.Len(t, s.files, 1)
	for _, data := range s.files {
		return append([]byte(nil), data...)
	}
	return nil
}

func installImageGeneratorToolFileGlobals(t *testing.T, db *gorm.DB) *imageGeneratorMemoryStorage {
	t.Helper()
	oldManager := workflowtoolfile.GlobalToolFileManager
	oldSignature := workflowtoolfile.GlobalFileSignature
	t.Cleanup(func() {
		workflowtoolfile.GlobalToolFileManager = oldManager
		workflowtoolfile.GlobalFileSignature = oldSignature
	})

	fileStorage := newImageGeneratorMemoryStorage()
	workflowtoolfile.GlobalToolFileManager = workflowtoolfile.NewToolFileManager(db, fileStorage)
	workflowtoolfile.GlobalFileSignature = workflowtoolfile.NewFileSignature(&config.Config{
		App: config.AppConfig{
			SecretKey:          "test-secret-key",
			FilesURL:           "http://files.example.test",
			FilesAccessTimeout: 3600,
		},
	})
	return fileStorage
}

func openImageGeneratorMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	mock.MatchExpectationsInOrder(false)

	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func requireDownloadURL(t *testing.T, raw string) {
	t.Helper()
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "1", parsed.Query().Get("download"))
}

var fakePNGBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}
