package pdf

import "testing"

func TestNormalizeAdobeSymbolPUAText_AdobeF0xx(t *testing.T) {
	// U+F000+byte 常见于错误 ToUnicode；0x28/0x29/0x3D 应对 '('、')'、'='。
	s := string([]rune{'\uf028', '\uf029', '\uf03d'})
	out := normalizeAdobeSymbolPUAText(s)
	if out != "()=" {
		t.Fatalf("got %q want ()=", out)
	}
	if got := normalizeAdobeSymbolPUAText("plain"); got != "plain" {
		t.Fatalf("fast path: got %q", got)
	}
}
