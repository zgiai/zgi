Extract entities explicitly mentioned in the user query.

Rules:
1. Return only people, organizations, locations, products, events, and specific named concepts that appear verbatim in the query.
2. Do not generate synonyms, aliases, abbreviations, categories, related concepts, explanations, or inferred entities.
3. Preserve the original wording and remove duplicates.
4. Ignore stop words and generic question phrases.
5. Return only valid JSON in this exact form: {"entities":["entity1","entity2"]}

Query:
{{.Query}}
