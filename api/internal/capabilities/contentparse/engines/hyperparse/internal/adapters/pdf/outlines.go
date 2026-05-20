package pdf

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// OutlineEntry is one flattened item from the document outline tree.
type OutlineEntry struct {
	Title      string `json:"title"`
	Level      int    `json:"level"`
	PageObject int    `json:"page_object,omitempty"`
	Dest       string `json:"dest,omitempty"`
	TargetRaw  string `json:"target_raw,omitempty"`
	TargetKind string `json:"target_kind,omitempty"` // page|dest|action|unknown
	Object     int    `json:"outline_item_object,omitempty"`
}

// OutlineWriteEntry is used to write bookmarks by page_index or page_object.
type OutlineWriteEntry struct {
	Title      string `json:"title"`
	Level      int    `json:"level"`
	PageIndex  int    `json:"page_index,omitempty"`
	PageObject int    `json:"page_object,omitempty"`
	TargetRaw  string `json:"target_raw,omitempty"`
	Dest       string `json:"dest,omitempty"` // Raw /Dest value such as "[3 0 R /XYZ null null null]".
	Action     string `json:"action,omitempty"`
}

var destPageRefRE = regexp.MustCompile(`(?:/Dest|/D)\s*(?:\[\s*)?(\d+)\s+(\d+)\s+R`)

// ExtractOutlineEntriesFromBytes parses Catalog /Outlines and flattens the tree preorder.
// It supports common /Dest and /D page references; named destinations leave page_object empty.
func ExtractOutlineEntriesFromBytes(data []byte, mode string) ([]OutlineEntry, error) {
	t0 := time.Now()
	fullDocDebugf("ExtractOutlineEntriesFromBytes begin")
	defer func() {
		fullDocDebugf("ExtractOutlineEntriesFromBytes done elapsed_ms=%d", time.Since(t0).Milliseconds())
	}()
	rootObj, ok := detectRootObjectNumber(data)
	if !ok {
		if mode == ValidationModeRelaxed {
			return []OutlineEntry{}, nil
		}
		return nil, fmt.Errorf("cannot detect root object")
	}
	cat, ok := parseCatalogObject(data, rootObj)
	if !ok {
		if mode == ValidationModeRelaxed {
			return []OutlineEntry{}, nil
		}
		return nil, fmt.Errorf("invalid catalog")
	}
	catBlock, ok := findObjectBlockByNumber(data, cat.objNumber)
	if !ok {
		if mode == ValidationModeRelaxed {
			return []OutlineEntry{}, nil
		}
		return nil, fmt.Errorf("catalog object not found")
	}
	outlinesRef, ok := parseIndirectRefObjectNumberByKey(catBlock, "/Outlines")
	if !ok || outlinesRef <= 0 {
		return []OutlineEntry{}, nil
	}
	rootBlock, ok := findObjectBlockByNumber(data, outlinesRef)
	if !ok {
		if mode == ValidationModeRelaxed {
			return []OutlineEntry{}, nil
		}
		return nil, fmt.Errorf("outlines root not found")
	}
	if !bytes.Contains(rootBlock, []byte("/Type /Outlines")) && !bytes.Contains(rootBlock, []byte("/Outlines")) {
		// Some generators omit /Type; still try /First.
	}
	first, ok := parseIndirectRefObjectNumberByKey(rootBlock, "/First")
	if !ok || first <= 0 {
		return []OutlineEntry{}, nil
	}
	visited := map[int]bool{}
	var out []OutlineEntry
	walkOutlineItems(data, first, 0, visited, &out)
	return out, nil
}

func walkOutlineItems(data []byte, objNum int, level int, visited map[int]bool, out *[]OutlineEntry) {
	for objNum > 0 {
		if visited[objNum] {
			break
		}
		visited[objNum] = true
		block, ok := findObjectBlockByNumber(data, objNum)
		if !ok {
			break
		}
		title := extractPDFStringByKey(block, "Title")
		pageObj, destSnippet, targetRaw, targetKind := extractOutlineDest(block)
		*out = append(*out, OutlineEntry{
			Title:      title,
			Level:      level,
			PageObject: pageObj,
			Dest:       destSnippet,
			TargetRaw:  targetRaw,
			TargetKind: targetKind,
			Object:     objNum,
		})
		if child, ok := parseIndirectRefObjectNumberByKey(block, "/First"); ok && child > 0 {
			walkOutlineItems(data, child, level+1, visited, out)
		}
		next := 0
		if n, ok := parseIndirectRefObjectNumberByKey(block, "/Next"); ok {
			next = n
		}
		objNum = next
	}
}

