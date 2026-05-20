package calculator

import (
	"context"
	"fmt"
	"math"

	"github.com/zgiai/ginext/internal/modules/tools"
	"github.com/zgiai/ginext/internal/modules/tools/builtin"
)

// CalculateTool performs deterministic binary arithmetic.
type CalculateTool struct {
	*builtin.BuiltinTool
}

// NewCalculateTool creates a calculate tool.
func NewCalculateTool(tenantID string) *CalculateTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "calculate",
			Author:   "System",
			Provider: "calculator",
			Label: tools.I18nText{
				"en_US":   "Calculate",
				"zh_Hans": "计算",
			},
			Icon: "calculator",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   "Perform deterministic arithmetic between two numbers.",
				"zh_Hans": "对两个数字进行确定性的算术计算。",
			},
			LLM: "Perform deterministic arithmetic between two finite numbers.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:             "operation",
				Label:            tools.I18nText{"en_US": "Operation", "zh_Hans": "操作"},
				HumanDescription: tools.I18nText{"en_US": "Choose the arithmetic operation applied to the left and right operands.", "zh_Hans": "选择要应用到左操作数和右操作数上的算术操作。"},
				LLMDescription:   "Arithmetic operation: add, subtract, multiply, divide, power, or mod.",
				Type:             tools.ToolParameterTypeSelect,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				SupportVariable:  true,
				Options: []tools.ToolParameterOption{
					{Value: "add", Label: tools.I18nText{"en_US": "Add", "zh_Hans": "加法"}},
					{Value: "subtract", Label: tools.I18nText{"en_US": "Subtract", "zh_Hans": "减法"}},
					{Value: "multiply", Label: tools.I18nText{"en_US": "Multiply", "zh_Hans": "乘法"}},
					{Value: "divide", Label: tools.I18nText{"en_US": "Divide", "zh_Hans": "除法"}},
					{Value: "power", Label: tools.I18nText{"en_US": "Power", "zh_Hans": "幂运算"}},
					{Value: "mod", Label: tools.I18nText{"en_US": "Modulo", "zh_Hans": "取模"}},
				},
			},
			{
				Name:             "left",
				Label:            tools.I18nText{"en_US": "Left", "zh_Hans": "左操作数"},
				HumanDescription: tools.I18nText{"en_US": "The first number in the calculation, for example 10 in 10 + 3.", "zh_Hans": "计算中的第一个数字，例如 10 + 3 中的 10。"},
				LLMDescription:   "Left operand.",
				Type:             tools.ToolParameterTypeNumber,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				Placeholder:      tools.I18nText{"en_US": "10", "zh_Hans": "例如：10"},
				SupportVariable:  true,
			},
			{
				Name:             "right",
				Label:            tools.I18nText{"en_US": "Right", "zh_Hans": "右操作数"},
				HumanDescription: tools.I18nText{"en_US": "The second number in the calculation, for example 3 in 10 + 3.", "zh_Hans": "计算中的第二个数字，例如 10 + 3 中的 3。"},
				LLMDescription:   "Right operand.",
				Type:             tools.ToolParameterTypeNumber,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				Placeholder:      tools.I18nText{"en_US": "3", "zh_Hans": "例如：3"},
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
	return &CalculateTool{BuiltinTool: builtin.NewBuiltinTool(entity, tenantID)}
}

// Invoke executes the calculation.
func (t *CalculateTool) Invoke(
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
	left, err := numberParam(toolParameters, "left")
	if err != nil {
		return nil, err
	}
	right, err := numberParam(toolParameters, "right")
	if err != nil {
		return nil, err
	}
	precision, err := precisionParam(toolParameters)
	if err != nil {
		return nil, err
	}
	result, err := calculate(operation, left, right)
	if err != nil {
		return nil, err
	}
	result, err = roundValue(result, precision)
	if err != nil {
		return nil, err
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"operation": operation,
		"left":      left,
		"right":     right,
		"result":    result,
		"precision": precision,
	})}, nil
}

func calculate(operation string, left float64, right float64) (float64, error) {
	switch operation {
	case "add":
		return left + right, nil
	case "subtract":
		return left - right, nil
	case "multiply":
		return left * right, nil
	case "divide":
		if right == 0 {
			return 0, fmt.Errorf("division by zero")
		}
		return left / right, nil
	case "power":
		return math.Pow(left, right), nil
	case "mod":
		if right == 0 {
			return 0, fmt.Errorf("modulo by zero")
		}
		return math.Mod(left, right), nil
	default:
		return 0, fmt.Errorf("invalid operation: %s", operation)
	}
}

// ForkToolRuntime creates a copy of the tool with a new runtime.
func (t *CalculateTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &CalculateTool{
		BuiltinTool: t.BuiltinTool.ForkToolRuntime(runtime),
	}
}

var _ tools.Tool = (*CalculateTool)(nil)
