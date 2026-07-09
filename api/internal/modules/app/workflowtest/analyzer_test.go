package workflowtest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnalyzeWorkflowTestResultEvaluatesTaskChecks(t *testing.T) {
	snapshot := CaseSnapshot{
		Content:        "summarize",
		ExpectedResult: "summary",
		Turns: CaseTurns{{
			Role:    "user",
			Content: "summarize",
			Inputs: JSONMap{
				caseModeInputKey: "task",
				expectedChecksInputKey: map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"id":           "check_node",
							"type":         "node",
							"operator":     "visited",
							"target_id":    "extract",
							"target_label": "Extract",
							"severity":     "normal",
						},
						map[string]interface{}{
							"id":         "check_output",
							"type":       "output_contains",
							"operator":   "contains",
							"values":     []interface{}{"summary"},
							"match_mode": "keyword",
							"severity":   "critical",
						},
						map[string]interface{}{
							"id":       "check_latency",
							"type":     "latency",
							"operator": "lte",
							"value_ms": 100,
							"severity": "hint",
						},
					},
				},
			},
		}},
	}
	result := &RunCaseResult{Outputs: map[string]interface{}{
		"answer":       "summary ready",
		"elapsed_time": float64(50),
		"workflow_trace": map[string]interface{}{
			"nodes": []map[string]interface{}{
				{"node_id": "extract", "node_name": "Extract", "node_type": "document-extractor", "status": "succeeded"},
			},
		},
	}}

	analysis := analyzeWorkflowTestResult(snapshot, result)

	require.Equal(t, "task", analysis.Mode)
	require.Equal(t, "passed", analysis.Summary.Status)
	require.Equal(t, 4, analysis.Summary.Total)
	require.Equal(t, 4, analysis.Summary.Passed)
	require.Len(t, analysis.Trace.Nodes, 1)
	require.Equal(t, "extract", analysis.Trace.Nodes[0].NodeID)
}

func TestWorkflowActualOutputTextPrefersNestedBusinessOutput(t *testing.T) {
	outputs := map[string]interface{}{
		"elapsed_time":    10186.9,
		"status":          "succeeded",
		"task_id":         "task-1",
		"workflow_run_id": "run-1",
		"node_results": map[string]interface{}{
			"llm_node": map[string]interface{}{"status": "succeeded"},
		},
		"outputs": map[string]interface{}{
			"summary": "**结构化接收记录**\n\n- 发件方：宏达供应链公司",
		},
	}

	actual := workflowActualOutputText(outputs)

	require.Equal(t, "**结构化接收记录**\n\n- 发件方：宏达供应链公司", actual)
	require.NotContains(t, actual, "workflow_run_id")
	require.NotContains(t, actual, "node_results")
}

func TestAnalyzeWorkflowTestResultUsesNestedSummaryForTaskChecks(t *testing.T) {
	snapshot := CaseSnapshot{
		Content: "请处理确认单",
		Turns: CaseTurns{{
			Role:    "user",
			Content: "请处理确认单",
			Inputs: JSONMap{
				caseModeInputKey: "task",
				expectedChecksInputKey: map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"id":         "check_summary",
							"type":       "output_contains",
							"operator":   "contains",
							"values":     []interface{}{"宏达供应链公司"},
							"match_mode": "keyword",
							"severity":   "critical",
						},
					},
				},
			},
		}},
	}
	result := &RunCaseResult{Outputs: map[string]interface{}{
		"elapsed_time":    10186.9,
		"status":          "succeeded",
		"workflow_run_id": "run-1",
		"node_results":    map[string]interface{}{"llm_node": map[string]interface{}{"status": "succeeded"}},
		"outputs": map[string]interface{}{
			"summary": "**结构化接收记录**\n\n- 发件方：宏达供应链公司",
		},
	}}

	analysis := analyzeWorkflowTestResult(snapshot, result)

	require.Equal(t, "passed", analysis.Summary.Status)
	require.Equal(t, "passed", analysis.Comparisons.Checks[0].Status)
}

