package catalogsync

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"

	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/modelmeta"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/logger"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
)

const defaultReconnectDelay = 2 * time.Second

type catalogPublishStream interface {
	Recv() (*pb.CatalogPublishEvent, error)
}

type catalogRPCClient interface {
	WatchCatalogPublishes(ctx context.Context, req *pb.WatchCatalogPublishesRequest) (catalogPublishStream, error)
	GetPublishedCatalog(ctx context.Context, req *pb.GetPublishedCatalogRequest) (*pb.GetPublishedCatalogResponse, error)
}

type grpcCatalogClient struct {
	client pb.CatalogServiceClient
}

func (c *grpcCatalogClient) WatchCatalogPublishes(ctx context.Context, req *pb.WatchCatalogPublishesRequest) (catalogPublishStream, error) {
	return c.client.WatchCatalogPublishes(ctx, req)
}

func (c *grpcCatalogClient) GetPublishedCatalog(ctx context.Context, req *pb.GetPublishedCatalogRequest) (*pb.GetPublishedCatalogResponse, error) {
	return c.client.GetPublishedCatalog(ctx, req)
}

type SyncState struct {
	SyncKey            string     `gorm:"column:sync_key;primaryKey"`
	LastAppliedVersion int64      `gorm:"column:last_applied_version"`
	LastAppliedAt      *time.Time `gorm:"column:last_applied_at"`
	LastError          string     `gorm:"column:last_error"`
	CreatedAt          time.Time  `gorm:"column:created_at"`
	UpdatedAt          time.Time  `gorm:"column:updated_at"`
}

func (SyncState) TableName() string {
	return "llm_catalog_sync_states"
}

const defaultSyncKey = "platform_catalog"

type Synchronizer struct {
	db             *gorm.DB
	grpcAddr       string
	newClient      func(addr string) (catalogRPCClient, io.Closer, error)
	reconnectDelay time.Duration
}

func NewSynchronizer(db *gorm.DB, grpcAddr string) *Synchronizer {
	return &Synchronizer{
		db:             db,
		grpcAddr:       strings.TrimSpace(grpcAddr),
		newClient:      newGRPCCatalogClient,
		reconnectDelay: defaultReconnectDelay,
	}
}

func newSynchronizerWithClient(db *gorm.DB, client catalogRPCClient) *Synchronizer {
	return &Synchronizer{
		db:       db,
		grpcAddr: "in-memory",
		newClient: func(string) (catalogRPCClient, io.Closer, error) {
			return client, noopCloser{}, nil
		},
		reconnectDelay: 10 * time.Millisecond,
	}
}

func (s *Synchronizer) Start(ctx context.Context) error {
	if s == nil || s.db == nil || s.grpcAddr == "" {
		return nil
	}

	client, closer, stream, err := s.connect(ctx)
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return err
	}

	go s.run(ctx, client, closer, stream)
	return nil
}

func (s *Synchronizer) run(ctx context.Context, client catalogRPCClient, closer io.Closer, stream catalogPublishStream) {
	currentClient := client
	currentCloser := closer
	currentStream := stream

	for {
		err := s.consumeStream(ctx, currentClient, currentStream)
		if currentCloser != nil {
			_ = currentCloser.Close()
		}
		if ctx.Err() != nil {
			return
		}

		logger.WarnContext(ctx, "Catalog watch stream ended, reconnecting", zap.Error(err))
		for {
			if !sleepContext(ctx, s.reconnectDelay) {
				return
			}

			currentClient, currentCloser, currentStream, err = s.connect(ctx)
			if err == nil {
				break
			}
			if currentCloser != nil {
				_ = currentCloser.Close()
			}
			logger.WarnContext(ctx, "Catalog synchronizer reconnect failed",
				zap.String("grpc_addr", s.grpcAddr),
				zap.Error(err),
			)
			if ctx.Err() != nil {
				return
			}
		}
	}
}

func (s *Synchronizer) connect(ctx context.Context) (catalogRPCClient, io.Closer, catalogPublishStream, error) {
	client, closer, err := s.newClient(s.grpcAddr)
	if err != nil {
		return nil, closer, nil, err
	}

	lastSeenVersion, err := s.loadLastAppliedVersion(ctx)
	if err != nil {
		return nil, closer, nil, err
	}

	stream, err := client.WatchCatalogPublishes(ctx, &pb.WatchCatalogPublishesRequest{
		LastSeenVersion: lastSeenVersion,
	})
	if err != nil {
		return nil, closer, nil, err
	}

	return client, closer, stream, nil
}

