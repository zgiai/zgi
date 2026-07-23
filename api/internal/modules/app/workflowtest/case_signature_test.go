package workflowtest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneratedCaseSignatureMatchesLegacyContentOnlyCase(t *testing.T) {
	registry := newGeneratedCaseSignatureRegistry([]Case{{
		Content:      "今日客诉热点",
		QuestionType: CaseTypeCore,
	}})

	_, ok := registry.reserve(GeneratedCase{
		Content:      " 今日客诉热点 ",
		QuestionType: CaseTypeCore,
		Turns:        []CaseTurn{{Role: "user", Content: "今日客诉热点"}},
	})

	require.False(t, ok)
}

func TestGeneratedCaseSignatureUsesConversationTurns(t *testing.T) {
	registry := newGeneratedCaseSignatureRegistry([]Case{{
		Content:      "先查制度再追问",
		QuestionType: CaseTypeCore,
		Turns: CaseTurns{
			{Role: "user", Content: "报销制度怎么规定？"},
			{Role: "user", Content: "那差旅费也一样吗？"},
		},
	}})

	_, ok := registry.reserve(GeneratedCase{
		Content:      "先查制度再追问",
		QuestionType: CaseTypeCore,
		Turns: []CaseTurn{
			{Role: "user", Content: "报销制度怎么规定？"},
			{Role: "assistant", Content: "这不是用户输入"},
			{Role: "user", Content: "那差旅费也一样吗？"},
		},
	})

	require.False(t, ok)
}

func TestGenerateUniqueCaseForIndexRetriesDuplicateCase(t *testing.T) {
	service := &Service{}
	generator := &fakeCaseGenerator{results: []*GenerateCasesResult{
		{Cases: []GeneratedCase{{
			Content:        "今日客诉热点",
			ExpectedResult: "生成简报",
			QuestionType:   CaseTypeCore,
		}}},
		{Cases: []GeneratedCase{{
			Content:        "服务工单趋势",
			ExpectedResult: "生成简报",
			QuestionType:   CaseTypeCore,
		}}},
	}}
	registry := newGeneratedCaseSignatureRegistry([]Case{{
		Content:      "今日客诉热点",
		QuestionType: CaseTypeCore,
	}})

	item, err := service.generateUniqueCaseForIndex(context.Background(), generator, GenerateCasesRequest{
		CaseMode: "task",
		Prompt:   "base prompt",
	}, GenerateCasesRequest{CaseMode: "task"}, 0, nil, registry)

	require.NoError(t, err)
	require.Equal(t, "服务工单趋势", item.Content)
	require.Len(t, generator.requests, 2)
	require.Contains(t, generator.requests[0].Prompt, "今日客诉热点")
	require.Contains(t, generator.requests[1].Prompt, "今日客诉热点")
}

func TestGeneratedCaseSignatureIncludesTaskVariablesAndFixtures(t *testing.T) {
	registry := newGeneratedCaseSignatureRegistry(nil)
	_, ok := registry.reserve(GeneratedCase{
		Content:      "请处理这个合同",
		QuestionType: CaseTypeCore,
		Turns: []CaseTurn{{
			Role:    "user",
			Content: "请处理这个合同",
			Inputs:  JSONMap{"document_type": "合同"},
		}},
		FileFixtures: []GeneratedFileFixture{{
			Title: "租赁合同",
			Facts: []string{"甲方：宏达公司"},
		}},
	})
	require.True(t, ok)

	_, ok = registry.reserve(GeneratedCase{
		Content:      "请处理这个合同",
		QuestionType: CaseTypeCore,
		Turns: []CaseTurn{{
			Role:    "user",
			Content: "请处理这个合同",
			Inputs:  JSONMap{"document_type": "发票"},
		}},
		FileFixtures: []GeneratedFileFixture{{
			Title: "租赁合同",
			Facts: []string{"甲方：宏达公司"},
		}},
	})
	require.True(t, ok)

	_, ok = registry.reserve(GeneratedCase{
		Content:      "请处理这个合同",
		QuestionType: CaseTypeCore,
		Turns: []CaseTurn{{
			Role:    "user",
			Content: "请处理这个合同",
			Inputs:  JSONMap{"document_type": "合同"},
		}},
		FileFixtures: []GeneratedFileFixture{{
			Title: "租赁合同",
			Facts: []string{"甲方：宏达公司"},
		}},
	})
	require.False(t, ok)
}

func TestGeneratedCaseSignatureRejectsSameTemplateWithChangedBusinessFacts(t *testing.T) {
	registry := newGeneratedCaseSignatureRegistry(nil)
	_, ok := registry.reserve(GeneratedCase{
		Content:        "今天北美区硬件产品线完成销售额420万元，同比增长8%，但渠道库存周转天数上升至45天，高于警戒线。请生成一份销售业绩简报。",
		ExpectedResult: "生成销售业绩简报",
		QuestionType:   CaseTypeCore,
	})
	require.True(t, ok)

	_, ok = registry.reserve(GeneratedCase{
		Content:        "今天华东区SaaS产品线的销售额比昨天下降了22%，但新客户注册量上涨了18%。请基于这个情况生成一份销售业绩简报。",
		ExpectedResult: "生成销售业绩简报",
		QuestionType:   CaseTypeCore,
	})
	require.False(t, ok)
}

func TestGeneratedCaseSignatureAllowsDifferentUserIntentInSameScenario(t *testing.T) {
	registry := newGeneratedCaseSignatureRegistry(nil)
	_, ok := registry.reserve(GeneratedCase{
		Content:      "今天北美区硬件产品线完成销售额420万元，同比增长8%，但渠道库存周转天数上升至45天。请生成一份销售业绩简报。",
		QuestionType: CaseTypeCore,
	})
	require.True(t, ok)

	_, ok = registry.reserve(GeneratedCase{
		Content:      "今天北美区硬件产品线完成销售额420万元，同比增长8%，但渠道库存周转天数上升至45天。请分析库存周转异常的可能原因。",
		QuestionType: CaseTypeExtension,
	})
	require.True(t, ok)
}
