# Confidence Rules

Use this reference to assign confidence values.

## Confidence Scale

- `0.90` to `1.00`: field label and value are directly stated in the contract; evidence is explicit.
- `0.70` to `0.89`: value is explicit but field wording differs from configured label.
- `0.40` to `0.69`: value is partially supported but context is ambiguous.
- `0.01` to `0.39`: weak support; prefer `uncertain` unless the user asked for candidates.
- `0`: missing field or no evidence.

## Status and Confidence

- `extracted`: usually `0.70` to `1.00`.
- `inferred`: usually `0.40` to `0.79`; use only when allowed.
- `missing`: always `0`.
- `conflict`: use the confidence of the strongest candidate and explain the conflict in `notes`.
- `uncertain`: usually below `0.70`.
- `defaulted`: use `0` unless the default itself is a configuration rule; explain in `notes`.

## Evidence Rules

- Evidence should be a short quote or exact phrase from the contract.
- If the value is normalized, evidence must still contain the original form.
- Do not use evidence from outside the contract text.
- Do not raise confidence because a field is required.
