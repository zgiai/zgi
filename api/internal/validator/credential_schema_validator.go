package validator

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
)

// CredentialFormSchema represents a credential form schema
type CredentialFormSchema struct {
	Variable    string      `json:"variable"`
	Label       interface{} `json:"label"` // I18n object
	Type        string      `json:"type"`
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Options     []Option    `json:"options,omitempty"`
	Placeholder interface{} `json:"placeholder,omitempty"` // I18n object
	MaxLength   *int        `json:"max_length,omitempty"`
	ShowOn      []ShowOn    `json:"show_on,omitempty"`
}

// Option represents a select option
type Option struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ShowOn represents conditional showing
type ShowOn struct {
	Variable string `json:"variable"`
	Value    string `json:"value"`
}

// ProviderCredentialSchema represents provider credential schema
type ProviderCredentialSchema struct {
	CredentialFormSchemas []CredentialFormSchema `json:"credential_form_schemas"`
}

// ModelCredentialSchema represents model credential schema
type ModelCredentialSchema struct {
	Model                 string                 `json:"model"`
	CredentialFormSchemas []CredentialFormSchema `json:"credential_form_schemas"`
}

// ProviderCredentialSchemaValidator validates provider credentials
type ProviderCredentialSchemaValidator struct {
	schema ProviderCredentialSchema
}

// NewProviderCredentialSchemaValidator creates a new provider credential schema validator
func NewProviderCredentialSchemaValidator(schema ProviderCredentialSchema) *ProviderCredentialSchemaValidator {
	return &ProviderCredentialSchemaValidator{schema: schema}
}

// ValidateAndFilter validates provider credentials and returns filtered credentials
func (v *ProviderCredentialSchemaValidator) ValidateAndFilter(credentials map[string]interface{}) (map[string]interface{}, error) {
	logger.Debug("Credential schema validation started",
		zap.Int("credential_field_count", len(credentials)),
		zap.Strings("credential_fields", credentialFieldNames(credentials)),
		zap.Int("schema_field_count", len(v.schema.CredentialFormSchemas)),
		zap.Strings("schema_fields", schemaFieldNames(v.schema.CredentialFormSchemas)),
	)

	result, err := v.validateCredentialFormSchemas(v.schema.CredentialFormSchemas, credentials)
	if err != nil {
		logger.Warn("Credential schema validation failed", zap.Error(err))
		return nil, err
	}

	logger.Debug("Credential schema validation succeeded",
		zap.Int("filtered_field_count", len(result)),
		zap.Strings("filtered_fields", credentialFieldNames(result)),
	)
	return result, nil
}

// validateCredentialFormSchemas validates credentials against form schemas for ProviderCredentialSchemaValidator
func (v *ProviderCredentialSchemaValidator) validateCredentialFormSchemas(schemas []CredentialFormSchema, credentials map[string]interface{}) (map[string]interface{}, error) {
	logger.Debug("Validating credential fields",
		zap.Int("credential_field_count", len(credentials)),
		zap.Strings("credential_fields", credentialFieldNames(credentials)),
		zap.Int("schema_field_count", len(schemas)),
	)

	filtered := make(map[string]interface{})
	schemaMap := make(map[string]CredentialFormSchema)

	// Build schema map
	for _, schema := range schemas {
		logger.Debug("Credential schema field loaded",
			zap.String("field", schema.Variable),
			zap.String("field_type", schema.Type),
			zap.Bool("required", schema.Required),
			zap.Bool("has_default", schema.Default != nil),
			zap.Int("option_count", len(schema.Options)),
		)
		schemaMap[schema.Variable] = schema
	}

	// Validate provided credentials
	for key, value := range credentials {
		logger.Debug("Validating credential field",
			zap.String("field", key),
			zap.String("value_type", credentialValueType(value)),
		)

		schema, exists := schemaMap[key]
		if !exists {
			logger.Debug("Unknown credential field skipped", zap.String("field", key))
			// Skip unknown fields but don't fail
			continue
		}

		// Validate field type and constraints
		if err := v.validateField(schema, value); err != nil {
			logger.Warn("Credential field validation failed",
				zap.String("field", key),
				zap.String("field_type", schema.Type),
				zap.Error(err),
			)
			return nil, fmt.Errorf("validation failed for field '%s': %w", key, err)
		}

		logger.Debug("Credential field validation passed", zap.String("field", key))
		filtered[key] = value
		delete(schemaMap, key)
	}

	// Check required fields
	for variable, schema := range schemaMap {
		if schema.Required {
			logger.Warn("Required credential field missing", zap.String("field", variable))
			return nil, fmt.Errorf("required field '%s' is missing", variable)
		}

		// Set default value if provided
		if schema.Default != nil {
			logger.Debug("Credential default value applied", zap.String("field", variable))
			filtered[variable] = schema.Default
		}
	}

	logger.Debug("Credential schema validation completed",
		zap.Int("filtered_field_count", len(filtered)),
		zap.Strings("filtered_fields", credentialFieldNames(filtered)),
	)
	return filtered, nil
}

