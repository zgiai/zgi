package service

import (
	"context"
	"errors"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type fakeVideoArtifactSaver struct {
	storedURL string
	err       error
	calls     int
	gotURL    string
}

func (f *fakeVideoArtifactSaver) SaveRemoteVideo(_ context.Context, _ Scope, videoURL string) (string, error) {
	f.calls++
	f.gotURL = videoURL
	if f.err != nil {
		return "", f.err
	}
	return f.storedURL, nil
}

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

func TestStoreVideoArtifactTransfersSucceededURL(t *testing.T) {
	saver := &fakeVideoArtifactSaver{storedURL: "https://files.example.com/console/api/files/tools/stored.mp4?expires_at=0"}
	svc := &service{artifactSaver: saver}
	payload := map[string]any{}

	got := svc.storeVideoArtifact(context.Background(), Scope{}, "https://upstream.example.com/video.mp4", payload)

	if got != saver.storedURL {
		t.Fatalf("storeVideoArtifact() = %q, want stored URL %q", got, saver.storedURL)
	}
	if saver.calls != 1 || saver.gotURL != "https://upstream.example.com/video.mp4" {
		t.Fatalf("artifact saver calls = %d url = %q, want one call with upstream URL", saver.calls, saver.gotURL)
	}
	if payload["video_transfer_status"] != "succeeded" || payload["stored_video_url"] != saver.storedURL {
		t.Fatalf("payload transfer metadata = %#v, want succeeded stored URL", payload)
	}
}

func TestStoreVideoArtifactKeepsUpstreamURLWhenTransferFails(t *testing.T) {
	saver := &fakeVideoArtifactSaver{err: errors.New("download failed")}
	svc := &service{artifactSaver: saver}
	payload := map[string]any{}
	const upstreamURL = "https://upstream.example.com/video.mp4"

	got := svc.storeVideoArtifact(context.Background(), Scope{}, upstreamURL, payload)

	if got != upstreamURL {
		t.Fatalf("storeVideoArtifact() = %q, want upstream URL %q", got, upstreamURL)
	}
	if payload["video_transfer_status"] != "failed" || payload["video_transfer_error"] != "download failed" {
		t.Fatalf("payload transfer metadata = %#v, want failed with error", payload)
	}
}

func TestStoreVideoArtifactSkipsStoredURL(t *testing.T) {
	saver := &fakeVideoArtifactSaver{storedURL: "https://should-not-be-used.example.com/video.mp4"}
	svc := &service{artifactSaver: saver}
	const storedURL = "https://files.example.com/console/api/files/tools/stored.mp4?expires_at=0"

	got := svc.storeVideoArtifact(context.Background(), Scope{}, storedURL, map[string]any{})

	if got != storedURL {
		t.Fatalf("storeVideoArtifact() = %q, want original stored URL %q", got, storedURL)
	}
	if saver.calls != 0 {
		t.Fatalf("artifact saver calls = %d, want 0", saver.calls)
	}
}