func extractOutlineDest(block []byte) (pageObject int, destSnippet string, targetRaw string, targetKind string) {
	targetKind = "unknown"
	if m := destPageRefRE.FindSubmatch(block); len(m) >= 2 {
		if n, err := strconv.Atoi(string(m[1])); err == nil && n > 0 {
			pageObject = n
		}
	}
	// Short preview for manual comparison.
	if idx := bytes.Index(block, []byte("/Dest")); idx >= 0 {
		targetKind = "dest"
		if pageObject > 0 {
			targetKind = "page"
		}
		targetRaw = extractTargetRawAfterKey(block, idx, "/Dest")
		end := idx + 120
		if end > len(block) {
			end = len(block)
		}
		destSnippet = string(bytes.TrimSpace(block[idx:end]))
		if len(destSnippet) > 200 {
			destSnippet = destSnippet[:200] + "..."
		}
		return pageObject, destSnippet, targetRaw, targetKind
	}
	if idx := bytes.Index(block, []byte("/A")); idx >= 0 {
		targetKind = "action"
		if pageObject > 0 {
			targetKind = "page"
		}
		targetRaw = extractTargetRawAfterKey(block, idx, "/A")
		end := idx + 160
		if end > len(block) {
			end = len(block)
		}
		destSnippet = string(bytes.TrimSpace(block[idx:end]))
		if len(destSnippet) > 200 {
			destSnippet = destSnippet[:200] + "..."
		}
		return pageObject, destSnippet, targetRaw, targetKind
	}
	return pageObject, destSnippet, targetRaw, targetKind
}

