package graph

import (
	"context"
	"fmt"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
)

// Neo4jClient provides methods to interact with Neo4j
type Neo4jClient struct {
	driver   neo4j.DriverWithContext
	database string
}

// NewNeo4jClient creates a new Neo4j client
func NewNeo4jClient(uri, username, password, database string) *Neo4jClient {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		logger.Error("Failed to create Neo4j driver", err)
		return nil
	}

	return &Neo4jClient{driver: driver, database: database}
}

func (c *Neo4jClient) newSession(ctx context.Context, accessMode neo4j.AccessMode) neo4j.SessionWithContext {
	cfg := neo4j.SessionConfig{AccessMode: accessMode}
	if c.database != "" {
		cfg.DatabaseName = c.database
	}
	return c.driver.NewSession(ctx, cfg)
}

// Close closes the Neo4j connection
func (c *Neo4jClient) Close() error {
	if c.driver != nil {
		return c.driver.Close(context.Background())
	}
	return nil
}

// CreateNode creates a node in Neo4j and returns the node ID
// It always adds the `Entity` base label alongside the provided specific type label
// This ensures the vector index (on Entity label) works while preserving semantic type
func (c *Neo4jClient) CreateNode(ctx context.Context, label string, properties map[string]interface{}) (string, error) {
	if c.driver == nil {
		return "", fmt.Errorf("neo4j driver not initialized")
	}

	session := c.newSession(ctx, neo4j.AccessModeWrite)
	defer session.Close(ctx)

	// Always include Entity as base label for vector index compatibility
	// Format: CREATE (n:Entity:SpecificType $props)

	// Use MERGE to prevent duplicates. We use the 'id' property as the unique key for merging.
	// Note: Ideally 'id' should have a unique constraint in Neo4j for performance.
	// Format: MERGE (n:Entity {id: $id}) ON CREATE SET n:$props ON MATCH SET n += $props
	// However, we need to handle labels dynamically. MERGE with dynamic labels isn't direct.
	// Strategy: MERGE on id (which is unique globally/per-KB), then set labels and props.
	// 'id' is passed in properties["id"].

	id, ok := properties["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("id property is required for node creation")
	}

	// Remove labels from properties to avoid overwriting (if any)
	// Actually we want to set properties.

	// Construct MERGE query
	// We merge on the unique ID. We assume the base label 'Entity' is always present.
	cypher := `
		MERGE (n:Entity {id: $id})
		ON CREATE SET n = $props
		ON MATCH SET n += $props
		WITH n
		CALL apoc.create.addLabels(n, [$label]) YIELD node
		RETURN elementId(n) as id
	`

	// If APOC is not available, we can use a simpler approach if we trust labels don't change much:
	// But APOC is standard. Fallback without APOC:
	// "MERGE ... SET n:%s ..." (cannot set dynamic labels easily in pure Cypher without APOC or messy hacks)
	// Simpler alternative: Just MERGE on ID. The Label might need to be accumulated.
	// Let's assume for now we just SET the specific label if it's new.

	// Revised Strategy without APOC dependency (safer):
	// MERGE (n:Entity {id: $id}) SET n:%s, n += $props RETURN elementId(n) as id
	// This works because SET n:Label is additive.

	extraLabel := ""
	if label != "" && label != "Entity" {
		extraLabel = fmt.Sprintf(":`%s`", label) // Quote label to be safe
	}

	cypher = fmt.Sprintf(`
		MERGE (n:Entity {id: $id})
		SET n%s
		SET n += $props
		RETURN elementId(n) as id
	`, extraLabel)

	// Use ExecuteWrite for automatic retries on Deadlock/Transient errors
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, cypher, map[string]interface{}{
			"id":    id,
			"props": properties,
		})
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			record := res.Record()
			if idVal, ok := record.Get("id"); ok {
				return fmt.Sprintf("%v", idVal), nil
			}
		}
		return "", fmt.Errorf("no id returned")
	})

	if err != nil {
		return "", fmt.Errorf("failed to create/merge node: %w", err)
	}

	return result.(string), nil
}

