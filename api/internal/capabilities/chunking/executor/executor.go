package executor

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/zgiai/ginext/internal/capabilities/chunking/quality"
	"github.com/zgiai/ginext/internal/contracts"
)

type Executor struct {
	partitioner Partitioner
	filter      *quality.UnitFilter
	limits      Limits
}

type Option func(*Executor)

func WithPartitioner(partitioner Partitioner) Option {
	return func(executor *Executor) {
		if partitioner != nil {
			executor.partitioner = partitioner
		}
	}
}

func WithLimits(limits Limits) Option {
	return func(executor *Executor) {
		executor.limits = normalizeLimits(limits)
	}
}

func WithQualityFilter(filter *quality.UnitFilter) Option {
	return func(executor *Executor) {
		executor.filter = filter
	}
}

func New(options ...Option) *Executor {
	executor := &Executor{
		partitioner: NewDefaultPartitioner(),
		filter:      quality.NewUnitFilter(),
		limits:      DefaultLimits(),
	}
	for _, option := range options {
		if option != nil {
			option(executor)
		}
	}
	executor.limits = normalizeLimits(executor.limits)
	return executor
}

type Result struct {
	Units      []contracts.ChunkUnit `json:"units"`
	Partitions []Partition           `json:"partitions"`
	Metrics    Metrics               `json:"metrics"`
}

type Metrics struct {
	PartitionCount             int            `json:"partition_count"`
	WorkerCount                int            `json:"worker_count"`
	UnitCount                  int            `json:"unit_count"`
	FilteredUnitCount          int            `json:"filtered_unit_count"`
	SourceElementFilteredCount int            `json:"source_element_filtered_count"`
	PartitionKindCount         map[string]int `json:"partition_kind_count,omitempty"`
	FilterReasons              map[string]int `json:"filter_reasons,omitempty"`
	SourceElementFilterReasons map[string]int `json:"source_element_filter_reasons,omitempty"`
	StableOrder                bool           `json:"stable_order"`
}

func (e *Executor) Execute(ctx context.Context, doc *contracts.ChunkSourceDocument, plan *contracts.ChunkPlan) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("chunk executor is nil")
	}
	if doc == nil {
		return nil, fmt.Errorf("chunk source document is nil")
	}

	limits := normalizeLimits(e.limits)
	partitions, err := e.partitioner.Partition(doc, plan, limits)
	if err != nil {
		return nil, err
	}
	if len(partitions) == 0 {
		return &Result{Metrics: Metrics{StableOrder: true}}, nil
	}

	workerCount := limits.MaxWorkers
	if workerCount > len(partitions) {
		workerCount = len(partitions)
	}
	if workerCount < 1 {
		workerCount = 1
	}

	jobs := make(chan Partition)
	results := make(chan partitionResult, len(partitions))
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for partition := range jobs {
				if ctx.Err() != nil {
					select {
					case errs <- ctx.Err():
					default:
					}
					return
				}
				units, sourceFilterMetrics := e.buildPartitionUnits(doc, partition)
				results <- partitionResult{Partition: partition, Units: units, SourceFilterMetrics: sourceFilterMetrics}
			}
		}()
	}

	for _, partition := range partitions {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return nil, ctx.Err()
		case jobs <- partition:
		}
	}
	close(jobs)
	wg.Wait()
	close(results)

	select {
	case err := <-errs:
		return nil, err
	default:
	}

	partitionResults := make([]partitionResult, 0, len(partitions))
	for result := range results {
		partitionResults = append(partitionResults, result)
	}
	stableOrder := isStableMergedOrder(partitionResults)
	units := stableMerge(partitionResults)
	sourceFilterMetrics := mergeSourceFilterMetrics(partitionResults)
	filterMetrics := quality.FilterMetrics{}
	if e.filter != nil {
		units, filterMetrics = e.filter.FilterUnits(units)
	}

	return &Result{
		Units:      units,
		Partitions: partitions,
		Metrics: Metrics{
			PartitionCount:             len(partitions),
			WorkerCount:                workerCount,
			UnitCount:                  len(units),
			FilteredUnitCount:          filterMetrics.RemovedCount,
			SourceElementFilteredCount: sourceFilterMetrics.RemovedCount,
			PartitionKindCount:         countPartitionKinds(partitions),
			FilterReasons:              filterMetrics.Reasons,
			SourceElementFilterReasons: sourceFilterMetrics.Reasons,
			StableOrder:                stableOrder && isStableOrder(units),
		},
	}, nil
}

