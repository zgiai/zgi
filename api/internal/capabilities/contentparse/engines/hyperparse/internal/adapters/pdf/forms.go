package pdf

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type FormFieldEntry struct {
	ObjectNumber int    `json:"object_number"`
	Name         string `json:"name,omitempty"`
	AltName      string `json:"alt_name,omitempty"`
	FieldType    string `json:"field_type,omitempty"`
	Value        string `json:"value,omitempty"`
	Flags        int    `json:"flags,omitempty"`
	PageObject   int    `json:"page_object,omitempty"`
	Rect         string `json:"rect,omitempty"`
}

var fieldsArrayRE = regexp.MustCompile(`(?s)/Fields\s*\[(.*?)\]`)
var rectRE = regexp.MustCompile(`(?s)/Rect\s*\[([^\]]+)\]`)

func ExtractFormFieldEntriesFromBytes(data []byte, mode string) ([]FormFieldEntry, error) {
	t0 := time.Now()
	fullDocDebugf("ExtractFormFieldEntriesFromBytes begin")
	defer func() {
		fullDocDebugf("ExtractFormFieldEntriesFromBytes done elapsed_ms=%d", time.Since(t0).Milliseconds())
	}()
	rootObj, ok := detectRootObjectNumber(data)
	if !ok {
		if mode == ValidationModeRelaxed {
			return []FormFieldEntry{}, nil
		}
		return nil, fmt.Errorf("cannot detect root object")
	}
	cat, ok := parseCatalogObject(data, rootObj)
	if !ok {
		if mode == ValidationModeRelaxed {
			return []FormFieldEntry{}, nil
		}
		return nil, fmt.Errorf("invalid catalog")
	}
	catBlock, ok := findObjectBlockByNumber(data, cat.objNumber)
	if !ok {
		return []FormFieldEntry{}, nil
	}
	acroFormObj, ok := parseIndirectRefObjectNumberByKey(catBlock, "/AcroForm")
	if !ok || acroFormObj <= 0 {
		return []FormFieldEntry{}, nil
	}
	acroBlock, ok := findObjectBlockByNumber(data, acroFormObj)
	if !ok {
		return []FormFieldEntry{}, nil
	}
	fieldRoots := parseFieldRefArray(acroBlock)
	if len(fieldRoots) == 0 {
		return []FormFieldEntry{}, nil
	}
	out := make([]FormFieldEntry, 0, len(fieldRoots))
	seen := map[int]bool{}
	for _, n := range fieldRoots {
		walkFieldTree(data, n, mode, seen, &out)
	}
	return out, nil
}

func walkFieldTree(data []byte, objNum int, mode string, seen map[int]bool, out *[]FormFieldEntry) {
	if objNum <= 0 || seen[objNum] {
		return
	}
	seen[objNum] = true
	block, ok := findObjectBlockByNumber(data, objNum)
	if !ok {
		return
	}
	ft := strings.TrimPrefix(strings.TrimSpace(extractNameByKey(block, "/FT")), "/")
	if ft != "" {
		entry := FormFieldEntry{
			ObjectNumber: objNum,
			Name:         extractPDFStringByKey(block, "T"),
			AltName:      extractPDFStringByKey(block, "TU"),
			FieldType:    ft,
			Value:        extractValueAsString(block),
			Flags:        parseIntByKey(block, "/Ff"),
		}
		if p, ok := parseIndirectRefObjectNumberByKey(block, "/P"); ok && p > 0 {
			entry.PageObject = p
		}
		if m := rectRE.FindSubmatch(block); len(m) >= 2 {
			entry.Rect = strings.TrimSpace(string(m[1]))
		}
		*out = append(*out, entry)
	}
	for _, kid := range parseKidsRefs(block) {
		walkFieldTree(data, kid, mode, seen, out)
	}
}

func parseFieldRefArray(acroBlock []byte) []int {
	m := fieldsArrayRE.FindSubmatch(acroBlock)
	if len(m) < 2 {
		return nil
	}
	return parseIndirectRefs(string(m[1]))
}

func parseKidsRefs(objBlock []byte) []int {
	pos := strings.Index(string(objBlock), "/Kids")
	if pos < 0 {
		return nil
	}
	rest := strings.TrimSpace(string(objBlock[pos+len("/Kids"):]))
	if !strings.HasPrefix(rest, "[") {
		return nil
	}
	end := strings.Index(rest, "]")
	if end < 0 {
		return nil
	}
	return parseIndirectRefs(rest[1:end])
}

func parseIndirectRefs(s string) []int {
	ms := pdfAnyIndirectRefRE.FindAllStringSubmatch(s, -1)
	out := make([]int, 0, len(ms))
	for _, m := range ms {
		if len(m) < 2 {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n <= 0 {
			continue
		}
		out = append(out, n)
	}
	return out
}

func extractNameByKey(block []byte, key string) string {
	pos := strings.Index(string(block), key)
	if pos < 0 {
		return ""
	}
	rest := strings.TrimSpace(string(block[pos+len(key):]))
	if !strings.HasPrefix(rest, "/") {
		return ""
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func parseIntByKey(block []byte, key string) int {
	pos := strings.Index(string(block), key)
	if pos < 0 {
		return 0
	}
	fields := strings.Fields(string(block[pos+len(key):]))
	if len(fields) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(fields[0])
	return n
}

func extractValueAsString(block []byte) string {
	if v := extractPDFStringByKey(block, "V"); strings.TrimSpace(v) != "" {
		return v
	}
	pos := strings.Index(string(block), "/V")
	if pos < 0 {
		return ""
	}
	rest := strings.TrimSpace(string(block[pos+len("/V"):]))
	if strings.HasPrefix(rest, "/") {
		fields := strings.Fields(rest)
		if len(fields) > 0 {
			return strings.TrimPrefix(fields[0], "/")
		}
	}
	return ""
}
