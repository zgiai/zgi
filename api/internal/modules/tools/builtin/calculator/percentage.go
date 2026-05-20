package calculator

import (
	"context"
	"fmt"

	"github.com/zgiai/ginext/internal/modules/tools"
	"github.com/zgiai/ginext/internal/modules/tools/builtin"
)

// PercentageTool performs deterministic percentage calculations.
type PercentageTool struct {
	*builtin.BuiltinTool
}

// NewPercentageTool creates a percentage tool.
func NewPercentageTool(tenantID string) *PercentageTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "percentage",
			Author:   "System",
			Provider: "calculator",
			Label: tools.I18nText{
				"en_US":   "Percentage",
				"zh_Hans": "百分比计算",
			},
			Icon: "percent",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   "Calculate percentages, percentage change, and percentage adjustments.",
				"zh_Hans": "计算百分比、百分比变化和按百分比增减后的结果。",
			},
			LLM: "Calculate percent_of, percentage change, or apply percentage increase/decrease.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:             "operation",
				Label:            tools.I18nText{"en_US": "Operation", "zh_Hans": "操作"},
				HumanDescription: tools.I18nText{"en_US": "Choose whether to calculate a percentage, a change rate, or apply an increase/decrease.", "zh_Hans": "选择计算百分比、变化率，或按百分比增加/减少。"},
				LLMDescription:   "Percentage operation: percent_of, change, apply_increase, or apply_decrease.",
				Type:             tools.ToolParameterTypeSelect,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				SupportVariable:  true,
				Options: []tools.ToolParameterOption{
					{Value: "percent_of", Label: tools.I18nText{"en_US": "Percent Of", "zh_Hans": "求百分比"}},
					{Value: "change", Label: tools.I18nText{"en_US": "Change", "zh_Hans": "变化率"}},
					{Value: "apply_increase", Label: tools.I18nText{"en_US": "Apply Increase", "zh_Hans": "按百分比增加"}},
					{Value: "apply_decrease", Label: tools.I18nText{"en_US": "Apply Decrease", "zh_Hans": "按百分比减少"}},
				},
			},
			{
				Name:             "value",
				Label:            tools.I18nText{"en_US": "Value", "zh_Hans": "数值"},
				HumanDescription: tools.I18nText{"en_US": "Base number used by percent_of, apply_increase, and apply_decrease operations.", "zh_Hans": "percent_of、apply_increase、apply_decrease 操作使用的基准数值。"},
				LLMDescription:   "Base value for percent_of, apply_increase, or apply_decrease.",
				Type:             tools.ToolParameterTypeNumber,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "200", "zh_Hans": "例如：200"},
				SupportVariable:  true,
			},
			{
				Name:             "percent",
				Label:            tools.I18nText{"en_US": "Percent", "zh_Hans": "百分比"},
				HumanDescription: tools.I18nText{"en_US": "Percentage value. Enter 15 for 15%, not 0.15.", "zh_Hans": "百分比数值。15% 请填写 15，而不是 0.15。"},
				LLMDescription:   "Percentage value, for example 15 for 15 percent.",
				Type:             tools.ToolParameterTypeNumber,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "15", "zh_Hans": "例如：15"},
				SupportVariable:  true,
			},
			{
				Name:             "from",
				Label:            tools.I18nText{"en_US": "From", "zh_Hans": "原始值"},
				HumanDescription: tools.I18nText{"en_US": "Original number used by the change operation.", "zh_Hans": "change 操作用于计算变化率的原始数值。"},
				LLMDescription:   "Original value for change operation.",
				Type:             tools.ToolParameterTypeNumber,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "80", "zh_Hans": "例如：80"},
				SupportVariable:  true,
			},
			{
				Name:             "to",
				Label:            tools.I18nText{"en_US": "To", "zh_Hans": "新值"},
				HumanDescription: tools.I18nText{"en_US": "New number used by the change operation.", "zh_Hans": "change 操作用于计算变化率的新数值。"},
				LLMDescription:   "New value for change operation.",
				Type:             tools.ToolParameterTypeNumber,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "100", "zh_Hans": "例如：100"},
				SupportVariable:  true,
			},
			{
				Name:             "precision",
				Label:            tools.I18nText{"en_US": "Precision", "zh_Hans": "小数位数"},
				HumanDescription: tools.I18nText{"en_US": "Number of decimal places used to round the result. Leave empty to use 6.", "zh_Hans": "结果保留的小数位数。留空时默认保留 6 位。"},
				LLMDescription:   "Decimal places to round the result to. Defaults to 6 and must be between 0 and 12.",
				Type:             tools.ToolParameterTypeNumber,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Default:          defaultPrecision,
				Placeholder:      tools.I18nText{"en_US": "2", "zh_Hans": "例如：2"},
				SupportVariable:  true,
			},
		},
		OutputType: "json",
		Tags:       []string{"utilities", "math"},
	}
	return &PercentageTool{BuiltinTool: builtin.NewBuiltinTool(entity, tenantID)}
}

// Invoke executes the percentage calculation.
func (t *PercentageTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = conversationID
	_ = appID
	_ = messageID

	operation := stringParam(toolParameters, "operation")
	precision, err := precisionParam(toolParameters)
	if err != nil {
		return nil, err
	}
	data, err := percentage(operation, toolParameters)
	if err != nil {
		return nil, err
	}
	result, err := roundValue(data["result"].(float64), precision)
	if err != nil {
		return nil, err
	}
	data["operation"] = operation
	data["result"] = result
	data["precision"] = precision
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(data)}, nil
}

func percentage(operation string, params map[string]interface{}) (map[string]interface{}, error) {
	switch operation {
	case "percent_of":
		value, err := numberParam(params, "value")
		if err != nil {
			return nil, err
		}
		percent, err := numberParam(params, "percent")
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"value":   value,
			"percent": percent,
			"result":  value * percent / 100,
		}, nil
	case "change":
		from, err := numberParam(params, "from")
		if err != nil {
			return nil, err
		}
		to, err := numberParam(params, "to")
		if err != nil {
			return nil, err
		}
		if from == 0 {
			return nil, fmt.Errorf("from must not be zero for change operation")
		}
		return map[string]interface{}{
			"from":   from,
			"to":     to,
			"result": (to - from) / from * 100,
		}, nil
	case "apply_increase":
		value, err := numberParam(params, "value")
		if err != nil {
			return nil, err
		}
		percent, err := numberParam(params, "percent")
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"value":   value,
			"percent": percent,
			"result":  value * (1 + percent/100),
		}, nil
	case "apply_decrease":
		value, err := numberParam(params, "value")
		if err != nil {
			return nil, err
		}
		percent, err := numberParam(params, "percent")
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"value":   value,
			"percent": percent,
			"result":  value * (1 - percent/100),
		}, nil
	default:
		return nil, fmt.Errorf("invalid operation: %s", operation)
	}
}

// ForkToolRuntime creates a copy of the tool with a new runtime.
func (t *PercentageTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &PercentageTool{
		BuiltinTool: t.BuiltinTool.ForkToolRuntime(runtime),
	}
}

var _ tools.Tool = (*PercentageTool)(nil)
