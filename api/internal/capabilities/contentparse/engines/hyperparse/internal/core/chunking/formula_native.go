package chunking

import (
	"strings"
	"unicode"
)

const (
	legacyFormulaPrefixFullWidth = "\u516c\u5f0f\uff1a"
	legacyFormulaPrefixASCII     = "\u516c\u5f0f:"
	legacyFormulaWhereKeyword    = "\u5176\u4e2d"
)

// Lightweight native formula detection aligned with docs/formulaParsingRules.md module 1.
// This only uses whole-segment text and does not implement fraction, radical,
// or matrix geometry rules that require character-level coordinates.

// isDocMathOrVariableRune marks runes that contribute to math-related ratios,
// including Latin variables, digits, and document-approved math symbols.
func isDocMathOrVariableRune(r rune) bool {
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
		return true
	}
	if unicode.IsDigit(r) {
		return true
	}
	// Greek letters from the formula rules.
	if (r >= '\u03B1' && r <= '\u03C9') || (r >= '\u0391' && r <= '\u03A9') {
		return true
	}
	switch r {
	case '.', ',', '+', '-', '*', '/', '=', '^', '_', '(', ')', '（', '）', '[', ']', '{', '}', '|', '‖',
		'×', '÷', '±', '≤', '≥', '≈', '≠', '<', '>',
		'∈', '∉', '∀', '∃', '∧', '∨', '¬', '⇒', '⇔',
		'∫', '∬', '∭', '∮', '∯', '∰', '∂', '∇', '∞',
		'∑', '∏', '∐',
		'√', '∛', '∜',
		'·', '∴', '∵', '…',
		'⁄', '⟨', '⟩',
		'$':
		return true
	case ' ', '\u00A0':
		return true
	}
	return false
}

// hasDocComplexMathSymbol checks for at least one non-basic math symbol beyond
// ASCII letters, digits, plus, minus, and equals.
func hasDocComplexMathSymbol(t string) bool {
	for _, r := range t {
		if (r >= '\u03B1' && r <= '\u03C9') || (r >= '\u0391' && r <= '\u03A9') {
			return true
		}
		switch r {
		case '×', '÷', '±', '≤', '≥', '≈', '≠', '<', '>',
			'∈', '∉', '∀', '∃', '∧', '∨', '¬', '⇒', '⇔',
			'∫', '∬', '∭', '∮', '∯', '∰', '∂', '∇', '∞',
			'∑', '∏', '∐',
			'√', '∛', '∜',
			'·', '∴', '∵', '…', '⁄', '⟨', '⟩', '‖',
			'^', '_':
			return true
		}
	}
	return false
}

func mathRelatedRuneRatio(t string) float64 {
	if t == "" {
		return 0
	}
	var tot, hit int
	for _, r := range t {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		tot++
		if isDocMathOrVariableRune(r) {
			hit++
		}
	}
	if tot == 0 {
		return 0
	}
	return float64(hit) / float64(tot)
}

// formulaCandidateScore increases confidence when multiple formula signals match.
func formulaCandidateScore(t string) int {
	t = strings.TrimSpace(t)
	score := 0
	if mathRelatedRuneRatio(t) >= 0.70 {
		score++
	}
	if hasDocComplexMathSymbol(t) {
		score++
	}
	// Longest simplified run of math-related characters.
	if longestMathRunLen(t) >= 3 {
		score++
	}
	if strings.ContainsRune(t, '=') && (formulaVarRE.MatchString(t) || strings.ContainsAny(t, "()（）")) {
		score++
	}
	if symbolCountLegacy(t) >= 3 && formulaVarRE.MatchString(t) && strings.ContainsAny(t, "()（）") {
		score++
	}
	return score
}

func longestMathRunLen(t string) int {
	cur, best := 0, 0
	for _, r := range t {
		if isDocMathOrVariableRune(r) || unicode.IsLetter(r) {
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 0
		}
	}
	return best
}

func symbolCountLegacy(t string) int {
	n := 0
	for _, r := range t {
		switch r {
		case '=', '+', '-', '*', '/', '^', '_', '(', ')', '（', '）', '×', '÷', '≤', '≥', '≈', '≠', '∑', '∏', '√', '∫':
			n++
		}
	}
	return n
}

// rejectPseudoFormula filters obvious short arithmetic snippets while keeping
// segments with TeX markers or complex math symbols.
func rejectPseudoFormula(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" {
		return true
	}
	if strings.ContainsAny(t, "$^_") {
		return false
	}
	if hasDocComplexMathSymbol(t) {
		return false
	}
	// Pure decimal numbers.
	if isPureDecimalNumber(t) {
		return true
	}
	// Very short basic arithmetic lines.
	if isOnlyBasicArithmeticLine(t) && len([]rune(t)) <= 12 {
		return true
	}
	// Long Latin words without digits, equals signs, or complex math symbols.
	if longLatinRunWithoutMath(t) {
		return true
	}
	return false
}

func isPureDecimalNumber(t string) bool {
	t = strings.TrimSpace(t)
	if t == "" {
		return false
	}
	dot := 0
	for _, r := range t {
		if unicode.IsDigit(r) {
			continue
		}
		if r == '.' {
			dot++
			if dot > 1 {
				return false
			}
			continue
		}
		return false
	}
	return true
}

