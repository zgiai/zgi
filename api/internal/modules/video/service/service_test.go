package service

import (
	"context"
	"errors"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type fakeVideoArtifactSaver struct {
	storedURL  string
	posterURL  string
	err        error
	posterErr  error
	calls      int
	posterCall int
	gotURL     string
	gotPoster  []byte
}

func (f *fakeVideoArtifactSaver) SaveRemoteVideo(_ context.Context, _ Scope, videoURL string) (string, error) {
	f.calls++
	f.gotURL = videoURL
	if f.err != nil {
		return "", f.err
	}
	return f.storedURL, nil
}

func (f *fakeVideoArtifactSaver) SavePosterImage(_ context.Context, _ Scope, poster []byte) (string, error) {
	f.posterCall++
	f.gotPoster = append([]byte(nil), poster...)
	if f.posterErr != nil {
		return "", f.posterErr
	}
	return f.posterURL, nil
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

	if len(videoReq.ReferenceURLs) != 1 || videoReq.ReferenceURLs[0] != "https://example.com/ref.png" {
		t.Fatalf("ReferenceURLs = %#v, want image reference", videoReq.ReferenceURLs)
	}
	if len(videoReq.ReferenceTypes) != 1 || videoReq.ReferenceTypes[0] != "image" {
		t.Fatalf("ReferenceTypes = %#v, want image", videoReq.ReferenceTypes)
	}
	if videoReq.ImageURL != "" || len(videoReq.ImageURLs) != 0 {
		t.Fatalf("legacy image references = %q/%v, want empty for omni reference", videoReq.ImageURL, videoReq.ImageURLs)
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

	if len(videoReq.ReferenceURLs) != 1 || videoReq.ReferenceURLs[0] != "https://example.com/ref.mp4" {
		t.Fatalf("ReferenceURLs = %#v, want video reference", videoReq.ReferenceURLs)
	}
	if len(videoReq.ReferenceTypes) != 1 || videoReq.ReferenceTypes[0] != "video" {
		t.Fatalf("ReferenceTypes = %#v, want video", videoReq.ReferenceTypes)
	}
	if videoReq.VideoURL != "" {
		t.Fatalf("VideoURL = %q, want empty for omni reference", videoReq.VideoURL)
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

func TestVideoErrorMessageExtractsUpstreamTextFromAdapterError(t *testing.T) {
	err := &adapter.AdapterError{
		Message: "Video generation failed: all 1 candidate channels failed for video model doubao-seedance-2-0-260128: upstream error: The parameter `content[3]` specified in the request is not valid: audio duration is too long",
	}

	got := videoErrorMessage(err)
	const want = "The parameter `content[3]` specified in the request is not valid: audio duration is too long"
	if got != want {
		t.Fatalf("videoErrorMessage() = %q, want %q", got, want)
	}
}

func TestStoreVideoArtifactTransfersSucceededURL(t *testing.T) {
	saver := &fakeVideoArtifactSaver{
		storedURL: "https://files.example.com/console/api/files/tools/stored.mp4?expires_at=0",
		posterURL: "https://files.example.com/console/api/files/tools/poster.jpg?expires_at=0",
	}
	svc := &service{
		artifactSaver: saver,
		posterExtractor: func(_ context.Context, _ string) ([]byte, error) {
			return []byte("poster"), nil
		},
	}
	payload := map[string]any{}

	got, posterURL := svc.storeVideoArtifact(context.Background(), Scope{}, "https://upstream.example.com/video.mp4", payload)

	if got != saver.storedURL {
		t.Fatalf("storeVideoArtifact() = %q, want stored URL %q", got, saver.storedURL)
	}
	if posterURL != saver.posterURL {
		t.Fatalf("storeVideoArtifact() poster = %q, want poster URL %q", posterURL, saver.posterURL)
	}
	if saver.calls != 1 || saver.gotURL != "https://upstream.example.com/video.mp4" {
		t.Fatalf("artifact saver calls = %d url = %q, want one call with upstream URL", saver.calls, saver.gotURL)
	}
	if saver.posterCall != 1 || string(saver.gotPoster) != "poster" {
		t.Fatalf("poster saver calls = %d poster = %q, want one call", saver.posterCall, string(saver.gotPoster))
	}
	if payload["video_transfer_status"] != "succeeded" || payload["stored_video_url"] != saver.storedURL {
		t.Fatalf("payload transfer metadata = %#v, want succeeded stored URL", payload)
	}
	if payload["poster_generation_status"] != "succeeded" || payload["poster_url"] != saver.posterURL {
		t.Fatalf("payload poster metadata = %#v, want succeeded poster URL", payload)
	}
}

func TestStoreVideoArtifactKeepsUpstreamURLWhenTransferFails(t *testing.T) {
	saver := &fakeVideoArtifactSaver{err: errors.New("download failed")}
	svc := &service{
		artifactSaver: saver,
		posterExtractor: func(_ context.Context, _ string) ([]byte, error) {
			return nil, errors.New("poster failed")
		},
	}
	payload := map[string]any{}
	const upstreamURL = "https://upstream.example.com/video.mp4"

	got, posterURL := svc.storeVideoArtifact(context.Background(), Scope{}, upstreamURL, payload)

	if got != upstreamURL {
		t.Fatalf("storeVideoArtifact() = %q, want upstream URL %q", got, upstreamURL)
	}
	if posterURL != "" {
		t.Fatalf("storeVideoArtifact() poster = %q, want empty", posterURL)
	}
	if payload["video_transfer_status"] != "failed" || payload["video_transfer_error"] != "download failed" {
		t.Fatalf("payload transfer metadata = %#v, want failed with error", payload)
	}
	if payload["poster_generation_status"] != "failed" || payload["poster_generation_error"] != "poster failed" {
		t.Fatalf("payload poster metadata = %#v, want failed with error", payload)
	}
}

func TestStoreVideoArtifactSkipsStoredURL(t *testing.T) {
	saver := &fakeVideoArtifactSaver{
		storedURL: "https://should-not-be-used.example.com/video.mp4",
		posterURL: "https://files.example.com/console/api/files/tools/poster.jpg?expires_at=0",
	}
	svc := &service{
		artifactSaver: saver,
		posterExtractor: func(_ context.Context, _ string) ([]byte, error) {
			return []byte("poster"), nil
		},
	}
	const storedURL = "https://files.example.com/console/api/files/tools/stored.mp4?expires_at=0"

	got, posterURL := svc.storeVideoArtifact(context.Background(), Scope{}, storedURL, map[string]any{})

	if got != storedURL {
		t.Fatalf("storeVideoArtifact() = %q, want original stored URL %q", got, storedURL)
	}
	if posterURL != saver.posterURL {
		t.Fatalf("storeVideoArtifact() poster = %q, want poster URL %q", posterURL, saver.posterURL)
	}
	if saver.calls != 0 {
		t.Fatalf("artifact saver calls = %d, want 0", saver.calls)
	}
	if saver.posterCall != 1 {
		t.Fatalf("poster saver calls = %d, want 1", saver.posterCall)
	}
}

func TestToolFileIDFromSignedVideoURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "absolute signed tool URL",
			raw:  "https://console.example.com/console/api/files/tools/abc123.mp4?expires_at=123&signature=secret",
			want: "abc123",
		},
		{
			name: "relative signed tool URL",
			raw:  "/console/api/files/tools/video-20260903-abcdef.mp4?expires_at=123",
			want: "video-20260903-abcdef",
		},
		{
			name: "tool URL without extension",
			raw:  "/console/api/files/tools/video-20260903-abcdef?expires_at=123",
			want: "video-20260903-abcdef",
		},
		{
			name: "upstream URL",
			raw:  "https://upstream.example.com/videos/result.mp4",
			want: "",
		},
		{
			name: "empty URL",
			raw:  "",
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toolFileIDFromSignedVideoURL(tc.raw)
			if got != tc.want {
				t.Fatalf("toolFileIDFromSignedVideoURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestNormalizeBackfillTaskIDsTrimsSplitsAndDeduplicates(t *testing.T) {
	got := normalizeBackfillTaskIDs([]string{" video-1,video-2 ", "video-1", "", " video-3 "})
	want := []string{"video-1", "video-2", "video-3"}
	if len(got) != len(want) {
		t.Fatalf("normalizeBackfillTaskIDs() = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("normalizeBackfillTaskIDs()[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}
