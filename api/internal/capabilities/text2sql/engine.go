package text2sql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/pkg/sql_base/guard"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlanalysis"
)

type Generator interface {
	Generate(ctx context.Context, request GenerationRequest) (string, error)
}

type Retriever interface {
	Retrieve(ctx context.Context, request Request) (SchemaSelection, error)
}

type GenerationRequest struct {
	Question         string
	Schema           string
	Model            Model
	AdditionalPrompt string
	Repair           *RepairRequest
}

type RepairRequest struct {
	PreviousResponse string
	Failure          string
}

type Engine struct {
	catalog   CatalogLoader
	generator Generator
	retriever Retriever
}

func NewEngine(catalog CatalogLoader, generator Generator, retriever Retriever) *Engine {
	return &Engine{catalog: catalog, generator: generator, retriever: retriever}
}

func (e *Engine) Generate(ctx context.Context, req Request) (*Result, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	selection, err := e.selectTables(ctx, req)
	if err != nil {
		return nil, err
	}
	catalog, err := e.loadCatalog(ctx, req, selection.Selected)
	if err != nil {
		return nil, err
	}
	selection.Selected = catalogTableRefs(catalog)
	if req.SelectionMode == SelectionModeManual {
		selection.Candidates = selection.Selected
	}
	schema := renderCatalog(catalog, req.SchemaRender)
	started := time.Now()
	raw, err := e.generator.Generate(ctx, GenerationRequest{Question: req.Question, Schema: schema, Model: req.GenerationModel, AdditionalPrompt: req.AdditionalPrompt})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGeneration, err)
	}
	parsed, validationErr := parseAndValidate(raw, catalog, req.GuardPolicy)
	repairCount := 0
	if validationErr != nil && req.RepairLimit == 1 && repairable(validationErr) {
		repairCount = 1
		raw, err = e.generator.Generate(ctx, GenerationRequest{
			Question:         req.Question,
			Schema:           schema,
			Model:            req.GenerationModel,
			AdditionalPrompt: req.AdditionalPrompt,
			Repair:           &RepairRequest{PreviousResponse: raw, Failure: validationErr.Error()},
		})
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrGeneration, err)
		}
		parsed, validationErr = parseAndValidate(raw, catalog, req.GuardPolicy)
	}
	if validationErr != nil {
		return nil, validationErr
	}
	duration := time.Since(started)
	return &Result{
		SQL:             parsed.SQL,
		RawResponse:     raw,
		AnalysisText:    parsed.Analysis,
		UsedFields:      parsed.UsedFields,
		Structured:      parsed.Structured,
		SchemaSelection: selection,
		Generation:      GenerationInfo{Model: req.GenerationModel.Name, Attempts: 1 + repairCount, Duration: duration, DurationMS: duration.Milliseconds()},
		Validation:      parsed.Validation,
		RepairCount:     repairCount,
	}, nil
}

func catalogTableRefs(catalog Catalog) []TableRef {
	refs := make([]TableRef, 0, len(catalog.Tables))
	for _, table := range catalog.Tables {
		refs = append(refs, table.Ref)
	}
	return refs
}

func validateRequest(req Request) error {
	if strings.TrimSpace(req.DataSourceID) == "" || strings.TrimSpace(req.Question) == "" || strings.TrimSpace(req.GenerationModel.Name) == "" {
		return ErrInvalidInput
	}
	if req.RepairLimit < 0 || req.RepairLimit > 1 {
		return ErrInvalidInput
	}
	switch req.SelectionMode {
	case SelectionModeManual:
		if len(req.ManualTables) == 0 {
			return ErrInvalidInput
		}
	case SelectionModeAutomatic:
	default:
		return ErrInvalidInput
	}
	return nil
}

