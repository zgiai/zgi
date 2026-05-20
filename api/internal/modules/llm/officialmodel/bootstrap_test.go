package officialmodel

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEnsureSynchronizerStarted_StartsOnlyOnceInCloudMode(t *testing.T) {
	resetSynchronizerBootstrapForTest()
	t.Cleanup(resetSynchronizerBootstrapForTest)

	db := &gorm.DB{}

	var calls atomic.Int32
	var gotAddr atomic.Value
	startSynchronizerLoop = func(context.Context, *gorm.DB, string) {
		calls.Add(1)
		gotAddr.Store("grpc.test:50051")
	}

	EnsureSynchronizerStarted(context.Background(), db, "CLOUD", "grpc.test:50051")
	EnsureSynchronizerStarted(context.Background(), db, "CLOUD", "grpc.test:50051")

	require.Eventually(t, func() bool {
		return calls.Load() == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, "grpc.test:50051", gotAddr.Load())
}

func TestEnsureSynchronizerStarted_SkipsNonCloudMode(t *testing.T) {
	resetSynchronizerBootstrapForTest()
	t.Cleanup(resetSynchronizerBootstrapForTest)

	db := &gorm.DB{}

	var calls atomic.Int32
	startSynchronizerLoop = func(context.Context, *gorm.DB, string) {
		calls.Add(1)
	}

	EnsureSynchronizerStarted(context.Background(), db, "SELF_HOSTED", "grpc.test:50051")

	time.Sleep(50 * time.Millisecond)
	require.Zero(t, calls.Load())
}

func TestEnsureSynchronizerStarted_UsesDefaultAddrWhenEmpty(t *testing.T) {
	resetSynchronizerBootstrapForTest()
	t.Cleanup(resetSynchronizerBootstrapForTest)

	db := &gorm.DB{}

	var gotAddr atomic.Value
	startSynchronizerLoop = func(context.Context, *gorm.DB, string) {
		gotAddr.Store(defaultOfficialModelGRPCAddr)
	}

	EnsureSynchronizerStarted(context.Background(), db, "CLOUD", "")

	require.Eventually(t, func() bool {
		return gotAddr.Load() != nil
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, defaultOfficialModelGRPCAddr, gotAddr.Load())
}
