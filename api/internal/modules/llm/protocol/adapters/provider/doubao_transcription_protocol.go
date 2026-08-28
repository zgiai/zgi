package provider

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	doubaoTranscriptionPath       = "/api/v3/sauc/bigmodel_nostream"
	doubaoTranscriptionModelName  = "bigmodel"
	doubaoTranscriptionSampleRate = 16000
	doubaoTranscriptionBits       = 16
	doubaoTranscriptionChannels   = 1

	doubaoASRClientFullRequest  = 0x1
	doubaoASRClientAudio        = 0x2
	doubaoASRServerFullResponse = 0x9
	doubaoASRServerError        = 0xf
	doubaoASRFlagSequence       = 0x1
	doubaoASRFlagLast           = 0x2
	doubaoASRJSON               = 0x1
	doubaoASRGZIP               = 0x1
)

type doubaoTranscriptionFullRequest struct {
	Audio   doubaoTranscriptionAudioConfig `json:"audio"`
	Request doubaoTranscriptionOptions     `json:"request"`
}

type doubaoTranscriptionAudioConfig struct {
	Format     string `json:"format"`
	Codec      string `json:"codec"`
	SampleRate int    `json:"rate"`
	Bits       int    `json:"bits"`
	Channels   int    `json:"channel"`
}

type doubaoTranscriptionOptions struct {
	ModelName      string `json:"model_name"`
	EnableITN      bool   `json:"enable_itn"`
	EnablePUNC     bool   `json:"enable_punc"`
	ShowUtterances bool   `json:"show_utterances"`
}

