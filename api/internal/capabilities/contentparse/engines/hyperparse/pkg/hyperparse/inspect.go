package hyperparse

import (
	"context"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/inspectsvc"
)

// PDFInspectInput mirrors the playground inspect request shape.
type PDFInspectInput = inspectsvc.PDFInspectInput

// RunPDFInspect executes the same full pipeline used by the synchronous HTTP inspect path.
func RunPDFInspect(ctx context.Context, in PDFInspectInput) (map[string]any, error) {
	return inspectsvc.RunPDFInspect(ctx, in)
}

// MarshalInspectResponse serializes a standard {"ok":true,"result":...} payload.
func MarshalInspectResponse(result map[string]any) ([]byte, error) {
	return inspectsvc.MarshalInspectResponse(result)
}

// InspectProgressSink receives complete partial {"ok":true,"result":...} payloads.
type InspectProgressSink = inspectsvc.InspectProgressSink

// RunPDFInspectProgressive runs batched VLM work and pushes partial results through sink.
func RunPDFInspectProgressive(ctx context.Context, in PDFInspectInput, taskID string, sink InspectProgressSink) {
	inspectsvc.RunPDFInspectProgressive(ctx, in, taskID, sink)
}