func TestAnalyzeWorkflowTestResultFailsCriticalTaskCheck(t *testing.T) {
	snapshot := CaseSnapshot{
		Content: "summarize",
		Turns: CaseTurns{{
			Role:    "user",
			Content: "summarize",
			Inputs: JSONMap{
				caseModeInputKey: "task",
				expectedChecksInputKey: map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"id":         "check_output",
							"type":       "output_contains",
							"operator":   "contains",
							"values":     []interface{}{"required field"},
							"match_mode": "keyword",
							"severity":   "critical",
						},
					},
				},
			},
		}},
	}
	result := &RunCaseResult{Outputs: map[string]interface{}{"answer": "plain text"}}

	analysis := analyzeWorkflowTestResult(snapshot, result)

	require.Equal(t, "failed", analysis.Summary.Status)
	require.Equal(t, 1, analysis.Summary.Failed)
	require.Equal(t, 1, analysis.Summary.CriticalFailed)
	require.Contains(t, analysis.Comparisons.Checks[0].Evidence, "required field")
}

func TestAnalyzeTaskSchemaAllowsReasonableBusinessMissingInfo(t *testing.T) {
	snapshot := CaseSnapshot{
		Content: "请处理合同接收文件",
		Turns: CaseTurns{{
			Role:    "user",
			Content: "请处理合同接收文件",
			Inputs: JSONMap{
				caseModeInputKey: "task",
				evaluationSchemaInputKey: map[string]interface{}{
					"goal_type":         "extract",
					"primary_objective": "从输入材料中抽取结构化接收记录。",
					"assertions": []interface{}{
						map[string]interface{}{
							"id":          "party",
							"type":        "must_include",
							"description": "识别合同相关方",
							"values":      []interface{}{"星辰科技有限公司", "远航设备制造厂"},
							"match_mode":  "keyword",
							"severity":    "critical",
						},
					},
					"allowed_extra_types": []interface{}{"missing_business_fields", "risk_notes"},
				},
			},
		}},
	}
	result := &RunCaseResult{Outputs: map[string]interface{}{"outputs": map[string]interface{}{
		"summary": "结构化接收记录\n相关方：星辰科技有限公司、远航设备制造厂\n缺失信息：合同总金额；验收标准\n潜在风险：验收标准未明确可能导致争议",
	}}}

	analysis := analyzeWorkflowTestResult(snapshot, result)

	require.NotNil(t, analysis.EvaluationSchema)
	require.Equal(t, "passed", analysis.Summary.Status)
	require.Equal(t, 0, analysis.Summary.CriticalFailed)
}

