package text2sql

import (
	"strings"
	"time"

	"github.com/zgiai/zgi/api/pkg/sql_base"
	"github.com/zgiai/zgi/api/pkg/sql_base/guard"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlanalysis"
)

type SelectionMode string

const (
	SelectionModeManual    SelectionMode = "manual"
	SelectionModeAutomatic SelectionMode = "automatic"
)

type Scope struct {
	OrganizationID string
	WorkspaceID    string
	AccountID      string
}

type Model struct {
	Provider   string
	Name       string
	Parameters map[string]any
}

type TableRef struct {
	AssetID    string
	PhysicalID int
	Schema     string
	Name       string
}

type Request struct {
	Scope            Scope
	DataSourceID     string
	Question         string
	SelectionMode    SelectionMode
	ManualTables     []TableRef
	GenerationModel  Model
	SchemaRender     SchemaRenderOptions
	AdditionalPrompt string
	GuardPolicy      *guard.Policy
	RepairLimit      int
}

type SchemaRenderOptions struct {
	Mode            string
	MaxColumns      int
	IncludeComments bool
}

type SchemaSelection struct {
	Candidates          []TableRef `json:"candidate_tables,omitempty"`
	Selected            []TableRef `json:"selected_tables"`
	RelationshipTables  []TableRef `json:"relationship_tables,omitempty"`
	UnresolvedRelations []string   `json:"unresolved_relationships,omitempty"`
}

type GenerationInfo struct {
	Model      string        `json:"model"`
	Attempts   int           `json:"attempts"`
	Duration   time.Duration `json:"-"`
	DurationMS int64         `json:"duration_ms"`
}

type ValidationInfo struct {
	Analysis sqlanalysis.Analysis `json:"analysis"`
	Guard    *guard.Result        `json:"guard,omitempty"`
}

type Result struct {
	SQL             string
	RawResponse     string
	AnalysisText    string
	UsedFields      []string
	Structured      map[string]any
	SchemaSelection SchemaSelection
	Generation      GenerationInfo
	Validation      ValidationInfo
	RepairCount     int
}

type Catalog struct {
	Tables []CatalogTable
}

type CatalogTable struct {
	Ref   TableRef
	Table *sql_base.Table

	// Description is the operator-facing business description stored on
	// data_source_tables. It carries the domain wording used by end users
	// (for example "营销域campaigns业务表"), which the physical table comment
	// does not. It is metadata only: it is never part of the schema
	// fingerprint, so filling it does not invalidate built schema indexes.
	Description string
}

// BusinessDescription returns the wording that best identifies the table in
// business terms, preferring the operator-maintained description and falling
// back to the physical table comment.
func (t CatalogTable) BusinessDescription() string {
	if description := strings.TrimSpace(t.Description); description != "" {
		return description
	}
	if t.Table != nil && t.Table.Comment != nil {
		return strings.TrimSpace(*t.Table.Comment)
	}
	return ""
}
