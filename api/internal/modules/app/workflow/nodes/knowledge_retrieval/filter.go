package knowledgeretrieval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"

	ge "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/llm"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	dmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

// MetadataCondition aliases MetadataFilteringCondition for downstream usage (manual/automatic modes).
type MetadataCondition = MetadataFilteringCondition

// MetadataFilter defines interface to build metadata filter for retrieval.
type MetadataFilter interface {
	Build(ctx context.Context, datasetIDs []string, query string, vp *ge.VariablePool) (map[string][]string, *MetadataCondition, error)
}

// NewMetadataFilter creates a metadata filter implementation based on node data.
// Currently  support disabled/automatic/manual modes.
func NewMetadataFilter(n Node, tenantID string) MetadataFilter {
	// disabled or nil -> no-op
	if n.NodeData.MetadataFilteringMode == nil || *n.NodeData.MetadataFilteringMode == disabled {
		return &noopMetadataFilter{}
	}
	// automatic mode -> llm assisted builder
	if *n.NodeData.MetadataFilteringMode == automatic {
		return &autoMetadataFilter{n: n, tenantID: tenantID}
	}
	// manual -> sqlMetadataFilter
	return &sqlMetadataFilter{cond: n.NodeData.MetadataFilteringConditions}
}

// noopMetadataFilter is a no-op implementation that applies no filtering.
type noopMetadataFilter struct{}

// Build returns nil values indicating no metadata filtering.
func (f *noopMetadataFilter) Build(ctx context.Context, datasetIDs []string, query string, vp *ge.VariablePool) (map[string][]string, *MetadataCondition, error) {
	return nil, nil, nil
}

// sqlMetadataFilter builds SQL filters against documents.doc_metadata JSONB
type sqlMetadataFilter struct {
	db   *gorm.DB
	cond *MetadataFilteringCondition
}