func extractTargetRawAfterKey(block []byte, keyPos int, key string) string {
	rest := bytes.TrimSpace(block[keyPos+len(key):])
	if len(rest) == 0 {
		return ""
	}
	// Common forms: /Dest [...] or /A <<...>>.
	if rest[0] == '[' {
		if raw, ok := extractBalanced(rest, '[', ']'); ok {
			return string(raw)
		}
	}
	if len(rest) >= 2 && rest[0] == '<' && rest[1] == '<' {
		if raw, ok := extractBalancedDict(rest); ok {
			return string(raw)
		}
	}
	// Fallback for name/literal tokens.
	fields := strings.Fields(string(rest))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func extractBalanced(src []byte, open, close byte) ([]byte, bool) {
	if len(src) == 0 || src[0] != open {
		return nil, false
	}
	depth := 0
	for i := 0; i < len(src); i++ {
		if src[i] == open {
			depth++
		} else if src[i] == close {
			depth--
			if depth == 0 {
				return src[:i+1], true
			}
		}
	}
	return nil, false
}

func extractBalancedDict(src []byte) ([]byte, bool) {
	if len(src) < 2 || src[0] != '<' || src[1] != '<' {
		return nil, false
	}
	depth := 0
	for i := 0; i < len(src)-1; i++ {
		if src[i] == '<' && src[i+1] == '<' {
			depth++
			i++
			continue
		}
		if src[i] == '>' && src[i+1] == '>' {
			depth--
			i++
			if depth == 0 {
				return src[:i+1], true
			}
		}
	}
	return nil, false
}

type outlineNode struct {
	OutlineWriteEntry
	objNum int

	parent int
	first  int
	last   int
	prev   int
	next   int

	destPageObject int
	destRaw        string
	actionRaw      string
}

// AppendIncrementalOutlinesFromEntries writes an outline tree incrementally:
//   - only traditional xref-table PDFs are supported;
//   - new outlines root, item objects, and catalog are appended, then trailer /Root
//     points to the new catalog.
func AppendIncrementalOutlinesFromEntries(data []byte, entries []OutlineWriteEntry) ([]byte, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("empty outline entries")
	}
	eofPos := bytes.LastIndex(data, []byte("%%EOF"))
	if eofPos < 0 {
		return nil, fmt.Errorf("missing %%EOF")
	}
	startXRefTagPos := bytes.LastIndex(data[:eofPos], []byte("startxref"))
	if startXRefTagPos < 0 {
		return nil, fmt.Errorf("missing startxref")
	}
	startXRef, err := parseStartXRefValue(data[startXRefTagPos+len("startxref") : eofPos])
	if err != nil || startXRef < 0 || startXRef >= len(data) {
		return nil, fmt.Errorf("invalid startxref")
	}
	xType, _ := detectXRefType(data, startXRef, eofPos)
	if xType != "table" && xType != "stream" {
		return nil, fmt.Errorf("outlines append: unsupported xref type")
	}

	baseSize := 0
	if xType == "table" {
		stats, err := validateXRefTable(data, startXRef, eofPos, ValidationModeRelaxed)
		if err != nil {
			return nil, err
		}
		if !stats.hasTrailerSize {
			return nil, fmt.Errorf("outlines append: trailer missing /Size")
		}
		baseSize = stats.trailerSize
	} else {
		baseSize = detectNextObjectNumberByScan(data)
		if baseSize <= 0 {
			return nil, fmt.Errorf("outlines append: cannot determine next object number for xref stream file")
		}
	}

	oldRootObj, ok := parseRootObjectFromTrailer(data)
	if !ok {
		return nil, fmt.Errorf("outlines append: cannot parse /Root")
	}
	cat, ok := parseCatalogObject(data, oldRootObj)
	if !ok || !cat.hasPagesRef {
		return nil, fmt.Errorf("outlines append: invalid catalog/pages chain")
	}
	oldCatalogBlock, ok := findObjectBlockByNumber(data, cat.objNumber)
	if !ok {
		return nil, fmt.Errorf("outlines append: catalog object not found")
	}

	pageInfos, err := DetectPageInfosBytes(data, ValidationModeRelaxed)
	if err != nil {
		return nil, fmt.Errorf("outlines append: detect pages failed: %w", err)
	}
	pageObjSet := make(map[int]bool, len(pageInfos))
	pageObjByIndex := make(map[int]int, len(pageInfos))
	for i, p := range pageInfos {
		pageObjSet[p.ObjectNumber] = true
		pageObjByIndex[i+1] = p.ObjectNumber
	}

	nodes, err := buildOutlineNodes(entries, pageObjSet, pageObjByIndex)
	if err != nil {
		return nil, err
	}

	firstNewObj := baseSize
	outlinesRootObj := firstNewObj
	for i := range nodes {
		nodes[i].objNum = firstNewObj + 1 + i
	}
	newCatalogObj := firstNewObj + 1 + len(nodes)
	newSize := newCatalogObj + 1

	rootFirstObj, rootLastObj := 0, 0
	for i := range nodes {
		if nodes[i].Level == 0 {
			rootFirstObj = nodes[i].objNum
			break
		}
	}
	for i := len(nodes) - 1; i >= 0; i-- {
		if nodes[i].Level == 0 {
			rootLastObj = nodes[i].objNum
			break
		}
	}
	if rootFirstObj <= 0 || rootLastObj <= 0 {
		return nil, fmt.Errorf("outlines append: no level-0 entries")
	}

	var buf bytes.Buffer
	buf.Write(data)
	offsetByObj := map[int]int{}

	writeObj := func(objNum int, body string) {
		offsetByObj[objNum] = buf.Len()
		buf.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", objNum, body))
	}

	writeObj(outlinesRootObj, fmt.Sprintf("<< /Type /Outlines /First %d 0 R /Last %d 0 R /Count %d >>", rootFirstObj, rootLastObj, countOpenNodes(nodes, -1)))
	for i := range nodes {
		writeObj(nodes[i].objNum, buildOutlineItemDict(nodes, i, outlinesRootObj))
	}
	catalogBody, err := buildCatalogBodyWithOutlines(oldCatalogBlock, outlinesRootObj)
	if err != nil {
		return nil, err
	}
	writeObj(newCatalogObj, catalogBody)

	xrefPos := buf.Len()
	objNums := make([]int, 0, len(offsetByObj))
	for n := range offsetByObj {
		objNums = append(objNums, n)
	}
	sort.Ints(objNums)
	startObj := objNums[0]
	count := len(objNums)
	buf.WriteString("xref\n")
	buf.WriteString(fmt.Sprintf("%d %d\n", startObj, count))
	for _, n := range objNums {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", offsetByObj[n]))
	}
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root %d 0 R /Prev %d >>\n", newSize, newCatalogObj, startXRef))
	buf.WriteString(fmt.Sprintf("startxref\n%d\n", xrefPos))
	buf.WriteString("%%EOF\n")
	return buf.Bytes(), nil
}