func TestAnalyzeTaskSchemaNormalizesNaturalLanguageAssertions(t *testing.T) {
	snapshot := CaseSnapshot{
		Content: "请处理这份供应商交付确认单。",
		Turns: CaseTurns{{
			Role:    "user",
			Content: "请处理这份供应商交付确认单。",
			Inputs: JSONMap{
				caseModeInputKey: "task",
				evaluationSchemaInputKey: map[string]interface{}{
					"goal_type": "extract",
					"assertions": []interface{}{
						map[string]interface{}{
							"type":        "must_include",
							"description": "输出应包含交付方：迅达物流有限公司、输出应包含收货方：华东制造厂、输出应包含交付日期：2024年5月10日、输出应包含货物描述：工业传感器模块（型号SNS-8800）共200件、输出应包含验收状态：部分验收，其中15件外包装破损拒收、输出应包含未履约事项：剩余50件将于5月20日前补发、不得声称文件无法读取、内容为空、解析失败或要求重新上传",
							"values": []interface{}{
								"输出应包含交付方：迅达物流有限公司、输出应包含收货方：华东制造厂、输出应包含交付日期：2024年5月10日、输出应包含货物描述：工业传感器模块（型号SNS-8800）共200件、输出应包含验收状态：部分验收，其中15件外包装破损拒收、输出应包含未履约事项：剩余50件将于5月20日前补发、不得声称文件无法读取、内容为空、解析失败或要求重新上传",
							},
							"severity": "critical",
						},
					},
				},
			},
		}},
	}
	result := &RunCaseResult{Outputs: map[string]interface{}{"outputs": map[string]interface{}{
		"summary": "结构化接收记录\n交付方：迅达物流有限公司\n收货方：华东制造厂\n交付日期：2024年5月10日\n货物描述：工业传感器模块（型号 SNS-8800），共计200件\n验收状态：部分验收，实收185件，拒收15件，原因：外包装破损\n未履约事项：剩余50件尚未交付，承诺于2024年5月20日前完成补发。",
	}}}

	analysis := analyzeWorkflowTestResult(snapshot, result)

	if analysis.Summary.Status != "passed" {
		t.Fatalf("expected passed, got %s, summary=%+v checks=%+v", analysis.Summary.Status, analysis.Summary, analysis.Comparisons.Checks)
	}
	require.GreaterOrEqual(t, analysis.Summary.Passed, 7)
	require.Equal(t, 0, analysis.Summary.Failed)
	require.Equal(t, 0, analysis.Summary.Review)
}

func TestTaskSchemaTreatsExampleMissingItemsAsOptional(t *testing.T) {
	snapshot := CaseSnapshot{
		Content:        "请处理这份客户投诉文件。",
		ExpectedResult: "输出应包含投诉方、事件时间线、主张事实、诉求内容、证据引用，并明确列出缺失信息（如发票、客服联系记录）。不得声称文件无法读取、内容为空、解析失败或要求重新上传。",
		Turns: CaseTurns{{
			Role:    "user",
			Content: "请处理这份客户投诉文件。",
			Inputs: JSONMap{
				caseModeInputKey: "task",
				evaluationSchemaInputKey: map[string]interface{}{
					"goal_type": "extract",
					"assertions": []interface{}{
						map[string]interface{}{
							"type":        "must_include",
							"description": "输出应明确列出缺失信息（如发票、客服联系记录）",
							"values":      []interface{}{"输出应明确列出缺失信息（如发票、客服联系记录）"},
							"severity":    "critical",
						},
						map[string]interface{}{
							"type":        "must_include",
							"description": "输出应明确指出缺失发票和客服联系记录",
							"values":      []interface{}{"输出应明确指出缺失发票和客服联系记录"},
							"severity":    "critical",
						},
					},
				},
			},
		}},
	}
	result := &RunCaseResult{Outputs: map[string]interface{}{"outputs": map[string]interface{}{
		"summary": "结构化接收记录\n投诉人：王丽\n缺失信息：具体联系客服的日期与时间；客服沟通渠道；物流单号。",
	}}}

	analysis := analyzeWorkflowTestResult(snapshot, result)

	require.Equal(t, "passed", analysis.Summary.Status)
	require.Equal(t, 0, analysis.Summary.Failed)
	require.Equal(t, 0, analysis.Summary.Review)
}

