package provider

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestDoubaoAdapterTranscribeRejectsUnsafeAudioBaseURL(t *testing.T) {
	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:           "test-key",
		GuardOutboundURL: true,
		GuardOutboundDNS: true,
		CustomParams: map[string]interface{}{
			"audio_base_url": "http://127.0.0.1:8080",
		},
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	_, err = a.Transcribe(t.Context(), &adapter.TranscriptionRequest{
		Model: "volc.seedasr.sauc.duration",
		Audio: bytes.NewReader([]byte("pcm")),
	})
	if err == nil || !strings.Contains(err.Error(), "blocked unsafe target") {
		t.Fatalf("Transcribe() error = %v, want blocked unsafe target", err)
	}
}

func TestDoubaoAdapterTranscribeStreamsPCMOverNativeV3Protocol(t *testing.T) {
	audio := bytes.Repeat([]byte{0x2a}, 3200)
	serverErrors := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverErrors <- serveDoubaoTranscriptionTestConnection(w, r, audio)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey: "test-key",
		CustomParams: map[string]interface{}{
			"audio_base_url": server.URL,
		},
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	result, err := a.Transcribe(t.Context(), &adapter.TranscriptionRequest{
		RequestID: "62bfaf55-bd31-4a17-bd7c-b52abac85691",
		Model:     "volc.seedasr.sauc.duration",
		Audio:     bytes.NewReader(audio),
	})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if serverErr := <-serverErrors; serverErr != nil {
		t.Fatalf("streaming ASR server error = %v", serverErr)
	}
	if result == nil || result.RequestID != "62bfaf55-bd31-4a17-bd7c-b52abac85691" || result.Text != "转换后的文字。" {
		t.Fatalf("result = %#v", result)
	}
}

func serveDoubaoTranscriptionTestConnection(w http.ResponseWriter, r *http.Request, wantAudio []byte) error {
	if got, want := r.URL.Path, "/api/v3/sauc/bigmodel_nostream"; got != want {
		return fmt.Errorf("path = %q, want %q", got, want)
	}
	if got, want := r.Header.Get("X-Api-Key"), "test-key"; got != want {
		return fmt.Errorf("X-Api-Key = %q, want %q", got, want)
	}
	if got, want := r.Header.Get("X-Api-Resource-Id"), "volc.seedasr.sauc.duration"; got != want {
		return fmt.Errorf("X-Api-Resource-Id = %q, want %q", got, want)
	}
	if got, want := r.Header.Get("X-Api-Request-Id"), "62bfaf55-bd31-4a17-bd7c-b52abac85691"; got != want {
		return fmt.Errorf("X-Api-Request-Id = %q, want %q", got, want)
	}

	connection, err := (&websocket.Upgrader{}).Upgrade(w, r, http.Header{"X-Tt-Logid": []string{"stream-trace"}})
	if err != nil {
		return fmt.Errorf("upgrade websocket: %w", err)
	}
	defer connection.Close()

	messageType, message, err := connection.ReadMessage()
	if err != nil {
		return fmt.Errorf("read full request: %w", err)
	}
	clientType, sequence, payload, err := decodeDoubaoTranscriptionClientFrame(message)
	if err != nil {
		return err
	}
	if messageType != websocket.BinaryMessage || clientType != 1 || sequence != 1 {
		return fmt.Errorf("full request frame = type %d client_type %d sequence %d", messageType, clientType, sequence)
	}
	var fullRequest struct {
		Audio struct {
			Format     string `json:"format"`
			Codec      string `json:"codec"`
			SampleRate int    `json:"rate"`
			Bits       int    `json:"bits"`
			Channels   int    `json:"channel"`
		} `json:"audio"`
		Request struct {
			ModelName      string `json:"model_name"`
			EnableITN      bool   `json:"enable_itn"`
			EnablePUNC     bool   `json:"enable_punc"`
			ShowUtterances bool   `json:"show_utterances"`
		} `json:"request"`
	}
	if err := json.Unmarshal(payload, &fullRequest); err != nil {
		return fmt.Errorf("decode full request: %w", err)
	}
	if fullRequest.Audio.Format != "pcm" || fullRequest.Audio.Codec != "raw" || fullRequest.Audio.SampleRate != 16000 || fullRequest.Audio.Bits != 16 || fullRequest.Audio.Channels != 1 {
		return fmt.Errorf("audio config = %#v", fullRequest.Audio)
	}
	if fullRequest.Request.ModelName != "bigmodel" || !fullRequest.Request.EnableITN || !fullRequest.Request.EnablePUNC || !fullRequest.Request.ShowUtterances {
		return fmt.Errorf("recognition config = %#v", fullRequest.Request)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, encodeDoubaoTranscriptionServerFrame(1, false, map[string]any{})); err != nil {
		return fmt.Errorf("write full response: %w", err)
	}

	messageType, message, err = connection.ReadMessage()
	if err != nil {
		return fmt.Errorf("read audio request: %w", err)
	}
	clientType, sequence, payload, err = decodeDoubaoTranscriptionClientFrame(message)
	if err != nil {
		return err
	}
	if messageType != websocket.BinaryMessage || clientType != 2 || sequence != -2 {
		return fmt.Errorf("audio frame = type %d client_type %d sequence %d", messageType, clientType, sequence)
	}
	if !bytes.Equal(payload, wantAudio) {
		return fmt.Errorf("audio length = %d, want %d", len(payload), len(wantAudio))
	}
	response := map[string]any{
		"audio_info": map[string]any{"duration": 100},
		"result":     map[string]any{"text": "转换后的文字。"},
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, encodeDoubaoTranscriptionServerFrame(2, true, response)); err != nil {
		return fmt.Errorf("write final response: %w", err)
	}
	return nil
}

func decodeDoubaoTranscriptionClientFrame(message []byte) (byte, int32, []byte, error) {
	if len(message) < 12 {
		return 0, 0, nil, fmt.Errorf("client frame is too short")
	}
	headerSize := int(message[0]&0x0f) * 4
	clientType := message[1] >> 4
	flags := message[1] & 0x0f
	if headerSize < 4 || flags&1 == 0 || len(message) < headerSize+8 {
		return 0, 0, nil, fmt.Errorf("invalid client frame header")
	}
	sequence := int32(binary.BigEndian.Uint32(message[headerSize : headerSize+4]))
	payloadSize := int(binary.BigEndian.Uint32(message[headerSize+4 : headerSize+8]))
	if payloadSize != len(message)-(headerSize+8) {
		return 0, 0, nil, fmt.Errorf("client payload size = %d, actual %d", payloadSize, len(message)-(headerSize+8))
	}
	reader, err := gzip.NewReader(bytes.NewReader(message[headerSize+8:]))
	if err != nil {
		return 0, 0, nil, fmt.Errorf("open client payload: %w", err)
	}
	defer reader.Close()
	payload, err := io.ReadAll(reader)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("read client payload: %w", err)
	}
	return clientType, sequence, payload, nil
}

func encodeDoubaoTranscriptionServerFrame(sequence int32, last bool, payload any) []byte {
	encoded, _ := json.Marshal(payload)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, _ = writer.Write(encoded)
	_ = writer.Close()

	flags := byte(1)
	if last {
		flags = 3
	}
	frame := []byte{0x11, 0x90 | flags, 0x11, 0x00}
	frame = binary.BigEndian.AppendUint32(frame, uint32(sequence))
	frame = binary.BigEndian.AppendUint32(frame, uint32(compressed.Len()))
	return append(frame, compressed.Bytes()...)
}
