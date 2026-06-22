# Field Schema Contract

Use this reference to interpret the configured field list.

## Field Configuration

Each field should include:

- `key`: stable machine-readable field key. Required.
- `label`: human-readable field name. Required.
- `type`: field type. Required.
- `required`: boolean indicating whether the field is required in the output.
- `description`: what the field means.
- `extraction_rule`: how to find the field in contract text.
- `default_value`: optional value to use only when the field is absent and the configuration explicitly allows a default.
- `empty_value`: value to return when missing. Default to `未提取到`.
- `enum_values`: optional allowed values for enum fields.
- `normalization_rule`: optional rule for standardized output.

## Supported Field Types

- `string`
- `number`
- `money`
- `date`
- `date_range`
- `party`
- `boolean`
- `enum`
- `array`
- `clause`

## Field Configuration Example

```json
{
  "fields": [
    {
      "key": "contract_name",
      "label": "合同名称",
      "type": "string",
      "required": true,
      "description": "合同标题或正文中明确出现的合同名称",
      "extraction_rule": "优先从首页标题、合同抬头、文件标题中提取",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "contract_amount",
      "label": "合同金额",
      "type": "money",
      "required": true,
      "description": "合同总金额，保留币种",
      "extraction_rule": "优先提取含总价、合同金额、价款、费用总额的条款",
      "default_value": "",
      "empty_value": "未提取到"
    }
  ]
}
```

## Rules

- Do not silently create new field keys in production output.
- If the user provides a rough field list, normalize it into this schema before extraction.
- If `type` is missing and cannot be safely inferred from the field label, ask for clarification.
- Required fields must appear in output even when missing.
- `default_value` is used only when explicitly configured. It is not evidence from the contract.
- If no contract value exists and no explicit default is configured, return the configured `empty_value` or `未提取到`.
- If no contract value exists but an explicit default is configured and allowed, return the default with `extraction_status: "defaulted"`.
