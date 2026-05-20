package local

import (
	"os"
	"strings"
	"time"

	localocr "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/ocr"
)

type OCREngineStatus struct {
	Key       string `json:"key"`
	Provider  string `json:"provider,omitempty"`
	Available bool   `json:"available"`
	Default   bool   `json:"default,omitempty"`
	Path      string `json:"path,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

func OCREngineStatuses() []OCREngineStatus {
	cfg := localocr.LoadConfig(10 * time.Second)
	defaultEngine := cfg.EngineName()
	tesseractStatus := ocrCommandStatus(localocr.EngineTesseract, cfg.TesseractPath, "tesseract")
	paddleStatus := ocrCommandStatus(localocr.EnginePaddleOCR, cfg.PaddleCommand, "paddleocr")
	tesseractStatus.Default = defaultEngine == localocr.EngineTesseract
	paddleStatus.Default = defaultEngine == localocr.EnginePaddleOCR

	autoAvailable := tesseractStatus.Available || paddleStatus.Available
	autoProvider := defaultEngine
	if autoProvider == localocr.EngineTesseract && !tesseractStatus.Available && paddleStatus.Available {
		autoProvider = localocr.EnginePaddleOCR
	}
	if autoProvider == localocr.EnginePaddleOCR && !paddleStatus.Available && tesseractStatus.Available {
		autoProvider = localocr.EngineTesseract
	}
	auto := OCREngineStatus{
		Key:       "auto",
		Provider:  autoProvider,
		Available: autoAvailable,
		Default:   true,
	}
	if !autoAvailable {
		auto.Reason = "no local OCR command is available"
	} else {
		auto.Reason = "uses the configured local OCR engine"
	}

	return []OCREngineStatus{auto, tesseractStatus, paddleStatus}
}

func ocrCommandStatus(key string, configuredCommand string, fallbackCommand string) OCREngineStatus {
	status := OCREngineStatus{
		Key:      key,
		Provider: key,
	}
	command := strings.TrimSpace(configuredCommand)
	if command == "" {
		command = fallbackCommand
	}
	if command == "" {
		status.Reason = "command is not configured"
		return status
	}
	path, err := resolveOCRCommand(command)
	if err != nil {
		status.Reason = err.Error()
		return status
	}
	status.Available = true
	status.Path = path
	status.Reason = "command is available"
	return status
}

func resolveOCRCommand(command string) (string, error) {
	if strings.ContainsAny(command, `/\`) {
		if _, err := os.Stat(command); err != nil {
			return "", err
		}
		return command, nil
	}
	return localocr.ResolveCommand(command)
}