func isOnlyBasicArithmeticLine(t string) bool {
	t = strings.TrimSpace(t)
	for _, r := range t {
		if unicode.IsDigit(r) || r == ' ' {
			continue
		}
		switch r {
		case '+', '-', '*', '/', '=', '(', ')', '.':
			continue
		default:
			return false
		}
	}
	return true
}

func longLatinRunWithoutMath(t string) bool {
	if strings.ContainsRune(t, '=') || hasDocComplexMathSymbol(t) {
		return false
	}
	best, cur := 0, 0
	for _, r := range t {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 0
		}
	}
	return best > 6
}

// looksLikeFormulaText detects native formula candidates while preserving
// compatibility with the legacy Chinese formula prefix.
func looksLikeFormulaText(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || len([]rune(t)) < 6 || len([]rune(t)) > 260 {
		return false
	}
	if looksLikeListItem(t) || looksLikeHeading(t) {
		return false
	}
	if strings.HasPrefix(t, "formula:") || strings.HasPrefix(t, legacyFormulaPrefixFullWidth) || strings.HasPrefix(t, legacyFormulaPrefixASCII) {
		return true
	}
	low := strings.ToLower(t)
	if strings.Contains(low, "http://") || strings.Contains(low, "https://") {
		return false
	}
	if !hasAnyClassicFormulaMarker(t) {
		return false
	}

	if rejectPseudoFormula(t) {
		return false
	}

	// Formula rules module 1.2: score at least two matched conditions.
	if formulaCandidateScore(t) >= 2 {
		return true
	}

	hasEq := strings.ContainsRune(t, '=')
	hasParen := strings.ContainsAny(t, "()（）")
	hasVar := formulaVarRE.MatchString(t)
	if strings.ContainsRune(t, '$') {
		hasVar = true
	}
	if hasEq && (hasVar || hasParen) {
		return true
	}
	if symbolCountLegacy(t) >= 3 && hasVar && hasParen {
		return true
	}
	return false
}

func hasAnyClassicFormulaMarker(t string) bool {
	for _, r := range t {
		switch r {
		case '=', '+', '-', '*', '/', '^', '_', '(', ')', '（', '）', '×', '÷', '≤', '≥', '≈', '≠', '∑', '∏', '√', '∫', '$':
			return true
		}
		if (r >= '\u03B1' && r <= '\u03C9') || (r >= '\u0391' && r <= '\u03A9') {
			return true
		}
		if r == '∈' || r == '∀' || r == '∃' || r == '∂' || r == '∇' || r == '∞' || r == '∬' || r == '∭' {
			return true
		}
	}
	return false
}

// latexHintFromNativeSegment converts common Unicode math symbols into a
// best-effort LaTeX hint for downstream consumers.
func latexHintFromNativeSegment(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		switch r {
		case '×':
			b.WriteString(`\times `)
		case '÷':
			b.WriteString(`\div `)
		case '≤':
			b.WriteString(`\leq `)
		case '≥':
			b.WriteString(`\geq `)
		case '≠':
			b.WriteString(`\neq `)
		case '≈':
			b.WriteString(`\approx `)
		case '∞':
			b.WriteString(`\infty `)
		case '∑':
			b.WriteString(`\sum `)
		case '∏':
			b.WriteString(`\prod `)
		case '∫':
			b.WriteString(`\int `)
		case '√':
			b.WriteString(`\sqrt `)
		case 'α':
			b.WriteString(`\alpha `)
		case 'β':
			b.WriteString(`\beta `)
		case 'γ':
			b.WriteString(`\gamma `)
		case 'δ':
			b.WriteString(`\delta `)
		case 'ε', 'ϵ':
			b.WriteString(`\epsilon `)
		case 'θ':
			b.WriteString(`\theta `)
		case 'λ':
			b.WriteString(`\lambda `)
		case 'μ':
			b.WriteString(`\mu `)
		case 'π':
			b.WriteString(`\pi `)
		case 'σ':
			b.WriteString(`\sigma `)
		case 'φ', 'ϕ':
			b.WriteString(`\phi `)
		case 'ω':
			b.WriteString(`\omega `)
		case 'Δ':
			b.WriteString(`\Delta `)
		case 'Σ':
			b.WriteString(`\Sigma `)
		case 'Ω':
			b.WriteString(`\Omega `)
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// IsDocMathFontBaseName checks the math font whitelist using case-insensitive
// substring matching. Input is usually a parsed PDF /BaseFont name.
func IsDocMathFontBaseName(base string) bool {
	base = strings.TrimSpace(base)
	if base == "" {
		return false
	}
	n := strings.ToLower(strings.ReplaceAll(base, " ", ""))
	subs := []string{
		"cambriamath", "timesnewroman", "latinmodernmath", "stix",
		"mathjax", "katex", "xitsmath", "texgyre", "asana",
		"eulermath", "lmmath", "unicode-math", "gyremath", "firamath",
	}
	for _, s := range subs {
		if strings.Contains(n, s) {
			return true
		}
	}
	return false
}