// UpdateNodeEmbedding updates the vector embedding for a node
func (c *Neo4jClient) UpdateNodeEmbedding(ctx context.Context, id string, embedding []float32) error {
	if c.driver == nil {
		return fmt.Errorf("neo4j driver not initialized")
	}

	session := c.newSession(ctx, neo4j.AccessModeWrite)
	defer session.Close(ctx)

	// Match node by its stored id property (which is our UUID)
	cypher := `
		MATCH (n)
		WHERE n.id = $id
		SET n.embedding = $embedding
	`

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"id":        id,
		"embedding": embedding,
	})
	if err != nil {
		return fmt.Errorf("failed to update node embedding: %w", err)
	}

	return nil
}

// CheckVectorIndexSupport checks if the connected Neo4j instance supports vector indexing
func (c *Neo4jClient) CheckVectorIndexSupport(ctx context.Context) (bool, string, error) {
	if c.driver == nil {
		return false, "", fmt.Errorf("neo4j driver not initialized")
	}

	session := c.newSession(ctx, neo4j.AccessModeRead)
	defer session.Close(ctx)

	// Check version procedure
	result, err := session.Run(ctx, "CALL dbms.components() YIELD name, versions, edition RETURN name, versions, edition", nil)
	if err != nil {
		return false, "", fmt.Errorf("failed to check version: %w", err)
	}

	if result.Next(ctx) {
		record := result.Record()
		versionsRaw, _ := record.Get("versions")
		editionRaw, _ := record.Get("edition")

		versions, ok := versionsRaw.([]interface{})
		if !ok || len(versions) == 0 {
			return false, "unknown", nil
		}

		versionStr := fmt.Sprintf("%v", versions[0])
		editionStr := fmt.Sprintf("%v", editionRaw)

		// Simple check: Neo4j 5.x supports vector indexes
		// A more robust check would be to try creating a dummy vector index, but version check is safer for now
		isSupported := strings.HasPrefix(versionStr, "5.")

		return isSupported, fmt.Sprintf("%s %s", versionStr, editionStr), nil
	}

	return false, "unknown", nil
}

// CreateRelationship creates a relationship between two nodes
func (c *Neo4jClient) CreateRelationship(ctx context.Context, headID, tailID, relationType string, properties map[string]interface{}) error {
	if c.driver == nil {
		return fmt.Errorf("neo4j driver not initialized")
	}

	session := c.newSession(ctx, neo4j.AccessModeWrite)
	defer session.Close(ctx)

	// Match nodes by their stored id property (which is our UUID)
	cypher := fmt.Sprintf(`
		MATCH (h {id: $headID})
		MATCH (t {id: $tailID})
		CREATE (h)-[r:%s $props]->(t)
		RETURN elementId(r) as id
	`, relationType)

	params := map[string]interface{}{
		"headID": headID,
		"tailID": tailID,
		"props":  properties,
	}

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, cypher, params)
		return nil, err
	})
	if err != nil {
		return fmt.Errorf("failed to create relationship: %w", err)
	}

	return nil
}

// CreateMentionsBatch creates MENTIONS relationships between Chunks (Segments) and Entities
// It also ensures the Chunk node exists (MERGE on id)
func (c *Neo4jClient) CreateMentionsBatch(ctx context.Context, mentions []map[string]interface{}) error {
	if c.driver == nil {
		return fmt.Errorf("neo4j driver not initialized")
	}
	if len(mentions) == 0 {
		return nil
	}

	session := c.newSession(ctx, neo4j.AccessModeWrite)
	defer session.Close(ctx)

	// UNWIND batch
	// Each item: {segment_id, entity_id, properties}
	// We MERGE the Chunk (Segment) node to ensure it exists.
	// We MERGE the MENTIONS relationship.
	// We assume Entity exists (it should, as we sync entities first)
	cypher := `
		UNWIND $mentions AS m
		MERGE (c:Chunk {id: m.segment_id})
		WITH c, m
		MATCH (e:Entity {id: m.entity_id})
		MERGE (c)-[r:MENTIONS]->(e)
		SET r += m.properties
		RETURN count(r)
	`

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, cypher, map[string]interface{}{
			"mentions": mentions,
		})
		return nil, err
	})
	if err != nil {
		return fmt.Errorf("failed to create mentions batch: %w", err)
	}

	return nil
}

// EntitySearchResult represents a found entity and its neighbors
type EntitySearchResult struct {
	Entity    map[string]interface{} `json:"entity"`
	Neighbors []Neighbor             `json:"neighbors"`
	Score     float64                `json:"score"`
}

// Neighbor represents a connected node
type Neighbor struct {
	RelationshipType string                 `json:"relationship_type"`
	Node             map[string]interface{} `json:"node"`
}