func TestAnalyzeTaskSchemaMatchesDomainEquivalentPhrases(t *testing.T) {
	tests := []struct {
		name      string
		expected  string
		actual    string
		assertion string
	}{
		{
			name:     "credit risk as negative credit record",
			expected: "输出应提及违约金、诉讼及信用风险",
			actual:   "风险提示：承担合同约定的违约金；承担诉讼相关成本；对企业信用记录造成负面影响。",
		},
		{
			name:     "original contract signing date as contract signed date",
			expected: "缺失信息包括：合同编号、原合同签署日期",
			actual:   "缺失或待澄清信息：合同编号或具体名称未列明；合同签订日期及履行情况细节未列明。",
		},
		{
			name:     "parties listed separately",
			expected: "输出应包含签约方：星辰科技有限公司与云帆数据服务公司",
			actual:   "相关方：甲方：星辰科技有限公司；乙方：云帆数据服务公司。",
		},
		{
			name:     "complaint party label",
			expected: "输出应标注投诉方为王丽",
			actual:   "相关方：投诉人：王丽；被投诉方：商家。",
		},
		{
			name:     "return and shipping fee claims",
			expected: "输出应列出换货和补偿物流费两项诉求",
			actual:   "诉求：要求商家办理换货；要求补偿因退换货产生的物流费用50元。",
		},
		{
			name:     "video evidence wording",
			expected: "输出应引用视频链接作为证据",
			actual:   "证据：提供了订单截图及开箱视频链接（https://example.com/video8892345）作为证据。",
		},
		{
			name:     "handover parties with role labels",
			expected: "输出应包含交接双方：甲方为广州数智科技有限公司、乙方为成都云启信息技术有限公司",
			actual:   "相关方信息：移交方（乙方）：成都云启信息技术有限公司；接收方（甲方）：广州数智科技有限公司。",
		},
		{
			name:     "handover legacy issue with inserted details",
			expected: "输出应包含遗留问题：用户权限模块尚未完成联调",
			actual:   "遗留问题：用户权限模块与SSO集成尚未完成联调，存在登录异常。",
		},
		{
			name:     "handover responsibility split",
			expected: "输出应包含责任划分：乙方负责在6月20日前修复遗留问题，甲方负责提供测试环境",
			actual:   "乙方义务：须于2024年6月20日前完成修复；甲方义务：为乙方提供独立测试环境的访问权限。",
		},
		{
			name:     "legal risk with modal word omitted",
			expected: "输出应包含潜在法律风险：可能提起诉讼并申请财产保全",
			actual:   "法律后果与风险提示：将依法向人民法院提起诉讼，并申请采取财产保全措施。",
		},
		{
			name:     "meta assertion responsibility included",
			expected: "责任划分说明已包含",
			actual:   "责任划分说明：系统日常运行由B公司负责，历史数据完整性问题由A公司负责。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := CaseSnapshot{
				Content:        "请处理这份任务输入。",
				ExpectedResult: tt.expected,
				Turns: CaseTurns{{
					Role:    "user",
					Content: "请处理这份任务输入。",
					Inputs: JSONMap{
						caseModeInputKey: "task",
						evaluationSchemaInputKey: map[string]interface{}{
							"goal_type": "extract",
							"assertions": []interface{}{
								map[string]interface{}{
									"type":        "must_include",
									"description": tt.expected,
									"values":      []interface{}{tt.expected},
									"severity":    "critical",
								},
							},
						},
					},
				}},
			}
			result := &RunCaseResult{Outputs: map[string]interface{}{"outputs": map[string]interface{}{
				"summary": tt.actual,
			}}}

			analysis := analyzeWorkflowTestResult(snapshot, result)

			require.Equal(t, "passed", analysis.Summary.Status)
			require.Equal(t, 0, analysis.Summary.Failed)
		})
	}
}

func TestAnalyzeTaskSchemaDoesNotFailMetaAvailabilityLabel(t *testing.T) {
	snapshot := CaseSnapshot{
		Content: "请处理这份客户投诉文件。",
		Turns: CaseTurns{{
			Role:    "user",
			Content: "请处理这份客户投诉文件。",
			Inputs: JSONMap{
				caseModeInputKey: "task",
				evaluationSchemaInputKey: map[string]interface{}{
					"goal_type": "extract",
					"assertions": []interface{}{
						map[string]interface{}{
							"type":        "must_include",
							"description": "输出应标注订单号和产品名称已提供",
							"values":      []interface{}{"输出应标注订单号和产品名称已提供"},
							"severity":    "critical",
						},
					},
				},
			},
		}},
	}
	result := &RunCaseResult{Outputs: map[string]interface{}{"outputs": map[string]interface{}{
		"summary": "投诉人：王丽；订单号：ORD88923；商品为智能手表，屏幕有划痕且无法开机。",
	}}}

	analysis := analyzeWorkflowTestResult(snapshot, result)

	require.Equal(t, "passed", analysis.Summary.Status)
	require.Equal(t, 0, analysis.Summary.Failed)
	require.Equal(t, 0, analysis.Summary.Review)
}

