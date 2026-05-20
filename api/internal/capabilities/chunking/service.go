package chunking

import (
	"context"
	"fmt"

	"github.com/zgiai/ginext/internal/capabilities/chunking/executor"
	"github.com/zgiai/ginext/internal/contracts"
)

// Service keeps the parse-to-chunk planning flow behind one capability boundary.
// It deliberately stops at planning in this foundation step; production chunk
// building and dataset loading remain on the existing path until explicit
// shadow validation and cutover are added.
type Service struct {
	mapper   contracts.ChunkSourceMapper
	planner  contracts.ChunkPlanner
	executor *executor.Executor
}

func NewService(mapper contracts.ChunkSourceMapper, planner contracts.ChunkPlanner, options ...executor.Option) *Service {
	if mapper == nil {
		mapper = NewCanonicalMapper()
	}
	if planner == nil {
		planner = NewDefaultPlanner()
	}
	return &Service{mapper: mapper, planner: planner, executor: executor.New(options...)}
}

type PlanResult struct {
	Source *contracts.ChunkSourceDocument `json:"source"`
	Plan   *contracts.ChunkPlan           `json:"plan"`
}

func (s *Service) PlanFromArtifact(ctx context.Context, artifact *contracts.ParseArtifact, useCase contracts.ChunkUseCase) (*PlanResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, fmt.Errorf("chunking service is nil")
	}
	source, err := s.mapper.FromParseArtifact(artifact)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	plan, err := s.planner.Plan(source, useCase)
	if err != nil {
		return nil, err
	}
	return &PlanResult{Source: source, Plan: plan}, nil
}

type ExecuteResult struct {
	Source    *contracts.ChunkSourceDocument `json:"source"`
	Plan      *contracts.ChunkPlan           `json:"plan"`
	Execution *executor.Result               `json:"execution"`
}

func (s *Service) ExecuteFromArtifact(ctx context.Context, artifact *contracts.ParseArtifact, useCase contracts.ChunkUseCase) (*ExecuteResult, error) {
	planned, err := s.PlanFromArtifact(ctx, artifact, useCase)
	if err != nil {
		return nil, err
	}
	if s.executor == nil {
		return nil, fmt.Errorf("chunk executor is nil")
	}
	executed, err := s.executor.Execute(ctx, planned.Source, planned.Plan)
	if err != nil {
		return nil, err
	}
	return &ExecuteResult{
		Source:    planned.Source,
		Plan:      planned.Plan,
		Execution: executed,
	}, nil
}
