package calculator

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

type expressionParser struct {
	input      string
	pos        int
	normalized strings.Builder
}

func newExpressionParser(expression string) (*expressionParser, error) {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		return nil, fmt.Errorf("expression is required")
	}
	if len(trimmed) > maxExpressionLength {
		return nil, fmt.Errorf("expression length must be <= %d", maxExpressionLength)
	}
	return &expressionParser{input: trimmed}, nil
}

func (p *expressionParser) parse() (decimal.Decimal, error) {
	result, err := p.parseAddSub()
	if err != nil {
		return decimal.Zero, err
	}
	p.skipSpaces()
	if !p.eof() {
		return decimal.Zero, p.unexpectedTokenError()
	}
	return result, nil
}

func (p *expressionParser) parseAddSub() (decimal.Decimal, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return decimal.Zero, err
	}
	for {
		p.skipSpaces()
		if p.consume('+') {
			right, err := p.parseMulDiv()
			if err != nil {
				return decimal.Zero, err
			}
			left = left.Add(right)
			continue
		}
		if p.consume('-') {
			right, err := p.parseMulDiv()
			if err != nil {
				return decimal.Zero, err
			}
			left = left.Sub(right)
			continue
		}
		return left, nil
	}
}

func (p *expressionParser) parseMulDiv() (decimal.Decimal, error) {
	left, err := p.parsePower()
	if err != nil {
		return decimal.Zero, err
	}
	for {
		p.skipSpaces()
		if p.consume('*') {
			right, err := p.parsePower()
			if err != nil {
				return decimal.Zero, err
			}
			left = left.Mul(right)
			continue
		}
		if p.consume('/') {
			right, err := p.parsePower()
			if err != nil {
				return decimal.Zero, err
			}
			if right.IsZero() {
				return decimal.Zero, fmt.Errorf("division by zero")
			}
			left = left.Div(right)
			continue
		}
		if p.consume('%') {
			right, err := p.parsePower()
			if err != nil {
				return decimal.Zero, err
			}
			if right.IsZero() {
				return decimal.Zero, fmt.Errorf("modulo by zero")
			}
			left = left.Mod(right)
			continue
		}
		return left, nil
	}
}

func (p *expressionParser) parsePower() (decimal.Decimal, error) {
	left, err := p.parseUnary()
	if err != nil {
		return decimal.Zero, err
	}
	p.skipSpaces()
	if !p.consume('^') {
		return left, nil
	}
	right, err := p.parsePower()
	if err != nil {
		return decimal.Zero, err
	}
	return decimalPower(left, right)
}

func (p *expressionParser) parseUnary() (decimal.Decimal, error) {
	p.skipSpaces()
	if p.consume('+') {
		return p.parseUnary()
	}
	if p.consume('-') {
		value, err := p.parseUnary()
		if err != nil {
			return decimal.Zero, err
		}
		return value.Neg(), nil
	}
	return p.parsePrimary()
}

func (p *expressionParser) parsePrimary() (decimal.Decimal, error) {
	p.skipSpaces()
	if p.consume('(') {
		value, err := p.parseAddSub()
		if err != nil {
			return decimal.Zero, err
		}
		p.skipSpaces()
		if !p.consume(')') {
			return decimal.Zero, fmt.Errorf("mismatched parentheses")
		}
		return value, nil
	}
	return p.parseNumber()
}

func (p *expressionParser) parseNumber() (decimal.Decimal, error) {
	p.skipSpaces()
	start := p.pos
	dotSeen := false
	digitSeen := false
	for !p.eof() {
		ch := p.input[p.pos]
		if ch >= '0' && ch <= '9' {
			digitSeen = true
			p.pos++
			continue
		}
		if ch == '.' && !dotSeen {
			dotSeen = true
			p.pos++
			continue
		}
		break
	}
	if !digitSeen {
		return decimal.Zero, p.unexpectedTokenError()
	}
	raw := p.input[start:p.pos]
	normalized := normalizeNumberLiteral(raw)
	p.normalized.WriteString(normalized)
	value, err := decimal.NewFromString(normalized)
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid number literal: %s", raw)
	}
	return value, nil
}

func (p *expressionParser) consume(ch byte) bool {
	p.skipSpaces()
	if p.eof() || p.input[p.pos] != ch {
		return false
	}
	p.pos++
	p.normalized.WriteByte(ch)
	return true
}

func (p *expressionParser) skipSpaces() {
	for !p.eof() {
		switch p.input[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			if isSupportedExpressionByte(p.input[p.pos]) {
				return
			}
			return
		}
	}
}

func (p *expressionParser) eof() bool {
	return p.pos >= len(p.input)
}

func (p *expressionParser) unexpectedTokenError() error {
	if p.eof() {
		return fmt.Errorf("unexpected end of expression")
	}
	ch := p.input[p.pos]
	if !isSupportedExpressionByte(ch) {
		return fmt.Errorf("expression contains unsupported character: %q", ch)
	}
	return fmt.Errorf("invalid expression near %q", p.input[p.pos:])
}

func isSupportedExpressionByte(ch byte) bool {
	return (ch >= '0' && ch <= '9') ||
		ch == '.' ||
		ch == '+' ||
		ch == '-' ||
		ch == '*' ||
		ch == '/' ||
		ch == '%' ||
		ch == '^' ||
		ch == '(' ||
		ch == ')'
}

func normalizeNumberLiteral(raw string) string {
	if strings.HasPrefix(raw, ".") {
		raw = "0" + raw
	}
	if strings.HasSuffix(raw, ".") {
		raw += "0"
	}
	return raw
}

func decimalPower(base decimal.Decimal, exponent decimal.Decimal) (decimal.Decimal, error) {
	if !exponent.Equal(exponent.Truncate(0)) {
		return decimal.Zero, fmt.Errorf("power exponent must be an integer")
	}
	exp := exponent.IntPart()
	if exp > maxExpressionPowerAbsExp || exp < -maxExpressionPowerAbsExp {
		return decimal.Zero, fmt.Errorf("power exponent absolute value must be <= %d", maxExpressionPowerAbsExp)
	}
	if exp == 0 {
		return decimal.NewFromInt(1), nil
	}
	if exp < 0 && base.IsZero() {
		return decimal.Zero, fmt.Errorf("division by zero")
	}
	result := decimal.NewFromInt(1)
	absExp := exp
	if absExp < 0 {
		absExp = -absExp
	}
	for i := int64(0); i < absExp; i++ {
		result = result.Mul(base)
	}
	if exp < 0 {
		return decimal.NewFromInt(1).Div(result), nil
	}
	return result, nil
}
