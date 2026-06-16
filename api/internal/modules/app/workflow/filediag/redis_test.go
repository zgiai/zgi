package filediag

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	zredis "github.com/zgiai/zgi/api/pkg/redis"
)

func TestAppendErrorNoopsWithoutRedisClient(t *testing.T) {
	previous := zredis.GetClient()
	zredis.SetClient(nil)
	t.Cleanup(func() {
		zredis.SetClient(previous)
	})

	AppendError(context.Background(), "workflow_file_lookup_missing", "missing file", map[string]string{
		"upload_file_id": "file-1",
	})
}

func TestAppendErrorWritesCappedStream(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	previous := zredis.GetClient()
	zredis.SetClient(client)
	t.Cleanup(func() {
		_ = client.Close()
		zredis.SetClient(previous)
	})

	AppendError(context.Background(), "workflow_file_lookup_missing", "missing file", map[string]string{
		"upload_file_id": "file-1",
		"workspace_id":   "workspace-1",
	})

	length, err := client.XLen(context.Background(), redisStreamKey).Result()
	if err != nil {
		t.Fatalf("XLen failed: %v", err)
	}
	if length != 1 {
		t.Fatalf("expected one diagnostic event, got %d", length)
	}

	entries, err := client.XRange(context.Background(), redisStreamKey, "-", "+").Result()
	if err != nil {
		t.Fatalf("XRange failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one diagnostic entry, got %d", len(entries))
	}
	if got := entries[0].Values["event"]; got != "workflow_file_lookup_missing" {
		t.Fatalf("unexpected event: %v", got)
	}
	if got := entries[0].Values["upload_file_id"]; got != "file-1" {
		t.Fatalf("unexpected upload_file_id: %v", got)
	}

	ttl, err := client.TTL(context.Background(), redisStreamKey).Result()
	if err != nil {
		t.Fatalf("TTL failed: %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("expected stream ttl to be set, got %s", ttl)
	}
}
