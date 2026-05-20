package retrieval

import (
	"math"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

// FusedResult represents a combined result from vector and graph retrieval
type FusedResult struct {
	EntityID     string   `json:"entity_id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	VectorScore  float64  `json:"vector_score"`
	GraphScore   float64  `json:"graph_score"`
	FinalScore   float64  `json:"final_score"`
	RelatedFacts []string `json:"related_facts,omitempty"`
}

// HybridFusionConfig contains configuration for hybrid fusion
type HybridFusionConfig struct {
	Alpha               float64 // Weight for vector scores (0-1), graph weight = 1 - Alpha
	MaxResults          int     // Maximum number of results to return
	MinVectorScore      float64 // Minimum vector score threshold
	MinGraphScore       float64 // Minimum graph score threshold to filter weak signals
	BoostConnected      float64 // Bonus score for entities with graph connections
	Temperature         float64 // Temperature scaling parameter to sharpen vector scores (default 1.0)
	GlobalMaxGraphScore float64 // Global max for stable log-normalization of graph scores (default 10.0)
}

// DefaultHybridFusionConfig returns sensible default configuration
func DefaultHybridFusionConfig() HybridFusionConfig {
	return HybridFusionConfig{
		Alpha:               0.7, // Vector scores weighted 70%
		MaxResults:          10,
		MinVectorScore:      0.0,
		MinGraphScore:       0.01, // Filter out very weak graph signals
		BoostConnected:      0.03, // Small boost only; avoid over-promoting weak graph hits
		Temperature:         1.0,
		GlobalMaxGraphScore: 10.0,
	}
}

// HybridFusion combines vector retrieval results with graph context
// using a weighted fusion algorithm
// queryEntities: The list of entities extracted from the original query (for hard constraint checking)
func HybridFusion(vectorResults []EntityMatch, graphContext *GraphContext, alpha float64, queryEntities []string, externalScores map[string]float64) []FusedResult {
	return HybridFusionWithConfig(vectorResults, graphContext, HybridFusionConfig{
		Alpha:               alpha,
		MaxResults:          30,
		MinVectorScore:      0.0,
		MinGraphScore:       0.01,
		BoostConnected:      0.03,
		Temperature:         1.0,
		GlobalMaxGraphScore: 10.0,
	}, queryEntities, externalScores)
}

// HybridFusionWithConfig performs hybrid fusion with custom configuration
func HybridFusionWithConfig(vectorResults []EntityMatch, graphContext *GraphContext, cfg HybridFusionConfig, queryEntities []string, externalScores map[string]float64) []FusedResult {
	// Validate alpha
	if cfg.Alpha < 0 {
		cfg.Alpha = 0
	}
	if cfg.Alpha > 1 {
		cfg.Alpha = 1
	}

	// Build graph entity lookup and compute graph scores
	graphEntityScores := computeGraphScores(graphContext, cfg.GlobalMaxGraphScore, queryEntities)

	// Override with external scores (from Smart Expansion)
	// MUST normalize these scores to [0, 1] because vectorScores are [0, 1].
	// Otherwise a raw smart score of 20.0 will mathematically destroy the fusion scale.
	if len(externalScores) > 0 {
		maxExt := 0.0
		for _, score := range externalScores {
			if score > maxExt {
				maxExt = score
			}
		}
		for name, score := range externalScores {
			if maxExt > 0 {
				graphEntityScores[name] = score / maxExt
			} else {
				graphEntityScores[name] = 0.0
			}
		}
	}

	graphEntityFacts := buildEntityFacts(graphContext)

	// Merge results
	resultMap := make(map[string]*FusedResult)

	// Process vector results
	for _, vm := range vectorResults {
		if vm.Score < cfg.MinVectorScore {
			continue
		}

		key := vm.Name
		if key == "" {
			key = vm.EntityID
		}

		// Normalize vector score (convert distance to similarity if needed)
		// Assuming lower distance = better, we invert: similarity = 1 / (1 + distance)
		vectorSimilarity := normalizeVectorScore(vm.Score, cfg.Temperature)

		// Merge logic: If name already exists, keep the best vector evidence
		if existing, ok := resultMap[key]; ok {
			if vectorSimilarity > existing.VectorScore {
				existing.VectorScore = vectorSimilarity
				existing.EntityID = vm.EntityID
			}
			continue
		}

		result := &FusedResult{
			EntityID:    vm.EntityID,
			Name:        vm.Name,
			Type:        vm.Type,
			VectorScore: vectorSimilarity,
		}

		// Add graph score if entity is in graph context
		if graphScore, ok := graphEntityScores[vm.Name]; ok {
			result.GraphScore = graphScore
			// Boost connected entities
			result.VectorScore += cfg.BoostConnected
		}

		// Add related facts
		if facts, ok := graphEntityFacts[vm.Name]; ok {
			result.RelatedFacts = facts
		}

		resultMap[key] = result
	}

	// Add graph-only entities (entities found in graph but not in vector search)
	if graphContext != nil {
		for _, ge := range graphContext.Entities {
			key := ge.Name
			if key == "" {
				key = ge.ID
			}

			if _, exists := resultMap[key]; !exists {
				result := &FusedResult{
					EntityID:   ge.ID,
					Name:       ge.Name,
					Type:       ge.Type,
					GraphScore: graphEntityScores[ge.Name],
				}
				if facts, ok := graphEntityFacts[ge.Name]; ok {
					result.RelatedFacts = facts
				}
				resultMap[key] = result
			}
		}
	}

	// Compute final scores
	results := make([]FusedResult, 0, len(resultMap))
	for _, r := range resultMap {
		r.FinalScore = computeFinalScore(r.VectorScore, r.GraphScore, cfg.Alpha)

		results = append(results, *r)
	}

	// Sort by final score (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	// Limit results
	if cfg.MaxResults > 0 && len(results) > cfg.MaxResults {
		results = results[:cfg.MaxResults]
	}

	topFinalScore := 0.0
	topVectorScore := 0.0
	topGraphScore := 0.0
	if len(results) > 0 {
		topFinalScore = results[0].FinalScore
		topVectorScore = results[0].VectorScore
		topGraphScore = results[0].GraphScore
	}
	logger.Debug("hybrid fusion completed",
		zap.Float64("alpha", cfg.Alpha),
		zap.Int("result_count", len(results)),
		zap.Float64("top_final_score", topFinalScore),
		zap.Float64("top_vector_score", topVectorScore),
		zap.Float64("top_graph_score", topGraphScore),
	)

	return results
}

// computeGraphScores calculates importance scores for entities based on graph structure
// using global Log-Normalization to tame power-law hubs. Applies BFS distance decay
// from explicit semantic anchors to suppress irrelevant hub nodes.
func computeGraphScores(gc *GraphContext, globalMax float64, queryEntities []string) map[string]float64 {
	scores := make(map[string]float64)

	if gc == nil {
		return scores
	}

	// 1. Build adjacency list and count connections
	adj := make(map[string][]string)
	connectionCount := make(map[string]int)

	for _, rel := range gc.Relationships {
		adj[rel.HeadEntity] = append(adj[rel.HeadEntity], rel.TailEntity)
		adj[rel.TailEntity] = append(adj[rel.TailEntity], rel.HeadEntity)
		connectionCount[rel.HeadEntity]++
		connectionCount[rel.TailEntity]++
	}

	// 2. Identify anchors from query entities
	queryNamesLower := make(map[string]bool)
	for _, qe := range queryEntities {
		queryNamesLower[strings.ToLower(qe)] = true
	}

	queue := make([]string, 0)
	distances := make(map[string]int)

	for _, entity := range gc.Entities {
		if queryNamesLower[strings.ToLower(entity.Name)] || queryNamesLower[strings.ToLower(entity.CanonicalName)] {
			queue = append(queue, entity.Name)
			distances[entity.Name] = 0
		}
	}

	// 3. BFS to calculate shortest path distance to any anchor
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		dist := distances[curr]
		for _, neighbor := range adj[curr] {
			if _, visited := distances[neighbor]; !visited {
				distances[neighbor] = dist + 1
				queue = append(queue, neighbor)
			}
		}
	}

	// 4. Compute distance-decayed scores and find the true max in current graph
	decayedScoresMap := make(map[string]float64)
	var maxDecayedScore float64 = 0

	for _, entity := range gc.Entities {
		connections := connectionCount[entity.Name]

		// KEY: Raw score combining term frequency (source text) and degree centrality (graph)
		rawScore := float64(entity.SourceCount) + float64(connections)

		// KEY: Distance Decay Penalty to curb Hub node hijacking
		dist, reachable := distances[entity.Name]
		decayFactor := 0.1 // Base penalty for topologically unreachable nodes (disjoint subgraphs)
		if reachable {
			decayFactor = math.Pow(0.8, float64(dist))
		}

		decayedScore := rawScore * decayFactor
		decayedScoresMap[entity.Name] = decayedScore

		if decayedScore > maxDecayedScore {
			maxDecayedScore = decayedScore
		}
	}

	// 5. Global Unified Log-Normalization
	// KEY: Ensure all entities share the same mathematical denominator ceiling,
	// preventing "flat ceiling effect" where multiple Hubs cap at exactly 1.0.
	denominatorMax := math.Max(maxDecayedScore, globalMax)

	for entityName, decayedScore := range decayedScoresMap {
		if denominatorMax > 0 {
			scores[entityName] = math.Log1p(decayedScore) / math.Log1p(denominatorMax)
		} else {
			scores[entityName] = 0.0
		}
	}

	return scores
}

// buildEntityFacts extracts related facts for each entity
func buildEntityFacts(gc *GraphContext) map[string][]string {
	facts := make(map[string][]string)

	if gc == nil {
		return facts
	}

	for _, rel := range gc.Relationships {
		fact := rel.HeadEntity + " " + rel.RelationType + " " + rel.TailEntity

		// Add to head entity
		facts[rel.HeadEntity] = append(facts[rel.HeadEntity], fact)
		// Add to tail entity
		facts[rel.TailEntity] = append(facts[rel.TailEntity], fact)
	}

	return facts
}

// normalizeVectorScore normalizes vector score and applies temperature scaling.
// Note: Neo4j vector index search uses cosine similarity implicitly and returns
// score in range [0, 1] directly, where closer to 1.0 is better.
func normalizeVectorScore(score float64, temp float64) float64 {
	var normalized float64
	// For similarities in [0, 1], return as is
	if score >= 0 && score <= 1.0 {
		normalized = score
	} else if score > 1.0 {
		// For L2 distance: similarity = 1 / (1 + distance)
		normalized = 1.0 / (1.0 + score)
	} else {
		// KEY: Negative scores are truncated to 0, since negative cosine distance
		// indicates orthogonality/irrelevance, which is meaningless for retrieval.
		normalized = 0.0
	}

	// KEY: Temperature scaling to sharpen high-score bands and increase resolution
	if temp > 0 && temp != 1.0 {
		normalized = math.Pow(normalized, 1.0/temp)
	}

	return normalized
}

// computeFinalScore calculates the weighted combination of vector and graph scores.
// Improved Logic: Penalize single-source evidence slightly (discount) and reward
// dual-validation (bonus). This averts the "hub takeover" where a strong single
// proof blindly outranks a legitimate doubly-verified entity.
func computeFinalScore(vectorScore, graphScore, alpha float64) float64 {
	const crossValidationBonus = 0.05
	const singleSourceDiscount = 0.92

	// If only graph search found the entity, discount to curb hub bias
	if vectorScore <= 0 {
		return graphScore * singleSourceDiscount
	}
	// If only vector search found the entity, discount softly
	if graphScore <= 0 {
		return vectorScore * singleSourceDiscount
	}

	// If both sources concur, apply the weighted fusion AND reward
	baseScore := alpha*vectorScore + (1-alpha)*graphScore
	return baseScore + crossValidationBonus
}

// RetrievalResult is the final output of hybrid retrieval
type RetrievalResult struct {
	FusedResults []FusedResult `json:"fused_results"`
	GraphContext *GraphContext `json:"graph_context,omitempty"`
	QueryInfo    QueryInfo     `json:"query_info"`
}

// QueryInfo contains metadata about the retrieval query
type QueryInfo struct {
	OriginalQuery     string   `json:"original_query"`
	ExtractedEntities []string `json:"extracted_entities"`
	VectorResultCount int      `json:"vector_result_count"`
	GraphEntityCount  int      `json:"graph_entity_count"`
}
