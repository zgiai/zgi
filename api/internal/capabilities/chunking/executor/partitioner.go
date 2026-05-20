package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/zgiai/ginext/internal/contracts"
)

type PartitionKind string

const (
	PartitionKindDocument PartitionKind = "document"
	PartitionKindPage     PartitionKind = "page"
	PartitionKindSection  PartitionKind = "section"
	PartitionKindTable    PartitionKind = "table"
)

type Partition struct {
	Index       int
	ID          string
	Kind        PartitionKind
	Page        int
	StartOrder  int
	EndOrder    int
	Elements    []contracts.ChunkSourceElement
	Metadata    map[string]any
	Description string
}

type Partitioner interface {
	Partition(doc *contracts.ChunkSourceDocument, plan *contracts.ChunkPlan, limits Limits) ([]Partition, error)
}

type DefaultPartitioner struct{}

func NewDefaultPartitioner() *DefaultPartitioner {
	return &DefaultPartitioner{}
}

func (p *DefaultPartitioner) Partition(doc *contracts.ChunkSourceDocument, plan *contracts.ChunkPlan, limits Limits) ([]Partition, error) {
	if doc == nil {
		return nil, fmt.Errorf("chunk source document is nil")
	}
	limits = normalizeLimits(limits)
	elements := sortedElements(doc.Elements)
	if len(elements) == 0 {
		return nil, nil
	}
	if plan == nil {
		return partitionByDocument(elements, limits), nil
	}

	switch strings.TrimSpace(strings.ToLower(plan.Segmentation)) {
	case "section_aware":
		return partitionBySection(elements, limits), nil
	case "page_layout_aware":
		return partitionByPage(elements, limits), nil
	case "table_aware":
		return partitionTableAware(elements, limits), nil
	default:
		switch strings.TrimSpace(strings.ToLower(plan.ParentMode)) {
		case "page_aware", "visual_preview", "page_context":
			return partitionByPage(elements, limits), nil
		case "section":
			return partitionBySection(elements, limits), nil
		case "table_first", "table_context":
			return partitionTableAware(elements, limits), nil
		default:
			return partitionByDocument(elements, limits), nil
		}
	}
}

func sortedElements(elements []contracts.ChunkSourceElement) []contracts.ChunkSourceElement {
	out := append([]contracts.ChunkSourceElement(nil), elements...)
	sort.SliceStable(out, func(i, j int) bool {
		return elementOrder(out[i], i) < elementOrder(out[j], j)
	})
	return out
}

func partitionByDocument(elements []contracts.ChunkSourceElement, limits Limits) []Partition {
	return reindexPartitions(splitLargePartition(Partition{
		ID:          "document-0",
		Kind:        PartitionKindDocument,
		Page:        firstPage(elements),
		Elements:    elements,
		Description: "document fallback partition",
	}, limits))
}

func partitionByPage(elements []contracts.ChunkSourceElement, limits Limits) []Partition {
	grouped := make([]Partition, 0)
	var current []contracts.ChunkSourceElement
	currentPage := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		partitionIndex := len(grouped)
		grouped = append(grouped, splitLargePartition(Partition{
			ID:          fmt.Sprintf("page-%d-%d", normalizePage(currentPage), partitionIndex),
			Kind:        PartitionKindPage,
			Page:        currentPage,
			Elements:    append([]contracts.ChunkSourceElement(nil), current...),
			Description: "page partition",
		}, limits)...)
	}
	for _, element := range elements {
		page := normalizePage(element.Page)
		if len(current) > 0 && page != currentPage {
			flush()
			current = current[:0]
		}
		currentPage = page
		current = append(current, element)
	}
	flush()
	return reindexPartitions(grouped)
}