func TestAnalyzeTaskSchemaInfersPartialAcceptanceFromEvidence(t *testing.T) {
	snapshot := CaseSnapshot{
		Content: "请处理供应商交付确认单。",
		Turns: CaseTurns{{
			Role:    "user",
			Content: "请处理供应商交付确认单。",
			Inputs: JSONMap{
				caseModeInputKey: "task",
				evaluationSchemaInputKey: map[string]interface{}{
					"goal_type": "extract",
					"assertions": []interface{}{
						map[string]interface{}{
							"type":        "must_include",
							"description": "输出应包含验收状态：部分验收，其中30件外包装破损拒收",
							"values":      []interface{}{"输出应包含验收状态：部分验收，其中30件外包装破损拒收"},
							"severity":    "critical",
						},
					},
				},
			},
		}},
	}
	result := &RunCaseResult{Outputs: map[string]interface{}{"outputs": map[string]interface{}{
		"summary": "交付事实：本次实收170件，拒收数量30件，原因是外包装破损；剩余50件尚未交付。",
	}}}

	analysis := analyzeWorkflowTestResult(snapshot, result)

	require.Equal(t, "passed", analysis.Summary.Status)
	require.Equal(t, "assertion_state_present", analysis.Comparisons.Checks[0].Type)
	require.Equal(t, "passed", analysis.Comparisons.Checks[0].Status)
}

func TestAnalyzeTaskSchemaMarksMissingFieldsByEquivalentWording(t *testing.T) {
	snapshot := CaseSnapshot{
		Content: "请处理供应商交付确认单。",
		Turns: CaseTurns{{
			Role:    "user",
			Content: "请处理供应商交付确认单。",
			Inputs: JSONMap{
				caseModeInputKey: "task",
				evaluationSchemaInputKey: map[string]interface{}{
					"goal_type": "extract",
					"assertions": []interface{}{
						map[string]interface{}{
							"type":        "must_include",
							"description": "输出应标注签收人姓名缺失",
							"values":      []interface{}{"输出应标注签收人姓名缺失"},
							"severity":    "critical",
						},
						map[string]interface{}{
							"type":        "must_include",
							"description": "输出应标注验收标准依据缺失",
							"values":      []interface{}{"输出应标注验收标准依据缺失"},
							"severity":    "critical",
						},
					},
				},
			},
		}},
	}
	result := &RunCaseResult{Outputs: map[string]interface{}{"outputs": map[string]interface{}{
		"summary": "缺失信息：签收栏为空；验收标准未列明。",
	}}}

	analysis := analyzeWorkflowTestResult(snapshot, result)

	require.Equal(t, "passed", analysis.Summary.Status)
	require.Equal(t, 3, analysis.Summary.Passed)
	require.Equal(t, "assertion_missing_field_marked", analysis.Comparisons.Checks[0].Type)
	require.Equal(t, "assertion_missing_field_marked", analysis.Comparisons.Checks[1].Type)
}

