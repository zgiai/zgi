package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

const (
	workflowCommittedTailLimit     = int64(512)
	workflowCommittedTailTTL       = time.Hour
	workflowCommittedTailTimeout   = 25 * time.Millisecond
	workflowCommittedTailChannel   = "workflow:runtime:events:v2"
	workflowCommittedTailQueueSize = 256
)

type workflowCommittedTailPublishRequest struct {
	workflowRunID string
	arguments     []interface{}
}

var workflowCommittedTailPublisher = struct {
	sync.Once
	queue chan workflowCommittedTailPublishRequest
}{queue: make(chan workflowCommittedTailPublishRequest, workflowCommittedTailQueueSize)}

var publishWorkflowCommittedTailScript = goredis.NewScript(`
local added = 0
for index = 4, #ARGV, 2 do
  local score = ARGV[index]
  local payload = ARGV[index + 1]
  local existing = redis.call('ZRANGEBYSCORE', KEYS[1], score, score)
  if #existing == 0 then
    redis.call('ZADD', KEYS[1], score, payload)
    added = added + 1
  elseif #existing > 1 then
    redis.call('ZREMRANGEBYSCORE', KEYS[1], score, score)
    redis.call('ZADD', KEYS[1], score, existing[1])
  end
end
local count = redis.call('ZCARD', KEYS[1])
local limit = tonumber(ARGV[1])
if count > limit then
  redis.call('ZREMRANGEBYRANK', KEYS[1], 0, count - limit - 1)
end
redis.call('EXPIRE', KEYS[1], tonumber(ARGV[2]))
if added > 0 then
  redis.call('PUBLISH', KEYS[2], ARGV[3])
end
return added
`)

func publishWorkflowCommittedTail(ctx context.Context, workflowRunID string, events ...*workflowpause.RunEventPayload) {
	if redisutil.GetClient() == nil || strings.TrimSpace(workflowRunID) == "" || len(events) == 0 {
		return
	}
	request, ok := prepareWorkflowCommittedTailPublish(workflowRunID, events)
	if !ok {
		return
	}
	workflowCommittedTailPublisher.Do(func() { go runWorkflowCommittedTailPublisher() })
	select {
	case workflowCommittedTailPublisher.queue <- request:
		recordWorkflowRedisTailPublishQueued(ctx, "queued")
	default:
		// Redis is only a committed-event cache. Dropping a cache publication is
		// preferable to applying backpressure to the durable PostgreSQL path.
		recordWorkflowRedisTailPublishQueued(ctx, "dropped")
	}
}

func publishWorkflowCommittedTailWindow(ctx context.Context, pauseService *workflowpause.Service, tenantID, workflowRunID string, throughSequence int) bool {
	if pauseService == nil || throughSequence <= 0 || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(workflowRunID) == "" {
		return false
	}
	const windowSize = 64
	afterSequence := throughSequence - windowSize
	if afterSequence < 0 {
		afterSequence = 0
	}
	payload, err := pauseService.ListEvents(ctx, tenantID, workflowRunID, afterSequence, windowSize)
	if err != nil || payload == nil || len(payload.Events) == 0 {
		return false
	}
	events := make([]*workflowpause.RunEventPayload, 0, len(payload.Events))
	for index := range payload.Events {
		event := payload.Events[index]
		if event.Sequence > throughSequence {
			break
		}
		events = append(events, &event)
	}
	publishWorkflowCommittedTail(ctx, workflowRunID, events...)
	return len(events) > 0
}

func prepareWorkflowCommittedTailPublish(workflowRunID string, events []*workflowpause.RunEventPayload) (workflowCommittedTailPublishRequest, bool) {
	arguments := make([]interface{}, 0, 3+len(events)*2)
	arguments = append(arguments, workflowCommittedTailLimit, int64(workflowCommittedTailTTL/time.Second), workflowRunID)
	lastSequence := 0
	for _, event := range events {
		if event == nil || event.Sequence <= 0 || event.SchemaVersion < 2 {
			continue
		}
		payload, err := json.Marshal(event)
		if err != nil {
			continue
		}
		arguments = append(arguments, event.Sequence, string(payload))
		if event.Sequence > lastSequence {
			lastSequence = event.Sequence
		}
	}
	if len(arguments) == 3 {
		return workflowCommittedTailPublishRequest{}, false
	}
	arguments[2] = fmt.Sprintf("%s:%d", workflowRunID, lastSequence)
	return workflowCommittedTailPublishRequest{workflowRunID: workflowRunID, arguments: arguments}, true
}

func runWorkflowCommittedTailPublisher() {
	for request := range workflowCommittedTailPublisher.queue {
		publishWorkflowCommittedTailNow(request)
	}
}

func publishWorkflowCommittedTailNow(request workflowCommittedTailPublishRequest) {
	client := redisutil.GetClient()
	if client == nil {
		recordWorkflowRedisTailPublish(context.Background(), fmt.Errorf("redis unavailable"), 0)
		return
	}
	writeCtx, cancel := context.WithTimeout(context.Background(), workflowCommittedTailTimeout)
	defer cancel()
	startedAt := time.Now()
	_, err := publishWorkflowCommittedTailScript.Run(writeCtx, client,
		[]string{workflowCommittedTailKey(request.workflowRunID), workflowCommittedTailChannel}, request.arguments...).Result()
	recordWorkflowRedisTailPublish(writeCtx, err, time.Since(startedAt))
}

func readWorkflowCommittedTailAfter(ctx context.Context, workflowRunID string, afterSequence, limit int) ([]workflowpause.RunEventPayload, bool) {
	client := redisutil.GetClient()
	if client == nil || strings.TrimSpace(workflowRunID) == "" || limit <= 0 {
		recordWorkflowRedisTailRead(ctx, "unavailable")
		return nil, false
	}
	readCtx, cancel := context.WithTimeout(ctx, workflowCommittedTailTimeout)
	defer cancel()
	values, err := client.ZRangeByScore(readCtx, workflowCommittedTailKey(workflowRunID), &goredis.ZRangeBy{
		Min: "(" + strconv.Itoa(afterSequence),
		Max: "+inf",
	}).Result()
	if err != nil || len(values) == 0 {
		recordWorkflowRedisTailRead(ctx, "miss")
		return nil, false
	}
	eventsBySequence := make(map[int]workflowpause.RunEventPayload, len(values))
	for _, value := range values {
		var event workflowpause.RunEventPayload
		if err := json.Unmarshal([]byte(value), &event); err != nil || event.Sequence <= afterSequence {
			recordWorkflowRedisTailRead(ctx, "invalid")
			return nil, false
		}
		eventsBySequence[event.Sequence] = event
	}
	events := make([]workflowpause.RunEventPayload, 0, len(eventsBySequence))
	for _, event := range eventsBySequence {
		events = append(events, event)
	}
	sort.Slice(events, func(left, right int) bool { return events[left].Sequence < events[right].Sequence })
	if len(events) > limit {
		events = events[:limit]
	}
	expected := afterSequence + 1
	for _, event := range events {
		if event.Sequence != expected {
			recordWorkflowRedisTailRead(ctx, "gap")
			return nil, false
		}
		expected++
	}
	recordWorkflowRedisTailRead(ctx, "hit")
	return events, true
}

func workflowCommittedTailKey(workflowRunID string) string {
	return "workflow:run:" + workflowRunID + ":events:v2"
}

func parseWorkflowCommittedTailSignal(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	index := strings.LastIndexByte(message, ':')
	if index <= 0 {
		return message
	}
	return message[:index]
}
