package graph

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// ScoredNode represents a node with its relevance score and context edges
type ScoredNode struct {
	Node     *Node
	Score    float64
	IsAnchor bool
	Source   string // "anchor" or "expanded"
	Edges    []Edge // Path edges contributing to this node's context
}

// Edge represents a simplified graph relationship
type Edge struct {
	Head string
	Tail string
	Type string
}

// Node used in internal logic (simplified representation)
type Node struct {
	ID     int64
	Labels []string
	Props  map[string]any
	Degree int
	Name   string // Helper for edge construction
}

// ExpansionConfig defines how aggressive the search should be
type ExpansionConfig struct {
	MaxHops   int
	MaxLimit  int
	EdgeTypes []string // Optional: filter specifically for "Super Nodes"
}

// RetrieveEnrichedContext implements the "Smart Expansion" protocol to retrieve relevant graph context
func (c *Neo4jClient) RetrieveEnrichedContext(ctx context.Context, kbID string, keywords []string) ([]*ScoredNode, error) {
	if c.driver == nil {
		return nil, fmt.Errorf("neo4j driver is nil")
	}

	// 1. Anchor Search
	anchors, err := c.findAnchors(ctx, kbID, keywords)
	if err != nil {
		return nil, fmt.Errorf("failed to find anchors: %w", err)
	}

	if len(anchors) == 0 {
		return []*ScoredNode{}, nil
	}

	// DEDUPLICATE: Ensure each unique node ID is expanded only once
	uniqueAnchors := make([]*Node, 0)
	seenAnchorIDs := make(map[int64]bool)
	for _, a := range anchors {
		if !seenAnchorIDs[a.ID] {
			seenAnchorIDs[a.ID] = true
			uniqueAnchors = append(uniqueAnchors, a)
		}
	}

	// Map to track unique nodes (ID -> ScoredNode)
	nodeMap := make(map[int64]*ScoredNode)
	var mu sync.Mutex

	// Helper to safely add/update nodes
	addNode := func(n *Node, source string, baseScore float64, isAnchor bool, edges []Edge) {
		mu.Lock()
		defer mu.Unlock()

		if existing, exists := nodeMap[n.ID]; exists {
			if isAnchor {
				existing.IsAnchor = true
			}
			// Merge edges if new ones found (simple append, dedup later if needed)
			if len(edges) > 0 {
				existing.Edges = append(existing.Edges, edges...)
			}
			return
		}

		score := baseScore
		// CRITICAL: Hard Rank Guarantee for Anchors
		if isAnchor {
			score += 10.0
		}

		nodeMap[n.ID] = &ScoredNode{
			Node:     n,
			Score:    score,
			IsAnchor: isAnchor,
			Source:   source,
			Edges:    edges,
		}
	}

	// Add Unique Anchors to map immediately
	for _, anchor := range uniqueAnchors {
		addNode(anchor, "anchor", 10.0, true, nil)
	}

	// 2. Adaptive Expansion (Parallel)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)

	for _, anchor := range uniqueAnchors {
		wg.Add(1)
		go func(a *Node) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Expansion Strategy: Adapt coverage based on node connectivity
			config := ExpansionConfig{}
			if a.Degree > 50 {
				config.MaxHops = 1
				config.MaxLimit = 10
			} else {
				config.MaxHops = 2
				config.MaxLimit = 30 // Reduced from 50 to prevent path explosion in large sub-graphs
			}

			neighbors, err := c.expandNode(ctx, kbID, a.ID, config)
			if err != nil {
				return
			}

			for _, neighbor := range neighbors {
				dist := len(neighbor.Edges)
				if dist == 0 {
					dist = 1
				}

				topoScore := float64(neighbor.Node.Degree)
				score := ((topoScore * 0.1) + 2.0) / float64(dist+1)

				addNode(neighbor.Node, "expanded", score, false, neighbor.Edges)
			}
		}(anchor)
	}

	wg.Wait()

	// 3. Convert to slice and Sort
	results := make([]*ScoredNode, 0, len(nodeMap))
	for _, sn := range nodeMap {
		results = append(results, sn)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

// findAnchors locates the initial entry points based on keywords
func (c *Neo4jClient) findAnchors(ctx context.Context, kbID string, keywords []string) ([]*Node, error) {
	session := c.newSession(ctx, neo4j.AccessModeRead)
	defer session.Close(ctx)

	var searchNames []string
	seenNames := make(map[string]bool)
	for _, k := range keywords {
		variants := []string{k, strings.ToLower(k), strings.ToUpper(k)}
		if len(k) > 0 {
			variants = append(variants, strings.ToUpper(k[:1])+strings.ToLower(k[1:]))
		}
		for _, v := range variants {
			if !seenNames[v] {
				seenNames[v] = true
				searchNames = append(searchNames, v)
			}
		}
	}

	cypherQuery := `
		MATCH (n:Entity)
		WHERE n.kb_id = $kb_id
		AND (n.name IN $keywords OR n.canonical_name IN $keywords)
		
		WITH n, COUNT { (n)--() } as degree
		
		RETURN id(n) as id, labels(n) as labels, properties(n) as props, degree
		LIMIT 50
	`

	result, err := session.Run(ctx, cypherQuery, map[string]interface{}{
		"kb_id":    kbID,
		"keywords": searchNames,
	})
	if err != nil {
		return nil, err
	}

	var anchors []*Node
	for result.Next(ctx) {
		record := result.Record()
		idVal, _ := record.Get("id")
		labelsVal, _ := record.Get("labels")
		propsVal, _ := record.Get("props")
		degreeVal, _ := record.Get("degree")

		id, _ := idVal.(int64)
		labels, _ := labelsVal.([]interface{})
		props, _ := propsVal.(map[string]interface{})
		degree, _ := degreeVal.(int64)

		nodeLabels := make([]string, len(labels))
		for i, l := range labels {
			nodeLabels[i] = fmt.Sprintf("%v", l)
		}

		rName, _ := props["name"].(string)
		if rName == "" {
			// Fallback to title or fileName for document nodes
			if t, ok := props["title"].(string); ok {
				rName = t
			} else if f, ok := props["fileName"].(string); ok {
				rName = f
			}
		}

		anchors = append(anchors, &Node{
			ID:     id,
			Labels: nodeLabels,
			Props:  props,
			Degree: int(degree),
			Name:   rName,
		})
	}

	// [DIAGNOSTIC] If no anchors found with kb_id, try a global search to see what's actually there
	if len(anchors) == 0 {
		logger.Warn(fmt.Sprintf("[DIAG_GRAPH] No anchors found for kb_id='%s' with keywords %v. Trying global search...", kbID, searchNames), nil)
		diagQuery := `
			MATCH (n:Entity)
			WHERE (n.name IN $keywords OR n.canonical_name IN $keywords)
			RETURN n.kb_id as real_kb_id, n.name as name, n.title as title, labels(n) as labels LIMIT 3
		`
		diagRes, _ := session.Run(ctx, diagQuery, map[string]interface{}{"keywords": searchNames})
		for diagRes.Next(ctx) {
			rec := diagRes.Record()
			realID, _ := rec.Get("real_kb_id")
			realName, _ := rec.Get("name")
			realTitle, _ := rec.Get("title")
			rLabels, _ := rec.Get("labels")
			logger.Warn(fmt.Sprintf("[DIAG_RESULT] NODE FOUND GLOBALLY: name='%v', title='%v', kb_id='%v', labels=%v", realName, realTitle, realID, rLabels), nil)
		}
	}

	return anchors, nil
}

// expandNode returns neighbors AND the edges connecting them
func (c *Neo4jClient) expandNode(ctx context.Context, kbID string, nodeID int64, config ExpansionConfig) ([]*ScoredNode, error) {
	session := c.newSession(ctx, neo4j.AccessModeRead)
	defer session.Close(ctx)

	hopsStr := fmt.Sprintf("1..%d", config.MaxHops)

	// Query returns node 'm' AND the path relationships 'rels'
	// We enforce kb_id and degree limits on ALL nodes in the path to prevent escaping the domain
	// or hitting Super-Hubs that cause path explosion.
	cypherQuery := fmt.Sprintf(`
		MATCH (start) WHERE id(start) = $start_id

		// Un-path expansion to find destination nodes uniquely
		MATCH (start)-[*%s]-(m)
		WHERE (m.kb_id = $kb_id OR m:Chunk OR m:Document OR m:File)
		  AND (m.type IS NULL OR NOT (m.type IN ['Date', 'Time']))
		  AND COUNT { (m)--() } <= 100

		WITH DISTINCT m
		LIMIT 100

		WITH m, COUNT { (m)--() } as degree

		// Extract local environment edges of dest m
		OPTIONAL MATCH (m)-[r]-(p)
		WHERE p.kb_id = $kb_id AND id(p) <> id(m)
		WITH m, degree, r, p LIMIT 500

		WITH m, degree, collect(DISTINCT {
			head: m.name,
			tail: p.name,
			type: type(r)
		}) as edges

		RETURN id(m) as id, labels(m) as labels, properties(m) as props, degree, edges
		ORDER BY degree DESC
		LIMIT $limit
	`, hopsStr)

	result, err := session.Run(ctx, cypherQuery, map[string]interface{}{
		"start_id": nodeID,
		"kb_id":    kbID,
		"limit":    config.MaxLimit,
	})
	if err != nil {
		return nil, err
	}

	var neighbors []*ScoredNode
	for result.Next(ctx) {
		record := result.Record()

		id, _ := record.Get("id")
		labelsVal, _ := record.Get("labels")
		propsVal, _ := record.Get("props")
		degreeVal, _ := record.Get("degree")
		edgesVal, _ := record.Get("edges")

		nodeID, _ := id.(int64)

		var labels []string
		if l, ok := labelsVal.([]interface{}); ok {
			for _, label := range l {
				if s, ok := label.(string); ok {
					labels = append(labels, s)
				}
			}
		}

		props, _ := propsVal.(map[string]interface{})
		degree, _ := degreeVal.(int64)
		name, _ := props["name"].(string)

		// Parse Edges
		var edges []Edge
		if edgeList, ok := edgesVal.([]interface{}); ok {
			for _, e := range edgeList {
				if eMap, ok := e.(map[string]interface{}); ok {
					head, _ := eMap["head"].(string)
					tail, _ := eMap["tail"].(string)
					typ, _ := eMap["type"].(string)
					if head != "" && tail != "" {
						edges = append(edges, Edge{Head: head, Tail: tail, Type: typ})
					}
				}
			}
		}

		// Log labels and props for deep diagnosis
		var nodeLabels []string
		if l, ok := labelsVal.([]interface{}); ok {
			for _, lab := range l {
				if s, ok := lab.(string); ok {
					nodeLabels = append(nodeLabels, s)
				}
			}
		}
		logger.Debug(fmt.Sprintf("expandNode FOUND: node_id=%d, labels=%v, edges_count=%d", nodeID, nodeLabels, len(edges)))

		neighbors = append(neighbors, &ScoredNode{
			Node: &Node{
				ID:     nodeID,
				Labels: labels,
				Props:  props,
				Degree: int(degree),
				Name:   name,
			},
			Edges: edges,
		})
	}

	return neighbors, nil
}
