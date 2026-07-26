package workflow

import (
	"context"
	"database/sql/driver"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/zgiai/zgi/api/pkg/logger"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/gorm"
)

const workflowRuntimeEventNotifyChannel = "workflow_runtime_events"

var workflowRuntimeNotifications = struct {
	sync.RWMutex
	subscribers map[string]map[chan struct{}]struct{}
}{subscribers: map[string]map[chan struct{}]struct{}{}}

var workflowRuntimeListenerOnce sync.Once
var workflowRuntimeRedisListenerOnce sync.Once

func subscribeWorkflowRuntimeEvents(db *gorm.DB, workflowRunID string) (<-chan struct{}, func()) {
	signal := make(chan struct{}, 1)
	workflowRuntimeNotifications.Lock()
	if workflowRuntimeNotifications.subscribers[workflowRunID] == nil {
		workflowRuntimeNotifications.subscribers[workflowRunID] = map[chan struct{}]struct{}{}
	}
	workflowRuntimeNotifications.subscribers[workflowRunID][signal] = struct{}{}
	workflowRuntimeNotifications.Unlock()

	if db != nil && db.Dialector.Name() == "postgres" {
		workflowRuntimeListenerOnce.Do(func() { go listenWorkflowRuntimeEvents(db) })
	}
	if redisutil.GetClient() != nil {
		workflowRuntimeRedisListenerOnce.Do(func() { go listenWorkflowRuntimeRedisEvents() })
	}
	return signal, func() {
		workflowRuntimeNotifications.Lock()
		delete(workflowRuntimeNotifications.subscribers[workflowRunID], signal)
		if len(workflowRuntimeNotifications.subscribers[workflowRunID]) == 0 {
			delete(workflowRuntimeNotifications.subscribers, workflowRunID)
		}
		workflowRuntimeNotifications.Unlock()
	}
}

func listenWorkflowRuntimeRedisEvents() {
	for {
		client := redisutil.GetClient()
		if client == nil {
			time.Sleep(time.Second)
			continue
		}
		pubsub := client.Subscribe(context.Background(), workflowCommittedTailChannel)
		for message := range pubsub.Channel() {
			if workflowRunID := parseWorkflowCommittedTailSignal(message.Payload); workflowRunID != "" {
				publishWorkflowRuntimeEventSignal(workflowRunID)
			}
		}
		_ = pubsub.Close()
		time.Sleep(time.Second)
	}
}

func publishWorkflowRuntimeEventSignal(workflowRunID string) {
	workflowRuntimeNotifications.RLock()
	defer workflowRuntimeNotifications.RUnlock()
	for signal := range workflowRuntimeNotifications.subscribers[workflowRunID] {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
}

func listenWorkflowRuntimeEvents(db *gorm.DB) {
	for {
		if err := listenWorkflowRuntimeEventsConnection(db); err != nil {
			logger.Warn("workflow runtime event listener disconnected", "error", err)
		}
		time.Sleep(time.Second)
	}
}

func listenWorkflowRuntimeEventsConnection(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "LISTEN "+workflowRuntimeEventNotifyChannel); err != nil {
		return err
	}
	return conn.Raw(func(driverConn interface{}) error {
		provider, ok := driverConn.(interface{ Conn() *pgx.Conn })
		if !ok {
			return fmt.Errorf("postgres driver does not expose pgx connection: %T", driverConn)
		}
		for {
			notification, err := provider.Conn().WaitForNotification(ctx)
			if err != nil {
				if err == driver.ErrBadConn {
					return err
				}
				return err
			}
			if notification != nil {
				publishWorkflowRuntimeEventSignal(notification.Payload)
			}
		}
	})
}