func (f *sqlMetadataFilter) Build(ctx context.Context, datasetIDs []string, query string, vp *ge.VariablePool) (map[string][]string, *MetadataCondition, error) {
	if f.cond == nil || len(f.cond.Conditions) == 0 {
		return nil, nil, nil
	}

	db := f.db
	q := db.Model(&dmodel.Document{}).
		Where("dataset_id IN ? AND indexing_status = ? AND enabled = ? AND archived = ?", datasetIDs, dmodel.DocumentStatusCompleted, true, false)

	// Build where clauses
	whereClauses := make([]string, 0)
	whereArgs := make([]any, 0)

	// Normalize condition values using variable pool (template expansion and type/whitespace normalization)
	if err := normalizeConditionValues(f.cond, vp); err != nil {
		return nil, nil, err
	}

	for i, c := range f.cond.Conditions {
		if c.Value == nil && c.ComparisonOperator != "empty" && c.ComparisonOperator != "not empty" {
			continue
		}

		keyParam := c.Name
		switch c.ComparisonOperator {
		case "contains":
			whereClauses = append(whereClauses, "documents.doc_metadata ->> ? LIKE ?")
			whereArgs = append(whereArgs, keyParam, fmt.Sprintf("%%%v%%", c.Value))
		case "not contains":
			whereClauses = append(whereClauses, "documents.doc_metadata ->> ? NOT LIKE ?")
			whereArgs = append(whereArgs, keyParam, fmt.Sprintf("%%%v%%", c.Value))
		case "start with":
			whereClauses = append(whereClauses, "documents.doc_metadata ->> ? LIKE ?")
			whereArgs = append(whereArgs, keyParam, fmt.Sprintf("%v%%", c.Value))
		case "end with":
			whereClauses = append(whereClauses, "documents.doc_metadata ->> ? LIKE ?")
			whereArgs = append(whereArgs, keyParam, fmt.Sprintf("%%%v", c.Value))
		case "in":
			whereClauses = append(whereClauses, "documents.doc_metadata ->> ? = any(string_to_array(?,','))")
			whereArgs = append(whereArgs, keyParam, stringifyListNormalized(c.Value))
		case "not in":
			whereClauses = append(whereClauses, "documents.doc_metadata ->> ? != all(string_to_array(?,','))")
			whereArgs = append(whereArgs, keyParam, stringifyListNormalized(c.Value))
		case "=", "is":
			// numeric vs string equality
			switch c.Value.(type) {
			case int, int64, float32, float64:
				whereClauses = append(whereClauses, "(documents.doc_metadata ->> ?)::double precision = ?")
				whereArgs = append(whereArgs, keyParam, c.Value)
			default:
				whereClauses = append(whereClauses, "documents.doc_metadata ->> ? = ?")
				whereArgs = append(whereArgs, keyParam, fmt.Sprintf(`"%v"`, c.Value))
			}
		case "is not", "≠":
			// numeric vs string inequality
			switch c.Value.(type) {
			case int, int64, float32, float64:
				whereClauses = append(whereClauses, "(documents.doc_metadata ->> ?)::double precision <> ?")
				whereArgs = append(whereArgs, keyParam, c.Value)
			default:
				whereClauses = append(whereClauses, "documents.doc_metadata ->> ? <> ?")
				whereArgs = append(whereArgs, keyParam, fmt.Sprintf(`"%v"`, c.Value))
			}
		case "empty":
			// treat key missing OR JSON literal null as empty
			whereClauses = append(whereClauses, "(documents.doc_metadata -> ? IS NULL OR documents.doc_metadata ->> ? = 'null')")
			whereArgs = append(whereArgs, keyParam, keyParam)
		case "not empty":
			// require key present AND not JSON literal null
			whereClauses = append(whereClauses, "(documents.doc_metadata -> ? IS NOT NULL AND (documents.doc_metadata ->> ?) <> 'null')")
			whereArgs = append(whereArgs, keyParam, keyParam)
		case "before", "<":
			whereClauses = append(whereClauses, "(documents.doc_metadata ->> ?)::double precision < ?")
			whereArgs = append(whereArgs, keyParam, c.Value)
		case "after", ">":
			whereClauses = append(whereClauses, "(documents.doc_metadata ->> ?)::double precision > ?")
			whereArgs = append(whereArgs, keyParam, c.Value)
		case "≤", "<=":
			whereClauses = append(whereClauses, "(documents.doc_metadata ->> ?)::double precision <= ?")
			whereArgs = append(whereArgs, keyParam, c.Value)
		case "≥", ">=":
			whereClauses = append(whereClauses, "(documents.doc_metadata ->> ?)::double precision >= ?")
			whereArgs = append(whereArgs, keyParam, c.Value)
		default:
			// skip unsupported
			_ = i
		}
	}

	if len(whereClauses) == 0 {
		var retCond *MetadataCondition
		if f.cond != nil && len(f.cond.Conditions) > 0 {
			retCond = f.cond
		}
		return nil, retCond, nil
	}

	// Apply logical operator
	op := "OR"
	if f.cond.LogicalOperator == "and" || f.cond.LogicalOperator == "AND" {
		op = "AND"
	}
	combined := combineClauses(whereClauses, op)
	q = q.Where(combined, whereArgs...)

	type row struct {
		ID        string
		DatasetID string
	}

	var rows []row
	if err := q.Select("id, dataset_id").Find(&rows).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	// group ids by dataset
	groups := make(map[string][]string)
	for _, r := range rows {
		groups[r.DatasetID] = append(groups[r.DatasetID], r.ID)
	}
	// return normalized metadata condition for downstream usage
	var retCond *MetadataCondition
	if f.cond != nil && len(f.cond.Conditions) > 0 {
		retCond = f.cond
	}
	return groups, retCond, nil
}

func combineClauses(clauses []string, op string) string {
	if len(clauses) == 1 {
		return clauses[0]
	}
	s := "("
	for i, c := range clauses {
		if i > 0 {
			s += " " + op + " "
		}
		s += "(" + c + ")"
	}
	s += ")"
	return s
}

func stringifyList(v interface{}) string {
	switch vv := v.(type) {
	case []string:
		if len(vv) == 0 {
			return ""
		}
		s := ""
		for i, x := range vv {
			if i > 0 {
				s += ","
			}
			s += x
		}
		return s
	case []any:
		if len(vv) == 0 {
			return ""
		}
		s := ""
		for i, x := range vv {
			if i > 0 {
				s += ","
			}
			s += fmt.Sprintf("%v", x)
		}
		return s
	default:
		return fmt.Sprintf("%v", vv)
	}
}

// stringifyListNormalized normalizes a comma-separated string list by trimming spaces
// and escaping single quotes to double single-quotes, aligning with default behavior.
func stringifyListNormalized(v interface{}) string {
	if s, ok := v.(string); ok {
		parts := strings.Split(s, ",")
		if len(parts) == 0 {
			return ""
		}
		for i := range parts {
			parts[i] = strings.ReplaceAll(strings.TrimSpace(parts[i]), "'", "''")
		}
		return strings.Join(parts, ",")
	}
	return stringifyList(v)
}