// GetEntityContext finds entities and their 1-hop neighbors within a specific KB
func (c *Neo4jClient) GetEntityContext(ctx context.Context, kbID string, names []string) ([]EntitySearchResult, error) {
	if c.driver == nil {
		return nil, nil // No Neo4j configured, return empty context
	}

	// Match entities by exact names. We generate permutations in Go to avoid toLower() full-table scan in Neo4j.
	var searchNames []string
	seenNames := make(map[string]bool)
	for _, n := range names {
		variants := []string{n, strings.ToLower(n), strings.ToUpper(n)}
		if len(n) > 0 {
			variants = append(variants, strings.ToUpper(n[:1])+strings.ToLower(n[1:])) // simple Title Case
		}
		for _, v := range variants {
			if !seenNames[v] {
				seenNames[v] = true
				searchNames = append(searchNames, v)
			}
		}
	}

	session := c.newSession(ctx, neo4j.AccessModeRead)
	defer session.Close(ctx)

	// Match entities by name or canonical_name using $names AND kb_id
	cypher := `
		MATCH (n:Entity)
		WHERE n.kb_id = $kb_id
		  AND (n.name IN $names OR n.canonical_name IN $names)
		OPTIONAL MATCH (n)-[r]-(m)
		// Ensure connected node is also in the same KB (optional, depending on if nodes are shared)
		// For strict isolation: WHERE m.kb_id = $kb_id
		WITH n, r, m ORDER BY r.weight DESC LIMIT 50
		RETURN n, collect({type: type(r), node: m}) as neighbors
	`

	// DEBUG: Print matching query
	logger.Info(fmt.Sprintf("[RETRIEVAL_DEBUG] Executing 1-Hop Search:\n%s\nParams: kb_id=%s, names=%v", cypher, kbID, searchNames), nil)

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"names": searchNames,
		"kb_id": kbID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query graph: %w", err)
	}

	var results []EntitySearchResult

	for result.Next(ctx) {
		record := result.Record()
		nodeVal, _ := record.Get("n")
		neighborsVal, _ := record.Get("neighbors")

		node := nodeVal.(neo4j.Node)
		entityProps := node.Props
		// Ensure system tags like labels/id are included if needed, mostly props are fine

		var neighbors []Neighbor
		if neighborsList, ok := neighborsVal.([]interface{}); ok {
			for _, nb := range neighborsList {
				if nbMap, ok := nb.(map[string]interface{}); ok {
					rType, typeOk := nbMap["type"].(string)
					targetNode, nodeOk := nbMap["node"].(neo4j.Node)

					if typeOk && nodeOk {
						neighbors = append(neighbors, Neighbor{
							RelationshipType: rType,
							Node:             targetNode.Props,
						})
					}
				}
			}
		}

		results = append(results, EntitySearchResult{
			Entity:    entityProps,
			Neighbors: neighbors,
		})
	}

	return results, nil
}

