package socialops

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
)

const (
	CurrentConfigurationRecordKind = "configuration"
	CurrentConfigurationRecordKey  = "current"
)

// SaveCurrentConfiguration stores the canonical secret-free configuration in
// the existing scoped artifact journal. The journal CAS revision, rather than
// a caller-supplied value inside the JSON, becomes the configuration revision.
func (s *Service) SaveCurrentConfiguration(ctx context.Context, scope Scope, expectedRevision int64, configuration Configuration) (Configuration, Record, error) {
	if s == nil || !validScope(scope) || configuration.Scope != scope || expectedRevision < 0 {
		return Configuration{}, Record{}, invalid()
	}
	configuration.Revision = expectedRevision + 1
	if err := configuration.Validate(); err != nil {
		return Configuration{}, Record{}, err
	}
	data, err := json.Marshal(configuration)
	if err != nil {
		return Configuration{}, Record{}, err
	}
	artifact, err := s.PutArtifact(ctx, scope, "application/json", data)
	if err != nil {
		return Configuration{}, Record{}, err
	}
	record, err := s.SaveRecord(ctx, scope, CurrentConfigurationRecordKind, CurrentConfigurationRecordKey, expectedRevision, artifact.ID)
	if err != nil {
		return Configuration{}, Record{}, err
	}
	if record.Revision != configuration.Revision {
		return Configuration{}, Record{}, conflict()
	}
	return configuration, record, nil
}

// LoadCurrentConfiguration restores the configuration through the scoped
// journal and rejects undeclared authority fields or mismatched revisions.
func (s *Service) LoadCurrentConfiguration(ctx context.Context, scope Scope) (Configuration, Record, error) {
	if s == nil || !validScope(scope) {
		return Configuration{}, Record{}, invalid()
	}
	record, err := s.GetRecord(ctx, scope, CurrentConfigurationRecordKind, CurrentConfigurationRecordKey)
	if err != nil {
		return Configuration{}, Record{}, err
	}
	artifact, err := s.GetArtifact(ctx, scope, record.ArtifactID)
	if err != nil {
		return Configuration{}, Record{}, err
	}
	if artifact.MIME != "application/json" || artifact.Size <= 0 || artifact.Size > MaxArtifactBytes {
		return Configuration{}, Record{}, invalid()
	}
	data, err := s.ReadArtifact(ctx, scope, artifact.ID)
	if err != nil {
		return Configuration{}, Record{}, err
	}
	var configuration Configuration
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&configuration); err != nil {
		return Configuration{}, Record{}, invalid()
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return Configuration{}, Record{}, invalid()
	}
	if configuration.Scope != scope || configuration.Revision != record.Revision {
		return Configuration{}, Record{}, invalid()
	}
	if err = configuration.Validate(); err != nil {
		return Configuration{}, Record{}, err
	}
	return configuration, record, nil
}