func (s *Synchronizer) consumeStream(ctx context.Context, client catalogRPCClient, stream catalogPublishStream) error {
	eventCh := make(chan *pb.CatalogPublishEvent, 1)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		for {
			event, err := stream.Recv()
			if err != nil {
				errCh <- err
				return
			}
			select {
			case eventCh <- event:
			default:
				select {
				case <-eventCh:
				default:
				}
				eventCh <- event
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case event, ok := <-eventCh:
			if !ok {
				select {
				case err := <-errCh:
					return err
				default:
					return io.EOF
				}
			}
			latest := latestCatalogEvent(event, eventCh)
			if latest == nil || latest.GetVersion() <= 0 {
				continue
			}
			if err := s.applyPublishedVersion(ctx, client, latest); err != nil {
				return err
			}
		}
	}
}

func (s *Synchronizer) applyPublishedVersion(ctx context.Context, client catalogRPCClient, event *pb.CatalogPublishEvent) error {
	resp, err := client.GetPublishedCatalog(ctx, &pb.GetPublishedCatalogRequest{Version: event.GetVersion()})
	if err != nil {
		return err
	}

	catalog := modelmeta.PublishedCatalog{
		Version:     resp.GetVersion(),
		PublishedAt: time.UnixMilli(resp.GetPublishedAt()).UTC(),
		Providers:   make([]modelmeta.PublishedProvider, 0, len(resp.GetProviders())),
		Models:      make([]modelmeta.PublishedModel, 0, len(resp.GetModels())),
	}

	for _, provider := range resp.GetProviders() {
		catalog.Providers = append(catalog.Providers, modelmeta.PublishedProvider{
			Provider:        provider.GetProvider(),
			ProviderName:    provider.GetProviderName(),
			Description:     provider.GetDescription(),
			Tagline:         provider.GetTagline(),
			LogoURL:         provider.GetLogoUrl(),
			Website:         provider.GetWebsite(),
			APIDocsURL:      provider.GetApiDocsUrl(),
			PricingURL:      provider.GetPricingUrl(),
			CountryCode:     provider.GetCountryCode(),
			FoundedYear:     int(provider.GetFoundedYear()),
			Status:          provider.GetStatus(),
			IsActive:        provider.GetIsActive(),
			IsSystemEnabled: provider.GetIsSystemEnabled(),
			Metadata:        decodeJSONMap(provider.GetMetadataJson()),
		})
	}

	for _, model := range resp.GetModels() {
		catalog.Models = append(catalog.Models, modelmeta.PublishedModel{
			Provider:               model.GetProvider(),
			Model:                  model.GetModel(),
			ModelName:              model.GetModelName(),
			Type:                   model.GetType(),
			Family:                 model.GetFamily(),
			FamilyName:             model.GetFamilyName(),
			Status:                 model.GetStatus(),
			Tagline:                model.GetTagline(),
			IsFlagship:             model.GetIsFlagship(),
			IsRecommended:          model.GetIsRecommended(),
			IsFeatured:             model.GetIsFeatured(),
			IsNew:                  model.GetIsNew(),
			AccessType:             model.GetAccessType(),
			Currency:               model.GetCurrency(),
			ContextWindow:          int(model.GetContextWindow()),
			MaxOutputTokens:        int(model.GetMaxOutputTokens()),
			InputPrice:             model.GetInputPrice(),
			OutputPrice:            model.GetOutputPrice(),
			CachedInputPrice:       model.GetCachedInputPrice(),
			UseCases:               model.GetUseCases(),
			InputModalities:        model.GetInputModalities(),
			OutputModalities:       model.GetOutputModalities(),
			KnowledgeCutoff:        model.GetKnowledgeCutoff(),
			IsActive:               model.GetIsActive(),
			IsSystemEnabled:        model.GetIsSystemEnabled(),
			SupportedParameters:    json.RawMessage(model.GetSupportedParametersJson()),
			ConfigParameters:       normalizeConfigParametersPayload(model.GetConfigParametersJson()),
			Endpoints:              publishedModelEndpoints(model.GetEndpoints()),
			EndpointsAuthoritative: model.GetEndpoints() != nil,
			Features:               publishedModelFeatures(model.GetFeatures()),
			Tools:                  publishedModelTools(model.GetTools()),
			Parameters:             publishedModelParameters(model.GetParameters()),
		})
	}

	service := modelmeta.NewService(s.db)
	if err := service.ApplyPublishedCatalog(ctx, catalog); err != nil {
		_ = service.RecordPublishedCatalogSyncError(ctx, err.Error())
		return err
	}

	return nil
}

