package text2sql

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/zgiai/zgi/api/pkg/sql_base"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

const defaultRRFK = 60.0

var schemaSearchFields = []string{"asset_table_id", "data_source_id", "schema_name", "table_name", "physical_table_id", "fingerprint", "content"}

type fieldSearchVectorDB interface {
	SearchVectorsByFields(ctx context.Context, className, dataSourceID string, vector []float64, limit int, returnFields []string) ([]map[string]interface{}, error)
	SearchByFullTextField(ctx context.Context, className, dataSourceID, searchField, query string, limit int, returnFields []string) ([]map[string]interface{}, error)
}

type EmbeddingModelResolver interface {
	ResolveEmbedding(ctx context.Context, organizationID, dataSourceID string) (provider, model string, err error)
}

type RetrievalService struct {
	repo       *IndexRepository
	models     EmbeddingModelResolver
	embedder   Embedder
	vectors    vectordb.VectorDB
	sql        sql_base.SQLBase
	vectorTopK int
	bm25TopK   int
	finalTopK  int
}

func NewRetrievalService(repo *IndexRepository, models EmbeddingModelResolver, embedder Embedder, vectors vectordb.VectorDB, sql ...sql_base.SQLBase) *RetrievalService {
	// finalTopK is deliberately larger than the number of tables a single
	// question needs: on wide multi-domain sources the correct table is often
	// outranked by structurally identical tables from neighbouring domains, and
	// the generation prompt is where the business description can settle it.
	service := &RetrievalService{repo: repo, models: models, embedder: embedder, vectors: vectors, vectorTopK: 40, bm25TopK: 40, finalTopK: 16}
	if len(sql) > 0 {
		service.sql = sql[0]
	}
	return service
}

func (s *RetrievalService) Retrieve(ctx context.Context, req Request) (SchemaSelection, error) {
	if s == nil || s.repo == nil || s.models == nil || s.embedder == nil || s.vectors == nil || s.sql == nil {
		return SchemaSelection{}, ErrIndexNotReady
	}
	provider, model, err := s.models.ResolveEmbedding(ctx, req.Scope.OrganizationID, req.DataSourceID)
	if err != nil || provider == "" || model == "" {
		return SchemaSelection{}, fmt.Errorf("%w: embedding model", ErrIndexNotReady)
	}
	rows, err := s.repo.Ready(ctx, req.Scope.WorkspaceID, req.DataSourceID, provider, model)
	if err != nil || len(rows) == 0 {
		return SchemaSelection{}, ErrIndexNotReady
	}
	tables, err := s.repo.Snapshot(ctx, req.Scope.OrganizationID, req.DataSourceID, s.sql)
	if err != nil {
		return SchemaSelection{}, fmt.Errorf("%w: %w", ErrIndexNotReady, err)
	}
	var complete bool
	rows, complete = CompleteIndexes(tables, rows)
	if !complete {
		return SchemaSelection{}, fmt.Errorf("%w (%d/%d tables ready)", ErrIndexNotReady, len(rows), len(tables))
	}
	collection := collectionName(req.Scope.WorkspaceID, provider, model)
	queryVector, err := s.embedder.Embed(ctx, provider, model, req.Question)
	if err != nil {
		return SchemaSelection{}, fmt.Errorf("schema retrieval embedding: %w", err)
	}
	fieldSearch, ok := s.vectors.(fieldSearchVectorDB)
	if !ok {
		return SchemaSelection{}, fmt.Errorf("schema retrieval unavailable: vector store does not support field-aware search")
	}
	var vectorResults, bm25Results []map[string]interface{}
	var vectorErr, bm25Err error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		vectorResults, vectorErr = fieldSearch.SearchVectorsByFields(ctx, collection, req.DataSourceID, queryVector, s.vectorTopK, schemaSearchFields)
	}()
	go func() {
		defer wg.Done()
		bm25Results, bm25Err = fieldSearch.SearchByFullTextField(ctx, collection, req.DataSourceID, "content", req.Question, s.bm25TopK, schemaSearchFields)
	}()
	wg.Wait()
	if vectorErr != nil || bm25Err != nil {
		return SchemaSelection{}, fmt.Errorf("schema retrieval unavailable: vector=%v bm25=%v", vectorErr, bm25Err)
	}
	ranked := fuseRRF(vectorResults, bm25Results, defaultRRFK, req.Question)
	byAsset := make(map[string]SchemaIndex, len(rows))
	for _, row := range rows {
		byAsset[row.AssetTableID] = row
	}
	selected := make([]TableRef, 0, min(s.finalTopK, len(ranked)))
	for _, candidate := range ranked {
		row, ok := byAsset[candidate.ID]
		if !ok {
			continue
		}
		selected = append(selected, TableRef{AssetID: row.AssetTableID, PhysicalID: row.PhysicalTableID, Schema: row.SchemaName, Name: row.PhysicalTableName})
		if len(selected) == s.finalTopK {
			break
		}
	}
	if len(selected) == 0 {
		return SchemaSelection{}, fmt.Errorf("%w: no relevant table", ErrIndexNotReady)
	}
	relationshipTables := s.expandRelationships(ctx, selected, rows)
	selected = append(selected, relationshipTables...)
	return SchemaSelection{Selected: selected, Candidates: selected[:len(selected)-len(relationshipTables)], RelationshipTables: relationshipTables}, nil
}