func credentialFieldNames(credentials map[string]interface{}) []string {
	fields := make([]string, 0, len(credentials))
	for key := range credentials {
		fields = append(fields, key)
	}
	sort.Strings(fields)
	return fields
}

func schemaFieldNames(schemas []CredentialFormSchema) []string {
	fields := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		fields = append(fields, schema.Variable)
	}
	sort.Strings(fields)
	return fields
}

func credentialValueType(value interface{}) string {
	if value == nil {
		return "nil"
	}
	return fmt.Sprintf("%T", value)
}

// validateField validates a single field against its schema for ProviderCredentialSchemaValidator
func (v *ProviderCredentialSchemaValidator) validateField(schema CredentialFormSchema, value interface{}) error {
	// Check if value is nil and field is required
	if value == nil && schema.Required {
		return fmt.Errorf("field is required but got nil")
	}

	if value == nil {
		return nil // Optional field with nil value is OK
	}

	switch schema.Type {
	case "secret-input", "text-input", "textarea":
		return v.validateStringField(schema, value)
	case "select":
		return v.validateSelectField(schema, value)
	case "boolean":
		return v.validateBooleanField(schema, value)
	case "number":
		return v.validateNumberField(schema, value)
	default:
		// Unknown type, just accept it
		return nil
	}
}

// validateStringField validates string fields for ProviderCredentialSchemaValidator
func (v *ProviderCredentialSchemaValidator) validateStringField(schema CredentialFormSchema, value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", value)
	}

	if schema.MaxLength != nil && len(str) > *schema.MaxLength {
		return fmt.Errorf("string length %d exceeds maximum %d", len(str), *schema.MaxLength)
	}

	return nil
}

// validateSelectField validates select fields for ProviderCredentialSchemaValidator
func (v *ProviderCredentialSchemaValidator) validateSelectField(schema CredentialFormSchema, value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("expected string for select field, got %T", value)
	}

	// Check if value is in options
	for _, option := range schema.Options {
		if option.Value == str {
			return nil
		}
	}

	validValues := make([]string, len(schema.Options))
	for i, option := range schema.Options {
		validValues[i] = option.Value
	}

	return fmt.Errorf("value '%s' is not valid, expected one of: %s", str, strings.Join(validValues, ", "))
}

// validateBooleanField validates boolean fields for ProviderCredentialSchemaValidator
func (v *ProviderCredentialSchemaValidator) validateBooleanField(schema CredentialFormSchema, value interface{}) error {
	_, ok := value.(bool)
	if !ok {
		return fmt.Errorf("expected boolean, got %T", value)
	}
	return nil
}

// validateNumberField validates number fields for ProviderCredentialSchemaValidator
func (v *ProviderCredentialSchemaValidator) validateNumberField(schema CredentialFormSchema, value interface{}) error {
	switch reflect.TypeOf(value).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return nil
	default:
		return fmt.Errorf("expected number, got %T", value)
	}
}

// ModelCredentialSchemaValidator validates model credentials
type ModelCredentialSchemaValidator struct {
	modelType string
	schema    ModelCredentialSchema
}

// NewModelCredentialSchemaValidator creates a new model credential schema validator
func NewModelCredentialSchemaValidator(modelType string, schema ModelCredentialSchema) *ModelCredentialSchemaValidator {
	return &ModelCredentialSchemaValidator{
		modelType: modelType,
		schema:    schema,
	}
}

