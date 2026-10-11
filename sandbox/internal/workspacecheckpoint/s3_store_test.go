package workspacecheckpoint

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestS3StoreCrossWorkerCheckpoint(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "checkpoint-test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "checkpoint-test")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	var mu sync.Mutex
	var stored []byte
	var checksum string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/checkpoints/zgi/test/"+testKey+".tar" {
			http.NotFound(w, request)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch request.Method {
		case http.MethodPut:
			if request.Header.Get("If-None-Match") != "*" {
				t.Errorf("missing immutable-write condition")
			}
			if stored != nil {
				w.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			stored, _ = io.ReadAll(request.Body)
			checksum = request.Header.Get("X-Amz-Meta-Sha256")
		case http.MethodGet:
			if stored == nil {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `<Error><Code>NoSuchKey</Code></Error>`)
				return
			}
			w.Header().Set("X-Amz-Meta-Sha256", checksum)
			_, _ = w.Write(stored)
		case http.MethodDelete:
			stored = nil
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	settings := S3Config{Bucket: "checkpoints", Region: "us-east-1", Endpoint: server.URL, Prefix: "zgi/test", UsePathStyle: true}
	first, err := NewS3Store(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewS3Store(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Open(context.Background(), testKey); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("missing archive: %v", err)
	}
	content := []byte("workspace bytes before worker replacement")
	if err := first.Save(context.Background(), testKey, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	if err := second.Save(context.Background(), testKey, bytes.NewReader(content)); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if err := second.Save(context.Background(), testKey, strings.NewReader("other bytes")); !errors.Is(err, agentexecution.ErrIdempotencyConflict) {
		t.Fatalf("conflicting replay: %v", err)
	}
	reader, err := second.Open(context.Background(), testKey)
	if err != nil {
		t.Fatal(err)
	}
	got, readErr := io.ReadAll(reader)
	readErr = errors.Join(readErr, reader.Close())
	if readErr != nil || !bytes.Equal(got, content) {
		t.Fatalf("recovered archive = %q, %v", got, readErr)
	}
	mu.Lock()
	stored[0] ^= 1
	mu.Unlock()
	reader, err = second.Open(context.Background(), testKey)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr = io.ReadAll(reader)
	_ = reader.Close()
	if !errors.Is(readErr, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("corrupt archive: %v", readErr)
	}
	if err := second.Delete(context.Background(), testKey); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Open(context.Background(), testKey); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("deleted archive: %v", err)
	}
}

func TestS3StoreConfigRejectsUnsafeEndpoints(t *testing.T) {
	for _, settings := range []S3Config{
		{}, {Bucket: "checkpoint", Region: "us-east-1", Endpoint: "http://example.com"},
		{Bucket: "checkpoint", Region: "us-east-1", Endpoint: "https://user:pass@example.com"},
		{Bucket: "checkpoint", Region: "us-east-1", Prefix: "../escape"},
	} {
		if _, err := NewS3Store(context.Background(), settings); !errors.Is(err, agentexecution.ErrInvalidRequest) {
			t.Fatalf("settings %+v: %v", settings, err)
		}
	}
}

func TestS3StoreRealServer(t *testing.T) {
	endpoint := os.Getenv("ZGI_CHECKPOINT_S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("set ZGI_CHECKPOINT_S3_TEST_ENDPOINT for real S3-compatible storage acceptance")
	}
	settings := S3Config{Bucket: "zgi-checkpoint-test", Region: "us-east-1", Endpoint: endpoint, Prefix: "m5-acceptance", UsePathStyle: true}
	first, err := NewS3Store(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.client.CreateBucket(context.Background(), &s3.CreateBucketInput{Bucket: aws.String(settings.Bucket)}); err != nil && !isS3Code(err, "BucketAlreadyOwnedByYou") {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := first.Delete(context.Background(), testKey); err != nil {
			t.Errorf("delete object: %v", err)
		}
	})
	content := []byte("cross-worker checkpoint on real S3-compatible storage")
	if err := first.Save(context.Background(), testKey, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	second, err := NewS3Store(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := second.Open(context.Background(), testKey)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(reader)
	err = errors.Join(err, reader.Close())
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatalf("replacement worker read %q, %v", actual, err)
	}
	if err := second.Save(context.Background(), testKey, bytes.NewReader(content)); err != nil {
		t.Fatalf("idempotent replacement worker write: %v", err)
	}
}