func (s *RetrievalService) expandRelationships(ctx context.Context, selected []TableRef, rows []SchemaIndex) []TableRef {
	if s.sql == nil {
		return nil
	}
	byName := make(map[string]SchemaIndex, len(rows))
	seen := make(map[string]struct{}, len(selected))
	for _, row := range rows {
		byName[strings.ToLower(row.SchemaName+"."+row.PhysicalTableName)] = row
	}
	for _, ref := range selected {
		seen[ref.AssetID] = struct{}{}
	}
	var related []TableRef
	for _, ref := range selected {
		table, err := s.sql.GetTable(ctx, ref.PhysicalID)
		if err != nil || table == nil {
			continue
		}
		for _, relation := range table.Relationships {
			row, ok := byName[strings.ToLower(relation.TargetTableSchema+"."+relation.TargetTableName)]
			if !ok {
				continue
			}
			if _, exists := seen[row.AssetTableID]; exists {
				continue
			}
			seen[row.AssetTableID] = struct{}{}
			related = append(related, TableRef{AssetID: row.AssetTableID, PhysicalID: row.PhysicalTableID, Schema: row.SchemaName, Name: row.PhysicalTableName})
		}
	}
	return related
}

type rankedCandidate struct {
	ID    string
	Score float64
}

func fuseRRF(vector, bm25 []map[string]interface{}, k float64, question string) []rankedCandidate {
	scores := map[string]float64{}
	evidence := map[string]float64{}
	add := func(results []map[string]interface{}) {
		for rank, result := range results {
			id := resultString(result, "asset_table_id")
			if id == "" {
				continue
			}
			scores[id] += 1 / (k + float64(rank+1))
			if _, seen := evidence[id]; !seen {
				evidence[id] = lexicalOverlap(question, searchableText(result))
			}
		}
	}
	add(vector)
	add(bm25)
	out := make([]rankedCandidate, 0, len(scores))
	for id, score := range scores {
		out = append(out, rankedCandidate{ID: id, Score: score})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		// RRF ties are common when neighbouring domains hold structurally
		// identical tables. Break them with the amount of literal lexical
		// evidence between the question and the indexed document instead of the
		// asset id, whose ordering is effectively random. The id stays as the
		// final key so the result remains deterministic.
		if left, right := evidence[out[i].ID], evidence[out[j].ID]; left != right {
			return left > right
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// searchableText concatenates the indexed text a search result exposes.
func searchableText(result map[string]interface{}) string {
	var builder strings.Builder
	for _, field := range []string{"table_name", "content"} {
		if value := resultString(result, field); value != "" {
			builder.WriteString(value)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

// lexicalOverlap scores how much of the document is literally present in the
// question, using character bigrams so that it works for Chinese text, where
// whitespace tokenisation carries no signal.
func lexicalOverlap(question, document string) float64 {
	documentBigrams := bigrams(document)
	if len(documentBigrams) == 0 {
		return 0
	}
	questionBigrams := bigrams(question)
	shared := 0
	for bigram := range documentBigrams {
		if _, ok := questionBigrams[bigram]; ok {
			shared++
		}
	}
	return float64(shared) / float64(len(documentBigrams))
}

func bigrams(text string) map[string]struct{} {
	runes := []rune(strings.ToLower(text))
	if len(runes) < 2 {
		return nil
	}
	out := make(map[string]struct{}, len(runes)-1)
	for index := 0; index+1 < len(runes); index++ {
		out[string(runes[index:index+2])] = struct{}{}
	}
	return out
}

func resultString(result map[string]interface{}, key string) string {
	if value, ok := result[key].(string); ok {
		return strings.TrimSpace(value)
	}
	if properties, ok := result["properties"].(map[string]interface{}); ok {
		if value, ok := properties[key].(string); ok {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