func detectNextObjectNumberByScan(data []byte) int {
	maxObj := 0
	cursor := 0
	for {
		rel := bytes.Index(data[cursor:], []byte(" obj"))
		if rel < 0 {
			break
		}
		pos := cursor + rel
		lineStart := bytes.LastIndex(data[:pos], []byte("\n"))
		if lineStart < 0 {
			lineStart = 0
		} else {
			lineStart++
		}
		fields := strings.Fields(string(data[lineStart:pos]))
		if len(fields) >= 2 {
			objNum, err1 := strconv.Atoi(fields[len(fields)-2])
			genNum, err2 := strconv.Atoi(fields[len(fields)-1])
			if err1 == nil && err2 == nil && objNum > maxObj && genNum >= 0 {
				maxObj = objNum
			}
		}
		cursor = pos + len(" obj")
	}
	if maxObj <= 0 {
		return 0
	}
	return maxObj + 1
}

func buildCatalogBodyWithOutlines(catalogBlock []byte, outlinesRootObj int) (string, error) {
	dict, ok := extractFirstInlineDictFromObject(catalogBlock)
	if !ok {
		return "", fmt.Errorf("outlines append: cannot parse catalog dictionary")
	}
	out := replaceOrAppendIndirectRefKey(dict, "/Outlines", outlinesRootObj)
	if !strings.Contains(out, "/Type /Catalog") {
		out = "<< /Type /Catalog " + strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(out, ">>"), "<<")) + " >>"
	}
	return out, nil
}

func extractFirstInlineDictFromObject(objBlock []byte) (string, bool) {
	s := string(objBlock)
	start := strings.Index(s, "<<")
	if start < 0 {
		return "", false
	}
	rest := s[start:]
	depth := 0
	for i := 0; i < len(rest)-1; i++ {
		if rest[i] == '<' && rest[i+1] == '<' {
			depth++
			i++
			continue
		}
		if rest[i] == '>' && rest[i+1] == '>' {
			depth--
			i++
			if depth == 0 {
				return strings.TrimSpace(rest[:i+1]), true
			}
		}
	}
	return "", false
}

func replaceOrAppendIndirectRefKey(dict string, key string, objNum int) string {
	re := regexp.MustCompile(regexp.QuoteMeta(key) + `\s+\d+\s+\d+\s+R`)
	repl := fmt.Sprintf("%s %d 0 R", key, objNum)
	if re.MatchString(dict) {
		return re.ReplaceAllString(dict, repl)
	}
	body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(dict, ">>"), "<<"))
	if body == "" {
		return fmt.Sprintf("<< %s >>", repl)
	}
	return fmt.Sprintf("<< %s %s >>", body, repl)
}

