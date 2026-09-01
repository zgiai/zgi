package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
)

const (
	ChannelTransientDeliveryTaskType = "digital_human:channel_transient_delivery"
	ChannelProcessingMarkerTaskType  = "digital_human:channel_processing_marker"
	channelTransientTaskTimeout      = 20 * time.Second
	channelTransientTaskLifetime     = 2 * time.Minute
)

// ChannelTransientDeliveryRequest is intentionally carried only by the short-
// lived task queue. It is not a durable delivery intent and is safe to lose.
type ChannelTransientDeliveryRequest struct {
	RunID       uuid.UUID
	Kind        channelruntime.DeliveryKind
	DeliveryKey string
	Sequence    int64
	Text        string
	Delay       time.Duration
}

type ChannelTransientDispatcher interface {
	EnqueueTransient(context.Context, ChannelTransientDeliveryRequest) error
}

// ChannelProcessingMarkerDispatcher is an optional low-latency wake-up for a
// marker already persisted by ChannelService. Correctness comes from the
// durable sweep, so losing this queue task only delays the visual indicator.
type ChannelProcessingMarkerDispatcher interface {
	EnqueueProcessingMarker(context.Context, uuid.UUID) error
}

type AsynqChannelTransientDispatcher struct {
	tasks *queue.TaskManager
	now   func() time.Time
}

func NewAsynqChannelTransientDispatcher(tasks *queue.TaskManager) *AsynqChannelTransientDispatcher {
	if tasks == nil {
		panic("DigitalHuman transient channel dispatcher requires task manager")
	}
	return &AsynqChannelTransientDispatcher{tasks: tasks, now: func() time.Time { return time.Now().UTC() }}
}

type channelTransientTaskPayload struct {
	RunID       string                      `json:"run_id"`
	Kind        channelruntime.DeliveryKind `json:"kind"`
	DeliveryKey string                      `json:"delivery_key"`
	Sequence    int64                       `json:"sequence"`
	Text        string                      `json:"text"`
}

type channelProcessingMarkerTaskPayload struct {
	MarkerID string `json:"marker_id"`
}

func (dispatcher *AsynqChannelTransientDispatcher) EnqueueTransient(_ context.Context, request ChannelTransientDeliveryRequest) error {
	request.DeliveryKey = strings.TrimSpace(request.DeliveryKey)
	request.Text = boundedRunes(strings.TrimSpace(request.Text), maxChannelProgressRunes)
	if request.RunID == uuid.Nil ||
		(request.Kind != channelruntime.DeliveryKindAcknowledgement && request.Kind != channelruntime.DeliveryKindProgress) ||
		request.DeliveryKey == "" || utf8.RuneCountInString(request.DeliveryKey) > 128 || request.Sequence < 0 ||
		request.Text == "" || !utf8.ValidString(request.Text) {
		return ErrInvalidRequest
	}
	payload, err := json.Marshal(channelTransientTaskPayload{
		RunID: request.RunID.String(), Kind: request.Kind, DeliveryKey: request.DeliveryKey,
		Sequence: request.Sequence, Text: request.Text,
	})
	if err != nil {
		return fmt.Errorf("encode DigitalHuman transient channel task: %w", err)
	}
	task := asynq.NewTask(dispatcher.tasks.GetTaskTypeWithPrefix(ChannelTransientDeliveryTaskType), payload)
	options := []asynq.Option{
		asynq.Queue(runQueueName), asynq.MaxRetry(0), asynq.Timeout(channelTransientTaskTimeout),
		asynq.Deadline(dispatcher.now().Add(channelTransientTaskLifetime)),
		asynq.Retention(0), asynq.TaskID(channelTransientTaskID(request.RunID, request.Kind, request.DeliveryKey)),
	}
	if request.Delay > 0 {
		options = append(options, asynq.ProcessIn(request.Delay))
	}
	// Use the client directly so the platform's durable-task retention setting
	// is not applied to UX-only message content.
	if _, err := dispatcher.tasks.GetClient().Enqueue(task, options...); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return fmt.Errorf("enqueue DigitalHuman transient channel task: %w", err)
	}
	return nil
}

func channelTransientTaskID(runID uuid.UUID, kind channelruntime.DeliveryKind, key string) string {
	return "digital-human-channel-transient-" + channelDeliveryStageID(runID, kind, key).String()
}

func (dispatcher *AsynqChannelTransientDispatcher) EnqueueProcessingMarker(_ context.Context, markerID uuid.UUID) error {
	if markerID == uuid.Nil {
		return ErrInvalidRequest
	}
	payload, err := json.Marshal(channelProcessingMarkerTaskPayload{MarkerID: markerID.String()})
	if err != nil {
		return fmt.Errorf("encode DigitalHuman processing marker task: %w", err)
	}
	task := asynq.NewTask(dispatcher.tasks.GetTaskTypeWithPrefix(ChannelProcessingMarkerTaskType), payload)
	_, err = dispatcher.tasks.GetClient().Enqueue(task,
		asynq.Queue(runQueueName), asynq.MaxRetry(0), asynq.Timeout(channelTransientTaskTimeout),
		asynq.Deadline(dispatcher.now().Add(channelTransientTaskLifetime)), asynq.Retention(0),
		asynq.TaskID("digital-human-channel-processing-marker-"+markerID.String()),
	)
	if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return fmt.Errorf("enqueue DigitalHuman processing marker task: %w", err)
	}
	return nil
}

