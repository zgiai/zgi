Extract entities and relationships from the following text.
Return the result in JSON format with two fields:
1. "entities": a list of all entities (nouns, proper nouns)
2. "triples": a list of relationship triples, each with "subject", "predicate", "object"

REFERENCE SCHEMA (Ontology):
{{.SchemaJSON}}

Schema-specific rules:
{{.SchemaRules}}

Text: {{.SegmentText}}

Return only valid JSON, no additional text.
