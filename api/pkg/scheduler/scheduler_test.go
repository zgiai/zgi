package scheduler

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
)

func TestNewSchedulerRejectsNonPositiveConcurrency(t *testing.T) {
	for _, concurrency := range []int{0, -1} {
		_, err := NewScheduler(&config.Config{TaskQueue: config.TaskQueueConfig{Concurrency: concurrency}})
		require.Error(t, err, "concurrency %d", concurrency)
	}
}

func TestSchedulerQueueWeightIsIndependentOfConcurrency(t *testing.T) {
	for _, concurrency := range []int{1, 8, 64} {
		workerConfig, err := schedulerWorkerConfig(&config.Config{TaskQueue: config.TaskQueueConfig{Concurrency: concurrency}})
		require.NoError(t, err)
		assert.Equal(t, concurrency, workerConfig.Concurrency)
		assert.Equal(t, map[string]int{"scheduler": 1}, workerConfig.Queues)
		assert.NotNil(t, workerConfig.ErrorHandler)
	}
}

// The running server's queue map is what Asynq actually consumes after its own
// normalisation; it must never widen to the default queue.
func TestSchedulerServerOnlyConsumesSchedulerQueue(t *testing.T) {
	redisServer := miniredis.RunT(t)
	host, portText, err := net.SplitHostPort(redisServer.Addr())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	redisOpt := asynq.RedisClientOpt{Addr: redisServer.Addr()}

	s, err := NewScheduler(&config.Config{
		Redis:     config.RedisConfig{Host: host, Port: port},
		TaskQueue: config.TaskQueueConfig{Concurrency: 1},
	})
	require.NoError(t, err)
	client := asynq.NewClient(redisOpt)
	t.Cleanup(func() { _ = client.Close() })
	_, err = client.Enqueue(asynq.NewTask("digital_human:run", nil), asynq.Queue("default"), asynq.TaskID("business-delivery"))
	require.NoError(t, err)

	require.NoError(t, s.server.Start(s.mux))
	t.Cleanup(s.server.Shutdown)

	inspector := asynq.NewInspector(redisOpt)
	t.Cleanup(func() { _ = inspector.Close() })
	var servers []*asynq.ServerInfo
	require.Eventually(t, func() bool {
		servers, err = inspector.Servers()
		return err == nil && len(servers) == 1
	}, 5*time.Second, 50*time.Millisecond)
	assert.Equal(t, map[string]int{"scheduler": 1}, servers[0].Queues)

	// Give the worker time to poll; the business delivery must stay untouched.
	time.Sleep(1500 * time.Millisecond)
	info, err := inspector.GetTaskInfo("default", "business-delivery")
	require.NoError(t, err)
	assert.Equal(t, asynq.TaskStatePending, info.State)
	assert.Zero(t, info.Retried)
}
