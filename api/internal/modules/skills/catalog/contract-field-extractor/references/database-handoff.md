# Database Handoff

Use this reference when the user asks to write extracted contract fields to a database or contract ledger table.

## Handoff Rules

- This skill does not write database records directly.
- Before routing to a database skill, prepare a reviewed mapping from extraction fields to database columns.
- Ask for confirmation before writing records unless the surrounding workflow already has explicit approval.

## Required Confirmation Data

Confirm:

- Database or data source.
- Table.
- Column mapping.
- Unique key or duplicate handling rule.
- Whether missing fields should be written as empty values, `未提取到`, or null.
- Whether low-confidence fields should be written or held for review.

## Mapping Shape

```json
{
  "target_table": "contract_ledger",
  "unique_key": "contract_number",
  "field_mapping": [
    {
      "source_field_key": "contract_number",
      "target_column": "contract_no",
      "write_rule": "write extracted value only"
    },
    {
      "source_field_key": "contract_amount",
      "target_column": "amount",
      "write_rule": "write normalized_value"
    }
  ],
  "review_required": true
}
```

## Do Not

- Do not drop missing required fields from the handoff.
- Do not overwrite existing database records without explicit rule or confirmation.
- Do not write fields with `conflict` or `uncertain` status unless the user explicitly approves.