func (e *Engine) selectTables(ctx context.Context, req Request) (SchemaSelection, error) {
	if req.SelectionMode == SelectionModeManual {
		return SchemaSelection{Candidates: req.ManualTables, Selected: req.ManualTables}, nil
	}
	if e.retriever == nil {
		return SchemaSelection{}, ErrIndexNotReady
	}
	return e.retriever.Retrieve(ctx, req)
}

func (e *Engine) loadCatalog(ctx context.Context, req Request, refs []TableRef) (Catalog, error) {
	if e.catalog == nil {
		return Catalog{}, ErrSchemaLoad
	}
	return e.catalog.Load(ctx, CatalogRequest{Scope: req.Scope, DataSourceID: req.DataSourceID, Tables: refs})
}

type strictResponse struct {
	SQL        string   `json:"sql"`
	Analysis   string   `json:"analysis,omitempty"`
	UsedFields []string `json:"used_fields,omitempty"`
}

type validatedResponse struct {
	strictResponse
	Structured map[string]any
	Validation ValidationInfo
}

func parseAndValidate(raw string, catalog Catalog, policy *guard.Policy) (validatedResponse, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var payload strictResponse
	if err := decoder.Decode(&payload); err != nil {
		return validatedResponse{}, fmt.Errorf("%w: %v", ErrInvalidResponse, err)
	}
	if decoder.More() || strings.TrimSpace(payload.SQL) == "" {
		return validatedResponse{}, ErrInvalidResponse
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return validatedResponse{}, ErrInvalidResponse
	}
	analysis := sqlanalysis.Analyze(payload.SQL)
	if !analysis.Parsed() {
		return validatedResponse{}, fmt.Errorf("%w: %s", ErrSQLParse, analysis.ParseError)
	}
	if err := validateReferences(analysis, catalog); err != nil {
		return validatedResponse{}, err
	}
	validation := ValidationInfo{Analysis: analysis}
	if policy != nil {
		result := guard.Check(payload.SQL, *policy)
		validation.Guard = &result
		if result.Action == guard.ActionDeny {
			return validatedResponse{}, ErrGuardDenied
		}
	}
	structured := map[string]any{"sql": payload.SQL}
	if payload.Analysis != "" {
		structured["analysis"] = payload.Analysis
	}
	if len(payload.UsedFields) > 0 {
		structured["used_fields"] = payload.UsedFields
	}
	return validatedResponse{strictResponse: payload, Structured: structured, Validation: validation}, nil
}

func validateReferences(analysis sqlanalysis.Analysis, catalog Catalog) error {
	tables := make(map[string]CatalogTable, len(catalog.Tables)*2)
	columns := make(map[string]struct{})
	tableColumns := make(map[string]map[string]struct{}, len(catalog.Tables)*2)
	for _, item := range catalog.Tables {
		name := strings.ToLower(item.Table.Name)
		tables[name] = item
		qualifiedName := strings.ToLower(item.Table.Schema + "." + item.Table.Name)
		tables[qualifiedName] = item
		ownedColumns := make(map[string]struct{}, len(item.Table.Columns))
		for _, column := range item.Table.Columns {
			columnName := strings.ToLower(column.Name)
			columns[columnName] = struct{}{}
			ownedColumns[columnName] = struct{}{}
		}
		tableColumns[name] = ownedColumns
		tableColumns[qualifiedName] = ownedColumns
	}
	for _, ref := range analysis.Tables {
		key := strings.ToLower(ref.Name)
		if ref.Schema != "" {
			key = strings.ToLower(ref.Schema + "." + ref.Name)
		}
		if _, ok := tables[key]; !ok {
			return fmt.Errorf("%w: %s", ErrTableOutOfScope, key)
		}
	}
	allowedAliases := make(map[string]struct{}, len(analysis.OutputAliasReferences))
	for _, ref := range analysis.OutputAliasReferences {
		allowedAliases[strings.ToLower(ref.Name)] = struct{}{}
	}
	restricted := make(map[string]struct{}, len(analysis.RestrictedReferences))
	for _, ref := range analysis.RestrictedReferences {
		restricted[strings.ToLower(ref.Qualifier+"."+ref.Name)] = struct{}{}
	}
	derived := make(map[string]struct{}, len(analysis.DerivedColumnReferences))
	for _, ref := range analysis.DerivedColumnReferences {
		derived[strings.ToLower(ref.Qualifier+"."+ref.Name)] = struct{}{}
	}
	for _, ref := range analysis.Columns {
		key := strings.ToLower(ref.Qualifier + "." + ref.Name)
		if _, ok := derived[key]; ok {
			continue
		}
		if ref.Qualifier != "" {
			owner := strings.ToLower(ref.Qualifier)
			if aliased, ok := analysis.TableAliases[owner]; ok {
				owner = strings.ToLower(aliased.Name)
				if aliased.Schema != "" {
					owner = strings.ToLower(aliased.Schema + "." + aliased.Name)
				}
			}
			ownedColumns, ok := tableColumns[owner]
			if !ok {
				return fmt.Errorf("%w: %s", ErrColumnNotFound, key)
			}
			if _, ok := ownedColumns[strings.ToLower(ref.Name)]; !ok {
				return fmt.Errorf("%w: %s", ErrColumnNotFound, key)
			}
			continue
		}
		if _, ok := columns[strings.ToLower(ref.Name)]; !ok {
			_, aliasOK := allowedAliases[strings.ToLower(ref.Name)]
			_, required := restricted[key]
			if ref.Qualifier == "" && aliasOK && !required {
				continue
			}
			return fmt.Errorf("%w: %s", ErrColumnNotFound, ref.Name)
		}
	}
	return nil
}