// autoMetadataFilter calls LLM to infer metadata filter conditions and reuses SQL builder
type autoMetadataFilter struct {
	n        Node
	tenantID string
}

func (f *autoMetadataFilter) Build(
	ctx context.Context,
	datasetIDs []string,
	query string,
	vp *ge.VariablePool,
) (map[string][]string, *MetadataCondition, error) {
	if len(datasetIDs) == 0 || query == "" {
		return nil, nil, nil
	}

	db := f.n.db

	// 1) load all metadata field names for these datasets using proper model
	var metas []dmodel.DatasetMetadata
	if err := db.Where("dataset_id IN ?", datasetIDs).Find(&metas).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to fetch dataset_metadatas: %w", err)
	}

	if len(metas) == 0 {
		return nil, nil, nil
	}
	fields := make([]string, 0, len(metas))
	for _, m := range metas {
		if strings.TrimSpace(m.Name) == "" {
			continue
		}
		fields = append(fields, m.Name)
	}
	if len(fields) == 0 {
		return nil, nil, nil
	}

	invoker := f.n.llmInvoker
	if invoker == nil {
		return nil, nil, fmt.Errorf("llm invoker is not configured")
	}

	// 2) pick model config (metadata-specific or fallback to single retrieval config)
	modelCfg := f.n.NodeData.MetadataModelConfig
	if (modelCfg == nil || modelCfg.Name == "") && f.n.NodeData.SingleRetrievalConfig != nil {
		cfg := f.n.NodeData.SingleRetrievalConfig.ModelConfig
		modelCfg = &cfg
	}
	if modelCfg == nil || strings.TrimSpace(modelCfg.Name) == "" {
		return nil, nil, nil
	}

	// 3) build prompt messages using shared prompt template
	mode := modelCfg.Mode
	if mode == "" {
		mode = llm.ModeChat
	}

	t, err := getPromptTemplate(mode, fields, query)
	if err != nil {
		return nil, nil, err
	}

	messages := toPromptMessagesFromTemplate(t, mode)
	if len(messages) == 0 {
		return nil, nil, nil
	}

	// 4) invoke LLM (non-stream)
	res, err := invoker.Invoke(ctx, f.n.UserID, f.n.APPID, AppType, &InvokeRequest{
		ModelSlug:  modelCfg.Name,
		Messages:   messages,
		Parameters: modelCfg.CompletionParams,
		UserID:     f.n.UserID,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to invoke LLM: %w", err)
	}

	// 5) parse response text
	text := ""
	if res != nil {
		text = res.Text
	}
	if text == "" {
		return nil, nil, nil
	}

	// 6) parse JSON: expect {"metadata_map": [{...}]}
	var parsed struct {
		MetadataMap []struct {
			FieldName  string `json:"metadata_field_name"`
			FieldValue any    `json:"metadata_field_value"`
			Operator   string `json:"comparison_operator"`
		} `json:"metadata_map"`
	}

	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		// try to extract JSON substring (simple heuristic)
		start := strings.IndexByte(text, '{')
		end := strings.LastIndexByte(text, '}')
		if start >= 0 && end > start {
			_ = json.Unmarshal([]byte(text[start:end+1]), &parsed)
		}
	}

	if len(parsed.MetadataMap) == 0 {
		return nil, nil, nil
	}

	// filter only known fields
	fieldSet := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		fieldSet[f] = struct{}{}
	}

	conds := make([]Condition, 0, len(parsed.MetadataMap))
	for _, item := range parsed.MetadataMap {
		if _, ok := fieldSet[item.FieldName]; !ok {
			continue
		}
		conds = append(conds, Condition{
			Name:               item.FieldName,
			ComparisonOperator: item.Operator,
			Value:              item.FieldValue,
		})
	}
	if len(conds) == 0 {
		return nil, nil, nil
	}

	// Determine logical operator from node configuration
	logicalOp := "or" // default value
	if f.n.NodeData.MetadataFilteringConditions != nil && f.n.NodeData.MetadataFilteringConditions.LogicalOperator != "" {
		logicalOp = f.n.NodeData.MetadataFilteringConditions.LogicalOperator
	}

	// Reuse SQL builder with configured logical_operator and return condition
	sqlb := &sqlMetadataFilter{
		db: f.n.db,
		cond: &MetadataFilteringCondition{
			LogicalOperator: logicalOp,
			Conditions:      conds,
		},
	}
	groups, cond, err := sqlb.Build(ctx, datasetIDs, query, vp)
	if err != nil {
		return nil, nil, err
	}
	if cond == nil {
		cond = &MetadataCondition{}
	}
	return groups, cond, nil
}