func TestTaskAnalysisKeepsHighScoreMinorCriticalGapAsReview(t *testing.T) {
	analysis := &WorkflowTestAnalysis{
		Mode:             "task",
		EvaluationSchema: &TaskEvaluationSchema{GoalType: "extract"},
		Comparisons: WorkflowTestComparisons{Checks: []WorkflowTestCheckResult{
			{ID: "a", Type: "assertion_fact_present", Label: "发函方", Status: "passed", Severity: "critical"},
			{ID: "b", Type: "assertion_fact_present", Label: "收函方", Status: "passed", Severity: "critical"},
			{ID: "c", Type: "assertion_fact_present", Label: "时限", Status: "passed", Severity: "critical"},
			{ID: "d", Type: "assertion_missing_field_marked", Label: "原合同签署日期", Status: "failed", Severity: "critical"},
			{ID: "e", Type: "assertion_missing_policy", Label: "缺失信息处理策略", Status: "passed", Severity: "normal"},
		}},
	}

	finalizeWorkflowTestAnalysis(analysis)

	require.Equal(t, "review", analysis.Summary.Status)
	require.InDelta(t, 4.0, analysis.Summary.ReferenceScore, 0.01)

	merged := mergeAnalysisWithJudgeResult(analysis, &JudgeResult{
		Status:     BatchItemStatusFailed,
		Reason:     "缺少原合同签署日期，因此不通过。",
		Confidence: 0.9,
	})
	require.Equal(t, BatchItemStatusReview, merged.Status)
	require.Contains(t, merged.Reason, "结构化评价参考分")
}

func TestAnalyzeTaskMissingFieldExpectationPassesWithChinesePlaceholders(t *testing.T) {
	snapshot := CaseSnapshot{
		Content:        "我上传了一份接收确认文件，但里面只写了事情大概，没写具体日期和对方是谁，系统能处理吗？",
		ExpectedResult: "应识别输入对应文件解析失败或内容缺失场景，明确标记缺失信息为无日期和无相关方，不得编造不存在内容。",
		Turns: CaseTurns{{
			Role:    "user",
			Content: "我上传了一份接收确认文件，但里面只写了事情大概，没写具体日期和对方是谁，系统能处理吗？",
			Inputs: JSONMap{
				caseModeInputKey: "task",
				expectedChecksInputKey: map[string]interface{}{
					"conditions": []interface{}{
						map[string]interface{}{
							"id":         "missing_fields",
							"type":       "output_contains",
							"operator":   "contains",
							"values":     []interface{}{"missing field: date", "missing field: related party"},
							"match_mode": "semantic",
							"severity":   "normal",
						},
					},
				},
			},
		}},
	}
	result := &RunCaseResult{Outputs: map[string]interface{}{"outputs": map[string]interface{}{
		"summary": "结构化接收记录\n相关方：接收方：[未明确，需补充]；交付方：[未明确，需补充]\n日期：接收确认日期：[未明确，需补充具体日期]\n事实：已确认收到相关交付内容\n缺失信息：接收方与交付方的完整名称；接收日期。",
	}}}

	analysis := analyzeWorkflowTestResult(snapshot, result)

	require.Equal(t, "passed", analysis.Summary.Status)
	require.Equal(t, 1, analysis.Summary.Total)
	require.Equal(t, 1, analysis.Summary.Passed)
	require.Empty(t, analysis.Suggestions)

	merged := mergeAnalysisWithJudgeResult(analysis, &JudgeResult{
		Status:     BatchItemStatusReview,
		Reason:     "输出明确标记了日期和相关方等关键字段缺失。",
		Suggestion: "missing field: date、missing field: related party: 请在工作流回复生成策略中补充这些业务要点。",
		Confidence: 0.5,
	})
	require.Equal(t, BatchItemStatusPassed, merged.Status)
	require.Empty(t, merged.Suggestion)
}

