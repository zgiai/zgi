package graphconfig

import (
	"fmt"

	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func ExtractNodeType(config map[string]any) (shared.NodeType, error) {
	if config == nil {
		return "", fmt.Errorf("node config is nil")
	}

	dataMap := ToMap(config["data"])
	if dataMap != nil {
		if nodeType, ok := dataMap["type"].(string); ok && nodeType != "" {
			return ParseNodeType(nodeType)
		}
	}

	if nodeType, ok := config["type"].(string); ok && nodeType != "" {
		return ParseNodeType(nodeType)
	}

	if dataMap == nil {
		return "", fmt.Errorf("node data is missing")
	}
	return "", fmt.Errorf("node type missing")
}

func ParseNodeType(raw string) (shared.NodeType, error) {
	if raw == "" {
		return "", fmt.Errorf("node type missing")
	}

	nodeType := normalizeNodeType(raw)
	if !shared.IsExecutableNodeType(nodeType) {
		return "", fmt.Errorf("unsupported node type %q", raw)
	}
	return nodeType, nil
}

func normalizeNodeType(raw string) shared.NodeType {
	switch raw {
	case "http_request":
		return shared.HTTPRequest
	default:
		return shared.NodeType(raw)
	}
}

func ToMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func Clone(original map[string]any) map[string]any {
	if original == nil {
		return map[string]any{}
	}
	clone := make(map[string]any, len(original))
	for key, value := range original {
		clone[key] = value
	}
	return clone
}

func ReachableFromStart(startNodeID string, edges []map[string]any) map[string]bool {
	adjacency := make(map[string][]string)
	for _, edge := range edges {
		source, _ := edge["source"].(string)
		target, _ := edge["target"].(string)
		if source != "" && target != "" {
			adjacency[source] = append(adjacency[source], target)
		}
	}

	visited := map[string]bool{startNodeID: true}
	queue := []string{startNodeID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, neighbor := range adjacency[current] {
			if !visited[neighbor] {
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}
	return visited
}
