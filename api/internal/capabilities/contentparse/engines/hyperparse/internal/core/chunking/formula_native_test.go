package chunking

import "testing"

func TestLooksLikeFormulaText_Greek(t *testing.T) {
	if !looksLikeFormulaText("α + β = γ 式中") {
		t.Fatal("expected Greek equality line as formula candidate")
	}
}

func TestRejectPseudoFormula_BasicArithmetic(t *testing.T) {
	if !rejectPseudoFormula("1 + 2 = 3") {
		t.Fatal("expected short basic arithmetic as pseudo")
	}
	if rejectPseudoFormula("$a_i$ = 1") {
		t.Fatal("should not reject LaTeX-like fragment")
	}
}

func TestIsDocMathFontBaseName(t *testing.T) {
	if !IsDocMathFontBaseName("ABCDEE+CambriaMath") {
		t.Fatal("expected Cambria Math")
	}
	if IsDocMathFontBaseName("SimSun") {
		t.Fatal("body font")
	}
}

func TestLongLatinRun(t *testing.T) {
	if !longLatinRunWithoutMath("abcdefgh") {
		t.Fatal("expected long latin run")
	}
	if longLatinRunWithoutMath("a = b + cdefgh") {
		t.Fatal("should not reject when '=' present")
	}
}
