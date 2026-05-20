package graph

import (
	"fmt"
	"strings"
)

// CypherBuilder provides a fluent interface for building Cypher queries
type CypherBuilder struct {
	matchClauses  []string
	whereClauses  []string
	returnClauses []string
	orderBy       string
	limit         int
	params        map[string]interface{}
}

// NewCypherBuilder creates a new CypherBuilder instance
func NewCypherBuilder() *CypherBuilder {
	return &CypherBuilder{
		matchClauses:  make([]string, 0),
		whereClauses:  make([]string, 0),
		returnClauses: make([]string, 0),
		params:        make(map[string]interface{}),
	}
}

// Match adds a MATCH clause
func (b *CypherBuilder) Match(pattern string) *CypherBuilder {
	b.matchClauses = append(b.matchClauses, pattern)
	return b
}

// OptionalMatch adds an OPTIONAL MATCH clause
func (b *CypherBuilder) OptionalMatch(pattern string) *CypherBuilder {
	b.matchClauses = append(b.matchClauses, "OPTIONAL MATCH "+pattern)
	return b
}

// Where adds a WHERE condition
func (b *CypherBuilder) Where(condition string) *CypherBuilder {
	b.whereClauses = append(b.whereClauses, condition)
	return b
}

// And adds an AND condition to WHERE
func (b *CypherBuilder) And(condition string) *CypherBuilder {
	return b.Where(condition)
}

// Return adds RETURN fields
func (b *CypherBuilder) Return(fields ...string) *CypherBuilder {
	b.returnClauses = append(b.returnClauses, fields...)
	return b
}

// OrderBy sets the ORDER BY clause
func (b *CypherBuilder) OrderBy(field string) *CypherBuilder {
	b.orderBy = field
	return b
}

// Limit sets the LIMIT clause
func (b *CypherBuilder) Limit(limit int) *CypherBuilder {
	b.limit = limit
	return b
}

// WithParam adds a parameter
func (b *CypherBuilder) WithParam(name string, value interface{}) *CypherBuilder {
	b.params[name] = value
	return b
}

// Build constructs the final Cypher query and parameters
func (b *CypherBuilder) Build() (string, map[string]interface{}) {
	var sb strings.Builder

	// Build MATCH clauses
	for i, match := range b.matchClauses {
		if i == 0 && !strings.HasPrefix(match, "OPTIONAL") {
			sb.WriteString("MATCH ")
		}
		if i > 0 && !strings.HasPrefix(match, "OPTIONAL") {
			sb.WriteString("\nMATCH ")
		}
		if strings.HasPrefix(match, "OPTIONAL") {
			sb.WriteString("\n")
		}
		sb.WriteString(match)
	}

	// Build WHERE clause
	if len(b.whereClauses) > 0 {
		sb.WriteString("\nWHERE ")
		sb.WriteString(strings.Join(b.whereClauses, " AND "))
	}

	// Build RETURN clause
	if len(b.returnClauses) > 0 {
		sb.WriteString("\nRETURN ")
		sb.WriteString(strings.Join(b.returnClauses, ", "))
	}

	// Build ORDER BY clause
	if b.orderBy != "" {
		sb.WriteString("\nORDER BY ")
		sb.WriteString(b.orderBy)
	}

	// Build LIMIT clause
	if b.limit > 0 {
		sb.WriteString(fmt.Sprintf("\nLIMIT %d", b.limit))
	}

	return sb.String(), b.params
}

// Reset clears the builder for reuse
func (b *CypherBuilder) Reset() *CypherBuilder {
	b.matchClauses = make([]string, 0)
	b.whereClauses = make([]string, 0)
	b.returnClauses = make([]string, 0)
	b.orderBy = ""
	b.limit = 0
	b.params = make(map[string]interface{})
	return b
}

// BuildEntitySearch creates a query to find entities by name
func BuildEntitySearch(names []string, kbID string) (string, map[string]interface{}) {
	return NewCypherBuilder().
		Match("(n)").
		Where("toLower(n.name) IN [x IN $names | toLower(x)]").
		And("n.kb_id = $kbID").
		Return("n").
		WithParam("names", names).
		WithParam("kbID", kbID).
		Build()
}

// BuildNeighborQuery creates a query to find 1-hop neighbors
func BuildNeighborQuery(entityNames []string, kbID string, limit int) (string, map[string]interface{}) {
	return NewCypherBuilder().
		Match("(n)").
		Where("toLower(n.name) IN [x IN $names | toLower(x)]").
		And("n.kb_id = $kbID").
		OptionalMatch("(n)-[r]-(m)").
		Return("n", "collect({type: type(r), node: m}) as neighbors").
		Limit(limit).
		WithParam("names", entityNames).
		WithParam("kbID", kbID).
		Build()
}

// BuildTwoHopQuery creates a query to find 2-hop neighbors
func BuildTwoHopQuery(entityNames []string, kbID string, limit int) (string, map[string]interface{}) {
	return NewCypherBuilder().
		Match("(n)").
		Where("toLower(n.name) IN [x IN $names | toLower(x)]").
		And("n.kb_id = $kbID").
		OptionalMatch("(n)-[r1]-(m1)-[r2]-(m2)").
		Return("n", "m1", "m2", "type(r1) as r1_type", "type(r2) as r2_type").
		Limit(limit).
		WithParam("names", entityNames).
		WithParam("kbID", kbID).
		Build()
}

// BuildRelationshipQuery creates a query to find specific relationship types
func BuildRelationshipQuery(headName, relationType, kbID string, limit int) (string, map[string]interface{}) {
	builder := NewCypherBuilder().
		Match(fmt.Sprintf("(h)-[r:%s]->(t)", relationType)).
		Where("toLower(h.name) = toLower($headName)").
		And("h.kb_id = $kbID").
		Return("h", "r", "t").
		Limit(limit).
		WithParam("headName", headName).
		WithParam("kbID", kbID)

	return builder.Build()
}
