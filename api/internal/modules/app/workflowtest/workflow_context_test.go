package workflowtest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildWorkflowRecognitionContextSummarizesDraftNodes(t *testing.T) {
	draft := map[string]any{
		"graph": map[string]any{
			"nodes": []any{
				map[string]any{
					"id": "start",
					"data": map[string]any{
						"type":  "start",
						"title": "Start",
						"variables": []any{
							map[string]any{"variable": "query", "type": "string", "label": "用户问题", "description": "用户原始输入"},
						},
					},
				},
				map[string]any{
					"id": "llm",
					"data": map[string]any{
						"type":  "llm",
						"title": "售后处理",
						"desc":  "判断退款和投诉",
						"prompt_template": []any{
							map[string]any{"role": "system", "text": "你负责识别售后退款、投诉升级和订单查询。"},
							map[string]any{"role": "user", "text": "{{#sys.query#}}"},
						},
					},
				},
				map[string]any{
					"id": "branch",
					"data": map[string]any{
						"type":  "if-else",
						"title": "意图分支",
						"cases": []any{
							map[string]any{
								"case_id": "refund",
								"conditions": []any{
									map[string]any{
										"variable_selector":   []any{"sys", "query"},
										"comparison_operator": "contains",
										"value":               "退款",
									},
								},
							},
						},
					},
				},
				map[string]any{
					"id": "qa",
					"data": map[string]any{
						"type":     "question-answer",
						"title":    "补充订单号",
						"question": "请提供订单号",
						"choices": []any{
							map[string]any{"label": "已有订单号", "value": "has_order"},
						},
						"extraction_fields": []any{
							map[string]any{"name": "order_id", "description": "订单号"},
						},
					},
				},
			},
			"edges": []any{
				map[string]any{"source": "start", "target": "llm"},
				map[string]any{"source": "llm", "target": "branch"},
				map[string]any{"source": "branch", "target": "qa"},
			},
		},
	}

	summary := buildWorkflowRecognitionContext(draft)

	require.Contains(t, summary, "Workflow structure summary")
	require.Contains(t, summary, "query")
	require.Contains(t, summary, "用户原始输入")
	require.Contains(t, summary, "售后处理")
	require.Contains(t, summary, "售后退款")
	require.Contains(t, summary, "投诉升级")
	require.Contains(t, summary, "sys.query contains 退款")
	require.Contains(t, summary, "请提供订单号")
	require.Contains(t, summary, "order_id")
	require.Contains(t, summary, "start -> llm")
	require.LessOrEqual(t, len(strings.Split(summary, "\n")), 80)
}

func TestBuildWorkflowRecognitionContextParsesStringGraphField(t *testing.T) {
	draft := map[string]any{
		"graph": `{"nodes":[{"id":"llm","data":{"type":"llm","title":"订单查询","prompt_template":[{"role":"system","text":"处理订单查询和物流进度"}]}}],"edges":[]}`,
	}

	summary := buildWorkflowRecognitionContext(draft)

	require.Contains(t, summary, "订单查询")
	require.Contains(t, summary, "处理订单查询和物流进度")
}