func publishedModelEndpoints(source *pb.CatalogModelEndpoints) *llmmodel.ModelEndpoints {
	if source == nil {
		return nil
	}
	return &llmmodel.ModelEndpoints{
		ChatCompletions:  source.GetChatCompletions(),
		Responses:        source.GetResponses(),
		Realtime:         source.GetRealtime(),
		Assistants:       source.GetAssistants(),
		Batch:            source.GetBatch(),
		Embeddings:       source.GetEmbeddings(),
		FineTuning:       source.GetFineTuning(),
		ImageGeneration:  source.GetImageGeneration(),
		Vision:           source.GetVision(),
		SpeechGeneration: source.GetSpeechGeneration(),
		Transcription:    source.GetTranscription(),
		Translation:      source.GetTranslation(),
		Moderation:       source.GetModeration(),
		Videos:           source.GetVideos(),
		ImageEdit:        source.GetImageEdit(),
	}
}

func publishedModelFeatures(source *pb.CatalogModelFeatures) *llmmodel.ModelFeatures {
	if source == nil {
		return nil
	}
	return &llmmodel.ModelFeatures{
		Streaming:        source.GetStreaming(),
		FunctionCalling:  source.GetFunctionCalling(),
		StructuredOutput: source.GetStructuredOutput(),
		JsonMode:         source.GetJsonMode(),
		Distillation:     source.GetDistillation(),
		Reasoning:        source.GetReasoning(),
		SystemPrompt:     source.GetSystemPrompt(),
		Logprobs:         source.GetLogprobs(),
		WebSearch:        source.GetWebSearch(),
		FileSearch:       source.GetFileSearch(),
		CodeInterpreter:  source.GetCodeInterpreter(),
		ComputerUse:      source.GetComputerUse(),
		Mcp:              source.GetMcp(),
		ReasoningEffort:  source.GetReasoningEffort(),
		Attachment:       source.GetAttachment(),
	}
}

func publishedModelTools(source *pb.CatalogModelTools) *llmmodel.ModelTools {
	if source == nil {
		return nil
	}
	return &llmmodel.ModelTools{
		WebSearch:         source.GetWebSearch(),
		FileSearch:        source.GetFileSearch(),
		ImageGeneration:   source.GetImageGeneration(),
		CodeInterpreter:   source.GetCodeInterpreter(),
		ComputerUse:       source.GetComputerUse(),
		Mcp:               source.GetMcp(),
		ParallelToolCalls: source.GetParallelToolCalls(),
	}
}

func publishedModelParameters(source *pb.CatalogModelParameters) *llmmodel.ModelParameters {
	if source == nil {
		return nil
	}
	return &llmmodel.ModelParameters{
		SupportsTemperature:      source.GetTemperature(),
		SupportsTopP:             source.GetTopP(),
		SupportsPresencePenalty:  source.GetPresencePenalty(),
		SupportsFrequencyPenalty: source.GetFrequencyPenalty(),
		SupportsLogitBias:        source.GetLogitBias(),
		SupportsSeed:             source.GetSeed(),
		SupportsStop:             source.GetStop(),
		MaxStopSequences:         int(source.GetMaxStopSequences()),
	}
}

func (s *Synchronizer) loadLastAppliedVersion(ctx context.Context) (int64, error) {
	var states []SyncState
	err := s.db.WithContext(ctx).
		Where("sync_key = ?", defaultSyncKey).
		Limit(1).
		Find(&states).Error
	if err != nil {
		return 0, err
	}
	if len(states) == 0 {
		return 0, nil
	}
	return states[0].LastAppliedVersion, nil
}

func newGRPCCatalogClient(addr string) (catalogRPCClient, io.Closer, error) {
	dialOptions := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	dialOptions = append(dialOptions, observability.GRPCDialOptions()...)
	conn, err := grpc.Dial(addr, dialOptions...)
	if err != nil {
		return nil, nil, err
	}
	return &grpcCatalogClient{client: pb.NewCatalogServiceClient(conn)}, conn, nil
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func decodeJSONMap(raw string) map[string]interface{} {
	if strings.TrimSpace(raw) == "" {
		return map[string]interface{}{}
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return map[string]interface{}{}
	}
	if decoded == nil {
		return map[string]interface{}{}
	}
	return decoded
}

func normalizeConfigParametersPayload(raw string) json.RawMessage {
	params, err := llmmodel.NormalizeConfigParametersJSON([]byte(raw))
	if err != nil {
		return json.RawMessage("[]")
	}

	data, err := json.Marshal(params)
	if err != nil {
		return json.RawMessage("[]")
	}
	return data
}

func latestCatalogEvent(first *pb.CatalogPublishEvent, eventCh <-chan *pb.CatalogPublishEvent) *pb.CatalogPublishEvent {
	latest := first
	for {
		select {
		case next, ok := <-eventCh:
			if !ok {
				return latest
			}
			if next != nil {
				latest = next
			}
		default:
			return latest
		}
	}
}
