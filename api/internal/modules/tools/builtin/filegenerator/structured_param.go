package filegenerator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func structuredObjectJSON(value interface{}, name string, maxBytes int) ([]byte, error) {
	var data []byte
	switch typed := value.(type) {
	case string:
		data = []byte(strings.TrimSpace(typed))
	case json.RawMessage:
		data = bytes.TrimSpace(typed)
	case map[string]interface{}:
		if typed == nil {
			return nil, fmt.Errorf("%s is required", name)
		}
		encoded, err := json.Marshal(typed)
		if err != nil {
			return nil, fmt.Errorf("%s object must be JSON serializable: %w", name, err)
		}
		data = encoded
	case nil:
		return nil, fmt.Errorf("%s is required", name)
	default:
		return nil, fmt.Errorf("%s must be an object or JSON object string, got %T", name, value)
	}

	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, fmt.Errorf("%s is required", name)
	}
	if maxBytes > 0 && len(data) > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", name, maxBytes)
	}
	return data, nil
}

func requireJSONDecoderEOF(decoder *json.Decoder, name string) error {
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%s must contain exactly one JSON object", name)
		}
		return fmt.Errorf("%s contains trailing JSON data: %w", name, err)
	}
	return nil
}
