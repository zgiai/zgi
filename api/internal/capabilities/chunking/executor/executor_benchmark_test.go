package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	datasetadapter "github.com/zgiai/ginext/internal/capabilities/chunking/adapters/dataset"
	"github.com/zgiai/ginext/internal/contracts"
)

func BenchmarkExecutorThroughput(b *testing.B) {
	for _, size := range []int{1000, 10000} {
		for _, workers := range []int{1, 2, 4, 8} {
			for _, partitionSize := range []int{32, 64, 128} {
				name := fmt.Sprintf("elements=%d/workers=%d/partition=%d", size, workers, partitionSize)
				b.Run(name, func(b *testing.B) {
					doc := benchmarkChunkSourceDocument(size)
					plan := &contracts.ChunkPlan{
						UseCase:      contracts.ChunkUseCaseDatasetIndex,
						Segmentation: "page_layout_aware",
					}
					exec := New(WithLimits(Limits{
						MaxWorkers:       workers,
						MaxPartitionSize: partitionSize,
					}))

					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						result, err := exec.Execute(context.Background(), doc, plan)
						if err != nil {
							b.Fatal(err)
						}
						if len(result.Units) == 0 {
							b.Fatal("expected chunk units")
						}
					}
				})
			}
		}
	}
}

func BenchmarkExecutorLargeDocument100MB(b *testing.B) {
	doc := benchmarkChunkSourceDocumentBytes(100 * 1024 * 1024)
	plan := &contracts.ChunkPlan{
		UseCase:      contracts.ChunkUseCaseDatasetIndex,
		Segmentation: "page_layout_aware",
	}

	for _, workers := range []int{1, 4, 8} {
		b.Run(fmt.Sprintf("execute/workers=%d/partition=64", workers), func(b *testing.B) {
			exec := New(WithLimits(Limits{MaxWorkers: workers, MaxPartitionSize: 64}))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result, err := exec.Execute(context.Background(), doc, plan)
				if err != nil {
					b.Fatal(err)
				}
				if len(result.Units) == 0 {
					b.Fatal("expected chunk units")
				}
			}
		})
	}

	b.Run("execute_and_dataset_adapter/workers=8/partition=64", func(b *testing.B) {
		exec := New(WithLimits(Limits{MaxWorkers: 8, MaxPartitionSize: 64}))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result, err := exec.Execute(context.Background(), doc, plan)
			if err != nil {
				b.Fatal(err)
			}
			chunks := datasetadapter.UnitsToTransformedChunksWithOptions(result.Units, datasetadapter.AdapterOptions{
				BuildChildren:     true,
				SubchunkMaxTokens: 1000,
				SubchunkOverlap:   50,
				SubchunkSeparator: "\n\n",
			})
			if len(chunks) == 0 {
				b.Fatal("expected transformed chunks")
			}
		}
	})
}

func BenchmarkExecutorExtractedPDFText(b *testing.B) {
	textDir := os.Getenv("PDF_BENCH_TEXT_DIR")
	if textDir == "" {
		b.Skip("set PDF_BENCH_TEXT_DIR to a directory containing extracted .txt files")
	}
	entries, err := os.ReadDir(textDir)
	if err != nil {
		b.Fatal(err)
	}

	plan := &contracts.ChunkPlan{
		UseCase:      contracts.ChunkUseCaseDatasetIndex,
		Segmentation: "page_layout_aware",
	}
	exec := New(WithLimits(Limits{MaxWorkers: 8, MaxPartitionSize: 64}))

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".txt" {
			continue
		}
		path := filepath.Join(textDir, entry.Name())
		doc := benchmarkChunkSourceDocumentFromTextFile(b, path)
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))

		b.Run(name+"/execute", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result, err := exec.Execute(context.Background(), doc, plan)
				if err != nil {
					b.Fatal(err)
				}
				if len(result.Units) == 0 {
					b.Fatal("expected chunk units")
				}
			}
		})
		b.Run(name+"/execute_and_dataset_adapter", func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result, err := exec.Execute(context.Background(), doc, plan)
				if err != nil {
					b.Fatal(err)
				}
				chunks := datasetadapter.UnitsToTransformedChunksWithOptions(result.Units, datasetadapter.AdapterOptions{
					BuildChildren:     true,
					SubchunkMaxTokens: 1000,
					SubchunkOverlap:   50,
					SubchunkSeparator: "\n\n",
				})
				if len(chunks) == 0 {
					b.Fatal("expected transformed chunks")
				}
			}
		})
	}
}