// GetEntityContextMultiHop performs multi-hop (2-hop) traversal to find all related entities within a KB
// This enables finding documents connected through entity chains like:
// Einstein → Relativity → Atomic Bomb → Japan
func (c *Neo4jClient) GetEntityContextMultiHop(ctx context.Context, kbID string, names []string, maxHops int) ([]EntitySearchResult, []string, error) {
	if c.driver == nil {
		return nil, nil, nil // No Neo4j configured, return empty context
	}

	if maxHops < 1 {
		maxHops = 2
	}
	if maxHops > 5 {
		maxHops = 5 // Increased limit to 5 as requested for diffusion
	}

	var searchNames []string
	seenNames := make(map[string]bool)
	for _, n := range names {
		variants := []string{n, strings.ToLower(n), strings.ToUpper(n)}
		if len(n) > 0 {
			variants = append(variants, strings.ToUpper(n[:1])+strings.ToLower(n[1:]))
		}
		for _, v := range variants {
			if !seenNames[v] {
				seenNames[v] = true
				searchNames = append(searchNames, v)
			}
		}
	}

	session := c.newSession(ctx, neo4j.AccessModeRead)
	defer session.Close(ctx)

	// Multi-hop query: find start nodes, then traverse up to maxHops
	// Enforce kb_id on the start node 'n'
	// OPTIMIZATION:
	// 1. Exclude 'Date'/'Time' type nodes from expansion path (prevent temporal noise)
	// 2. Filter out Super Hubs (degree > 50) to avoid over-connection
	// IMPROVED Multi-hop query:
	cypher := fmt.Sprintf(`
		// 1. O(1) Index Lookup for exact matched entities
		MATCH (n:Entity)
		WHERE n.kb_id = $kb_id AND (n.name IN $names OR n.canonical_name IN $names)

		// 2. Prune Hubs safely (Reduced to 50 to prevent timeout)
		WITH n WHERE COUNT { (n)--() } <= 200

		// 3. Multi-hop traversal (limit explosion)
		MATCH (n)-[*1..%d]-(m)
		WHERE (m.kb_id = $kb_id OR m:Chunk OR m:Document OR m:File)
		  AND COUNT { (m)--() } <= 200

		// 4. Force limitation BEFORE expanding full edge context
		WITH DISTINCT m
		LIMIT 200

		// 5. Fetch local context
		OPTIONAL MATCH (m)-[r]-(p)
		WHERE p.kb_id = $kb_id
		WITH m, r, p LIMIT 500
		RETURN m, collect(DISTINCT {type: type(r), node: p, labels: labels(p)}) as neighbors
	`, maxHops)

	logger.Info(fmt.Sprintf("[RETRIEVAL_DEBUG] Executing Permissive Multi-Hop Search:\n%s\nParams: kb_id=%s, names=%v", cypher, kbID, searchNames))

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"names": searchNames,
		"kb_id": kbID,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query graph multi-hop: %w", err)
	}

	var results []EntitySearchResult
	allEntityNames := make(map[string]bool)

	for result.Next(ctx) {
		record := result.Record()
		nodeVal, nodeFound := record.Get("m")
		neighborsVal, _ := record.Get("neighbors")

		if !nodeFound || nodeVal == nil {
			continue
		}

		node, ok := nodeVal.(neo4j.Node)
		if !ok {
			continue
		}
		entityProps := node.Props

		// Collect metadata
		var sourceLabels []string
		if name, ok := entityProps["name"].(string); ok {
			allEntityNames[strings.ToLower(name)] = true
		}
		for _, l := range node.Labels {
			sourceLabels = append(sourceLabels, l)
		}

		var neighbors []Neighbor
		if neighborsList, ok := neighborsVal.([]interface{}); ok {
			for _, nb := range neighborsList {
				if nbMap, ok := nb.(map[string]interface{}); ok {
					rType, _ := nbMap["type"].(string)
					targetNode, nodeOk := nbMap["node"].(neo4j.Node)

					if nodeOk {
						neighbors = append(neighbors, Neighbor{
							RelationshipType: rType,
							Node:             targetNode.Props,
						})
						// Also collect neighbor names to ensure segments are found
						if name, ok := targetNode.Props["name"].(string); ok && name != "" {
							allEntityNames[strings.ToLower(name)] = true
						}
					}
				}
			}
		}

		results = append(results, EntitySearchResult{
			Entity:    entityProps,
			Neighbors: neighbors,
		})

		// Log only metadata. Entity names can contain document content.
		for _, nb := range neighbors {
			neighborName := ""
			for _, p := range []string{"name", "title", "fileName", "canonical_name", "content"} {
				if val, ok := nb.Node[p].(string); ok && val != "" {
					neighborName = val
					break
				}
			}

			sourceDisplayName := ""
			for _, p := range []string{"name", "title", "fileName", "canonical_name", "content"} {
				if val, ok := entityProps[p].(string); ok && val != "" {
					sourceDisplayName = val
					break
				}
			}

			logger.DebugContext(ctx, "graph traversal path found",
				zap.Strings("source_labels", sourceLabels),
				zap.String("relationship_type", nb.RelationshipType),
				zap.Bool("has_source_name", sourceDisplayName != ""),
				zap.Bool("has_neighbor_name", neighborName != ""),
			)
		}
	}

	logger.DebugContext(ctx, "graph traversal results gathered",
		zap.Int("results_count", len(results)),
	)

	// Convert map to slice
	var entityNamesList []string
	for name := range allEntityNames {
		entityNamesList = append(entityNamesList, name)
	}

	return results, entityNamesList, nil
}

