package inspectsvc

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type progressiveVLMPageJob struct {
	RenderIndex int
	PageNumber  int
	DataURL     string
}

type progressiveVLMPageResult struct {
	RenderIndex     int
	PageNumber      int
	DataURL         string
	Engine          string
	RenderElapsedMs int64
	VLMElapsedMs    int64
	VLMTiming       VLMCallTimingBreakdown
	Attempted       bool
	Model           string
	Items           []map[string]any
	Raw             string
	Err             error
}

func streamProgressiveVLMPageResults(pageDataURLs []string, renderedPages []int, concurrency int) <-chan progressiveVLMPageResult {
	rendered := make(chan renderedPDFPageResult, len(pageDataURLs))
	go func() {
		defer close(rendered)
		for idx, pageDataURL := range pageDataURLs {
			rendered <- renderedPDFPageResult{
				RenderIndex: idx,
				PageNumber:  renderedPages[idx],
				DataURL:     pageDataURL,
			}
		}
	}()
	return streamProgressiveVLMPageResultsFromRenderedPages(rendered, concurrency, nil)
}

func streamProgressiveVLMPageResultsFromRenderedPages(renderedPages <-chan renderedPDFPageResult, concurrency int, oversizedPages map[int]oversizedPagePlan) <-chan progressiveVLMPageResult {
	out := make(chan progressiveVLMPageResult)
	if concurrency <= 0 {
		concurrency = 1
	}
	var wg sync.WaitGroup
	for workerIdx := 0; workerIdx < concurrency; workerIdx++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rendered := range renderedPages {
				if rendered.Err != nil {
					out <- progressiveVLMPageResult{
						RenderIndex:     rendered.RenderIndex,
						PageNumber:      rendered.PageNumber,
						Engine:          rendered.Engine,
						RenderElapsedMs: rendered.ElapsedMs,
						Err:             rendered.Err,
					}
					continue
				}
				vlmStartedAt := time.Now()
				model, items, raw, timing, err := callStructuredVLMFallbackForRenderedPageProfiled(
					rendered.DataURL,
					rendered.PageNumber,
					oversizedPages[rendered.PageNumber],
					CallDashscopeVLMFallbackStructuredProfiled,
				)
				out <- progressiveVLMPageResult{
					RenderIndex:     rendered.RenderIndex,
					PageNumber:      rendered.PageNumber,
					DataURL:         rendered.DataURL,
					Engine:          rendered.Engine,
					RenderElapsedMs: rendered.ElapsedMs,
					VLMElapsedMs:    time.Since(vlmStartedAt).Milliseconds(),
					VLMTiming:       timing,
					Attempted:       true,
					Model:           model,
					Items:           items,
					Raw:             raw,
					Err:             err,
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

func flattenProgressiveVLMPageItems(results map[int]progressiveVLMPageResult) []map[string]any {
	if len(results) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(results))
	for idx := range results {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	out := make([]map[string]any, 0, len(indexes)*2)
	for _, idx := range indexes {
		out = append(out, results[idx].Items...)
	}
	return out
}

func joinProgressiveVLMRawReplies(results map[int]progressiveVLMPageResult) string {
	if len(results) == 0 {
		return ""
	}
	indexes := make([]int, 0, len(results))
	for idx := range results {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	parts := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		raw := strings.TrimSpace(results[idx].Raw)
		if raw != "" {
			parts = append(parts, raw)
		}
	}
	return strings.Join(parts, "\n\n")
}

func rebuildProgressiveRenderedPages(results map[int]progressiveVLMPageResult) ([]string, []int) {
	if len(results) == 0 {
		return nil, nil
	}
	indexes := make([]int, 0, len(results))
	for idx := range results {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	dataURLs := make([]string, 0, len(indexes))
	pageNumbers := make([]int, 0, len(indexes))
	for _, idx := range indexes {
		result := results[idx]
		if result.DataURL == "" || result.PageNumber < 1 {
			continue
		}
		dataURLs = append(dataURLs, result.DataURL)
		pageNumbers = append(pageNumbers, result.PageNumber)
	}
	return dataURLs, pageNumbers
}