func buildOutlineNodes(entries []OutlineWriteEntry, pageObjSet map[int]bool, pageObjByIndex map[int]int) ([]outlineNode, error) {
	nodes := make([]outlineNode, 0, len(entries))
	lastAtLevel := map[int]int{}

	for i, e := range entries {
		e.Title = strings.TrimSpace(e.Title)
		if e.Title == "" {
			return nil, fmt.Errorf("outline[%d]: empty title", i)
		}
		if e.Level < 0 {
			return nil, fmt.Errorf("outline[%d]: level must be >= 0", i)
		}
		if i == 0 && e.Level != 0 {
			return nil, fmt.Errorf("outline[0]: level must be 0")
		}
		if i > 0 && e.Level > entries[i-1].Level+1 {
			return nil, fmt.Errorf("outline[%d]: level jumps too deep (%d -> %d)", i, entries[i-1].Level, e.Level)
		}

		destPageObj := e.PageObject
		if destPageObj <= 0 && e.PageIndex > 0 {
			destPageObj = pageObjByIndex[e.PageIndex]
		}
		destRaw := strings.TrimSpace(e.Dest)
		actionRaw := strings.TrimSpace(e.Action)
		if destPageObj <= 0 && destRaw == "" && actionRaw == "" {
			return nil, fmt.Errorf("outline[%d]: page target/dest/action is required", i)
		}
		if destPageObj > 0 && !pageObjSet[destPageObj] {
			return nil, fmt.Errorf("outline[%d]: page_object=%d not found in document", i, destPageObj)
		}

		n := outlineNode{
			OutlineWriteEntry: e,
			parent:            -1,
			prev:              -1,
			next:              -1,
			first:             -1,
			last:              -1,
			destPageObject:    destPageObj,
			destRaw:           destRaw,
			actionRaw:         actionRaw,
		}
		if e.Level > 0 {
			p, ok := lastAtLevel[e.Level-1]
			if !ok {
				return nil, fmt.Errorf("outline[%d]: missing parent for level=%d", i, e.Level)
			}
			n.parent = p
		}
		nodes = append(nodes, n)
		cur := len(nodes) - 1

		prevSibling := -1
		if n.parent >= 0 {
			prevSibling = nodes[n.parent].last
			if nodes[n.parent].first < 0 {
				nodes[n.parent].first = cur
			}
			nodes[n.parent].last = cur
		} else if v, ok := lastAtLevel[0]; ok && v != cur {
			prevSibling = v
		}
		if prevSibling >= 0 {
			nodes[cur].prev = prevSibling
			nodes[prevSibling].next = cur
		}

		lastAtLevel[e.Level] = cur
		for k := range lastAtLevel {
			if k > e.Level {
				delete(lastAtLevel, k)
			}
		}
	}
	return nodes, nil
}

func buildOutlineItemDict(nodes []outlineNode, idx int, outlinesRootObj int) string {
	n := nodes[idx]
	var b strings.Builder
	b.WriteString("<< ")
	b.WriteString("/Title (")
	b.WriteString(escapePDFLiteralForWrite(n.Title))
	b.WriteString(") ")
	if n.parent >= 0 {
		b.WriteString(fmt.Sprintf("/Parent %d 0 R ", nodes[n.parent].objNum))
	} else {
		b.WriteString(fmt.Sprintf("/Parent %d 0 R ", outlinesRootObj))
	}
	if n.actionRaw != "" {
		b.WriteString("/A ")
		b.WriteString(n.actionRaw)
		b.WriteByte(' ')
	} else if n.destRaw != "" {
		b.WriteString("/Dest ")
		b.WriteString(n.destRaw)
		b.WriteByte(' ')
	} else {
		b.WriteString(fmt.Sprintf("/Dest [%d 0 R /XYZ null null null] ", n.destPageObject))
	}
	if n.prev >= 0 {
		b.WriteString(fmt.Sprintf("/Prev %d 0 R ", nodes[n.prev].objNum))
	}
	if n.next >= 0 {
		b.WriteString(fmt.Sprintf("/Next %d 0 R ", nodes[n.next].objNum))
	}
	if n.first >= 0 {
		b.WriteString(fmt.Sprintf("/First %d 0 R /Last %d 0 R /Count %d ", nodes[n.first].objNum, nodes[n.last].objNum, countOpenNodes(nodes, idx)))
	}
	b.WriteString(">>")
	return b.String()
}

func countOpenNodes(nodes []outlineNode, parentIdx int) int {
	count := 0
	for i := range nodes {
		if nodes[i].parent == parentIdx {
			count++
			count += countOpenNodes(nodes, i)
		}
	}
	return count
}
