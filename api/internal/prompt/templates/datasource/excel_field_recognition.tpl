You are helping refine metadata for a spreadsheet import workflow.

Current table draft:
Name: {{.TableName}}
Description: {{.TableDescription}}

Source file: {{.SourceFileName}}
Source sheet: {{.SourceSheetName}}
Operator language: {{.OperatorLanguage}}
Existing table names in this database:
{{.ExistingNamesJSON}}

Columns:
{{.ColumnsJSON}}

Return strict JSON only, with this exact shape:
{
  "table": {
    "name": "snake_case_table_name",
    "description": "short business description"
  },
  "columns": [
    {
      "source_column_index": 0,
      "source_column": "original source column",
      "name": "snake_case_field_name",
      "display_name": "human readable source label",
      "description": "short field description"
    }
  ]
}

Requirements:
1. Keep the output columns in the same order and count as the input columns.
2. Generate database table names with lowercase letters, digits, and underscores only. Table names must start with a lowercase letter.
3. Generate database field names with lowercase letters, digits, and underscores only. Field names must start with a lowercase letter.
4. Do not use reserved field names: id, uuid, created_time, updated_time.
5. Do not generate a table.name that exactly matches any existing table name listed above.
6. If the best semantic table name already exists, choose a clear semantic variant such as invoice_records_2, invoice_items, contract_billing_records, or employee_roster_2026. Do not fall back to imported_table.
7. Infer table.name from the source file, source sheet, and column set. It should describe the business entity or dataset, not the import mechanism.
8. Avoid generic table names such as table, data, sheet, excel, import, schema, fields, records, imported_table, or data_table unless the word is part of the actual business meaning.
9. Prefer concise semantic table names such as invoices, invoice_records, contract_billing_records, utility_fee_records, employee_roster, patient_admission_records.
10. Prefer concise semantic field names such as contract_no, invoice_no, customer_name, issue_date, total_amount.
11. Generate table.description in the operator language. Keep it concise, natural, and business-facing.
12. Keep table.name and columns[].name as database identifiers, not translated display text.
13. Use source_column, display_name, type, description, and sample_values to infer meaning. Do not generate record data.
14. If meaning is unclear, keep a conservative name based on the current field name.
15. Return JSON only. Do not add Markdown, explanations, comments, or extra keys.
