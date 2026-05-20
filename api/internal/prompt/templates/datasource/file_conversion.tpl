Please convert the following text content into records of the specified table structure.

Table structure:
{{.Columns}}

Text content:
{{.Content}}

Requirements:
1. Strictly convert content according to the field names, types and constraints of the table structure
2. For information that does not have a clear correspondence in the text, if the field is required (IsRequired=true), generate a reasonable default value; if the field is optional, it can be left blank
3. Date/time fields should be converted to ISO 8601 format (e.g.: 2023-10-15T14:30:00Z)
4. Numeric fields should be converted to numeric types, do not include commas or other formatting characters
5. Boolean fields should be converted to true or false
6. Strictly follow the following JSON array format to return results, do not add any explanations or other text:
[{"field1": "value1", "field2": "value2", ...}, {"field1": "value1", "field2": "value2", ...}]
7. Ensure that each record's fields completely match the table structure

{{if .Prompt}}
User prompt:
{{.Prompt}}
{{end}}