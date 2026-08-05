package provider

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const musicTestRequestID = "11111111-1111-1111-1111-111111111111"

func TestZGICloudAdapterGenerateMusicRequiresCompleteTrailer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/internal/audio/music/generations"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got := r.Header.Get(headerZGIRequestID); got != musicTestRequestID {
			t.Fatalf("request id = %q", got)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Trailer", headerZGIStreamStatus)
		_, _ = w.Write([]byte("MP3-A"))
		_, _ = w.Write([]byte("MP3-B"))
		w.Header().Set(headerZGIStreamStatus, zgiStreamStatusComplete)
	}))
	defer server.Close()

	cloud, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		ProviderName: zgiCloudAdapterName,
		BaseURL:      server.URL + "/v1/internal",
		AuthHook:     func(*http.Request) {},
	})
	if err != nil {
		t.Fatal(err)
	}

	var audio bytes.Buffer
	err = cloud.GenerateMusic(t.Context(), &adapter.MusicRequest{
		RequestID:      musicTestRequestID,
		Model:          "music-3.0",
		Mode:           adapter.MusicModeInstrumental,
		Prompt:         "warm piano",
		ResponseFormat: "mp3",
	}, &audio)
	if err != nil {
		t.Fatalf("GenerateMusic() error = %v", err)
	}
	if got, want := audio.String(), "MP3-AMP3-B"; got != want {
		t.Fatalf("audio = %q, want %q", got, want)
	}
}

func TestZGICloudAdapterGenerateMusicRejectsMissingCompleteTrailer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("partial"))
	}))
	defer server.Close()

	cloud, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		ProviderName: zgiCloudAdapterName,
		BaseURL:      server.URL + "/v1/internal",
		AuthHook:     func(*http.Request) {},
	})
	if err != nil {
		t.Fatal(err)
	}

	err = cloud.GenerateMusic(t.Context(), &adapter.MusicRequest{
		RequestID:      musicTestRequestID,
		Model:          "music-3.0",
		Mode:           adapter.MusicModeInstrumental,
		Prompt:         "warm piano",
		ResponseFormat: "mp3",
	}, io.Discard)
	if !errors.Is(err, adapter.ErrMusicStreamIncomplete) {
		t.Fatalf("GenerateMusic() error = %v, want ErrMusicStreamIncomplete", err)
	}
}

func TestZGICloudAdapterGenerateMusicLabelsTransportErrorsAsMusic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()

	cloud, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		ProviderName: zgiCloudAdapterName,
		BaseURL:      server.URL + "/v1/internal",
		AuthHook:     func(*http.Request) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = cloud.GenerateMusic(t.Context(), &adapter.MusicRequest{
		RequestID:      musicTestRequestID,
		Model:          "music-3.0",
		Mode:           adapter.MusicModeInstrumental,
		Prompt:         "warm piano",
		ResponseFormat: "mp3",
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "music request failed") {
		t.Fatalf("GenerateMusic() error = %v, want music transport context", err)
	}
}

func TestZGICloudAdapterRejectsOversizedPromptBeforeHTTP(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls++
	}))
	defer server.Close()

	cloud, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		ProviderName: zgiCloudAdapterName,
		BaseURL:      server.URL + "/v1/internal",
		AuthHook:     func(*http.Request) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = cloud.GenerateMusic(t.Context(), &adapter.MusicRequest{
		RequestID:      musicTestRequestID,
		Model:          "music-3.0",
		Mode:           adapter.MusicModeInstrumental,
		Prompt:         strings.Repeat("界", adapter.MaxMusicPromptRunes+1),
		ResponseFormat: "mp3",
	}, io.Discard)
	if !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("GenerateMusic() error = %v, want ErrInvalidRequest", err)
	}
	if calls != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls)
	}
}

func TestCopyMusicResponseRejectsOversizedAudio(t *testing.T) {
	var dst bytes.Buffer
	written, err := copyMusicResponse(&dst, bytes.NewBufferString("12345"), 4)
	if !errors.Is(err, adapter.ErrMusicResponseTooLarge) {
		t.Fatalf("copyMusicResponse() error = %v, want ErrMusicResponseTooLarge", err)
	}
	if written != 4 || dst.String() != "1234" {
		t.Fatalf("copyMusicResponse() = %d bytes, %q; want 4 bytes, %q", written, dst.String(), "1234")
	}
}

func TestCopyMusicResponseAcceptsAudioAtLimit(t *testing.T) {
	var dst bytes.Buffer
	written, err := copyMusicResponse(&dst, bytes.NewBufferString("1234"), 4)
	if err != nil {
		t.Fatalf("copyMusicResponse() error = %v", err)
	}
	if written != 4 || dst.String() != "1234" {
		t.Fatalf("copyMusicResponse() = %d bytes, %q; want 4 bytes, %q", written, dst.String(), "1234")
	}
}

func TestZGICloudAdapterCompensateMusicDelivery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/internal/audio/music/delivery-compensations"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got := r.Header.Get(headerZGIRequestID); got != musicTestRequestID {
			t.Fatalf("request id = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"billing_status":"compensated","refunded_credits":100}}`))
	}))
	defer server.Close()

	cloud, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		ProviderName: zgiCloudAdapterName,
		BaseURL:      server.URL + "/v1/internal",
		AuthHook:     func(*http.Request) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := cloud.CompensateMusicDelivery(t.Context(), musicTestRequestID); err != nil {
		t.Fatalf("CompensateMusicDelivery() error = %v", err)
	}
}

func TestZGICloudAdapterCompensationReportsTerminalNoCharge(t *testing.T) {
	for _, billingStatus := range []string{"rolled_back", "expired"} {
		t.Run(billingStatus, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"billing_status":"` + billingStatus + `","refunded_credits":100}}`))
			}))
			defer server.Close()

			cloud, err := NewZGICloudAdapter(&adapter.AdapterConfig{
				ProviderName: zgiCloudAdapterName,
				BaseURL:      server.URL + "/v1/internal",
				AuthHook:     func(*http.Request) {},
			})
			if err != nil {
				t.Fatal(err)
			}
			err = cloud.CompensateMusicDelivery(t.Context(), musicTestRequestID)
			if !errors.Is(err, adapter.ErrMusicCompensationNotCharged) {
				t.Fatalf("CompensateMusicDelivery() error = %v, want ErrMusicCompensationNotCharged", err)
			}
		})
	}
}
