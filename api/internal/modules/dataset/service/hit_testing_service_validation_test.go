package service

import (
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
)

func TestHitTestingArgsCheckQueryLength(t *testing.T) {
	service := &hitTestingService{}

	if err := service.HitTestingArgsCheck(&dto.HitTestingRequest{
		Query: strings.Repeat("图", maxHitTestingQueryLength),
	}); err != nil {
		t.Fatalf("expected a %d-character query to be valid: %v", maxHitTestingQueryLength, err)
	}

	err := service.HitTestingArgsCheck(&dto.HitTestingRequest{
		Query: strings.Repeat("图", maxHitTestingQueryLength+1),
	})
	if err == nil {
		t.Fatalf("expected a %d-character query to be rejected", maxHitTestingQueryLength+1)
	}
	if !strings.Contains(err.Error(), "1000 characters") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}