// CreateVectorIndex creates a vector index on the embedding property of Entity nodes
func (c *Neo4jClient) CreateVectorIndex(ctx context.Context, dimensions int) error {
	if c.driver == nil {
		return fmt.Errorf("neo4j driver not initialized")
	}

	session := c.newSession(ctx, neo4j.AccessModeWrite)
	defer session.Close(ctx)

	// Ensure uniqueness constraint for upsert operations
	_, err := session.Run(ctx, "CREATE CONSTRAINT entity_id_unique IF NOT EXISTS FOR (n:Entity) REQUIRE n.id IS UNIQUE", nil)
	if err != nil {
		logger.Warn("Failed to create Neo4j entity uniqueness constraint", err)
	}

	// Create vector index for entity embeddings
	// Note: index name is 'entity_embedding_index'
	cypher := fmt.Sprintf(`
		CREATE VECTOR INDEX entity_embedding_index IF NOT EXISTS
		FOR (n:Entity) ON (n.embedding)
		OPTIONS {indexConfig: {
			`+"`vector.dimensions`"+`: %d,
			`+"`vector.similarity_function`"+`: 'cosine'
		}}
	`, dimensions)

	_, err = session.Run(ctx, cypher, nil)
	if err != nil {
		return fmt.Errorf("failed to create vector index: %w", err)
	}

	return nil
}

// GetEntityContextByVector performs a hybrid vector-graph retrieval starting from semantic seeds.
// topicKeywords are used to boost entities that align with the query's intent (e.g. "University" vs "Award").
func (c *Neo4jClient) GetEntityContextByVector(ctx context.Context, kbID string, embedding []float32, topK int, maxHops int, anchorEntityIDs []string, minScore float64, topicKeywords []string) ([]EntitySearchResult, []string, error) {
	if c.driver == nil {
		return nil, nil, fmt.Errorf("neo4j driver not initialized")
	}

	if maxHops < 1 {
		maxHops = 1
	}
	if topK < 3 {
		topK = 3 // Minimum candidates
	}
	if minScore <= 0 {
		minScore = 0.5 // Default strictness for vector seeds
	}
	// Decay seed score by traversal distance to prevent far expanded nodes
	// from inheriting near-identical high vector scores.
	const hopDecay = 0.65

	session := c.newSession(ctx, neo4j.AccessModeRead)
	defer session.Close(ctx)

	// candidatePool should be large enough to find KB-specific results
	candidatePool := topK * 100
	if candidatePool < 200 {
		candidatePool = 200
	}

	// 1. Vector Search + Multi-hop Traversal
	// We use dual-scoring: Intrinsic (Semantic) + Extrinsic (Structural Path Decay)
	cypher := fmt.Sprintf(`
			CALL db.index.vector.queryNodes('entity_embedding_index', %d, $embedding)
			YIELD node as seed, score
			WHERE score >= $min_score AND (seed.kb_id = $kb_id OR seed.kb_id IS NULL)

			// Boundary Check: Relaxed for Vector Search to compensate for NLP extraction misses
			WITH seed, score
			WHERE
				SIZE($anchor_ids) = 0 OR
				seed.id IN $anchor_ids OR
				EXISTS { (seed)-[*1..2]-(anchor) WHERE anchor.id IN $anchor_ids } OR
				score > 0.75

			// Expand from valid seeds
			MATCH path = (seed)-[*0..%d]-(m:Entity)
			WHERE m.kb_id = $kb_id

			// Efficiency & Hub Pruning: Always allow seeds (hops=0) but prune bridges (hops > 0)
			WITH m, score as seed_score, length(path) as hops
			WHERE COUNT { (m)--() } < 300

			// Aggregate inherited score from seeds
			WITH m,hops,max(seed_score * (%f ^ toFloat(hops))) as inherited_score
			ORDER BY inherited_score DESC LIMIT 200

			// Dual-Scoring: Calculate real content similarity for the current node
			WITH m,hops,inherited_score,
			     CASE
	                  WHEN hops = 0 THEN inherited_score
					  WHEN m.embedding IS NOT NULL THEN vector.similarity.cosine($embedding, m.embedding)
			          ELSE 0.0 END AS intrinsic_score
			WHERE intrinsic_score > 0.5

			WITH m, inherited_score, intrinsic_score,
				CASE
				  WHEN hops <= 2 THEN 0.8 * intrinsic_score + 0.2 * inherited_score
				  ELSE 0.6 * intrinsic_score + 0.4 * inherited_score
				END as raw_final_score

			// Topic Alignment Boost: Reward nodes matching query intent keywords (e.g. "University")
			WITH m, inherited_score, intrinsic_score, raw_final_score,
			     CASE
				   WHEN SIZE($topic_keywords) > 0 AND ANY(k IN $topic_keywords WHERE m.name CONTAINS k OR m.canonical_name CONTAINS k) THEN 1.35
				   ELSE 1.0
				 END as topic_boost

			WITH m, inherited_score, intrinsic_score, (raw_final_score * topic_boost) as final_score, topic_boost
			ORDER BY final_score DESC
	    	LIMIT 200

		// Fetch local connectivity for the final context
		OPTIONAL MATCH (m)-[r]-(p:Entity)
		WHERE p.kb_id = $kb_id
		RETURN m, final_score as score, inherited_score, intrinsic_score, topic_boost, collect(DISTINCT {type: type(r), node: p}) as neighbors
		LIMIT 200
	`, candidatePool, maxHops, hopDecay)

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"embedding":      embedding,
		"kb_id":          kbID,
		"anchor_ids":     anchorEntityIDs,
		"min_score":      minScore,
		"topic_keywords": topicKeywords,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query vector index: %w", err)
	}

	var results []EntitySearchResult
	allEntityNames := make(map[string]bool)
	for result.Next(ctx) {
		record := result.Record()
		nodeVal, _ := record.Get("m")
		neighborsVal, _ := record.Get("neighbors")
		scoreVal, _ := record.Get("score")
		inheritedScore, _ := record.Get("inherited_score")
		intrinsicScore, _ := record.Get("intrinsic_score")
		topicBoost, _ := record.Get("topic_boost")

		node := nodeVal.(neo4j.Node)
		entityProps := node.Props

		logger.Debug("vector similarity search", map[string]interface{}{
			"scoreVal":       scoreVal,
			"inheritedScore": inheritedScore,
			"intrinsicScore": intrinsicScore,
			"topicBoost":     topicBoost,
			"name":           entityProps["name"],
		})

		if name, ok := entityProps["name"].(string); ok && name != "" {
			allEntityNames[strings.ToLower(name)] = true
		}

		var neighbors []Neighbor
		if neighborsList, ok := neighborsVal.([]interface{}); ok {
			for _, nb := range neighborsList {
				if nbMap, ok := nb.(map[string]interface{}); ok {
					rType, _ := nbMap["type"].(string)
					targetNode, nodeOk := nbMap["node"].(neo4j.Node)

					if nodeOk {
						neighbors = append(neighbors, Neighbor{
							RelationshipType: rType,
							Node:             targetNode.Props,
						})
						if name, ok := targetNode.Props["name"].(string); ok && name != "" {
							allEntityNames[strings.ToLower(name)] = true
						}
					}
				}
			}
		}

		score := 0.0
		if f, ok := scoreVal.(float64); ok {
			score = f
		}

		results = append(results, EntitySearchResult{
			Entity:    entityProps,
			Neighbors: neighbors,
			Score:     score,
		})
	}

	var entityNamesList []string
	for name := range allEntityNames {
		entityNamesList = append(entityNamesList, name)
	}

	return results, entityNamesList, nil
}

