package calculator_test

import (
	"context"
	"strings"
	"testing"

	"github.com/zgiai/ginext/internal/modules/tools"
	calculator "github.com/zgiai/ginext/internal/modules/tools/builtin/calculator"
)

func TestCalculatorTools_MetadataIncludesLocalizedHumanText(t *testing.T) {
	provider := calculator.NewProvider().GetEntity()
	if got := provider.Identity.Label.Get("zh_Hans"); got != "计算器工具" {
		t.Fatalf("provider zh_Hans label = %q, want localized text", got)
	}
	if got := provider.Identity.Description.Get("zh_Hans"); got == "" || got == provider.Identity.Description.Get("en_US") {
		t.Fatalf("provider zh_Hans description = %q, want localized text", got)
	}

	tests := []struct {
		name      string
		entity    tools.ToolEntity
		wantLabel string
	}{
		{name: "calculate", entity: calculator.NewCalculateTool("").GetEntity(), wantLabel: "计算"},
		{name: "percentage", entity: calculator.NewPercentageTool("").GetEntity(), wantLabel: "百分比计算"},
		{name: "expression", entity: calculator.NewExpressionTool("").GetEntity(), wantLabel: "表达式计算"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entity.Identity.Label.Get("zh_Hans"); got != tt.wantLabel {
				t.Fatalf("zh_Hans label = %q, want %q", got, tt.wantLabel)
			}
			if got := tt.entity.Description.Human.Get("zh_Hans"); got == "" || got == tt.entity.Description.Human.Get("en_US") {
				t.Fatalf("zh_Hans description = %q, want localized text", got)
			}
			for _, param := range tt.entity.Parameters {
				if got := param.Label.Get("zh_Hans"); got == "" || got == param.Label.Get("en_US") {
					t.Fatalf("parameter %s zh_Hans label = %q, want localized text", param.Name, got)
				}
				if got := param.HumanDescription.Get("zh_Hans"); got == "" || got == param.HumanDescription.Get("en_US") {
					t.Fatalf("parameter %s zh_Hans human description = %q, want localized text", param.Name, got)
				}
				if got := param.Placeholder.Get("zh_Hans"); param.Type != tools.ToolParameterTypeSelect && got == "" {
					t.Fatalf("parameter %s zh_Hans placeholder is empty", param.Name)
				}
				for _, option := range param.Options {
					if got := option.Label.Get("zh_Hans"); got == "" || got == option.Label.Get("en_US") {
						t.Fatalf("parameter %s option %s zh_Hans label = %q, want localized text", param.Name, option.Value, got)
					}
				}
			}
		})
	}
}

