package entities

type Graph struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Config    map[string]interface{} `json:"config"`
	Nodes     map[string]interface{} `json:"nodes"`
	Edges     []interface{}          `json:"edges"`
	Variables map[string]interface{} `json:"variables"`
	Metadata  map[string]interface{} `json:"metadata"`
}

func (g *Graph) GetRootNodeID() string {
	if rootID, ok := g.Config["root_node_id"].(string); ok {
		return rootID
	}
	return ""
}

func (g *Graph) GetNodeIDs() []string {
	var nodeIDs []string
	for nodeID := range g.Nodes {
		nodeIDs = append(nodeIDs, nodeID)
	}
	return nodeIDs
}

func (g *Graph) GetNodeConfig(nodeID string) map[string]interface{} {
	if nodeConfig, ok := g.Nodes[nodeID].(map[string]interface{}); ok {
		return nodeConfig
	}
	return nil
}

func (g *Graph) GetEdges() map[string][]interface{} {
	edgesMap := make(map[string][]interface{})
	for _, edge := range g.Edges {
		if edgeMap, ok := edge.(map[string]interface{}); ok {
			if source, ok := edgeMap["source"].(string); ok {
				edgesMap[source] = append(edgesMap[source], edge)
			}
		}
	}
	return edgesMap
}
