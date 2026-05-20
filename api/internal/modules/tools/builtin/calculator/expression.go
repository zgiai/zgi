package calculator

import (
	"context"
	"fmt"
	"math"

	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const (
	maxExpressionLength      = 512
	maxExpressionPowerAbsExp = 100
)

// ExpressionTool evaluates a restricted arithmetic expression.
type ExpressionTool struct {
	*builtin.BuiltinTool
}

// NewExpressionTool creates an expression evaluation tool.
func NewExpressionTool(tenantID string) *ExpressionTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "evaluate_expression",
			Author:   "System",
			Provider: "calculator",
			Label: tools.I18nText{
				"en_US":   "Evaluate Expression",
				"zh_Hans": "表达式计算",
			},
			Icon: "calculator",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   "Evaluate a deterministic arithmetic expression.",
				"zh_Hans": "计算一个确定性的算术表达式。",
			},
			LLM: "Evaluate one arithmetic expression with numbers, parentheses, +, -, *, /, %, and ^. Convert word problems into a single expression when possible.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name:             "expression",
				Label:            tools.I18nText{"en_US": "Expression", "zh_Hans": "表达式"},
				HumanDescription: tools.I18nText{"en_US": "Arithmetic expression using numbers, parentheses, +, -, *, /, %, and ^.", "zh_Hans": "由数字、括号和 +、-、*、/、%、^ 组成的算术表达式。"},
				LLMDescription:   "Arithmetic expression to evaluate, such as (9.5*10 + 4.8*15 + 15*5 - 30) * 0.85. Only numbers, parentheses, +, -, *, /, %, and ^ are allowed.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         true,
				Placeholder:      tools.I18nText{"en_US": "(9.5*10 + 4.8*15 + 15*5 - 30) * 0.85", "zh_Hans": "例如：(9.5*10 + 4.8*15 + 15*5 - 30) * 0.85"},
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
	return &ExpressionTool{BuiltinTool: builtin.NewBuiltinTool(entity, tenantID)}
}

// Invoke evaluates the expression.
func (t *ExpressionTool) Invoke(
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

	expression := rawStringParam(toolParameters, "expression")
	if expression == "" {
		return nil, fmt.Errorf("expression is required")
	}
	precision, err := precisionParam(toolParameters)
	if err != nil {
		return nil, err
	}
	normalized, result, err := evaluateExpression(expression)
	if err != nil {
		return nil, err
	}
	result = result.Round(int32(precision))
	resultValue, _ := result.Float64()
	if math.IsInf(resultValue, 0) || math.IsNaN(resultValue) {
		return nil, fmt.Errorf("result is not a finite number")
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"expression":            expression,
		"normalized_expression": normalized,
		"result":                resultValue,
		"precision":             precision,
	})}, nil
}

func evaluateExpression(expression string) (string, decimal.Decimal, error) {
	parser, err := newExpressionParser(expression)
	if err != nil {
		return "", decimal.Zero, err
	}
	result, err := parser.parse()
	if err != nil {
		return "", decimal.Zero, err
	}
	return parser.normalized.String(), result, nil
}

// ForkToolRuntime creates a copy of the tool with a new runtime.
func (t *ExpressionTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &ExpressionTool{
		BuiltinTool: t.BuiltinTool.ForkToolRuntime(runtime),
	}
}

var _ tools.Tool = (*ExpressionTool)(nil)
