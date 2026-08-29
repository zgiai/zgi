package provider

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const miniMaxMusicDoneStatus = 2

type miniMaxMusicPayload struct {
	Model           string                   `json:"model"`
	Prompt          string                   `json:"prompt,omitempty"`
	Lyrics          string                   `json:"lyrics,omitempty"`
	IsInstrumental  bool                     `json:"is_instrumental,omitempty"`
	LyricsOptimizer bool                     `json:"lyrics_optimizer,omitempty"`
	Stream          bool                     `json:"stream"`
	OutputFormat    string                   `json:"output_format"`
	AudioSetting    miniMaxMusicAudioSetting `json:"audio_setting"`
}

type miniMaxMusicAudioSetting struct {
	Format     string `json:"format"`
	SampleRate int    `json:"sample_rate"`
	Bitrate    int    `json:"bitrate"`
}

type miniMaxMusicResponse struct {
	Data struct {
		Audio  string `json:"audio"`
		Status int    `json:"status"`
	} `json:"data"`
	BaseResp *miniMaxBaseResponse `json:"base_resp"`
	Error    *miniMaxAPIError     `json:"error,omitempty"`
}

type miniMaxBaseResponse struct {
	StatusCode int    `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

type miniMaxAPIError struct {
	Message string          `json:"message"`
	Type    string          `json:"type"`
	Code    json.RawMessage `json:"code"`
}

type miniMaxLyricsResponse struct {
	SongTitle string               `json:"song_title"`
	StyleTags string               `json:"style_tags"`
	Lyrics    string               `json:"lyrics"`
	BaseResp  *miniMaxBaseResponse `json:"base_resp"`
}

func (a *MiniMaxAdapter) GenerateMusic(ctx context.Context, request *adapter.MusicRequest, dst io.Writer) error {
	payload, err := buildMiniMaxMusicPayload(request)
	if err != nil {
		return err
	}
	if dst == nil {
		return fmt.Errorf("%w: music destination is required", adapter.ErrInvalidRequest)
	}

	headers := a.runtimeHeaders("")
	headers["Accept"] = "text/event-stream"
	headers["Cache-Control"] = "no-cache"
	response, err := a.httpClient.DoSingleRequestNoRedirect(
		ctx,
		http.MethodPost,
		a.baseURL+"/music_generation",
		headers,
		payload,
	)
	if err != nil {
		var statusErr *adapter.HTTPStatusError
		if errors.As(err, &statusErr) {
			return miniMaxMusicError(statusErr.StatusCode, statusErr.Body)
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()

	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaType != "text/event-stream" {
		return fmt.Errorf("%w: minimax music requires text/event-stream", adapter.ErrMusicStreamIncomplete)
	}

	written := &miniMaxMusicSizeWriter{dst: dst}
	audioSeen := false
	finished := false
	err = readMiniMaxSSE(response.Body, func(data string) error {
		if data == "[DONE]" {
			return nil
		}
		var frame miniMaxMusicResponse
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			return fmt.Errorf("decode minimax music stream frame: %w", err)
		}
		if err := validateMiniMaxMusicResponse(response.StatusCode, frame); err != nil {
			return err
		}
		if frame.Data.Audio != "" {
			if err := writeMiniMaxHexAudio(written, frame.Data.Audio); err != nil {
				return err
			}
			audioSeen = true
		}
		if frame.Data.Status == miniMaxMusicDoneStatus {
			finished = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !audioSeen || !finished {
		return fmt.Errorf("%w: minimax music stream ended before complete audio", adapter.ErrMusicStreamIncomplete)
	}
	return nil
}

func (a *MiniMaxAdapter) GenerateLyrics(ctx context.Context, request *adapter.LyricsRequest) (*adapter.LyricsResult, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: lyrics request is required", adapter.ErrInvalidRequest)
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" || !utf8.ValidString(request.Prompt) || utf8.RuneCountInString(prompt) > adapter.MaxMusicPromptRunes {
		return nil, fmt.Errorf("%w: lyrics prompt is invalid", adapter.ErrInvalidRequest)
	}

	providerResponse, err := a.httpClient.DoSingleRequestNoRedirect(ctx, http.MethodPost, a.baseURL+"/lyrics_generation", a.runtimeHeaders(""), map[string]string{
		"mode":   "write_full_song",
		"prompt": prompt,
	})
	if err != nil {
		var statusErr *adapter.HTTPStatusError
		if errors.As(err, &statusErr) {
			return nil, miniMaxMusicError(statusErr.StatusCode, statusErr.Body)
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer providerResponse.Body.Close()
	var response miniMaxLyricsResponse
	if err := json.NewDecoder(io.LimitReader(providerResponse.Body, 1<<20)).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode minimax lyrics response: %w", err)
	}
	if response.BaseResp == nil {
		return nil, fmt.Errorf("minimax lyrics response contained no base_resp")
	}
	if response.BaseResp.StatusCode != 0 {
		return nil, newMiniMaxAdapterError(providerResponse.StatusCode, response.BaseResp.StatusCode, response.BaseResp.StatusMsg)
	}
	title := strings.TrimSpace(response.SongTitle)
	lyrics := strings.TrimSpace(response.Lyrics)
	if title == "" || lyrics == "" || !utf8.ValidString(lyrics) || utf8.RuneCountInString(lyrics) > adapter.MaxMusicLyricsRunes {
		return nil, fmt.Errorf("minimax lyrics response is invalid")
	}
	return &adapter.LyricsResult{
		Title:     title,
		StyleTags: splitMiniMaxStyleTags(response.StyleTags),
		Lyrics:    lyrics,
	}, nil
}

func buildMiniMaxMusicPayload(request *adapter.MusicRequest) (miniMaxMusicPayload, error) {
	if request == nil || strings.TrimSpace(request.Model) == "" || request.ResponseFormat != "mp3" {
		return miniMaxMusicPayload{}, fmt.Errorf("%w: model and mp3 response format are required", adapter.ErrInvalidRequest)
	}
	prompt := strings.TrimSpace(request.Prompt)
	lyrics := strings.TrimSpace(request.Lyrics)
	if !utf8.ValidString(request.Prompt) || !utf8.ValidString(request.Lyrics) ||
		utf8.RuneCountInString(prompt) > adapter.MaxMusicPromptRunes ||
		utf8.RuneCountInString(lyrics) > adapter.MaxMusicLyricsRunes {
		return miniMaxMusicPayload{}, fmt.Errorf("%w: music prompt or lyrics are invalid", adapter.ErrInvalidRequest)
	}
	payload := miniMaxMusicPayload{
		Model:        strings.TrimSpace(request.Model),
		Prompt:       prompt,
		Stream:       true,
		OutputFormat: "hex",
		AudioSetting: miniMaxMusicAudioSetting{Format: "mp3", SampleRate: 44100, Bitrate: 256000},
	}
	switch request.Mode {
	case adapter.MusicModeVocal:
		if lyrics == "" {
			return miniMaxMusicPayload{}, fmt.Errorf("%w: lyrics are required for vocal music", adapter.ErrInvalidRequest)
		}
		payload.Lyrics = lyrics
	case adapter.MusicModeAutoLyrics:
		if prompt == "" || lyrics != "" {
			return miniMaxMusicPayload{}, fmt.Errorf("%w: prompt is required and lyrics must be empty", adapter.ErrInvalidRequest)
		}
		payload.LyricsOptimizer = true
	case adapter.MusicModeInstrumental:
		if prompt == "" || lyrics != "" {
			return miniMaxMusicPayload{}, fmt.Errorf("%w: prompt is required and lyrics must be empty", adapter.ErrInvalidRequest)
		}
		payload.IsInstrumental = true
	default:
		return miniMaxMusicPayload{}, fmt.Errorf("%w: unsupported music mode", adapter.ErrInvalidRequest)
	}
	return payload, nil
}

func validateMiniMaxMusicResponse(statusCode int, response miniMaxMusicResponse) error {
	if response.Error != nil {
		code := strings.Trim(strings.TrimSpace(string(response.Error.Code)), `"`)
		return adapter.NewAdapterError(code, strings.TrimSpace(response.Error.Message), statusCode, adapter.ErrUpstreamError)
	}
	if response.BaseResp != nil && response.BaseResp.StatusCode != 0 {
		return newMiniMaxAdapterError(statusCode, response.BaseResp.StatusCode, response.BaseResp.StatusMsg)
	}
	return nil
}

