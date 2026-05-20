package pipeline

import (
	"context"
	"time"

	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/graph"
	"github.com/zgiai/ginext/pkg/logger"
)

// GraphOrchestrator manages the graph processing pipeline including extraction, resolution, and cleanup
type GraphOrchestrator struct {
	neo4jClient *graph.Neo4jClient
}

// NewGraphOrchestrator creates a new orchestrator
func NewGraphOrchestrator(client *graph.Neo4jClient) *GraphOrchestrator {
	return &GraphOrchestrator{
		neo4jClient: client,
	}
}

// StartConvergenceLoop runs background tasks for graph validation and convergence
func (o *GraphOrchestrator) StartConvergenceLoop(ctx context.Context, interval time.Duration) {
	logger.Info("Starting Graph Convergence Operator (Louvain)...", nil)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping Convergence Operator", nil)
			return
		case <-ticker.C:
			o.runCommunityDetection(ctx)
		}
	}
}

func (o *GraphOrchestrator) runCommunityDetection(ctx context.Context) {
	// Trigger Community Detection (Louvain) to identify dense clusters
	// logical candidates for "SameAs" merging.
	// Typically run per KB or globally depending on isolation policy.
	// Here assuming Global for the example.
	
	logger.Info("Running Periodic Community Detection...", nil)
	if err := o.neo4jClient.RunCommunityDetection(ctx, ""); err != nil {
		logger.Error("Community Detection Cycle Failed", err)
	} else {
		logger.Info("Community Detection Cycle Completed", nil)
	}
}