func TestAnalyzeTaskSchemaFailsTechnicalMissingClaim(t *testing.T) {
	snapshot := CaseSnapshot{
		Content: "请处理合同接收文件",
		Turns: CaseTurns{{
			Role:    "user",
			Content: "请处理合同接收文件",
			Inputs: JSONMap{
				caseModeInputKey: "task",
				evaluationSchemaInputKey: map[string]interface{}{
					"goal_type": "extract",
					"missing_policy": map[string]interface{}{
						"mode":          "explicit_markers_allowed",
						"forbid_claims": []interface{}{"文件无法读取", "需要转人工"},
					},
				},
			},
		}},
	}
	result := &RunCaseResult{Outputs: map[string]interface{}{"answer": "文件无法读取，需要转人工处理。"}}

	analysis := analyzeWorkflowTestResult(snapshot, result)

	require.Equal(t, "failed", analysis.Summary.Status)
	require.Equal(t, "缺失信息处理策略", analysis.Summary.MainIssue)
	require.Equal(t, 1, analysis.Summary.Failed)
}

func TestMergeAnalysisTrustsPassedTaskSchemaOverStrictJudgeFailure(t *testing.T) {
	analysis := &WorkflowTestAnalysis{
		Mode:             "task",
		EvaluationSchema: &TaskEvaluationSchema{GoalType: "extract"},
		Summary:          WorkflowTestAnalysisSummary{Status: "passed", Total: 2, Passed: 2, CriticalFailed: 0},
	}
	judgeResult := &JudgeResult{
		Status:     BatchItemStatusFailed,
		Reason:     "输出包含缺失信息，因此不通过。",
		Suggestion: "删除缺失信息。",
		Confidence: 0.9,
	}

	merged := mergeAnalysisWithJudgeResult(analysis, judgeResult)

	require.Equal(t, BatchItemStatusPassed, merged.Status)
	require.Empty(t, merged.Suggestion)
}

func TestMergeAnalysisKeepsTaskJudgePassedWhenOnlyStructuredReview(t *testing.T) {
	analysis := &WorkflowTestAnalysis{
		Mode:             "task",
		EvaluationSchema: &TaskEvaluationSchema{GoalType: "extract"},
		Summary:          WorkflowTestAnalysisSummary{Status: "review", Total: 3, Passed: 1, Review: 2, CriticalFailed: 0},
	}
	judgeResult := &JudgeResult{
		Status:     BatchItemStatusPassed,
		Reason:     "输出完整包含所有关键要素。",
		Confidence: 0.9,
	}

	merged := mergeAnalysisWithJudgeResult(analysis, judgeResult)

	require.Equal(t, BatchItemStatusPassed, merged.Status)
	require.Equal(t, "输出完整包含所有关键要素。", merged.Reason)
}

func TestAnalyzeWorkflowTestResultComparesConversationTurnExpectation(t *testing.T) {
	snapshot := CaseSnapshot{
		Content:        "conversation",
		ExpectedResult: "确认预算",
		Turns: CaseTurns{
			{
				Role:    "user",
				Content: "我要办活动",
				Inputs: JSONMap{
					caseModeInputKey:        "conversation",
					turnExpectationInputKey: "询问预算",
				},
			},
		},
	}
	result := &RunCaseResult{Outputs: map[string]interface{}{
		"turn_results": []map[string]interface{}{
			{"outputs": map[string]interface{}{"answer": "请问这次活动的预算是多少？"}},
		},
	}}

	analysis := analyzeWorkflowTestResult(snapshot, result)

	require.Equal(t, "conversation", analysis.Mode)
	require.Equal(t, "review", analysis.Summary.Status)
	require.Len(t, analysis.Comparisons.Turns, 1)
	require.Equal(t, "review", analysis.Comparisons.Turns[0].Status)
}

func TestWorkflowActualOutputTextPrefersNestedConversationTurnOutput(t *testing.T) {
	outputs := map[string]interface{}{
		"turn_results": []map[string]interface{}{
			{
				"outputs": map[string]interface{}{
					"elapsed_time": 12.3,
					"outputs": map[string]interface{}{
						"answer": "请问这次活动的预算是多少？",
					},
				},
			},
		},
	}

	turnOutputs := conversationTurnOutputs(outputs)
	actual := actualTurnOutputText(turnOutputs, 0, outputs)

	require.Equal(t, "请问这次活动的预算是多少？", actual)
}
