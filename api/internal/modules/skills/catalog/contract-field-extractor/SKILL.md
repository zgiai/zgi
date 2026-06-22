---
name: contract-field-extractor
description: Extract structured contract fields from existing contract text or already parsed contract content according to a configured field list. Use when the user asks for 合同字段抽取, 合同信息抽取, 合同关键信息提取, 合同台账, contract field extraction, contract metadata extraction, contract ledger data, or extracting parties, amount, dates, term, payment terms, renewal terms, breach clauses, or other configured contract fields. This skill does not parse uploaded files directly; it works only after contract text is available from another system or parser.
when_to_use: Use this skill when contract text is already available and the user needs configured field extraction with field descriptions, types, required flags, extraction rules, default values, empty-value handling, evidence, status, and confidence scores; use file-generator only when the user asks to export the structured result.
provider_type: builtin
provider_id: file_generator
runtime_type: hybrid
tools:
  - generate_file
max_calls_per_turn: 5
timeout_seconds: 10
display:
  icon: file-search
  category: productivity
  label:
    en_US: Contract Field Extractor
    zh_Hans: 合同字段抽取
  description:
    en_US: Extracts configured contract fields into structured results with evidence and confidence.
    zh_Hans: 按字段清单抽取合同关键信息，并返回证据、状态和置信度。
  when_to_use:
    en_US: Use when parsed contract text needs structured field extraction or ledger-ready output.
    zh_Hans: 当已解析合同文本需要结构化抽取或生成合同台账数据时使用。
  tags:
    en_US:
      - Contract
      - Extraction
      - Structured Data
    zh_Hans:
      - 合同
      - 字段抽取
      - 结构化
---

# Contract Field Extractor Skill

Use this skill to extract configured fields from contract text that is already available in the conversation or supplied by another system. This skill does not read files, parse PDF/DOCX bytes, OCR images, or inspect uploads directly.

## Scope

Use this skill for:

- Extracting contract parties, contract name, contract number, amount, currency, signing date, effective date, expiry date, term, payment terms, renewal terms, breach clauses, jurisdiction, contacts, and other configured fields.
- Producing JSON, Markdown, or CSV-ready structured contract extraction results.
- Preparing contract ledger rows for later database insertion or manual review.
- Returning confidence, evidence, source location, status, and notes for every configured field.

Do not use this skill to:

- Parse uploaded files directly.
- OCR scanned contracts or screenshots.
- Provide final legal advice, enforceability conclusions, or legal risk certification.
- Invent missing contract content.
- Write database records directly without a separate database handoff and confirmation.

## Workflow

1. Confirm contract text is available. It must be pasted text, prior chat context, or parsed contract content supplied by another system.
2. Confirm the configured field list is available. Do not invent a field list for a production extraction request.
3. If the user asks to use a built-in field template, read `field-templates.md` and use the exact template schema.
4. If contract text or field configuration is missing, call `request_user_input`.
5. Read `field-schema-contract.md` before interpreting field configuration.
6. Read `extraction-rules.md` before extracting values.
7. Extract every configured field, including required fields. Missing required fields must still appear in the output.
8. When no contract value exists and no explicit default is configured, return `value: "未提取到"` and `extraction_status: "missing"`.
9. When no contract value exists but the field has an explicit configured default and the rules allow using it, return that default with `extraction_status: "defaulted"`.
10. Read `confidence-rules.md` and assign confidence for every field.
11. Read `output-format.md` and return the structured result in the requested format. Default to JSON in the answer when no export is requested.
12. If the user asks to export, save, download, or generate a contract ledger file, read `contract-ledger-export.md` and call `generate_file`.
13. If the user asks to write results to a database, read `database-handoff.md`; prepare a reviewed mapping and ask for confirmation before routing to any database skill.

## Clarification Workflow

Call `request_user_input` instead of writing a plain clarification when missing information blocks reliable extraction.

Ask when:

- No contract text or parsed contract content is available.
- No field list is available and the user did not choose a built-in field template.
- Field definitions are missing required details such as `key`, `label`, or `type`.
- The user asks to export but the output format is ambiguous and cannot be inferred.
- The user asks to write to a database but database/table/field mapping is unclear.

Example `request_user_input` payload:

```json
{
  "message": "I can extract contract fields, but need the contract text and field configuration first.",
  "questions": [
    {
      "id": "contract_text",
      "question": "Please provide the parsed contract text or confirm which available contract text should be used."
    },
    {
      "id": "field_list",
      "question": "Please provide the field list, or choose a field template.",
      "options": [
        { "label": "basic contract info" },
        { "label": "parties and contacts" },
        { "label": "amount and payment" },
        { "label": "term and performance" },
        { "label": "risk and clauses" }
      ]
    }
  ]
}
```

After calling `request_user_input`, stop the turn and wait for the user's answer.

## References

Read the relevant references before producing final output:

| Task | Read reference |
| --- | --- |
| Use a built-in field template | `field-templates.md` |
| Interpret configured field list and field metadata | `field-schema-contract.md` |
| Extract values without inventing content | `extraction-rules.md` |
| Assign confidence and status | `confidence-rules.md` |
| Format structured result | `output-format.md` |
| Export JSON, CSV, Markdown, or ledger file | `contract-ledger-export.md` |
| Prepare database handoff mapping | `database-handoff.md` |

## Output Contract

Each configured field must produce one result object:

- `field_key`
- `field_label`
- `value`
- `normalized_value`
- `value_type`
- `extraction_status`
- `confidence`
- `evidence`
- `source_location`
- `notes`

When no value is explicitly present in the contract and no explicit default is configured, return:

- `value`: `未提取到`
- `normalized_value`: empty string
- `extraction_status`: `missing`
- `confidence`: `0`
- `evidence`: empty string

When no value is explicitly present in the contract but an explicit default is configured and allowed, return:

- `value`: the configured default value
- `normalized_value`: normalized configured default when applicable
- `extraction_status`: `defaulted`
- `confidence`: `0` unless the default rule says otherwise
- `evidence`: empty string
- `notes`: explain that the value came from configuration, not contract text

## Constraints

- Do not parse uploaded files directly. Rely on contract text already supplied by the platform or another system.
- Do not fabricate fields, values, dates, amounts, parties, clauses, or legal conclusions.
- Do not omit a configured field just because it is missing in the contract.
- Do not fill missing required fields with guesses. Use `未提取到` when there is no contract value and no explicit default.
- Do not output a value without evidence unless the field used an explicit default value.
- Do not treat a default value as extracted contract text. Use `extraction_status: "defaulted"`.
- Do not write to a database directly from this skill.
- For legal, financial, and compliance use, state that extracted results should be reviewed before use.