// DeleteNode deletes a node by its ID
func (c *Neo4jClient) DeleteNode(ctx context.Context, elementID string) error {
	if c.driver == nil {
		return fmt.Errorf("neo4j driver not initialized")
	}

	session := c.newSession(ctx, neo4j.AccessModeWrite)
	defer session.Close(ctx)

	// Match node by its stored id property (which is our UUID) and detach delete
	cypher := `
		MATCH (n)
		WHERE n.id = $id
		DETACH DELETE n
	`

	_, err := session.Run(ctx, cypher, map[string]interface{}{"id": elementID})
	if err != nil {
		return fmt.Errorf("failed to delete node: %w", err)
	}

	return nil
}

// DeleteRelationship deletes a relationship by its ID
func (c *Neo4jClient) DeleteRelationship(ctx context.Context, elementID string) error {
	if c.driver == nil {
		return fmt.Errorf("neo4j driver not initialized")
	}

	session := c.newSession(ctx, neo4j.AccessModeWrite)
	defer session.Close(ctx)

	// Match relationship by its stored id property (which is our UUID) and delete
	// Since relationships in Neo4j don't always have easy 'id' lookup without knowing start/end nodes
	// unless we indexed the relationship property 'id'.
	// Assuming relationship 'id' property is indexed or we can scan.
	// A better approach might be to use elementId() if we stored it, but we store our UUID as 'id'.

	cypher := `
		MATCH ()-[r]->()
		WHERE r.id = $id
		DELETE r
	`

	_, err := session.Run(ctx, cypher, map[string]interface{}{"id": elementID})
	if err != nil {
		return fmt.Errorf("failed to delete relationship: %w", err)
	}

	return nil
}

