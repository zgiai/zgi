package chunking

import (
	"fmt"
	"strings"
)

// expandTextsForFormulaChunks splits formulas embedded in the same text segment
// into independent TextLike values before rule application, allowing
// applyTextFormulaRule to emit standalone type=formula chunks.
func expandTextsForFormulaChunks(texts []TextLike) []TextLike {
	if len(texts) == 0 {
		return nil
	}
	out := make([]TextLike, 0, len(texts)+2)
	seq := 0
	for _, t := range texts {
		origOrder := t.Order
		pieces := extractFormulaTextPieces(t.Text)
		if len(pieces) <= 1 {
			nt := t
			nt.Order = seq
			seq++
			nt.SegKeyBase = origOrder
			nt.ChunkKey = ""
			out = append(out, nt)
			continue
		}
		parentBB := t.BBox
		for i, p := range pieces {
			nt := t
			nt.Text = p
			nt.Order = seq
			seq++
			nt.SegKeyBase = 0
			nt.ChunkKey = fmt.Sprintf("seg_%d_m%d", origOrder, i)
			// Split formula slices evenly within the parent bbox as a fallback;
			// alignExpandedTextBBoxes replaces this with geometry lines when matched.
			nt.BBox = sliceParentBBoxByLineRatio(parentBB, i, 1, len(pieces))
			out = append(out, nt)
		}
	}
	return out
}

func extractFormulaTextPieces(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.Contains(s, "$") {
		p := splitByDollarDelimiters(s)
		if len(p) > 1 || (len(p) == 1 && p[0] != s) {
			return p
		}
	}
	if strings.ContainsAny(s, "\n\r") {
		p := splitByNewlineFormulaIsolation(s)
		if len(p) > 1 {
			return p
		}
	}
	return []string{s}
}

func splitByDollarDelimiters(s string) []string {
	var out []string
	var b strings.Builder
	i := 0
	inInline := false
	flush := func() {
		t := strings.TrimSpace(b.String())
		b.Reset()
		if t != "" {
			out = append(out, t)
		}
	}
	for i < len(s) {
		if !inInline && i+1 < len(s) && s[i] == '$' && s[i+1] == '$' {
			flush()
			i += 2
			j := strings.Index(s[i:], "$$")
			if j < 0 {
				b.WriteString(s[i:])
				flush()
				return out
			}
			mid := strings.TrimSpace(s[i : i+j])
			if mid != "" {
				out = append(out, mid)
			}
			i += j + 2
			continue
		}
		if !inInline && s[i] == '$' {
			flush()
			inInline = true
			i++
			continue
		}
		if inInline && s[i] == '$' {
			flush()
			inInline = false
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	flush()
	return out
}

func splitByNewlineFormulaIsolation(s string) []string {
	raw := strings.Split(s, "\n")
	var lines []string
	for _, ln := range raw {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		lines = append(lines, ln)
	}
	if len(lines) <= 1 {
		return []string{strings.TrimSpace(s)}
	}
	type group struct {
		isFormula bool
		lines     []string
	}
	var groups []group
	for _, ln := range lines {
		f := lineQualifiesAsIsolatedFormulaLine(ln)
		if len(groups) == 0 {
			groups = append(groups, group{isFormula: f, lines: []string{ln}})
			continue
		}
		last := len(groups) - 1
		if groups[last].isFormula == f {
			groups[last].lines = append(groups[last].lines, ln)
		} else {
			groups = append(groups, group{isFormula: f, lines: []string{ln}})
		}
	}
	var out []string
	for _, g := range groups {
		t := strings.TrimSpace(strings.Join(g.lines, "\n"))
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	if len(out) <= 1 {
		return []string{strings.TrimSpace(s)}
	}
	return out
}

// lineQualifiesAsIsolatedFormulaLine decides whether a split line is more like
// a standalone formula than paragraph body text.
func lineQualifiesAsIsolatedFormulaLine(line string) bool {
	if looksLikeFormulaText(line) {
		return true
	}
	t := strings.TrimSpace(line)
	if len([]rune(t)) < 4 || len([]rune(t)) > 240 {
		return false
	}
	if rejectPseudoFormula(t) {
		return false
	}
	if formulaCandidateScore(t) >= 2 {
		return true
	}
	// Short line with equals plus variables/parentheses and a high math ratio.
	if strings.ContainsRune(t, '=') && (formulaVarRE.MatchString(t) || strings.ContainsAny(t, "()（）")) {
		if mathRelatedRuneRatio(t) >= 0.45 {
			return true
		}
	}
	return false
}