func miniMaxMusicError(statusCode int, body []byte) error {
	var response miniMaxMusicResponse
	if err := json.Unmarshal(body, &response); err == nil {
		if response.Error != nil || response.BaseResp != nil {
			if validationErr := validateMiniMaxMusicResponse(statusCode, response); validationErr != nil {
				return validationErr
			}
		}
	}
	return adapter.HandleNonJSONError(statusCode, body)
}

func newMiniMaxAdapterError(statusCode, code int, message string) error {
	return adapter.NewAdapterError(strconv.Itoa(code), strings.TrimSpace(message), statusCode, adapter.ErrUpstreamError)
}

func readMiniMaxSSE(body io.Reader, consume func(string) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64<<10), int(adapter.MaxGeneratedMusicBytes*2+(1<<20)))
	var dataLines []string
	dispatch := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		return consume(data)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if dispatchErr := dispatch(); dispatchErr != nil {
				return dispatchErr
			}
		} else if data, ok := strings.CutPrefix(line, "data:"); ok {
			dataLines = append(dataLines, strings.TrimSpace(data))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read minimax music stream: %w", err)
	}
	return dispatch()
}

func writeMiniMaxHexAudio(dst *miniMaxMusicSizeWriter, encoded string) error {
	if int64(hex.DecodedLen(len(encoded))) > adapter.MaxGeneratedMusicBytes-dst.written {
		return adapter.ErrMusicResponseTooLarge
	}
	audio := make([]byte, hex.DecodedLen(len(encoded)))
	written, err := hex.Decode(audio, []byte(encoded))
	if err != nil {
		return fmt.Errorf("decode minimax music audio: %w", err)
	}
	if _, err := dst.Write(audio[:written]); err != nil {
		return fmt.Errorf("write minimax music audio: %w", err)
	}
	return nil
}

type miniMaxMusicSizeWriter struct {
	dst     io.Writer
	written int64
}

func (w *miniMaxMusicSizeWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > adapter.MaxGeneratedMusicBytes-w.written {
		return 0, adapter.ErrMusicResponseTooLarge
	}
	n, err := w.dst.Write(p)
	w.written += int64(n)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

func splitMiniMaxStyleTags(value string) []string {
	parts := strings.Split(value, ",")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		if tag := strings.TrimSpace(part); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}
