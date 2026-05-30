package handler

import (
	"errors"
	"testing"

	datalibrarymodel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
)

func TestNormalizeUploadProcessingMode(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "defaults to process now", raw: "", want: UploadProcessingModeProcessNow, ok: true},
		{name: "trims whitespace", raw: "  store_only  ", want: UploadProcessingModeStoreOnly, ok: true},
		{name: "accepts process now", raw: UploadProcessingModeProcessNow, want: UploadProcessingModeProcessNow, ok: true},
		{name: "rejects invalid", raw: "parse_later", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeUploadProcessingMode(tt.raw)
			if ok != tt.ok {
				t.Fatalf("ok=%v want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("mode=%q want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeFileProcessingRequestMode(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "defaults to parse now", raw: "", want: FileProcessingRequestModeParseNow, ok: true},
		{name: "accepts reparse", raw: FileProcessingRequestModeReparse, want: FileProcessingRequestModeReparse, ok: true},
		{name: "accepts generate after confirm", raw: " generate_after_confirm ", want: FileProcessingRequestModeGenerateAfterConfirm, ok: true},
		{name: "rejects invalid", raw: "archive", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeFileProcessingRequestMode(tt.raw)
			if ok != tt.ok {
				t.Fatalf("ok=%v want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("mode=%q want %q", got, tt.want)
			}
		})
	}
}

func TestValidateFileProcessingRequestState(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		mode    string
		force   bool
		wantErr error
	}{
		{
			name:   "parse now from stored only",
			status: datalibrarymodel.DocumentAssetProductStatusStoredOnly,
			mode:   FileProcessingRequestModeParseNow,
		},
		{
			name:   "reparse from ready",
			status: datalibrarymodel.DocumentAssetProductStatusReady,
			mode:   FileProcessingRequestModeReparse,
		},
		{
			name:   "generate after confirm from confirming",
			status: datalibrarymodel.DocumentAssetProductStatusConfirming,
			mode:   FileProcessingRequestModeGenerateAfterConfirm,
		},
		{
			name:    "reject duplicate parsing without force",
			status:  datalibrarymodel.DocumentAssetProductStatusParsing,
			mode:    FileProcessingRequestModeParseNow,
			wantErr: errFileProcessingRequestAlreadyActive,
		},
		{
			name:   "allow duplicate parsing with force",
			status: datalibrarymodel.DocumentAssetProductStatusParsing,
			mode:   FileProcessingRequestModeParseNow,
			force:  true,
		},
		{
			name:    "reject parse now from ready",
			status:  datalibrarymodel.DocumentAssetProductStatusReady,
			mode:    FileProcessingRequestModeParseNow,
			wantErr: errFileProcessingRequestStateInvalid,
		},
		{
			name:    "reject generate from stored only",
			status:  datalibrarymodel.DocumentAssetProductStatusStoredOnly,
			mode:    FileProcessingRequestModeGenerateAfterConfirm,
			wantErr: errFileProcessingRequestStateInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFileProcessingRequestState(&datalibrarymodel.DocumentAsset{ProductStatus: tt.status}, tt.mode, tt.force)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err=%v want %v", err, tt.wantErr)
			}
		})
	}
}