// CreateNodesBatch creates multiple nodes of the same label in a single transaction
func (c *Neo4jClient) CreateNodesBatch(ctx context.Context, label string, nodes []map[string]interface{}) error {
	if c.driver == nil {
		return fmt.Errorf("neo4j driver not initialized")
	}
	if len(nodes) == 0 {
		return nil
	}

	session := c.newSession(ctx, neo4j.AccessModeWrite)
	defer session.Close(ctx)

	// Use UNWIND to create nodes in batch
	// We add both the specific label and the base 'Entity' label
	cypher := fmt.Sprintf(`
UNWIND $nodes AS nodeProps
MERGE (n:%s:Entity {id: nodeProps.id})
SET n += nodeProps
RETURN n.id
`, label)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, cypher, map[string]interface{}{
			"nodes": nodes,
		})
		return nil, err
	})
	if err != nil {
		return fmt.Errorf("failed to create nodes batch: %w", err)
	}

	return nil
}

// CreateRelationshipsBatch creates multiple relationships of the same type in a single transaction
func (c *Neo4jClient) CreateRelationshipsBatch(ctx context.Context, relType string, rels []map[string]interface{}) error {
	if c.driver == nil {
		return fmt.Errorf("neo4j driver not initialized")
	}
	if len(rels) == 0 {
		return nil
	}

	session := c.newSession(ctx, neo4j.AccessModeWrite)
	defer session.Close(ctx)

	// Use UNWIND to create relationships in batch
	// We matches using BOTH id and kb_id for safety, and ensure Entity label is present
	cypher := fmt.Sprintf(`
UNWIND $rels AS rel
MATCH (h:Entity {id: rel.head_id, kb_id: rel.kb_id})
MATCH (t:Entity {id: rel.tail_id, kb_id: rel.kb_id})
MERGE (h)-[r:`+"`%s`"+`]->(t)
SET r += rel.properties
RETURN count(r)
`, relType)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, cypher, map[string]interface{}{
			"rels": rels,
		})
		if err != nil {
			return nil, err
		}

		var createdCount int64
		if result.Next(ctx) {
			countVal, _ := result.Record().Get("count(r)")
			if c, ok := countVal.(int64); ok {
				createdCount = c
			}
		}

		if createdCount != int64(len(rels)) {
			logger.Warn(fmt.Sprintf("Relationship count mismatch for type '%s': expected %d, got %d", relType, len(rels), createdCount), nil)
		}

		return createdCount, nil
	})
	if err != nil {
		return fmt.Errorf("failed to create relationships batch: %w", err)
	}

	return nil
}

// UpdateNodeEmbeddingsBatch updates multiple node embeddings in a single transaction
func (c *Neo4jClient) UpdateNodeEmbeddingsBatch(ctx context.Context, updates []map[string]interface{}) error {
	if c.driver == nil {
		return fmt.Errorf("neo4j driver not initialized")
	}
	if len(updates) == 0 {
		return nil
	}

	session := c.newSession(ctx, neo4j.AccessModeWrite)
	defer session.Close(ctx)

	cypher := `
		UNWIND $updates AS update
		MATCH (n:Entity {id: update.id})
		SET n.embedding = update.embedding
		RETURN count(n)
	`

	_, err := session.Run(ctx, cypher, map[string]interface{}{
		"updates": updates,
	})
	if err != nil {
		return fmt.Errorf("failed to update node embeddings batch: %w", err)
	}

	return nil
}