func (e *Executor) buildPartitionUnits(doc *contracts.ChunkSourceDocument, partition Partition) ([]contracts.ChunkUnit, quality.FilterMetrics) {
	elements := partition.Elements
	sourceFilterMetrics := quality.FilterMetrics{}
	if e != nil && e.filter != nil {
		elements, sourceFilterMetrics = e.filter.FilterElements(elements)
	}

	content, markdown := partitionText(elements)
	if strings.TrimSpace(content) == "" && strings.TrimSpace(markdown) == "" {
		return nil, sourceFilterMetrics
	}

	unit := contracts.ChunkUnit{
		ChunkID:    chunkID(doc, partition, 0),
		DocumentID: doc.DocumentID,
		DatasetID:  doc.DatasetID,
		FileID:     doc.FileID,
		Kind:       chunkKindForPartition(partition),
		Content:    strings.TrimSpace(content),
		Markdown:   strings.TrimSpace(markdown),
		Pages:      partitionPages(elements),
		Order:      partition.StartOrder,
		BBox:       unionBBox(elements),
		Metadata: map[string]any{
			"partition_id":       partition.ID,
			"partition_kind":     string(partition.Kind),
			"source_element_ids": sourceElementIDs(elements),
			"source_start_order": partition.StartOrder,
			"source_end_order":   partition.EndOrder,
		},
	}
	if sourceFilterMetrics.RemovedCount > 0 {
		unit.Metadata["source_filtered_count"] = sourceFilterMetrics.RemovedCount
		unit.Metadata["source_filter_reasons"] = sourceFilterMetrics.Reasons
	}
	return []contracts.ChunkUnit{unit}, sourceFilterMetrics
}

func mergeSourceFilterMetrics(results []partitionResult) quality.FilterMetrics {
	merged := quality.FilterMetrics{Reasons: map[string]int{}}
	for _, result := range results {
		metrics := result.SourceFilterMetrics
		merged.InputCount += metrics.InputCount
		merged.OutputCount += metrics.OutputCount
		merged.RemovedCount += metrics.RemovedCount
		for reason, count := range metrics.Reasons {
			merged.Reasons[reason] += count
		}
	}
	return merged
}

func partitionText(elements []contracts.ChunkSourceElement) (string, string) {
	content := make([]string, 0, len(elements))
	markdown := make([]string, 0, len(elements))
	for _, element := range elements {
		if text := strings.TrimSpace(element.Content); text != "" {
			content = append(content, text)
		}
		if text := strings.TrimSpace(element.Markdown); text != "" {
			markdown = append(markdown, text)
		}
	}
	return strings.Join(content, "\n"), strings.Join(markdown, "\n")
}

func chunkID(doc *contracts.ChunkSourceDocument, partition Partition, index int) string {
	docID := strings.TrimSpace(doc.DocumentID)
	if docID == "" {
		docID = "document"
	}
	return fmt.Sprintf("%s:%s:%d", docID, partition.ID, index)
}

func chunkKindForPartition(partition Partition) contracts.ChunkKind {
	switch partition.Kind {
	case PartitionKindTable:
		return contracts.ChunkKindTable
	case PartitionKindSection:
		return contracts.ChunkKindParent
	default:
		if len(partition.Elements) == 1 {
			switch normalizedElementType(partition.Elements[0].Type) {
			case "heading", "title":
				return contracts.ChunkKindHeading
			case "table":
				return contracts.ChunkKindTable
			case "figure", "image":
				return contracts.ChunkKindFigure
			case "formula", "equation":
				return contracts.ChunkKindFormula
			}
		}
		return contracts.ChunkKindText
	}
}

func partitionPages(elements []contracts.ChunkSourceElement) []int {
	seen := make(map[int]struct{})
	pages := make([]int, 0)
	for _, element := range elements {
		page := normalizePage(element.Page)
		if page == 0 {
			continue
		}
		if _, ok := seen[page]; ok {
			continue
		}
		seen[page] = struct{}{}
		pages = append(pages, page)
	}
	return pages
}

func unionBBox(elements []contracts.ChunkSourceElement) *contracts.ParseBoundingBox {
	var out *contracts.ParseBoundingBox
	for _, element := range elements {
		if element.BBox == nil || element.BBox.Right <= element.BBox.Left || element.BBox.Bottom <= element.BBox.Top {
			continue
		}
		if out == nil {
			out = &contracts.ParseBoundingBox{
				Left:   element.BBox.Left,
				Top:    element.BBox.Top,
				Right:  element.BBox.Right,
				Bottom: element.BBox.Bottom,
			}
			continue
		}
		if element.BBox.Left < out.Left {
			out.Left = element.BBox.Left
		}
		if element.BBox.Top < out.Top {
			out.Top = element.BBox.Top
		}
		if element.BBox.Right > out.Right {
			out.Right = element.BBox.Right
		}
		if element.BBox.Bottom > out.Bottom {
			out.Bottom = element.BBox.Bottom
		}
	}
	return out
}

func sourceElementIDs(elements []contracts.ChunkSourceElement) []string {
	ids := make([]string, 0, len(elements))
	for _, element := range elements {
		if strings.TrimSpace(element.ElementID) != "" {
			ids = append(ids, strings.TrimSpace(element.ElementID))
		}
	}
	return ids
}

func countPartitionKinds(partitions []Partition) map[string]int {
	out := make(map[string]int)
	for _, partition := range partitions {
		out[string(partition.Kind)]++
	}
	return out
}
