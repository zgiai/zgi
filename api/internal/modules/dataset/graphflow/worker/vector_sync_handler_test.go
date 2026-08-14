package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
)

func TestRetryVectorSyncBatchByHalvingReachesSingletons(t *testing.T) {
	entities := make([]*model.Entity, 10)
	for i := range entities {
		entities[i] = &model.Entity{ID: uuid.New()}
	}

	var attemptedSizes []int
	var process func([]*model.Entity) error
	process = func(batch []*model.Entity) error {
		attemptedSizes = append(attemptedSizes, len(batch))
		if len(batch) == 1 {
			return nil
		}
		return retryVectorSyncBatchByHalving(
			context.Background(),
			batch,
			errors.New("provider rejected batch"),
			process,
			func([]*model.Entity, error) error {
				t.Fatal("singleton should succeed in this scenario")
				return nil
			},
		)
	}

	err := retryVectorSyncBatchByHalving(
		context.Background(),
		entities,
		errors.New("provider rejected batch"),
		process,
		func([]*model.Entity, error) error {
			t.Fatal("initial batch should be split")
			return nil
		},
	)
	if err != nil {
		t.Fatalf("adaptive retry returned error: %v", err)
	}

	singletons := 0
	for _, size := range attemptedSizes {
		if size == 1 {
			singletons++
		}
		if size > 5 {
			t.Fatalf("retry attempted batch size %d after the initial size 10 failure", size)
		}
	}
	if singletons != len(entities) {
		t.Fatalf("singleton attempts = %d, want %d; attempts=%v", singletons, len(entities), attemptedSizes)
	}
}