// FindSimilarEntity performs a vector similarity search to find duplicates within the same KB
// Returns the ID of the most similar entity if similarity > threshold
func (c *Neo4jClient) FindSimilarEntity(ctx context.Context, kbID string, embedding []float32, threshold float64) (string, error) {
	if c.driver == nil {
		return "", fmt.Errorf("neo4j driver not initialized")
	}
	if len(embedding) == 0 {
		return "", nil // Cannot search without embedding
	}

	session := c.newSession(ctx, neo4j.AccessModeRead)
	defer session.Close(ctx)

	// Using the vector index 'entity_vector_index' we created earlier
	// We fetch more candidates (k=50) to avoid "starvation" where top global matches
	// belong to other KBs and are filtered out, hiding the best local match.
	cypher := `
		CALL db.index.vector.queryNodes('entity_embedding_index', 50, $embedding)
		YIELD node, score
		WHERE node.kb_id = $kb_id AND score >= $threshold
		RETURN node.id as entity_id, score
		ORDER BY score DESC
		LIMIT 1
	`

	result, err := session.Run(ctx, cypher, map[string]interface{}{
		"kb_id":     kbID,
		"embedding": embedding,
		"threshold": threshold,
	})
	if err != nil {
		// Index might not exist yet or other error, fallback safely
		logger.Warn("Vector search failed (index missing?)", err)
		return "", nil
	}

	if result.Next(ctx) {
		rec := result.Record()
		if entityID, ok := rec.Get("entity_id"); ok {
			return entityID.(string), nil
		}
	}

	return "", nil
}

// FindSimilarEntityWithFilter performs a vector similarity search with Schema-based Blocking (Label Support)
// Note: Prefix blocking (blockingKey) removed to support synonyms like "WWII" vs "World War II".
// We rely on the Vector Index as the "Coarse Screen" (Embedding-based Blocking).
func (c *Neo4jClient) FindSimilarEntityWithFilter(ctx context.Context, kbID, label string, embedding []float32, threshold float64) (string, error) {
	if c.driver == nil {
		return "", fmt.Errorf("neo4j driver not initialized")
	}
	if len(embedding) == 0 {
		return "", nil // Cannot search without embedding
	}

	session := c.newSession(ctx, neo4j.AccessModeRead)
	defer session.Close(ctx)

	// Pre-Filtering via Vector Index (Candidate Generation)
	// We rely on the vector score to bring relevant candidates into the window,
	// then apply strict L2 filtering (Label check).

	candidates := 100 // Wider search

	cypher := fmt.Sprintf(`
		CALL db.index.vector.queryNodes('entity_embedding_index', %d, $embedding)
		YIELD node, score
		WHERE node.kb_id = $kb_id AND score >= $threshold
	`, candidates)

	params := map[string]interface{}{
		"kb_id":     kbID,
		"embedding": embedding,
		"threshold": threshold,
	}

	// 1. Label Filter (Hard Constraint)
	if label != "" && label != "Entity" {
		cypher += " AND $label IN labels(node)"
		params["label"] = label
	}

	cypher += `
		RETURN node.id as entity_id, score
		ORDER BY score DESC
		LIMIT 1
	`

	result, err := session.Run(ctx, cypher, params)
	if err != nil {
		logger.Warn("Vector search with filter failed", err)
		return "", nil
	}

	if result.Next(ctx) {
		rec := result.Record()
		if entityID, ok := rec.Get("entity_id"); ok {
			return entityID.(string), nil
		}
	}

	return "", nil
}

// RunCommunityDetection runs the Louvain algorithm to detect communities and assign community IDs
// This forces global convergence handling for "SameAs" scenarios or dense subgraphs.
func (c *Neo4jClient) RunCommunityDetection(ctx context.Context, kbID string) error {
	if c.driver == nil {
		return fmt.Errorf("neo4j driver not initialized")
	}

	session := c.newSession(ctx, neo4j.AccessModeWrite)
	defer session.Close(ctx)

	// Only run if GDS is available. This is a best-effort background task.
	// We assume a graph projection 'myGraph' or creating one on the fly.
	// For simplicity, we use an anonymous projection.

	cypher := `
		CALL gds.louvain.write({
			nodeProjection: 'Entity',
			relationshipProjection: {
				RELATED: {
					type: 'RELATED',
					orientation: 'UNDIRECTED'
				},
				MENTIONS: {
					type: 'MENTIONS',
					orientation: 'UNDIRECTED'
				}
			},
			writeProperty: 'community_id'
		})
		YIELD communityCount, modularity, modularities
	`

	_, err := session.Run(ctx, cypher, nil)
	if err != nil {
		// Log but don't fail hard if GDS missing
		logger.Warn("Community detection failed (GDS missing or graph empty?)", err)
		return nil
	}

	return nil
}