func TestCalculateTool_Operations_ReturnExpectedResults(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		left      float64
		right     float64
		want      float64
	}{
		{name: "add", operation: "add", left: 10, right: 3, want: 13},
		{name: "subtract", operation: "subtract", left: 10, right: 3, want: 7},
		{name: "multiply", operation: "multiply", left: 10, right: 3, want: 30},
		{name: "divide", operation: "divide", left: 10, right: 4, want: 2.5},
		{name: "power", operation: "power", left: 2, right: 3, want: 8},
		{name: "mod", operation: "mod", left: 10, right: 3, want: 1},
	}

	tool := calculator.NewCalculateTool("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := tool.Invoke(context.Background(), "user", map[string]interface{}{
				"operation": tt.operation,
				"left":      tt.left,
				"right":     tt.right,
				"precision": 6,
			}, nil, nil, nil)
			if err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			got := messages[0].Data["result"]
			if got != tt.want {
				t.Fatalf("result = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateTool_Precision_RoundsResult(t *testing.T) {
	tool := calculator.NewCalculateTool("")
	messages, err := tool.Invoke(context.Background(), "user", map[string]interface{}{
		"operation": "divide",
		"left":      10,
		"right":     3,
		"precision": 2,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if got := messages[0].Data["result"]; got != 3.33 {
		t.Fatalf("result = %v, want 3.33", got)
	}
}

func TestCalculateTool_InvalidInput_ReturnsClearError(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{name: "invalid operation", params: map[string]interface{}{"operation": "sqrt", "left": 1, "right": 2}, want: "invalid operation"},
		{name: "missing number", params: map[string]interface{}{"operation": "add", "left": 1}, want: "right is required"},
		{name: "non number", params: map[string]interface{}{"operation": "add", "left": "1", "right": 2}, want: "left must be a finite number"},
		{name: "division by zero", params: map[string]interface{}{"operation": "divide", "left": 1, "right": 0}, want: "division by zero"},
		{name: "precision too high", params: map[string]interface{}{"operation": "add", "left": 1, "right": 2, "precision": 13}, want: "precision must be between"},
	}

	tool := calculator.NewCalculateTool("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Invoke(context.Background(), "user", tt.params, nil, nil, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Invoke() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestExpressionTool_EvaluatesComplexArithmetic(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		precision  int
		want       float64
	}{
		{
			name:       "grocery discount",
			expression: "(9.5*10 + 4.8*15 + 15*5 - 30) * 0.85",
			precision:  2,
			want:       180.2,
		},
		{
			name:       "operator precedence",
			expression: "2 + 3 * 4",
			precision:  2,
			want:       14,
		},
		{
			name:       "power precedence",
			expression: "2*3^2",
			precision:  2,
			want:       18,
		},
		{
			name:       "right associative power",
			expression: "2^3^2",
			precision:  2,
			want:       512,
		},
		{
			name:       "decimal addition",
			expression: "0.1 + 0.2",
			precision:  2,
			want:       0.3,
		},
		{
			name:       "unary binds before power",
			expression: "-2^2",
			precision:  2,
			want:       4,
		},
		{
			name:       "parenthesized negative power",
			expression: "(-2)^2",
			precision:  2,
			want:       4,
		},
		{
			name:       "parentheses and unary",
			expression: "-(10 - 2) / 4",
			precision:  2,
			want:       -2,
		},
	}

	tool := calculator.NewExpressionTool("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := tool.Invoke(context.Background(), "user", map[string]interface{}{
				"expression": tt.expression,
				"precision":  tt.precision,
			}, nil, nil, nil)
			if err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			if got := messages[0].Data["result"]; got != tt.want {
				t.Fatalf("result = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpressionTool_InvalidInput_ReturnsClearError(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{name: "missing expression", params: map[string]interface{}{}, want: "expression is required"},
		{name: "unsupported character", params: map[string]interface{}{"expression": "sqrt(4)"}, want: "unsupported character"},
		{name: "division by zero", params: map[string]interface{}{"expression": "1 / 0"}, want: "division by zero"},
		{name: "modulo by zero", params: map[string]interface{}{"expression": "1 % 0"}, want: "modulo by zero"},
		{name: "non integer exponent", params: map[string]interface{}{"expression": "4 ^ 0.5"}, want: "power exponent must be an integer"},
		{name: "exponent too large", params: map[string]interface{}{"expression": "2 ^ 101"}, want: "power exponent absolute value"},
		{name: "mismatched parentheses", params: map[string]interface{}{"expression": "(1 + 2"}, want: "mismatched parentheses"},
		{name: "unsupported syntax", params: map[string]interface{}{"expression": "1 << 2"}, want: "unsupported character"},
		{name: "too long", params: map[string]interface{}{"expression": strings.Repeat("1+", 300) + "1"}, want: "expression length"},
	}

	tool := calculator.NewExpressionTool("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Invoke(context.Background(), "user", tt.params, nil, nil, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Invoke() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestPercentageTool_Operations_ReturnExpectedResults(t *testing.T) {
	tests := []struct {
		name      string
		params    map[string]interface{}
		want      float64
		resultKey string
	}{
		{name: "percent of", params: map[string]interface{}{"operation": "percent_of", "value": 200, "percent": 15, "precision": 2}, want: 30},
		{name: "change", params: map[string]interface{}{"operation": "change", "from": 80, "to": 100, "precision": 2}, want: 25},
		{name: "increase", params: map[string]interface{}{"operation": "apply_increase", "value": 100, "percent": 12.5, "precision": 2}, want: 112.5},
		{name: "decrease", params: map[string]interface{}{"operation": "apply_decrease", "value": 100, "percent": 12.5, "precision": 2}, want: 87.5},
	}

	tool := calculator.NewPercentageTool("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := tool.Invoke(context.Background(), "user", tt.params, nil, nil, nil)
			if err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			if got := messages[0].Data["result"]; got != tt.want {
				t.Fatalf("result = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPercentageTool_InvalidInput_ReturnsClearError(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{name: "invalid operation", params: map[string]interface{}{"operation": "ratio", "value": 1, "percent": 2}, want: "invalid operation"},
		{name: "missing value", params: map[string]interface{}{"operation": "percent_of", "percent": 2}, want: "value is required"},
		{name: "missing percent", params: map[string]interface{}{"operation": "apply_increase", "value": 2}, want: "percent is required"},
		{name: "zero from", params: map[string]interface{}{"operation": "change", "from": 0, "to": 2}, want: "from must not be zero"},
	}

	tool := calculator.NewPercentageTool("")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Invoke(context.Background(), "user", tt.params, nil, nil, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Invoke() error = %v, want %q", err, tt.want)
			}
		})
	}
}
