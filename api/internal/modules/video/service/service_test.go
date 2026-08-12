package service

import (
	"errors"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestEstimateVideoTaskCreditsUsesDefaultPerSecondPoints(t *testing.T) {
	got := estimateVideoTaskCredits(GenerateOptions{Duration: 4, Count: 1})
	const want int64 = 572000
	if got != want {
		t.Fatalf("estimateVideoTaskCredits() = %d, want %d", got, want)
	}
}

func TestVideoErrorMessageExtractsEmbeddedOpenAIErrorMessage(t *testing.T) {
	err := errors.New(`request failed after 3 retries: server error 500: {"error":{"message":"Error while downloading image, error: expected the width to be at least 300px, but received a 153x161px image instead","type":"BadRequest","code":"InvalidParameter"}}`)

	got := videoErrorMessage(err)
	const want = "Error while downloading image, error: expected the width to be at least 300px, but received a 153x161px image instead"
	if got != want {
		t.Fatalf("videoErrorMessage() = %q, want %q", got, want)
	}
}

func TestEstimateVideoTaskCreditsFallsBackToDefaults(t *testing.T) {
	got := estimateVideoTaskCredits(GenerateOptions{})
	const want int64 = 715000
	if got != want {
		t.Fatalf("estimateVideoTaskCredits() = %d, want %d", got, want)
	}
}

func TestBuildVideoRequestKeepsImageReferenceOutOfInputVideo(t *testing.T) {
	req := GenerateRequest{
		ReferenceURL:   "https://example.com/ref.png",
		ReferenceTypes: []string{"image"},
	}
	references := videoReferenceURLs(req)
	videoReq := buildVideoRequest("doubao", "seedance", "prompt", req, normalizeGenerateOptions(GenerateOptions{}), "user", references)

	if videoReq.ImageURL != "https://example.com/ref.png" {
		t.Fatalf("ImageURL = %q, want image reference", videoReq.ImageURL)
	}
	if videoReq.VideoURL != "" {
		t.Fatalf("VideoURL = %q, want empty for image reference", videoReq.VideoURL)
	}
	if hasVideoInputReference(req, videoReq, references) {
		t.Fatalf("hasVideoInputReference() = true, want false for image reference")
	}
}

func TestBuildVideoRequestMarksVideoReferenceAsInputVideo(t *testing.T) {
	req := GenerateRequest{
		ReferenceURL:   "https://example.com/ref.mp4",
		ReferenceTypes: []string{"video"},
	}
	references := videoReferenceURLs(req)
	videoReq := buildVideoRequest("doubao", "seedance", "prompt", req, normalizeGenerateOptions(GenerateOptions{}), "user", references)

	if videoReq.VideoURL != "https://example.com/ref.mp4" {
		t.Fatalf("VideoURL = %q, want video reference", videoReq.VideoURL)
	}
	if videoReq.ImageURL != "" || len(videoReq.ImageURLs) != 0 {
		t.Fatalf("image references = %q/%v, want empty for video reference", videoReq.ImageURL, videoReq.ImageURLs)
	}
	if !hasVideoInputReference(req, videoReq, references) {
		t.Fatalf("hasVideoInputReference() = false, want true for video reference")
	}
}

func TestVideoResponseErrorMessageExtractsNestedErrorMessage(t *testing.T) {
	resp := &adapter.VideoResponse{
		Status: "failed",
		Raw: map[string]any{
			"error": map[string]any{
				"code":    "OutputVideoSensitiveContentDetected.PolicyViolation",
				"message": "The request failed because the output video may be related to copyright restrictions.",
			},
		},
	}

	got := videoResponseErrorMessage(resp)
	const want = "The request failed because the output video may be related to copyright restrictions."
	if got != want {
		t.Fatalf("videoResponseErrorMessage() = %q, want %q", got, want)
	}
}

func TestVideoResponseErrorMessageExtractsWrappedDataErrorMessage(t *testing.T) {
	resp := &adapter.VideoResponse{
		Raw: map[string]any{
			"code":    float64(0),
			"message": "success",
			"data": map[string]any{
				"error": map[string]any{
					"code":    "InvalidParameter",
					"message": "Error while downloading image, error: expected the width to be at least 300px",
				},
			},
		},
	}

	got := videoResponseErrorMessage(resp)
	const want = "Error while downloading image, error: expected the width to be at least 300px"
	if got != want {
		t.Fatalf("videoResponseErrorMessage() = %q, want %q", got, want)
	}
}

func TestVideoErrorMessageExtractsUpstreamErrorText(t *testing.T) {
	err := errors.New("failed to parse response: upstream error: Error while downloading image")

	got := videoErrorMessage(err)
	const want = "Error while downloading image"
	if got != want {
		t.Fatalf("videoErrorMessage() = %q, want %q", got, want)
	}
}
