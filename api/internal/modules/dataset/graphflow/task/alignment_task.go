package task

import (
	"context"

	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/aligner"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/ginext/pkg/logger"
)

type EntityAlignmentTask struct {
	aligner  *aligner.CanonicalAligner
}

func NewEntityAlignmentTask(aligner *aligner.CanonicalAligner) *EntityAlignmentTask {
	return &EntityAlignmentTask{
		aligner: aligner,
	}
}

// Run executes the alignment process for a set of raw mentions
// Input: List of Raw Mentions (just extracted)
// Output: List of Canonical Entities (created or linked)
func (t *EntityAlignmentTask) Run(ctx context.Context, mentions []model.EntityMention) ([]model.Entity, error) {
	if len(mentions) == 0 {
		return []model.Entity{}, nil
	}

	logger.Info("GraphFlow: Starting Entity Alignment", map[string]interface{}{
		"mention_count": len(mentions),
	})

	// Delegate core logic to the Aligner service
	canonicalEntities, err := t.aligner.AlignEntities(ctx, mentions)
	if err != nil {
		logger.Error("GraphFlow: Alignment failed", err)
		return nil, err
	}

	logger.Info("GraphFlow: Entity Alignment Completed", map[string]interface{}{
		"canonical_count": len(canonicalEntities),
	})

	return canonicalEntities, nil
}