func RegisterChannelTransientTaskHandler(
	registry RunTaskRegistry,
	prefixer interface{ GetTaskTypeWithPrefix(string) string },
	service *ChannelService,
) {
	if registry == nil || prefixer == nil || service == nil {
		panic("DigitalHuman transient channel task handler requires registry, prefixer, and channel service")
	}
	taskType := prefixer.GetTaskTypeWithPrefix(ChannelTransientDeliveryTaskType)
	if !registry.Register(taskType, func(ctx context.Context, task *asynq.Task) error {
		request, err := decodeChannelTransientTask(task)
		if err != nil {
			logger.WarnContext(ctx, "discarding invalid DigitalHuman transient channel task", err)
			return nil
		}
		if err := service.DeliverTransient(ctx, request); err != nil {
			// This path intentionally has no retry or durable failure record. The
			// final answer remains the only authoritative channel delivery.
			logger.WarnContext(ctx, "DigitalHuman transient channel delivery was dropped",
				"run_id", request.RunID.String(), "kind", request.Kind, err)
		}
		return nil
	}) {
		panic("DigitalHuman transient channel task handler is already registered")
	}
	markerTaskType := prefixer.GetTaskTypeWithPrefix(ChannelProcessingMarkerTaskType)
	if !registry.Register(markerTaskType, func(ctx context.Context, task *asynq.Task) error {
		var payload channelProcessingMarkerTaskPayload
		decoder := json.NewDecoder(strings.NewReader(string(task.Payload())))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil {
			logger.WarnContext(ctx, "discarding invalid DigitalHuman processing marker task", err)
			return nil
		}
		markerID, err := uuid.Parse(strings.TrimSpace(payload.MarkerID))
		if err != nil || markerID == uuid.Nil {
			logger.WarnContext(ctx, "discarding invalid DigitalHuman processing marker identity", err)
			return nil
		}
		if err := service.DispatchProcessingMarkers(ctx, defaultDeliveryBatch); err != nil {
			logger.WarnContext(ctx, "DigitalHuman processing marker wake-up failed", "marker_id", markerID.String(), err)
		}
		return nil
	}) {
		panic("DigitalHuman processing marker task handler is already registered")
	}
}

func decodeChannelTransientTask(task *asynq.Task) (ChannelTransientDeliveryRequest, error) {
	if task == nil {
		return ChannelTransientDeliveryRequest{}, ErrInvalidRequest
	}
	var payload channelTransientTaskPayload
	decoder := json.NewDecoder(strings.NewReader(string(task.Payload())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return ChannelTransientDeliveryRequest{}, fmt.Errorf("decode DigitalHuman transient channel task: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ChannelTransientDeliveryRequest{}, errors.New("DigitalHuman transient channel task contains trailing data")
	}
	runID, err := uuid.Parse(strings.TrimSpace(payload.RunID))
	request := ChannelTransientDeliveryRequest{
		RunID: runID, Kind: payload.Kind, DeliveryKey: strings.TrimSpace(payload.DeliveryKey),
		Sequence: payload.Sequence, Text: boundedRunes(strings.TrimSpace(payload.Text), maxChannelProgressRunes),
	}
	if err != nil || request.RunID == uuid.Nil ||
		(request.Kind != channelruntime.DeliveryKindAcknowledgement && request.Kind != channelruntime.DeliveryKindProgress) ||
		request.DeliveryKey == "" || utf8.RuneCountInString(request.DeliveryKey) > 128 || request.Sequence < 0 ||
		request.Text == "" || !utf8.ValidString(request.Text) {
		return ChannelTransientDeliveryRequest{}, ErrInvalidRequest
	}
	return request, nil
}

func (service *ChannelService) DeliverTransient(ctx context.Context, request ChannelTransientDeliveryRequest) error {
	if service == nil || request.RunID == uuid.Nil ||
		(request.Kind != channelruntime.DeliveryKindAcknowledgement && request.Kind != channelruntime.DeliveryKindProgress) {
		return ErrInvalidRequest
	}
	target, err := service.store.RunChannelTransientTarget(ctx, request.RunID)
	if err != nil {
		if errors.Is(err, ErrRunNotFound) {
			return nil
		}
		return err
	}
	if target == nil {
		return nil
	}
	adapter, err := service.adapters.Resolve(target.Provider)
	if err != nil {
		return err
	}
	receipt, err := adapter.Deliver(ctx, channelruntime.DeliveryRequest{
		OrganizationID: target.OrganizationID, WorkspaceID: target.WorkspaceID, ActorID: target.ActorID,
		InstallationID: target.InstallationID, ConnectionID: target.ConnectionID,
		Subject: runtimeidentity.SubjectRef{
			Type: target.SubjectType, ID: target.SubjectID,
			OrganizationID: target.OrganizationID, WorkspaceID: target.WorkspaceID,
		},
		RunID:                  request.RunID,
		DeliveryID:             channelDeliveryStageID(request.RunID, request.Kind, request.DeliveryKey),
		ExternalConversationID: target.ExternalConversationID, ExternalThreadID: target.ExternalThreadID,
		Text: request.Text,
	})
	if err != nil {
		return err
	}
	if receipt == nil || strings.TrimSpace(receipt.ProviderMessageID) == "" {
		return channelruntime.ErrDeliveryOutcomeUnknown
	}
	return nil
}
