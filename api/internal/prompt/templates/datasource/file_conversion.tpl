Please convert the following text content into records of the specified table structure.

Table structure:
{{.Columns}}

Text content:
{{.Content}}

Requirements:
1. Strictly convert content according to the field names, types and constraints of the table structure
2. Extract only values that are explicitly present in the text content. If a value is absent, garbled, ambiguous, or only weakly implied, return an empty string for that field, even when IsRequired=true
3. Do not invent, infer, repair OCR noise, or generate default/example/placeholder values. Never output values such as Example, Sample, Default, Placeholder, 示例, 样例, 默认, 占位, 未知, or N/A
4. Treat the requirements, output format example, user prompt, and any instructional/guidance sections in the text as instructions only, not as record data
5. Date/time fields should be returned as readable date/time strings, preferably ISO 8601, only when the source date/time is explicitly present in the text. Do not return Excel serial dates, Unix timestamps, ordinal day numbers, or any internal numeric date representation such as 45160 unless the target field type is numeric/integer and its description explicitly asks for an Excel serial date.
6. Numeric fields should be converted to numeric types in the same source unit shown in the text. Do not include commas or formatting characters. For money and amounts, keep the original major currency unit and decimals, for example return 32600.00 as 32600.00, never 3260000 cents/fen or any scaled integer.
7. Boolean fields should be converted to true or false only when the text explicitly supports the value
8. Strictly follow the following JSON array format to return results, do not add any explanations or other text:
[{"field1": "value1", "field2": "value2", ...}, {"field1": "value1", "field2": "value2", ...}]
9. If the content contains any reliable value for any target field, return one record with the recognized values. Use null or an empty string for missing fields instead of returning [] only because required fields are missing.
10. Ensure that each record's fields completely match the table structure
11. If the text content does not contain any reliable record value, return an empty JSON array []

{{if .Prompt}}
User prompt:
{{.Prompt}}
{{end}}
