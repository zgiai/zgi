{{if .Content}}
Please analyze the following text content and generate candidate table field definitions.

Text content:
{{.Content}}
{{else}}
Please generate candidate table field definitions based solely on the user prompt below.
{{end}}

Goal:
Generate a comprehensive candidate field set for structured storage and later human review.
Prefer recall over brevity, but only include fields that are explicit or strongly supported by the source text.

Requirements:
1. Table fields must come from explicit or strongly supported information in the text. Do not invent speculative, hypothetical, or weakly implied fields.
2. The generated content must include a Name field. The field name must be in English, use snake_case, and follow database field naming conventions.
3. The generated content must include a Type field. The value must be exactly one of: boolean, text, timestamp, numeric, integer.
4. The generated content must include an IsRequired field with value true or false.
5. The generated content must include a Description field. The description must be a Chinese string explaining the field meaning.
6. Return only a JSON array. Do not include any explanation, markdown, headings, or extra text.
7. The number of fields must be determined by the source text itself. If the text contains many explicit and structurally meaningful data points, return a large candidate field set instead of compressing them into only a few summary fields.
8. Prefer broader coverage over aggressive compression, but do not create multiple fields that mean the same thing.
9. Do not return only high-level summary fields. If the text contains multiple distinct parties, dates, periods, amounts, rates, addresses, contact details, account details, conditions, obligations, restrictions, penalties, or payment-related concepts, keep them as separate candidate fields when their meanings differ.
10. Do not merge semantically different concepts into one generic field. Prefer precise names such as monthly_rent_amount, deposit_amount, overdue_penalty_rate instead of vague names such as amount or rate.
11. Field names must be unique. If similar concepts appear in different roles or contexts, distinguish them with precise names.
12. These are candidate fields for human review before persistence. If the source text clearly expresses a storable concept but some details are incomplete, include a conservative and meaningful candidate field. If evidence is weak or speculative, omit the field.
13. Avoid noisy extraction. Do not include boilerplate legal wording, repeated rhetorical text, or generic filler as fields unless it represents a clearly storable business fact.
14. Internally check whether the result covers the major explicit information categories present in the text, such as titles or identifiers, entities or parties, dates or periods, amounts or rates, locations or addresses, contact information, payment information, and terms or conditions. Do not output the checking process.

Output format example:
[{"Name":"user_name","Type":"text","IsRequired":true,"Description":"User name"}]

{{if .Prompt}}
User prompt:
{{.Prompt}}
{{end}}