func partitionBySection(elements []contracts.ChunkSourceElement, limits Limits) []Partition {
	partitions := make([]Partition, 0)
	var current []contracts.ChunkSourceElement
	section := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		partitions = append(partitions, splitLargePartition(Partition{
			ID:          fmt.Sprintf("section-%d", section),
			Kind:        PartitionKindSection,
			Page:        firstPage(current),
			Elements:    append([]contracts.ChunkSourceElement(nil), current...),
			Description: "section partition",
		}, limits)...)
	}
	for _, element := range elements {
		if len(current) > 0 && isHeading(element) {
			flush()
			current = current[:0]
			section++
		}
		current = append(current, element)
	}
	flush()
	return reindexPartitions(partitions)
}

func partitionTableAware(elements []contracts.ChunkSourceElement, limits Limits) []Partition {
	partitions := make([]Partition, 0)
	var textBuffer []contracts.ChunkSourceElement
	tableIndex := 0
	flushText := func() {
		if len(textBuffer) == 0 {
			return
		}
		partitions = append(partitions, splitLargePartition(Partition{
			ID:          fmt.Sprintf("text-%d", len(partitions)),
			Kind:        PartitionKindDocument,
			Page:        firstPage(textBuffer),
			Elements:    append([]contracts.ChunkSourceElement(nil), textBuffer...),
			Description: "non-table partition",
		}, limits)...)
		textBuffer = textBuffer[:0]
	}
	for _, element := range elements {
		if normalizedElementType(element.Type) == "table" {
			flushText()
			partitions = append(partitions, Partition{
				ID:          fmt.Sprintf("table-%d", tableIndex),
				Kind:        PartitionKindTable,
				Page:        normalizePage(element.Page),
				Elements:    []contracts.ChunkSourceElement{element},
				Description: "table partition",
			})
			tableIndex++
			continue
		}
		textBuffer = append(textBuffer, element)
	}
	flushText()
	return reindexPartitions(partitions)
}

func splitLargePartition(partition Partition, limits Limits) []Partition {
	limits = normalizeLimits(limits)
	if len(partition.Elements) <= limits.MaxPartitionSize {
		setPartitionBounds(&partition)
		return []Partition{partition}
	}
	out := make([]Partition, 0, (len(partition.Elements)+limits.MaxPartitionSize-1)/limits.MaxPartitionSize)
	for start := 0; start < len(partition.Elements); start += limits.MaxPartitionSize {
		end := start + limits.MaxPartitionSize
		if end > len(partition.Elements) {
			end = len(partition.Elements)
		}
		child := partition
		child.ID = fmt.Sprintf("%s-%d", partition.ID, len(out))
		child.Elements = append([]contracts.ChunkSourceElement(nil), partition.Elements[start:end]...)
		setPartitionBounds(&child)
		out = append(out, child)
	}
	return out
}

func reindexPartitions(partitions []Partition) []Partition {
	for i := range partitions {
		partitions[i].Index = i
		if partitions[i].ID == "" {
			partitions[i].ID = fmt.Sprintf("partition-%d", i)
		}
		setPartitionBounds(&partitions[i])
	}
	return partitions
}

func setPartitionBounds(partition *Partition) {
	if partition == nil || len(partition.Elements) == 0 {
		return
	}
	partition.Page = normalizePage(partition.Page)
	if partition.Page == 0 {
		partition.Page = firstPage(partition.Elements)
	}
	partition.StartOrder = elementOrder(partition.Elements[0], 0)
	partition.EndOrder = elementOrder(partition.Elements[len(partition.Elements)-1], len(partition.Elements)-1)
}

func firstPage(elements []contracts.ChunkSourceElement) int {
	for _, element := range elements {
		if element.Page > 0 {
			return element.Page
		}
	}
	return 0
}

func normalizePage(page int) int {
	if page < 0 {
		return 0
	}
	return page
}

func isHeading(element contracts.ChunkSourceElement) bool {
	switch normalizedElementType(element.Type) {
	case "heading", "title":
		return true
	default:
		return false
	}
}

func normalizedElementType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func elementOrder(element contracts.ChunkSourceElement, fallback int) int {
	if element.Ordinal > 0 {
		return element.Ordinal
	}
	return fallback + 1
}