func repairable(err error) bool {
	return !errors.Is(err, ErrGuardDenied) &&
		!errors.Is(err, ErrTableOutOfScope) &&
		!errors.Is(err, ErrIndexNotReady)
}

func renderCatalog(catalog Catalog, options SchemaRenderOptions) string {
	var builder strings.Builder
	for _, item := range catalog.Tables {
		ddl := strings.EqualFold(options.Mode, "ddl")
		if ddl {
			fmt.Fprintf(&builder, "CREATE TABLE %s.%s (\n", item.Table.Schema, item.Table.Name)
		} else {
			fmt.Fprintf(&builder, "TABLE %s.%s\n", item.Table.Schema, item.Table.Name)
		}
		// Render the business description on the table-level header so the model
		// can tell apart tables that share an identical column layout. It must
		// stay outside the column block: the DDL branch relies on the column
		// index to decide trailing commas.
		if options.IncludeComments {
			if description := item.BusinessDescription(); description != "" {
				fmt.Fprintf(&builder, "  -- %s\n", description)
			}
		}
		columnCount := len(item.Table.Columns)
		if options.MaxColumns > 0 && columnCount > options.MaxColumns {
			columnCount = options.MaxColumns
		}
		for index, column := range item.Table.Columns[:columnCount] {
			fmt.Fprintf(&builder, "  %s %s", column.Name, column.DataType)
			if ddl && index < columnCount-1 {
				builder.WriteString(",")
			}
			if options.IncludeComments && column.Comment != nil && strings.TrimSpace(*column.Comment) != "" {
				fmt.Fprintf(&builder, " -- %s", strings.TrimSpace(*column.Comment))
			}
			builder.WriteString("\n")
		}
		if columnCount < len(item.Table.Columns) {
			if ddl {
				builder.WriteString("  -- more columns omitted\n")
			} else {
				builder.WriteString("  ... more columns omitted\n")
			}
		}
		if ddl {
			builder.WriteString(");\n")
		}
		for _, relationship := range item.Table.Relationships {
			fmt.Fprintf(&builder, "  FOREIGN KEY %s -> %s.%s.%s\n", relationship.SourceColumnName, relationship.TargetTableSchema, relationship.TargetTableName, relationship.TargetColumnName)
		}
	}
	return strings.TrimSpace(builder.String())
}
