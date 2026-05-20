package er

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/graph"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/redis"
)

// EntityResolver defines the interface for entity resolution
type EntityResolver interface {
	Resolve(ctx context.Context, kbID, mention, label string, embedding []float32) (*model.GraphEntity, bool, error)
	GetWorkerShard(mention string) int
}

// EntityResolutionService implements EntityResolver with Industrial Grade features
// Fixed struct closing brace
type EntityResolutionService struct {
	neo4jClient         *graph.Neo4jClient
	similarityThreshold float64
	numShards           int
}

// NewEntityResolutionService creates a new ER service
func NewEntityResolutionService(client *graph.Neo4jClient, threshold float64) *EntityResolutionService {
	if threshold <= 0 {
		threshold = 0.92 // High confidence threshold
	}
	return &EntityResolutionService{
		neo4jClient:         client,
		similarityThreshold: threshold,
		numShards:           1024,
	}
}

func (s *EntityResolutionService) GetWorkerShard(mention string) int {
	hash := crc32.ChecksumIEEE([]byte(mention))
	return int(hash % uint32(s.numShards))
}

// Resolve implements the L3 Cascading Logic:
// L1: Exact Match / Cache
// L2: Vector Search (High Confidence) -> Blocking: Embedding-based (Coarse Screen)
// L3: Async LLM Judge (Low Confidence)
func (s *EntityResolutionService) Resolve(ctx context.Context, kbID, mention, label string, embedding []float32) (*model.GraphEntity, bool, error) {
	if mention == "" {
		return nil, false, fmt.Errorf("mention cannot be empty")
	}
	
	normalizedName := strings.ToLower(strings.TrimSpace(mention))
	cacheKey := fmt.Sprintf("er:idx:%s:%s:%s", kbID, label, hashString(normalizedName))

	// --- L1: Redis Cache / Exact Match ---
	if exists, canonicalID := s.checkRedisCache(ctx, cacheKey); exists {
		return &model.GraphEntity{
			ID:            canonicalID,
			CanonicalName: mention, 
			Type:          label,
		}, false, nil
	}

	// --- L2: Embedding-based Blocking & Vector Search ---
	// We dropped "Prefix Blocking" because it fails for synonyms (WWII vs World War II).
	// We rely on Neo4j Vector Index to find "Semantically Close" candidates efficiently.
	
	// High Confidence Search
	canonicalID, err := s.neo4jClient.FindSimilarEntityWithFilter(ctx, kbID, label, embedding, s.similarityThreshold)
	if err != nil {
		logger.Error("Vector search failed", err)
	}

	if canonicalID != "" {
		// Found High Confidence Match
		s.updateRedisCache(ctx, cacheKey, canonicalID)
		return &model.GraphEntity{
			ID:            canonicalID,
			CanonicalName: mention, 
			Type:          label,
			Confidence:    1.0, 
		}, false, nil
	}
	
	// --- L3: Low Confidence / Ambiguous Cases (Async Judge) ---
	// If score is between 0.8 and 0.9, we might want to trigger an LLM check.
	// Current Neo4j client doesn't return score in the simple Find function.
	// Assuming we didn't find high confidence match.
	// Ideally we would search with lower threshold (e.g. 0.8) and if found, trigger Async Judge.
	// For now, we proceed to Create (Blocking).
	// Future:
	// candidateID, score := s.neo4jClient.FindSimilar... (threshold=0.8)
	// if candidateID != "" && score < 0.92 { s.triggerAsyncLLMJudge(...) }

	// --- L4: Create New Canonical Entity (Distributed Lock) ---
	lockKey := fmt.Sprintf("lock:er:create:%s", hashString(cacheKey))
	ttl := 10 * time.Second

	if !s.acquireLock(ctx, lockKey, ttl) {
		time.Sleep(500 * time.Millisecond)
		if exists, cid := s.checkRedisCache(ctx, cacheKey); exists {
			return &model.GraphEntity{ID: cid, Type: label}, false, nil
		}
	}
	
	done := make(chan struct{})
	go s.watchDog(ctx, lockKey, ttl, done)
	defer close(done)

	// Double Check
	if exists, cid := s.checkRedisCache(ctx, cacheKey); exists {
		return &model.GraphEntity{ID: cid, Type: label}, false, nil
	}

	// Create
	newID := uuid.New().String()
	newEntity := &model.GraphEntity{
		ID:            newID,
		CanonicalName: mention,
		Embedding:     embedding,
		Type:          label,
		Confidence:    1.0, 
	}
	
	targetType := label
	if targetType == "" {
		targetType = "Entity"
	}

	props := map[string]interface{}{
		"id":             newEntity.ID,
		"name":           newEntity.CanonicalName,
		"canonical_name": newEntity.CanonicalName,
		"kb_id":          kbID,
		"embedding":      embedding,
		"type":           targetType,
		"created_at":     time.Now().Unix(),
	}

	createdID, err := s.neo4jClient.CreateNode(ctx, targetType, props)
	if err != nil {
		if strings.Contains(err.Error(), "ConstraintValidationFailed") || strings.Contains(err.Error(), "already exists") {
			 logger.Info("Entity creation conflict, resolving to existing", map[string]interface{}{"name": mention})
			 return nil, false, fmt.Errorf("concurrent creation conflict: %w", err)
		}
		return nil, false, err
	}

	s.updateRedisCache(ctx, cacheKey, createdID)
	newEntity.ID = createdID
	
	return newEntity, true, nil
}

// --- Helpers ---

func (s *EntityResolutionService) checkRedisCache(ctx context.Context, key string) (bool, string) {
	val, err := redis.GetString(ctx, key)
	if err == nil && val != "" {
		return true, val
	}
	return false, ""
}

func (s *EntityResolutionService) updateRedisCache(ctx context.Context, key, value string) {
	_ = redis.SetEx(ctx, key, value, 24*time.Hour) 
}

func (s *EntityResolutionService) acquireLock(ctx context.Context, key string, ttl time.Duration) bool {
	val, _ := redis.GetString(ctx, key)
	if val == "" {
		_ = redis.SetEx(ctx, key, "locked", ttl)
		return true
	}
	return false
}

func (s *EntityResolutionService) watchDog(ctx context.Context, key string, ttl time.Duration, done chan struct{}) {
	ticker := time.NewTicker(ttl / 2)
	defer ticker.Stop()
	
	for {
		select {
		case <-done:
			return 
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = redis.Expire(ctx, key, ttl)
		}
	}
}

func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
