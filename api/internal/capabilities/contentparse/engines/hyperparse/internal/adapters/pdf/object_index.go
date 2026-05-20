package pdf

import (
	"bytes"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

// objectIndexKey distinguishes PDF byte buffers by backing pointer and length.
type objectIndexKey struct {
	ptr uintptr
	len int
}

var pdfObjectIndexRegistry sync.Map // objectIndexKey -> *pdfObjectIndex

// pdfObjectIndex maps each direct object number to its object-header offset.
type pdfObjectIndex struct {
	byNum       map[int]int
	objStmNums  []int
	objStmBuilt bool
	mu          sync.Mutex
}

// RegisterObjectIndexForParse enables an object index for one parse of the same data.
// Long-running paths should call it to reduce ObjStm fallback from 1..upper to existing objects.
func RegisterObjectIndexForParse(data []byte) (unregister func()) {
	if len(data) == 0 {
		return func() {}
	}
	ptr := unsafe.SliceData(data)
	if ptr == nil {
		return func() {}
	}
	key := objectIndexKey{ptr: uintptr(unsafe.Pointer(ptr)), len: len(data)}
	idx := &pdfObjectIndex{byNum: buildPDFObjectIndexSinglePass(data)}
	pdfObjectIndexRegistry.Store(key, idx)
	return func() {
		pdfObjectIndexRegistry.Delete(key)
	}
}

func lookupPDFObjectIndex(data []byte) *pdfObjectIndex {
	if len(data) == 0 {
		return nil
	}
	ptr := unsafe.SliceData(data)
	if ptr == nil {
		return nil
	}
	key := objectIndexKey{ptr: uintptr(unsafe.Pointer(ptr)), len: len(data)}
	v, ok := pdfObjectIndexRegistry.Load(key)
	if !ok {
		return nil
	}
	return v.(*pdfObjectIndex)
}

// IndexedDirectObjectNumbers returns direct object numbers known to exist, sorted ascending.
// It replaces full 1..upper enumeration and avoids repeated lookups for missing objects.
func IndexedDirectObjectNumbers(data []byte) []int {
	idx := lookupPDFObjectIndex(data)
	if idx == nil || len(idx.byNum) == 0 {
		return nil
	}
	idx.mu.Lock()
	nums := make([]int, 0, len(idx.byNum))
	for n := range idx.byNum {
		nums = append(nums, n)
	}
	idx.mu.Unlock()
	sort.Ints(nums)
	return nums
}

// buildPDFObjectIndexSinglePass scans "N G obj" headers once and validates
// offsets against LinearOffset. After confirming an object, it skips to endobj
// to avoid stream false positives.
func buildPDFObjectIndexSinglePass(data []byte) map[int]int {
	m := make(map[int]int)
	if len(data) == 0 {
		return m
	}
	cursor := 0
	for cursor < len(data) {
		rel := bytes.Index(data[cursor:], []byte(" obj"))
		if rel < 0 {
			break
		}
		pos := cursor + rel
		lineStart := lastPDFLineBreakIndex(data[:pos])
		if lineStart < 0 {
			lineStart = 0
		} else {
			lineStart++
		}
		lineEnd := pos + len(" obj")
		if lineEnd > len(data) {
			cursor = pos + 1
			continue
		}
		line := data[lineStart:lineEnd]
		fields := strings.Fields(string(line))
		if len(fields) < 3 || fields[len(fields)-1] != "obj" {
			cursor = pos + 1
			continue
		}
		objNum, err1 := strconv.Atoi(fields[0])
		genNum, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil || objNum <= 0 {
			cursor = pos + 1
			continue
		}
		_ = genNum
		if lineStart > 0 {
			prev := data[lineStart-1]
			if prev >= '0' && prev <= '9' {
				cursor = pos + 1
				continue
			}
		}
		needle := []byte(fields[0] + " ")
		relN := bytes.Index(data[lineStart:lineEnd], needle)
		if relN < 0 {
			cursor = pos + 1
			continue
		}
		objStart := lineStart + relN
		if objStart > 0 {
			prev := data[objStart-1]
			if prev >= '0' && prev <= '9' {
				cursor = pos + 1
				continue
			}
		}
		blk2, s2, ok2 := findDirectObjectBlockByNumberLinearOffset(data, objNum)
		if !ok2 || s2 != objStart {
			cursor = pos + 1
			continue
		}
		if _, exists := m[objNum]; !exists {
			m[objNum] = s2
		}
		cursor = s2 + len(blk2)
	}
	return m
}

func lastPDFLineBreakIndex(b []byte) int {
	lf := bytes.LastIndexByte(b, '\n')
	cr := bytes.LastIndexByte(b, '\r')
	if cr > lf {
		return cr
	}
	return lf
}

func readObjectBlockAtOffset(data []byte, start int) ([]byte, bool) {
	if start < 0 || start >= len(data) {
		return nil, false
	}
	endRel := bytes.Index(data[start:], []byte("endobj"))
	if endRel < 0 {
		return nil, false
	}
	end := start + endRel + len("endobj")
	return data[start:end], true
}

func (idx *pdfObjectIndex) blockByNumber(data []byte, objNum int) ([]byte, bool) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.blockByNumberLocked(data, objNum)
}

// blockByNumberLocked is called while holding idx.mu, or after locking for one lookup.
func (idx *pdfObjectIndex) blockByNumberLocked(data []byte, objNum int) ([]byte, bool) {
	if idx.byNum == nil {
		idx.byNum = make(map[int]int)
	}
	if off, ok := idx.byNum[objNum]; ok {
		return readObjectBlockAtOffset(data, off)
	}
	blk, off, ok := findDirectObjectBlockByNumberLinearOffset(data, objNum)
	if !ok {
		return nil, false
	}
	idx.byNum[objNum] = off
	return blk, true
}

func (idx *pdfObjectIndex) objStmObjectNumbers(data []byte) []int {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.objStmBuilt {
		return idx.objStmNums
	}
	var nums []int
	for n := range idx.byNum {
		blk, ok := idx.blockByNumberLocked(data, n)
		if !ok || pdfTypeMarkerIndex(blk, "ObjStm") < 0 {
			continue
		}
		nums = append(nums, n)
	}
	sort.Ints(nums)
	idx.objStmNums = nums
	idx.objStmBuilt = true
	return idx.objStmNums
}
