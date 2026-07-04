package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
)

type memoryProviderSettingsRepo struct {
	item      *model.ProviderConfig
	upserted  bool
	updated   bool
	updateErr error
}

func (r *memoryProviderSettingsRepo) Create(context.Context, *model.ProviderConfig) error {
	return nil
}

func (r *memoryProviderSettingsRepo) GetByID(context.Context, uuid.UUID) (*model.ProviderConfig, error) {
	return nil, nil
}

func (r *memoryProviderSettingsRepo) GetByScopeAndKey(context.Context, string, *uuid.UUID, *uuid.UUID, string) (*model.ProviderConfig, error) {
	if r.item == nil {
		return nil, nil
	}
	clone := *r.item
	clone.CredentialsCiphertext = cloneMap(r.item.CredentialsCiphertext)
	clone.Metadata = cloneMap(r.item.Metadata)
	return &clone, nil
}

func (r *memoryProviderSettingsRepo) ListByScope(context.Context, string, *uuid.UUID, *uuid.UUID) ([]*model.ProviderConfig, error) {
	return nil, nil
}

func (r *memoryProviderSettingsRepo) UpsertByScopeAndKey(_ context.Context, item *model.ProviderConfig) error {
	r.upserted = true
	clone := *item
	clone.CredentialsCiphertext = cloneMap(item.CredentialsCiphertext)
	clone.Metadata = cloneMap(item.Metadata)
	r.item = &clone
	return nil
}

func (r *memoryProviderSettingsRepo) Update(_ context.Context, item *model.ProviderConfig) error {
	r.updated = true
	if r.updateErr != nil {
		return r.updateErr
	}
	clone := *item
	clone.CredentialsCiphertext = cloneMap(item.CredentialsCiphertext)
	clone.Metadata = cloneMap(item.Metadata)
	r.item = &clone
	return nil
}

func (r *memoryProviderSettingsRepo) Delete(context.Context, uuid.UUID) error {
	return nil
}

type passthroughCrypto struct{}

func (passthroughCrypto) Encrypt(plaintext string) (string, error)  { return plaintext, nil }
func (passthroughCrypto) Decrypt(ciphertext string) (string, error) { return ciphertext, nil }
func (passthroughCrypto) Hash(input string) string                  { return input }

type recordingParserValidator struct {
	err      error
	requests []ParserProviderValidationRequest
}

func (v *recordingParserValidator) Validate(_ context.Context, req ParserProviderValidationRequest) error {
	v.requests = append(v.requests, req)
	return v.err
}

func TestProviderSettingsUpsertValidatesEnabledReductoBeforeSave(t *testing.T) {
	repo := &memoryProviderSettingsRepo{}
	validator := &recordingParserValidator{}
	svc := NewProviderSettingsService(repo, passthroughCrypto{}, validator)
	enabled := true
	apiKey := "rk-live"

	item, err := svc.Upsert(context.Background(), uuid.New(), nil, ParserProviderReducto, ParserSettingsInput{
		Enabled: &enabled,
		APIKey:  &apiKey,
	})
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	if !repo.upserted {
		t.Fatal("expected provider config to be saved after validation")
	}
	if len(validator.requests) != 1 {
		t.Fatalf("validator calls = %d, want 1", len(validator.requests))
	}
	if validator.requests[0].APIKey != apiKey {
		t.Fatalf("validator API key = %q, want %q", validator.requests[0].APIKey, apiKey)
	}
	if item.Status != "available" || item.ValidationStatus != ParserValidationSuccess {
		t.Fatalf("status=%q validation=%q", item.Status, item.ValidationStatus)
	}
}

func TestProviderSettingsCheckRecordsFailedValidation(t *testing.T) {
	orgID := uuid.New()
	repo := &memoryProviderSettingsRepo{item: &model.ProviderConfig{
		ID:                    uuid.New(),
		Scope:                 "organization",
		OrganizationID:        &orgID,
		ProviderKey:           ParserProviderReducto,
		DisplayName:           "Reducto",
		Enabled:               true,
		BaseURL:               DefaultReductoBaseURL,
		CredentialsCiphertext: map[string]any{"api_key": "bad"},
		Metadata:              map[string]any{parserValidationStatusKey: ParserValidationSuccess},
	}}
	validator := &recordingParserValidator{err: errors.New("token rejected")}
	svc := NewProviderSettingsService(repo, passthroughCrypto{}, validator)

	_, err := svc.Check(context.Background(), orgID, nil, ParserProviderReducto)
	if !errors.Is(err, ErrParserValidationFailed) {
		t.Fatalf("Check() error = %v, want ErrParserValidationFailed", err)
	}
	if !repo.updated {
		t.Fatal("expected failed validation state to be saved")
	}
	if got := metadataString(repo.item.Metadata, parserValidationStatusKey); got != ParserValidationFailed {
		t.Fatalf("validation status = %q, want %q", got, ParserValidationFailed)
	}
	if got := metadataString(repo.item.Metadata, parserValidationMessageKey); got == "" {
		t.Fatal("expected validation failure message")
	}
}
