package music

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	musicPlaybackMetadataTimeout = 15 * time.Second
	musicWaveformPeakCount       = 160
	musicWaveformSampleRate      = 8000
)

type musicPlaybackMetadata struct {
	DurationMS    int64
	WaveformPeaks []int16
}

var extractGeneratedMusicPlaybackMetadata = extractMusicPlaybackMetadata

func extractMusicPlaybackMetadata(ctx context.Context, audio []byte) (musicPlaybackMetadata, error) {
	if len(audio) == 0 {
		return musicPlaybackMetadata{}, errors.New("music audio is empty")
	}

	metadataCtx, cancel := context.WithTimeout(ctx, musicPlaybackMetadataTimeout)
	defer cancel()

	tempDir, err := os.MkdirTemp("", "zgi-music-metadata-*")
	if err != nil {
		return musicPlaybackMetadata{}, fmt.Errorf("create music metadata temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	inputPath := filepath.Join(tempDir, "music.mp3")
	if err := os.WriteFile(inputPath, audio, 0o600); err != nil {
		return musicPlaybackMetadata{}, fmt.Errorf("write music metadata input: %w", err)
	}

	durationMS, durationErr := probeMusicDuration(metadataCtx, inputPath)
	peaks, sampleDurationMS, peaksErr := extractMusicWaveformPeaks(metadataCtx, inputPath, durationMS)
	if peaksErr != nil {
		if durationMS > 0 {
			return musicPlaybackMetadata{DurationMS: durationMS}, peaksErr
		}
		if durationErr != nil {
			return musicPlaybackMetadata{}, errors.Join(durationErr, peaksErr)
		}
		return musicPlaybackMetadata{}, peaksErr
	}
	if durationMS <= 0 {
		durationMS = sampleDurationMS
	}
	if durationMS <= 0 && len(peaks) == 0 {
		return musicPlaybackMetadata{}, errors.New("music metadata is empty")
	}
	return musicPlaybackMetadata{DurationMS: durationMS, WaveformPeaks: peaks}, nil
}

func probeMusicDuration(ctx context.Context, inputPath string) (int64, error) {
	cmd := exec.CommandContext(ctx,
		"ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return 0, commandError("probe music duration", err, stderr.String())
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil || !isPositiveFinite(seconds) {
		return 0, fmt.Errorf("probe music duration returned invalid value %q", strings.TrimSpace(string(output)))
	}
	return int64(math.Round(seconds * 1000)), nil
}

func extractMusicWaveformPeaks(ctx context.Context, inputPath string, durationMS int64) ([]int16, int64, error) {
	cmd := exec.CommandContext(ctx,
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputPath,
		"-vn",
		"-ac", "1",
		"-ar", strconv.Itoa(musicWaveformSampleRate),
		"-f", "s16le",
		"pipe:1",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, fmt.Errorf("open music waveform pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, 0, commandError("start music waveform extraction", err, stderr.String())
	}

	knownSampleCount := int64(0)
	if durationMS > 0 {
		knownSampleCount = int64(math.Ceil(float64(durationMS) * float64(musicWaveformSampleRate) / 1000))
	}
	rawPeaks := make([]int, musicWaveformPeakCount)
	var samples []int
	var sampleIndex int64
	var pending []byte
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := stdout.Read(buffer)
		if n > 0 {
			chunk := buffer[:n]
			if len(pending) > 0 {
				chunk = append(append([]byte(nil), pending...), chunk...)
				pending = nil
			}
			if len(chunk)%2 == 1 {
				pending = append(pending[:0], chunk[len(chunk)-1])
				chunk = chunk[:len(chunk)-1]
			}
			for offset := 0; offset < len(chunk); offset += 2 {
				peak := absPCM16(int16(binary.LittleEndian.Uint16(chunk[offset : offset+2])))
				if knownSampleCount > 0 {
					bucket := int((sampleIndex * musicWaveformPeakCount) / knownSampleCount)
					if bucket >= musicWaveformPeakCount {
						bucket = musicWaveformPeakCount - 1
					}
					if peak > rawPeaks[bucket] {
						rawPeaks[bucket] = peak
					}
				} else {
					samples = append(samples, peak)
				}
				sampleIndex++
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, os.ErrClosed) {
				_ = cmd.Wait()
				return nil, 0, fmt.Errorf("read music waveform pcm: %w", readErr)
			}
			break
		}
	}

	waitErr := cmd.Wait()
	if waitErr != nil {
		return nil, 0, commandError("extract music waveform", waitErr, stderr.String())
	}
	if sampleIndex == 0 {
		return nil, 0, errors.New("music waveform extraction returned no samples")
	}
	if knownSampleCount <= 0 {
		rawPeaks = buildMusicWaveformPeaksFromSamples(samples, musicWaveformPeakCount)
	}
	sampleDurationMS := int64(math.Round(float64(sampleIndex) * 1000 / float64(musicWaveformSampleRate)))
	return normalizeMusicWaveformPeaks(rawPeaks), sampleDurationMS, nil
}

func buildMusicWaveformPeaksFromSamples(samples []int, peakCount int) []int {
	if len(samples) == 0 || peakCount <= 0 {
		return nil
	}
	peakCount = min(peakCount, len(samples))
	peaks := make([]int, peakCount)
	for peakIndex := 0; peakIndex < peakCount; peakIndex++ {
		start := (peakIndex * len(samples)) / peakCount
		end := max(start+1, ((peakIndex+1)*len(samples))/peakCount)
		for _, sample := range samples[start:end] {
			if sample > peaks[peakIndex] {
				peaks[peakIndex] = sample
			}
		}
	}
	return peaks
}

func normalizeMusicWaveformPeaks(rawPeaks []int) []int16 {
	if len(rawPeaks) == 0 {
		return nil
	}
	maxPeak := 0
	for _, peak := range rawPeaks {
		if peak > maxPeak {
			maxPeak = peak
		}
	}
	normalized := make([]int16, len(rawPeaks))
	if maxPeak <= 0 {
		return normalized
	}
	for index, peak := range rawPeaks {
		normalized[index] = int16(math.Round(float64(peak) / float64(maxPeak) * 100))
	}
	return normalized
}

func absPCM16(sample int16) int {
	if sample == math.MinInt16 {
		return math.MaxInt16 + 1
	}
	if sample < 0 {
		return int(-sample)
	}
	return int(sample)
}

func isPositiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func commandError(operation string, err error, stderr string) error {
	message := strings.TrimSpace(stderr)
	if message == "" {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return fmt.Errorf("%s: %w: %s", operation, err, message)
}