func benchmarkChunkSourceDocument(size int) *contracts.ChunkSourceDocument {
	elements := make([]contracts.ChunkSourceElement, 0, size)
	for i := 0; i < size; i++ {
		page := i/40 + 1
		content := fmt.Sprintf("This is useful paragraph %d with enough business content to avoid low value filtering.", i)
		if i%37 == 0 {
			content = fmt.Sprintf("Page %d of 250", page)
		}
		if i%53 == 0 {
			content = strings.Repeat("A", 16)
		}
		elements = append(elements, contracts.ChunkSourceElement{
			ElementID: fmt.Sprintf("element-%d", i),
			Type:      "text",
			Page:      page,
			Content:   content,
			Markdown:  content,
			Ordinal:   i + 1,
			BBox: &contracts.ParseBoundingBox{
				Left:   0.05,
				Top:    float64(i%40) * 0.02,
				Right:  0.95,
				Bottom: float64(i%40)*0.02 + 0.015,
			},
		})
	}
	return &contracts.ChunkSourceDocument{
		DocumentID: "benchmark-doc",
		DatasetID:  "benchmark-dataset",
		FileID:     "benchmark-file",
		Elements:   elements,
	}
}

func benchmarkChunkSourceDocumentFromTextFile(b *testing.B, path string) *contracts.ChunkSourceDocument {
	b.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}

	const targetElementBytes = 1024
	lines := strings.Split(string(raw), "\n")
	elements := make([]contracts.ChunkSourceElement, 0, len(raw)/targetElementBytes+1)
	var builder strings.Builder
	ordinal := 1
	flush := func() {
		content := strings.TrimSpace(builder.String())
		builder.Reset()
		if content == "" {
			return
		}
		elements = append(elements, contracts.ChunkSourceElement{
			ElementID: fmt.Sprintf("%s-element-%d", filepath.Base(path), ordinal),
			Type:      "text",
			Page:      ordinal/80 + 1,
			Content:   content,
			Markdown:  content,
			Ordinal:   ordinal,
		})
		ordinal++
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			flush()
			continue
		}
		if builder.Len()+len(line)+1 > targetElementBytes {
			flush()
		}
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(line)
	}
	flush()

	if len(elements) == 0 {
		b.Fatalf("no benchmark elements extracted from %s", path)
	}
	return &contracts.ChunkSourceDocument{
		DocumentID: filepath.Base(path),
		DatasetID:  "benchmark-dataset",
		FileID:     filepath.Base(path),
		Elements:   elements,
	}
}

func benchmarkChunkSourceDocumentBytes(totalBytes int) *contracts.ChunkSourceDocument {
	const elementBytes = 1024
	count := totalBytes / elementBytes
	if count < 1 {
		count = 1
	}
	content := strings.Repeat("x", elementBytes)
	elements := make([]contracts.ChunkSourceElement, 0, count)
	for i := 0; i < count; i++ {
		elements = append(elements, contracts.ChunkSourceElement{
			ElementID: fmt.Sprintf("large-element-%d", i),
			Type:      "text",
			Page:      i/80 + 1,
			Content:   content,
			Ordinal:   i + 1,
		})
	}
	return &contracts.ChunkSourceDocument{
		DocumentID: "benchmark-100mb-doc",
		DatasetID:  "benchmark-dataset",
		FileID:     "benchmark-file",
		Elements:   elements,
	}
}
