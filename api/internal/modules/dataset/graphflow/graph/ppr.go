package graph

import (
	"context"
	"fmt"
	"sort"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// PPRResult represents the result of Personalized PageRank for a single node
type PPRResult struct {
	NodeID   string  `json:"node_id"`
	NodeName string  `json:"node_name"`
	Score    float64 `json:"score"`
}

// PPRConfig contains configuration for the PPR algorithm
type PPRConfig struct {
	Alpha         float64 // Damping factor (typically 0.85)
	MaxIterations int     // Maximum iterations before convergence
	Tolerance     float64 // Convergence tolerance
	TopK          int     // Number of top results to return
}

// DefaultPPRConfig returns sensible default configuration
func DefaultPPRConfig() PPRConfig {
	return PPRConfig{
		Alpha:         0.85,
		MaxIterations: 100,
		Tolerance:     0.0001,
		TopK:          20,
	}
}

// RunPPR executes Personalized PageRank starting from seed nodes
// This implementation uses Neo4j GDS library if available, otherwise falls back to a simple approximation
func RunPPR(ctx context.Context, neo4jClient *Neo4jClient, seedNodes []string, alpha float64, maxIterations int) ([]PPRResult, error) {
	cfg := PPRConfig{
		Alpha:         alpha,
		MaxIterations: maxIterations,
		Tolerance:     0.0001,
		TopK:          20,
	}
	return RunPPRWithConfig(ctx, neo4jClient, seedNodes, cfg)
}

// RunPPRWithConfig executes PPR with custom configuration
func RunPPRWithConfig(ctx context.Context, neo4jClient *Neo4jClient, seedNodes []string, cfg PPRConfig) ([]PPRResult, error) {
	if neo4jClient == nil {
		return nil, fmt.Errorf("neo4j client is nil")
	}

	if len(seedNodes) == 0 {
		return []PPRResult{}, nil
	}

	// Try to use Neo4j GDS for PPR (if GDS is installed)
	results, err := runPPRWithGDS(ctx, neo4jClient, seedNodes, cfg)
	if err == nil {
		return results, nil
	}

	// Fallback: use simple BFS-based approximation
	return runPPRSimple(ctx, neo4jClient, seedNodes, cfg)
}

// runPPRWithGDS attempts to use Neo4j Graph Data Science library
func runPPRWithGDS(ctx context.Context, neo4jClient *Neo4jClient, seedNodes []string, cfg PPRConfig) ([]PPRResult, error) {
	// Create a projected graph and run PageRank
	// This requires Neo4j GDS to be installed
	cypher := `
		CALL gds.pageRank.stream({
			nodeProjection: '*',
			relationshipProjection: {
				ALL: {
					type: '*',
					orientation: 'UNDIRECTED'
				}
			},
			maxIterations: $maxIterations,
			dampingFactor: $alpha,
			sourceNodes: [n IN $seedNodes | n]
		})
		YIELD nodeId, score
		WITH gds.util.asNode(nodeId) AS node, score
		RETURN node.id AS nodeId, node.name AS nodeName, score
		ORDER BY score DESC
		LIMIT $topK
	`

	params := map[string]interface{}{
		"seedNodes":     seedNodes,
		"maxIterations": cfg.MaxIterations,
		"alpha":         cfg.Alpha,
		"topK":          cfg.TopK,
	}

	// Execute query using neo4jClient
	// This will fail if GDS is not installed
	session := neo4jClient.newSession(ctx, neo4j.AccessModeRead)
	defer session.Close(ctx)

	result, err := session.Run(ctx, cypher, params)
	if err != nil {
		return nil, fmt.Errorf("GDS not available: %w", err)
	}

	var results []PPRResult
	for result.Next(ctx) {
		record := result.Record()
		nodeID, _ := record.Get("nodeId")
		nodeName, _ := record.Get("nodeName")
		score, _ := record.Get("score")

		results = append(results, PPRResult{
			NodeID:   fmt.Sprintf("%v", nodeID),
			NodeName: fmt.Sprintf("%v", nodeName),
			Score:    score.(float64),
		})
	}

	return results, nil
}

// runPPRSimple implements a simplified PPR using BFS-like propagation
// This is less accurate than true PPR but works without GDS
func runPPRSimple(ctx context.Context, neo4jClient *Neo4jClient, seedNodes []string, cfg PPRConfig) ([]PPRResult, error) {
	// Initialize scores: seed nodes start with equal probability
	scores := make(map[string]float64)
	initialScore := 1.0 / float64(len(seedNodes))
	for _, seed := range seedNodes {
		scores[seed] = initialScore
	}

	nodeNames := make(map[string]string) // nodeID -> name mapping

	// Get neighbors for all seed nodes and propagate scores
	for iter := 0; iter < cfg.MaxIterations; iter++ {
		newScores := make(map[string]float64)

		// Teleport probability: chance to return to seed nodes
		teleportProb := 1.0 - cfg.Alpha
		for _, seed := range seedNodes {
			newScores[seed] += teleportProb * initialScore
		}

		// For each node with current score, propagate to neighbors
		for nodeName, score := range scores {
			if score < cfg.Tolerance {
				continue
			}

			// Get neighbors from Neo4j
			neighbors, err := getNodeNeighbors(ctx, neo4jClient, nodeName)
			if err != nil {
				continue
			}

			if len(neighbors) > 0 {
				// Distribute score to neighbors
				shareScore := cfg.Alpha * score / float64(len(neighbors))
				for _, neighbor := range neighbors {
					newScores[neighbor.Name] += shareScore
					nodeNames[neighbor.ID] = neighbor.Name
				}
			}
		}

		// Check convergence
		maxDiff := 0.0
		for node, newScore := range newScores {
			diff := abs(newScore - scores[node])
			if diff > maxDiff {
				maxDiff = diff
			}
		}

		scores = newScores

		if maxDiff < cfg.Tolerance {
			break
		}
	}

	// Convert to results and sort
	results := make([]PPRResult, 0, len(scores))
	for name, score := range scores {
		results = append(results, PPRResult{
			NodeName: name,
			Score:    score,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Limit to top K
	if len(results) > cfg.TopK {
		results = results[:cfg.TopK]
	}

	return results, nil
}

// NeighborInfo represents basic neighbor information
type NeighborInfo struct {
	ID   string
	Name string
}

// getNodeNeighbors retrieves immediate neighbors of a node
func getNodeNeighbors(ctx context.Context, neo4jClient *Neo4jClient, nodeName string) ([]NeighborInfo, error) {
	// Updated to include MENTIONS and RELATED types specifically to ensure flow through Mention nodes
	cypher := `
		MATCH (n)-[r:RELATED|MENTIONS]-(m)
		WHERE toLower(n.name) = toLower($name)
		RETURN DISTINCT m.id AS id, m.name AS name
		LIMIT 50
	`

	session := neo4jClient.newSession(ctx, neo4j.AccessModeRead)
	defer session.Close(ctx)

	result, err := session.Run(ctx, cypher, map[string]interface{}{"name": nodeName})
	if err != nil {
		return nil, err
	}

	var neighbors []NeighborInfo
	for result.Next(ctx) {
		record := result.Record()
		id, _ := record.Get("id")
		name, _ := record.Get("name")

		neighbors = append(neighbors, NeighborInfo{
			ID:   fmt.Sprintf("%v", id),
			Name: fmt.Sprintf("%v", name),
		})
	}

	return neighbors, nil
}

// abs returns the absolute value of a float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