// ValidateAndFilter validates model credentials and returns filtered credentials
func (v *ModelCredentialSchemaValidator) ValidateAndFilter(credentials map[string]interface{}) (map[string]interface{}, error) {
	// Add model type to credentials
	filtered := make(map[string]interface{})
	for k, val := range credentials {
		filtered[k] = val
	}
	filtered["__model_type"] = v.modelType

	return v.validateCredentialFormSchemas(v.schema.CredentialFormSchemas, filtered)
}

// validateCredentialFormSchemas validates credentials against form schemas for ModelCredentialSchemaValidator
func (v *ModelCredentialSchemaValidator) validateCredentialFormSchemas(schemas []CredentialFormSchema, credentials map[string]interface{}) (map[string]interface{}, error) {
	filtered := make(map[string]interface{})
	schemaMap := make(map[string]CredentialFormSchema)

	// Build schema map
	for _, schema := range schemas {
		schemaMap[schema.Variable] = schema
	}

	// Validate provided credentials
	for key, value := range credentials {
		schema, exists := schemaMap[key]
		if !exists {
			// Skip unknown fields but don't fail
			continue
		}

		// Validate field type and constraints
		if err := v.validateField(schema, value); err != nil {
			return nil, fmt.Errorf("validation failed for field '%s': %w", key, err)
		}

		filtered[key] = value
		delete(schemaMap, key)
	}

	// Check required fields
	for variable, schema := range schemaMap {
		if schema.Required {
			return nil, fmt.Errorf("required field '%s' is missing", variable)
		}

		// Set default value if provided
		if schema.Default != nil {
			filtered[variable] = schema.Default
		}
	}

	return filtered, nil
}

// validateField validates a single field against its schema for ModelCredentialSchemaValidator
func (v *ModelCredentialSchemaValidator) validateField(schema CredentialFormSchema, value interface{}) error {
	// Check if value is nil and field is required
	if value == nil && schema.Required {
		return fmt.Errorf("field is required but got nil")
	}

	if value == nil {
		return nil // Optional field with nil value is OK
	}

	switch schema.Type {
	case "secret-input", "text-input", "textarea":
		return v.validateStringField(schema, value)
	case "select":
		return v.validateSelectField(schema, value)
	case "boolean":
		return v.validateBooleanField(schema, value)
	case "number":
		return v.validateNumberField(schema, value)
	default:
		// Unknown type, just accept it
		return nil
	}
}

// validateStringField validates string fields for ModelCredentialSchemaValidator
func (v *ModelCredentialSchemaValidator) validateStringField(schema CredentialFormSchema, value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", value)
	}

	if schema.MaxLength != nil && len(str) > *schema.MaxLength {
		return fmt.Errorf("string length %d exceeds maximum %d", len(str), *schema.MaxLength)
	}

	return nil
}

// validateSelectField validates select fields for ModelCredentialSchemaValidator
func (v *ModelCredentialSchemaValidator) validateSelectField(schema CredentialFormSchema, value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return fmt.Errorf("expected string for select field, got %T", value)
	}

	// Check if value is in options
	for _, option := range schema.Options {
		if option.Value == str {
			return nil
		}
	}

	validValues := make([]string, len(schema.Options))
	for i, option := range schema.Options {
		validValues[i] = option.Value
	}

	return fmt.Errorf("value '%s' is not valid, expected one of: %s", str, strings.Join(validValues, ", "))
}

// validateBooleanField validates boolean fields for ModelCredentialSchemaValidator
func (v *ModelCredentialSchemaValidator) validateBooleanField(schema CredentialFormSchema, value interface{}) error {
	_, ok := value.(bool)
	if !ok {
		return fmt.Errorf("expected boolean, got %T", value)
	}
	return nil
}

// validateNumberField validates number fields for ModelCredentialSchemaValidator
func (v *ModelCredentialSchemaValidator) validateNumberField(schema CredentialFormSchema, value interface{}) error {
	switch reflect.TypeOf(value).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return nil
	default:
		return fmt.Errorf("expected number, got %T", value)
	}
}
