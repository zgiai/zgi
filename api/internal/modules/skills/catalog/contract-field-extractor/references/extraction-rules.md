# Extraction Rules

Use this reference before extracting contract field values.

## General Rules

- Extract only content supported by the contract text.
- Preserve exact source wording for `evidence`.
- Return `未提取到` when the contract does not clearly contain a configured field and no explicit default is configured.
- Use `defaulted` only when no contract value is found, the field has an explicit configured default, and the field rule allows using that default.
- Do not infer parties, amounts, dates, terms, or obligations from context unless the field explicitly allows inference.
- If a field appears in multiple places with conflicting values, use `extraction_status: "conflict"`.
- If a field appears but is ambiguous, use `extraction_status: "uncertain"`.

## Field-Specific Rules

- `money`: keep original currency and amount in `value`; put a machine-friendly amount in `normalized_value` when possible.
- `date`: keep original date wording in `value`; normalize to `YYYY-MM-DD` only when unambiguous.
- `date_range`: include start and end dates when both are present; use `uncertain` if one side is unclear.
- `party`: preserve full legal name, role, and any identifier if present.
- `boolean`: return true or false only when explicitly supported by contract wording. Otherwise use `未提取到` unless an explicit default is configured.
- `enum`: return only allowed enum values. If none match, use `uncertain`, `missing`, or `defaulted` according to the field configuration.
- `array`: return all clearly listed values.
- `clause`: extract the relevant clause text or concise clause summary with evidence.

## Status Values

Use only these values:

- `extracted`: explicitly found in contract text.
- `inferred`: supported by context but not directly written; use only when field rules allow inference.
- `missing`: not found and no explicit default was used.
- `conflict`: multiple conflicting values found.
- `uncertain`: wording exists but is too ambiguous to trust.
- `defaulted`: no contract value found; explicit configured default was used.

## Missing Field Rule

For every field with no contract value and no explicit configured default:

```json
{
  "value": "未提取到",
  "normalized_value": "",
  "extraction_status": "missing",
  "confidence": 0,
  "evidence": "",
  "source_location": ""
}
```

## Defaulted Field Rule

For every field with no contract value but an explicit configured default:

```json
{
  "value": "the configured default",
  "normalized_value": "",
  "extraction_status": "defaulted",
  "confidence": 0,
  "evidence": "",
  "source_location": "",
  "notes": "Value came from field configuration, not contract text."
}
```
