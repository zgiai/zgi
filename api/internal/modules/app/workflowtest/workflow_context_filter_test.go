package workflowtest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildWorkflowRecognitionContextSkipsDisplayOnlyNodes(t *testing.T) {
	context := buildWorkflowRecognitionContext(map[string]any{
		"graph": map[string]any{
			"nodes": []any{
				map[string]any{"id": "start", "data": map[string]any{"type": "start", "title": "Start"}},
				map[string]any{"id": "note-1", "data": map[string]any{"type": "note", "title": "Run note"}},
				map[string]any{"id": "end", "data": map[string]any{"type": "end", "title": "End"}},
			},
			"edges": []any{
				map[string]any{"source": "start", "target": "note-1"},
				map[string]any{"source": "note-1", "target": "end"},
			},
		},
	})

	require.Contains(t, context, "Start")
	require.Contains(t, context, "End")
	require.False(t, strings.Contains(context, "Run note"))
	require.False(t, strings.Contains(context, "note-1"))
}
