package hyperparse

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
	extractlocal "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/local"
	extractmineru "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/mineru"
	extractreducto "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/reducto"
	extractvlm "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/vlm"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/pkg/storage"
)

var extractSemaphore = make(chan struct{}, maxSemaphore())

func maxSemaphore() int {
	n := runtime.NumCPU() / 2
	if n < 2 {
		return 2
	}
	return n
}

type HyperparseExtractor struct {
	filePath       string
	backend        string
	storage        storage.Storage
	assetNamespace string
}

func NewHyperparseExtractor(filePath, backend string) *HyperparseExtractor {
	return NewHyperparseExtractorWithStorage(filePath, backend, nil, "")
}

func NewHyperparseExtractorWithStorage(filePath, backend string, store storage.Storage, assetNamespace string) *HyperparseExtractor {
	return &HyperparseExtractor{
		filePath:       filePath,
		backend:        strings.ToLower(strings.TrimSpace(backend)),
		storage:        store,
		assetNamespace: assetNamespace,
	}
}

func (e *HyperparseExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	data, err := os.ReadFile(e.filePath)
	if err != nil {
		return nil, fmt.Errorf("hyperparse-sdk: read file: %w", err)
	}

	select {
	case extractSemaphore <- struct{}{}:
		defer func() { <-extractSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	var result *extractcommon.DocumentResult
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("hyperparse-sdk: backend=%s panic recovered: %v", e.backend, rec)
			}
		}()
		result, err = parseWithBackend(ctx, e.backend, filepath.Base(e.filePath), data)
	}()
	if err != nil {
		return nil, fmt.Errorf("hyperparse-sdk: parse backend=%s: %w", e.backend, err)
	}
	if e.backend == "mineru" {
		if err := persistMinerUImages(e.storage, result, e.assetNamespace); err != nil {
			return nil, fmt.Errorf("hyperparse-sdk: persist mineru images: %w", err)
		}
	}

	output := mapResultToExtractOutput(result, e.filePath, e.backend)
	if output == nil || (len(output.Elements) == 0 && strings.TrimSpace(output.Markdown) == "") {
		return nil, errors.New("hyperparse-sdk: empty extraction result")
	}
	return output, nil
}

func parseWithBackend(ctx context.Context, backend, filename string, data []byte) (*extractcommon.DocumentResult, error) {
	opts := extractcommon.ParseOptions{Mode: "relaxed"}
	switch backend {
	case "local":
		return extractlocal.New().ParseBytes(ctx, filename, data, opts)
	case "mineru":
		return extractmineru.New().ParseBytes(ctx, filename, data, opts)
	case "reducto":
		return extractreducto.New().ParseBytes(ctx, filename, data, opts)
	case "vlm":
		return extractvlm.New().ParseBytes(ctx, filename, data, opts)
	default:
		return nil, fmt.Errorf("unsupported backend: %s", backend)
	}
}