type doubaoTranscriptionResponse struct {
	Result struct {
		Text string `json:"text"`
	} `json:"result"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type doubaoTranscriptionFrame struct {
	Sequence int32
	Last     bool
	Payload  doubaoTranscriptionResponse
}

func resolveDoubaoAudioWebSocketEndpoint(config *adapter.AdapterConfig, path string) (string, error) {
	parsed, err := url.Parse(doubaoAudioBaseURL(config))
	if err != nil {
		return "", fmt.Errorf("parse doubao audio base url: %w", err)
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return "", fmt.Errorf("%w: doubao audio base url must use http or https", adapter.ErrInvalidConfig)
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(basePath, doubaoAudioAPIPrefix) {
		path = strings.TrimPrefix(path, doubaoAudioAPIPrefix)
	}
	parsed.Path = basePath + path
	return parsed.String(), nil
}

func encodeDoubaoTranscriptionFullRequest() ([]byte, error) {
	payload, err := json.Marshal(doubaoTranscriptionFullRequest{
		Audio: doubaoTranscriptionAudioConfig{
			Format:     "pcm",
			Codec:      "raw",
			SampleRate: doubaoTranscriptionSampleRate,
			Bits:       doubaoTranscriptionBits,
			Channels:   doubaoTranscriptionChannels,
		},
		Request: doubaoTranscriptionOptions{
			ModelName:      doubaoTranscriptionModelName,
			EnableITN:      true,
			EnablePUNC:     true,
			ShowUtterances: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal doubao transcription request: %w", err)
	}
	return encodeDoubaoTranscriptionClientFrame(doubaoASRClientFullRequest, 1, false, doubaoASRJSON, payload)
}

func encodeDoubaoTranscriptionAudio(sequence int32, audio []byte, last bool) ([]byte, error) {
	return encodeDoubaoTranscriptionClientFrame(doubaoASRClientAudio, sequence, last, 0, audio)
}

func encodeDoubaoTranscriptionClientFrame(messageType byte, sequence int32, last bool, serialization byte, payload []byte) ([]byte, error) {
	compressed, err := gzipDoubaoTranscriptionPayload(payload)
	if err != nil {
		return nil, err
	}
	flags := byte(doubaoASRFlagSequence)
	if last {
		flags |= doubaoASRFlagLast
	}
	frame := []byte{0x11, messageType<<4 | flags, serialization<<4 | doubaoASRGZIP, 0x00}
	frame = binary.BigEndian.AppendUint32(frame, uint32(sequence))
	frame = binary.BigEndian.AppendUint32(frame, uint32(len(compressed)))
	return append(frame, compressed...), nil
}

func gzipDoubaoTranscriptionPayload(payload []byte) ([]byte, error) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(payload); err != nil {
		return nil, fmt.Errorf("compress doubao transcription payload: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish doubao transcription payload: %w", err)
	}
	return compressed.Bytes(), nil
}

func writeDoubaoTranscriptionFrame(connection *websocket.Conn, frame []byte) error {
	if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return fmt.Errorf("write websocket frame: %w", err)
	}
	return nil
}

func readDoubaoTranscriptionFrame(connection *websocket.Conn) (doubaoTranscriptionFrame, error) {
	messageType, message, err := connection.ReadMessage()
	if err != nil {
		return doubaoTranscriptionFrame{}, fmt.Errorf("read websocket frame: %w", err)
	}
	if messageType != websocket.BinaryMessage {
		return doubaoTranscriptionFrame{}, fmt.Errorf("doubao transcription returned a non-binary websocket frame")
	}
	return parseDoubaoTranscriptionServerFrame(message)
}

func parseDoubaoTranscriptionServerFrame(message []byte) (doubaoTranscriptionFrame, error) {
	if len(message) < 4 {
		return doubaoTranscriptionFrame{}, fmt.Errorf("doubao transcription frame is too short")
	}
	headerSize := int(message[0]&0x0f) * 4
	if headerSize < 4 || headerSize > len(message) {
		return doubaoTranscriptionFrame{}, fmt.Errorf("doubao transcription frame has an invalid header size")
	}
	messageType := message[1] >> 4
	flags := message[1] & 0x0f
	serialization := message[2] >> 4
	compression := message[2] & 0x0f
	offset := headerSize
	frame := doubaoTranscriptionFrame{Last: flags&doubaoASRFlagLast != 0}
	if flags&doubaoASRFlagSequence != 0 {
		if len(message) < offset+4 {
			return doubaoTranscriptionFrame{}, fmt.Errorf("doubao transcription frame is missing its sequence")
		}
		frame.Sequence = int32(binary.BigEndian.Uint32(message[offset : offset+4]))
		offset += 4
	}

	providerCode := uint32(0)
	if messageType == doubaoASRServerError {
		if len(message) < offset+4 {
			return doubaoTranscriptionFrame{}, fmt.Errorf("doubao transcription error frame is missing its code")
		}
		providerCode = binary.BigEndian.Uint32(message[offset : offset+4])
		offset += 4
	} else if messageType != doubaoASRServerFullResponse {
		return doubaoTranscriptionFrame{}, fmt.Errorf("doubao transcription returned unsupported message type %d", messageType)
	}
	if len(message) < offset+4 {
		return doubaoTranscriptionFrame{}, fmt.Errorf("doubao transcription frame is missing its payload size")
	}
	payloadSize := int(binary.BigEndian.Uint32(message[offset : offset+4]))
	offset += 4
	if payloadSize != len(message)-offset {
		return doubaoTranscriptionFrame{}, fmt.Errorf("doubao transcription payload size = %d, actual %d", payloadSize, len(message)-offset)
	}
	payload := message[offset:]
	if compression == doubaoASRGZIP {
		reader, err := gzip.NewReader(bytes.NewReader(payload))
		if err != nil {
			return doubaoTranscriptionFrame{}, fmt.Errorf("open doubao transcription payload: %w", err)
		}
		payload, err = io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			return doubaoTranscriptionFrame{}, fmt.Errorf("read doubao transcription payload: %w", err)
		}
		if closeErr != nil {
			return doubaoTranscriptionFrame{}, fmt.Errorf("close doubao transcription payload: %w", closeErr)
		}
	} else if compression != 0 {
		return doubaoTranscriptionFrame{}, fmt.Errorf("doubao transcription returned unsupported compression %d", compression)
	}

	providerMessage := ""
	if len(payload) > 0 && serialization == doubaoASRJSON {
		if err := json.Unmarshal(payload, &frame.Payload); err != nil {
			return doubaoTranscriptionFrame{}, fmt.Errorf("decode doubao transcription payload: %w", err)
		}
	} else if len(payload) > 0 && messageType == doubaoASRServerError && serialization == 0 {
		providerMessage = strings.TrimSpace(string(payload))
	} else if len(payload) > 0 {
		return doubaoTranscriptionFrame{}, fmt.Errorf("doubao transcription returned unsupported serialization %d", serialization)
	}
	if messageType == doubaoASRServerError {
		message := strings.TrimSpace(frame.Payload.Error)
		if message == "" {
			message = strings.TrimSpace(frame.Payload.Message)
		}
		if message == "" {
			message = providerMessage
		}
		if message == "" {
			message = "doubao streaming transcription failed"
		}
		return doubaoTranscriptionFrame{}, adapter.NewAdapterError(
			strconv.FormatUint(uint64(providerCode), 10),
			message,
			http.StatusBadGateway,
			adapter.ErrUpstreamError,
		)
	}
	return frame, nil
}