// normalizeConditionValues expands templates and normalizes values in metadata conditions.
// - If value is a string template, it will be expanded using variable pool; the first segment is used.
// - Numbers become float64 or int based on segment type; strings collapse newlines/tabs and trim spaces.
// - If expanded segment is neither number nor string, returns an error (aligns with default behavior).
func normalizeConditionValues(cond *MetadataFilteringCondition, vp *ge.VariablePool) error {
	if cond == nil || vp == nil {
		return nil
	}

	wsRe := regexp.MustCompile(`[\r\n\t]+`)

	for i := range cond.Conditions {
		c := &cond.Conditions[i]

		if c.Value == nil || c.ComparisonOperator == "empty" || c.ComparisonOperator == "not empty" {
			continue
		}

		s, ok := c.Value.(string)
		if !ok || s == "" {
			continue
		}

		sg := vp.ConvertTemplate(s)
		if sg == nil || len(sg.Value) == 0 {
			// still normalize whitespace for plain strings
			c.Value = strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
			continue
		}

		seg := sg.Value[0]
		switch seg.GetType() {
		case shared.SegmentTypeFloat, shared.SegmentTypeInteger:
			c.Value = seg.ToObject()
		case shared.SegmentTypeString:
			v := seg.Text()
			v = wsRe.ReplaceAllString(v, " ")
			v = strings.TrimSpace(v)
			c.Value = v
		default:
			return fmt.Errorf("invalid expected metadata value type")
		}
	}
	return nil
}

func getPromptTemplate(modelMode llm.Mode, fileds []string, query string) (any, error) {

	fieldsJSONBytes, err := json.Marshal(fileds)
	if err != nil {
		return nil, err
	}
	fieldsJSON := string(fieldsJSONBytes)

	if modelMode == llm.ModeChat {
		system := llm.NodeChatModelMessage{Role: llm.PromptMessageRoleSystem, Text: metadataFilterSystemPrompt}
		user1 := llm.NodeChatModelMessage{Role: llm.PromptMessageRoleUser, Text: metadataFilterUserPrompt1}
		assistant1 := llm.NodeChatModelMessage{Role: llm.PromptMessageRoleAssistant, Text: metadataFilterAssistantPrompt1}
		user2 := llm.NodeChatModelMessage{Role: llm.PromptMessageRoleUser, Text: metadataFilterUserPrompt2}
		assistant2 := llm.NodeChatModelMessage{Role: llm.PromptMessageRoleAssistant, Text: metadataFilterAssistantPrompt2}
		user3 := llm.NodeChatModelMessage{Role: llm.PromptMessageRoleUser, Text: fmt.Sprintf(metadataFilterUserPrompt3JSONFormat, query, fieldsJSON)}
		return []llm.NodeChatModelMessage{system, user1, assistant1, user2, assistant2, user3}, nil
	}

	if modelMode == llm.ModeCompletion {
		// Build the completion prompt for metadata filtering.
		fieldsJSONBytes, _ := json.Marshal(fileds)
		fieldsJSON := string(fieldsJSONBytes)
		text := fmt.Sprintf(metadataFilterCompletionPromptTemplate, query, fieldsJSON)
		return llm.NodeCompletionModelPromptTemplate{Text: text}, nil
	}

	return nil, nil
}

func toPromptMessagesFromTemplate(t any, mode llm.Mode) []PromptMessage {
	switch mode {
	case llm.ModeChat:
		msgs, ok := t.([]llm.NodeChatModelMessage)
		if !ok {
			return nil
		}
		out := make([]PromptMessage, 0, len(msgs))
		for _, m := range msgs {
			out = append(out, PromptMessage{
				Role:    string(m.Role),
				Content: m.Text,
			})
		}
		return out
	case llm.ModeCompletion:
		if tmpl, ok := t.(llm.NodeCompletionModelPromptTemplate); ok {
			return []PromptMessage{
				{
					Role:    "user",
					Content: tmpl.Text,
				},
			}
		}
	}
	return nil
}
