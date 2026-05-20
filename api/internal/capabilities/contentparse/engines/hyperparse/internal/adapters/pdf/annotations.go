package pdf

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type AnnotationEntry struct {
	PageIndex    int    `json:"page_index"`
	ObjectNumber int    `json:"object_number"`
	Subtype      string `json:"subtype,omitempty"`
	Rect         string `json:"rect,omitempty"`
	Contents     string `json:"contents,omitempty"`
}

var annotsArrayRE = regexp.MustCompile(`(?s)/Annots\s*\[(.*?)\]`)
var annotsRefRE = regexp.MustCompile(`/Annots\s+(\d+)\s+0\s+R`)
var annotSubtypeRE = regexp.MustCompile(`/Subtype\s*/([^\s<>\[\]\(\)/]+)`)
var annotRectRE = regexp.MustCompile(`(?s)/Rect\s*\[([^\]]+)\]`)
var annotContentsRE = regexp.MustCompile(`(?s)/Contents\s*\((.*?)\)`)

func ExtractAnnotationEntriesFromBytes(data []byte, mode string) ([]AnnotationEntry, error) {
	pageInfos, err := DetectPageInfosBytes(data, mode)
	if err != nil {
		return nil, err
	}
	return ExtractAnnotationEntriesFromBytesWithPageInfos(data, mode, pageInfos)
}

// ExtractAnnotationEntriesFromBytesWithPageInfos reuses parsed page-tree data.
func ExtractAnnotationEntriesFromBytesWithPageInfos(data []byte, mode string, pageInfos []PageInfo) ([]AnnotationEntry, error) {
	t0 := time.Now()
	fullDocDebugf("ExtractAnnotationEntriesFromBytesWithPageInfos begin pages=%d", len(pageInfos))
	var rows []AnnotationEntry
	defer func() {
		fullDocDebugf("ExtractAnnotationEntriesFromBytesWithPageInfos done annotations=%d elapsed_ms=%d", len(rows), time.Since(t0).Milliseconds())
	}()
	rows = make([]AnnotationEntry, 0)
	for i := range pageInfos {
		pageIndex := i + 1
		pageBlock, err := ExtractObjectBlockByNumberBytes(data, pageInfos[i].ObjectNumber, mode)
		if err != nil {
			continue
		}
		annotRefs := extractAnnotRefsFromPageBlock(data, pageBlock, mode)
		for _, objNum := range annotRefs {
			ab, err := ExtractObjectBlockByNumberBytes(data, objNum, mode)
			if err != nil {
				continue
			}
			row := AnnotationEntry{PageIndex: pageIndex, ObjectNumber: objNum}
			if m := annotSubtypeRE.FindStringSubmatch(ab); len(m) >= 2 {
				row.Subtype = m[1]
			}
			if m := annotRectRE.FindStringSubmatch(ab); len(m) >= 2 {
				row.Rect = strings.TrimSpace(m[1])
			}
			if m := annotContentsRE.FindStringSubmatch(ab); len(m) >= 2 {
				row.Contents = m[1]
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func extractAnnotRefsFromPageBlock(data []byte, pageBlock, mode string) []int {
	if m := annotsArrayRE.FindStringSubmatch(pageBlock); len(m) >= 2 {
		return parseAnnotRefsFromAnnotsArrayBody(m[1])
	}
	m := annotsRefRE.FindStringSubmatch(pageBlock)
	if len(m) < 2 {
		return nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n <= 0 {
		return nil
	}
	annotArrBlock, err := ExtractObjectBlockByNumberBytes(data, n, mode)
	if err != nil {
		return nil
	}
	return parseAnnotRefsFromAnnotsArrayBody(annotArrBlock)
}

func parseAnnotRefsFromAnnotsArrayBody(body string) []int {
	matches := pdfAnyIndirectRefRE.FindAllStringSubmatch(body, -1)
	out := make([]int, 0, len(matches))
	for _, m := range matches {
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
