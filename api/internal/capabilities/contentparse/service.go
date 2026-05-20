package contentparse

import (
	"context"

	"github.com/zgiai/ginext/internal/contracts"
)

type Service struct {
	orchestrator *Orchestrator
}

func NewService(orchestrator *Orchestrator) contracts.ContentParseService {
	return &Service{orchestrator: orchestrator}
}

func (s *Service) Parse(ctx context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	return s.orchestrator.Parse(ctx, req)
}

func (s *Service) Health(ctx context.Context) (*contracts.ParseHealth, error) {
	return s.orchestrator.Health(ctx)
}
